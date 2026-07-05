package game

import (
	"strings"

	"ugataima/internal/spells"
)

// TimedCombatBuff is one active, timed party combat buff. Multiple buffs STACK
// additively (Day of the Gods, Hour of Power, Stone Skin, Heroism, ...): their
// ResistPct / OutBonus / InReduce sum across all active entries. This replaces
// the old single-slot dayGods*/hourPower* fields, so casting one buff no longer
// clobbers another and any number of buff spells can coexist.
type TimedCombatBuff struct {
	SpellID       string // spell id (HUD status icon + replace-on-recast key)
	Frames        int    // frames remaining
	OutBonus      int    // flat add to party outgoing damage
	OutDamageType string // empty/"all" applies to all damage; "physical" applies only to physical attacks
	InReduce      int    // flat reduction of incoming damage (after ResistPct)
	ResistPct     int    // % reduction of incoming damage (applied before InReduce)
}

func (b TimedCombatBuff) buffSpellID() string { return b.SpellID }

// addCombatBuff activates a buff (same-spell recast refreshes).
func (g *MMGame) addCombatBuff(b TimedCombatBuff) {
	g.combatBuffs = upsertBuff(g.combatBuffs, b)
}

// removeCombatBuff drops a combat buff by spell id (dispel). No-op if absent.
func (g *MMGame) removeCombatBuff(spellID string) {
	g.combatBuffs, _ = removeBuffByID(g, g.combatBuffs, spellID)
}

// combatBuffOutBonus sums the flat outgoing-damage bonus from all active buffs.
func (g *MMGame) combatBuffOutBonus() int {
	total := 0
	for i := range g.combatBuffs {
		total += g.combatBuffs[i].OutBonus
	}
	return total
}

// combatBuffOutBonusForDamageType sums outgoing-damage bonuses that apply to
// the supplied damage type. Empty/all buff types apply to every outgoing hit.
func (g *MMGame) combatBuffOutBonusForDamageType(damageType string) int {
	damageType = strings.ToLower(strings.TrimSpace(damageType))
	total := 0
	for i := range g.combatBuffs {
		buffType := strings.ToLower(strings.TrimSpace(g.combatBuffs[i].OutDamageType))
		if buffType == "" || buffType == "all" || buffType == damageType {
			total += g.combatBuffs[i].OutBonus
		}
	}
	return total
}

// combatBuffInReduce sums the flat incoming-damage reduction from all active buffs.
func (g *MMGame) combatBuffInReduce() int {
	total := 0
	for i := range g.combatBuffs {
		total += g.combatBuffs[i].InReduce
	}
	return total
}

// combatBuffResistPct sums the percentage incoming-damage reduction, capped at
// 90% so the party is never fully immune.
func (g *MMGame) combatBuffResistPct() int {
	total := 0
	for i := range g.combatBuffs {
		total += g.combatBuffs[i].ResistPct
	}
	if total > 90 {
		total = 90
	}
	return total
}

// combatBuffByID returns the active buff for a spell, if any (used by tests/UI).
func (g *MMGame) combatBuffByID(spellID string) (TimedCombatBuff, bool) {
	return buffByID(g.combatBuffs, spellID)
}

// CombatBuffSave is the JSON form of a TimedCombatBuff for save files. Only the
// caster/mastery-derived magnitudes are persisted; OutDamageType is a static
// spell property re-derived from the spell definition on restore (SSoT), so it
// can never drift from spells.yaml.
type CombatBuffSave struct {
	SpellID   string `json:"spell_id"`
	Frames    int    `json:"frames"`
	OutBonus  int    `json:"out_bonus,omitempty"`
	InReduce  int    `json:"in_reduce,omitempty"`
	ResistPct int    `json:"resist_pct,omitempty"`
}

// buildCombatBuffSaves serializes the active buff list for saving.
func buildCombatBuffSaves(buffs []TimedCombatBuff) []CombatBuffSave {
	if len(buffs) == 0 {
		return nil
	}
	out := make([]CombatBuffSave, len(buffs))
	for i, b := range buffs {
		out[i] = CombatBuffSave{b.SpellID, b.Frames, b.OutBonus, b.InReduce, b.ResistPct}
	}
	return out
}

// restoreCombatBuffs rebuilds the active buff list from a save.
func restoreCombatBuffs(saves []CombatBuffSave) []TimedCombatBuff {
	if len(saves) == 0 {
		return nil
	}
	out := make([]TimedCombatBuff, len(saves))
	for i, s := range saves {
		b := TimedCombatBuff{
			SpellID:   s.SpellID,
			Frames:    s.Frames,
			OutBonus:  s.OutBonus,
			InReduce:  s.InReduce,
			ResistPct: s.ResistPct,
		}
		// OutDamageType is static spell data, not run-state: re-derive from the
		// spell definition so it can't drift from spells.yaml (a buff cast before
		// a yaml edit would otherwise restore with the stale type).
		if def, err := spells.GetSpellDefinitionByID(spells.SpellID(s.SpellID)); err == nil {
			b.OutDamageType = def.OutgoingDamageType
		}
		out[i] = b
	}
	return out
}

// tickCombatBuffs decrements every active buff, refreshes its HUD status, and
// drops the expired ones. Called once per frame from updateSpecialEffects.
func (g *MMGame) tickCombatBuffs() {
	g.combatBuffs, _ = tickBuffList(g, g.combatBuffs, func(b *TimedCombatBuff) *int { return &b.Frames })
}
