package game

import (
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
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
	// Per-school resistance (Fire Shield: fire +50): joins the party's gear and
	// card resists in that school's mitigation slot, not the all-damage slot.
	ResistSchool    string
	ResistSchoolPct int
	// ArmorBonus: flat party AC while active (stoneskin draught).
	ArmorBonus int
}

func (b TimedCombatBuff) buffSpellID() string { return b.SpellID }

// timedCombatBuffFromItem is the one item-definition -> runtime mapping for
// timed draughts. Consumption and save restore must not copy this field list.
func timedCombatBuffFromItem(itemKey string, def *config.ItemDefinitionConfig, frames int) (TimedCombatBuff, bool) {
	if itemKey == "" || !def.HasTimedBuff() || frames <= 0 {
		return TimedCombatBuff{}, false
	}
	return TimedCombatBuff{
		SpellID:         itemKey,
		Frames:          frames,
		ResistSchool:    def.ResistBuffSchool,
		ResistSchoolPct: def.ResistBuffSchoolPct,
		ArmorBonus:      def.BuffArmorClass,
	}, true
}

// addCombatBuff activates a buff (same-spell recast refreshes).
func (g *MMGame) addCombatBuff(b TimedCombatBuff) {
	g.combatBuffs = upsertBuff(g.combatBuffs, b)
}

// removeCombatBuff drops a combat buff by spell id (dispel). No-op if absent.
func (g *MMGame) removeCombatBuff(spellID string) {
	g.combatBuffs, _ = removeBuffByID(g, g.combatBuffs, spellID)
}

// combatBuffOutBonusForDamageType sums outgoing-damage bonuses that apply to
// the supplied damage type. Empty/all buff types apply to every outgoing hit.
func (g *MMGame) combatBuffOutBonusForDamageType(damageType string) int {
	targetType, err := damagecalc.ParseType(damageType)
	if err != nil {
		targetType = damagecalc.Physical
	}
	total := 0
	for i := range g.combatBuffs {
		buffType := g.combatBuffs[i].OutDamageType
		if buffType == "" || buffType == "all" {
			total += g.combatBuffs[i].OutBonus
			continue
		}
		if typedBuff, parseErr := damagecalc.ParseType(buffType); parseErr == nil && typedBuff == targetType {
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

// combatBuffSchoolResistPct sums per-school resistance from active buffs
// (Fire Shield) for the given damage school.
func (g *MMGame) combatBuffSchoolResistPct(school string) int {
	targetType, err := damagecalc.ParseType(school)
	if err != nil {
		return 0
	}
	total := 0
	for i := range g.combatBuffs {
		buffType, parseErr := damagecalc.ParseType(g.combatBuffs[i].ResistSchool)
		if parseErr == nil && buffType == targetType {
			total += g.combatBuffs[i].ResistSchoolPct
		}
	}
	return total
}

// combatBuffByID returns the active buff for a spell, if any (used by tests/UI).
func (g *MMGame) combatBuffByID(spellID string) (TimedCombatBuff, bool) {
	return buffByID(g.combatBuffs, spellID)
}

// CombatBuffSave is the JSON form of a TimedCombatBuff for save files.
// Spell-derived magnitudes remain state, while static source metadata is
// re-derived from spells.yaml or items.yaml on restore.
type CombatBuffSave struct {
	SpellID         string `json:"spell_id"`
	Frames          int    `json:"frames"`
	OutBonus        int    `json:"out_bonus,omitempty"`
	InReduce        int    `json:"in_reduce,omitempty"`
	ResistPct       int    `json:"resist_pct,omitempty"`
	ResistSchoolPct int    `json:"resist_school_pct,omitempty"` // school itself re-derived from spells.yaml
	// ResistSchool and ArmorBonus remain for migration of older draught saves.
	ResistSchool string `json:"resist_school,omitempty"`
	ArmorBonus   int    `json:"armor_bonus,omitempty"`
}

// buildCombatBuffSaves serializes the active buff list for saving.
func buildCombatBuffSaves(buffs []TimedCombatBuff) []CombatBuffSave {
	if len(buffs) == 0 {
		return nil
	}
	out := make([]CombatBuffSave, len(buffs))
	for i, b := range buffs {
		out[i] = CombatBuffSave{
			SpellID:         b.SpellID,
			Frames:          b.Frames,
			OutBonus:        b.OutBonus,
			InReduce:        b.InReduce,
			ResistPct:       b.ResistPct,
			ResistSchoolPct: b.ResistSchoolPct,
			ResistSchool:    b.ResistSchool,
			ArmorBonus:      b.ArmorBonus,
		}
		if def, ok := config.GetItemDefinition(b.SpellID); ok && def.HasTimedBuff() {
			out[i].ResistSchoolPct = 0
			out[i].ResistSchool = ""
			out[i].ArmorBonus = 0
		}
	}
	return out
}

func savedCombatBuffItem(s CombatBuffSave) (*config.ItemDefinitionConfig, string, bool) {
	if def, ok := config.GetItemDefinition(s.SpellID); ok && def.HasTimedBuff() {
		return def, s.SpellID, true
	}
	if config.GlobalItems == nil {
		return nil, "", false
	}
	var matched *config.ItemDefinitionConfig
	var matchedKey string
	for key, def := range config.GlobalItems.Items {
		if !def.HasTimedBuff() {
			continue
		}
		if s.ResistSchool != "" && (def.ResistBuffSchool != s.ResistSchool || def.ResistBuffSchoolPct != s.ResistSchoolPct) {
			continue
		}
		if s.ArmorBonus > 0 && def.BuffArmorClass != s.ArmorBonus {
			continue
		}
		if s.ResistSchool == "" && s.ArmorBonus <= 0 {
			continue
		}
		if matched != nil {
			return nil, "", false
		}
		matched, matchedKey = def, key
	}
	return matched, matchedKey, matched != nil
}

// restoreCombatBuffs rebuilds the active buff list from a save.
func restoreCombatBuffs(saves []CombatBuffSave) []TimedCombatBuff {
	if len(saves) == 0 {
		return nil
	}
	out := make([]TimedCombatBuff, len(saves))
	for i, s := range saves {
		b := TimedCombatBuff{
			SpellID:         s.SpellID,
			Frames:          s.Frames,
			OutBonus:        s.OutBonus,
			InReduce:        s.InReduce,
			ResistPct:       s.ResistPct,
			ResistSchoolPct: s.ResistSchoolPct,
			ResistSchool:    s.ResistSchool, // draughts: not spell-backed, school rides the save
			ArmorBonus:      s.ArmorBonus,
		}
		// Static source data is re-derived so a YAML rebalance reaches old saves.
		if def, err := spells.GetSpellDefinitionByID(spells.SpellID(s.SpellID)); err == nil {
			b.OutDamageType = def.OutgoingDamageType
			b.ResistSchool = def.ResistBuffSchool
		} else if def, key, ok := savedCombatBuffItem(s); ok {
			if itemBuff, valid := timedCombatBuffFromItem(key, def, s.Frames); valid {
				b.SpellID = itemBuff.SpellID
				b.ResistSchool = itemBuff.ResistSchool
				b.ResistSchoolPct = itemBuff.ResistSchoolPct
				b.ArmorBonus = itemBuff.ArmorBonus
			}
		}
		out[i] = b
	}
	return out
}

// combatBuffArmorBonus sums flat AC from active buffs (stoneskin draught).
func (g *MMGame) combatBuffArmorBonus() int {
	total := 0
	for i := range g.combatBuffs {
		total += g.combatBuffs[i].ArmorBonus
	}
	return total
}

// tickCombatBuffs decrements every active buff, refreshes its HUD status, and
// drops the expired ones. Called once per frame from updateSpecialEffects.
func (g *MMGame) tickCombatBuffs() {
	g.combatBuffs, _ = tickBuffList(g, g.combatBuffs, func(b *TimedCombatBuff) *int { return &b.Frames })
}
