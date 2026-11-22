package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
	"ugataima/internal/status"
)

// TimedCombatBuff is one active, timed party combat buff. Multiple buffs STACK
// additively (Day of the Gods, Hour of Power, Stone Skin, Heroism, ...): their
// ResistPct / OutBonus / InReduce sum across all active entries. This replaces
// the old single-slot dayGods*/hourPower* fields, so casting one buff no longer
// clobbers another and any number of buff spells can coexist.
type TimedCombatBuff struct {
	CombatClock        bool // Legacy save metadata; all party buffs now share the mode clock.
	MasteryTier        int  // caster's tier for spell and technique magnitudes
	RecoveryPct        int
	ExtraActions       int
	DeferFirstTurnTick bool

	ID            string           // source id: spell, config.TechniqueID, config.CrateID or draught item (HUD status + replace-on-recast key)
	SourceID      string           // optional owner for system-granted cleanup
	Clock         status.DualClock // time remaining; idle for a charge- or absorb-limited buff
	OutBonus      int              // flat add to party outgoing damage
	OutPercent    int              // strongest active draught per school; static item data
	OutDamageType string           // empty/"all" applies to all damage; "physical" applies only to physical attacks
	InReduce      int              // flat reduction of incoming damage (after ResistPct)
	ResistPct     int              // % reduction of incoming damage (applied before InReduce)
	// Per-school resistance (Fire Shield: fire +50): joins the party's gear and
	// card resists in that school's mitigation slot, not the all-damage slot.
	ResistSchool    string
	ResistSchoolPct int
	// ArmorBonus: flat party AC while active (stoneskin draught).
	ArmorBonus    int
	DodgePct      int
	HPRegenPct    int
	ManaRegenPct  int
	Charges       int
	Absorb        int
	RangedWardPct int
	ReflectionPct int
}

func (b TimedCombatBuff) buffKey() string      { return b.ID }
func (b TimedCombatBuff) buffSourceID() string { return b.SourceID }

func (b TimedCombatBuff) active() bool { return b.Clock.Active() || b.Charges > 0 || b.Absorb > 0 }

// Negative HUD duration means budget-limited rather than time-limited.
func (b TimedCombatBuff) statusDuration() int {
	if !b.Clock.Active() && (b.Charges > 0 || b.Absorb > 0) {
		return -1
	}
	return b.Clock.Frames
}

// timedCombatBuffFromItem is the one item-definition -> runtime mapping for
// timed draughts. Consumption and save restore must not copy this field list.
func timedCombatBuffFromItem(itemKey string, def *config.ItemDefinitionConfig, frames int) (TimedCombatBuff, bool) {
	if itemKey == "" || !def.HasTimedBuff() || frames <= 0 {
		return TimedCombatBuff{}, false
	}
	if def.BrewedFrom != "" {
		itemKey = def.BrewedFrom
	}
	return TimedCombatBuff{
		ID:              itemKey,
		Clock:           status.FromFrames(frames, config.GetTargetTPS()),
		ResistSchool:    def.ResistBuffSchool,
		ResistSchoolPct: def.ResistBuffSchoolPct,
		ArmorBonus:      def.BuffArmorClass,
		DodgePct:        def.BuffDodgePct,
		OutDamageType:   def.DamageBuffSchool,
		OutPercent:      def.DamageBuffPct,
		HPRegenPct:      def.BuffHPRegenPct,
		ManaRegenPct:    def.BuffManaRegenPct,
	}, true
}

// timedCombatBuffFromSpell is the one spell-definition -> runtime mapping for
// party combat buffs: an ordinary cast (sourceID "") and a system grant such
// as Celestial Providence. Magnitudes scale with the caster's mastery where
// the spell authors a *_grandmaster cap.
func timedCombatBuffFromSpell(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter, frames int, sourceID string) TimedCombatBuff {
	return TimedCombatBuff{
		ID:              string(spellID),
		SourceID:        sourceID,
		Clock:           status.FromFrames(frames, config.GetTargetTPS()),
		OutBonus:        scaledSpellMasteryValue(def, caster, def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster),
		OutDamageType:   def.OutgoingDamageType,
		InReduce:        scaledIncomingDamageReduction(def, caster),
		ResistPct:       scaledSpellMasteryValue(def, caster, def.ResistBuffPct, def.ResistBuffPctGrandmaster),
		ResistSchool:    def.ResistBuffSchool,
		ResistSchoolPct: def.ResistBuffSchoolPct,
	}
}

// addCombatBuff activates a buff (same-spell recast refreshes).
func (g *MMGame) addCombatBuff(b TimedCombatBuff) {
	g.combatBuffs = upsertBuff(g.combatBuffs, b)
	g.setBuffStatus(b.ID, b.statusDuration())
	g.applyPartyRegenBuffs()
}

// refreshCombatBuffStatus shows a restored buff's HUD timer before its first tick.
func (g *MMGame) refreshCombatBuffStatus(b TimedCombatBuff) {
	g.updateBuffStatus(b.ID, b.statusDuration(), true)
}

// removeCombatBuff drops a combat buff by ownership id. Ordinary casts use the
// spell id; system-owned casts use their source id.
func (g *MMGame) removeCombatBuff(buffID string) bool {
	var removed bool
	g.combatBuffs, removed = removeBuffByID(g, g.combatBuffs, buffID)
	if removed {
		g.applyPartyRegenBuffs()
	}
	return removed
}

// Draught regeneration uses the strongest active percentage per resource,
// regardless of school. Card and natural regeneration remain independent.
func (g *MMGame) applyPartyRegenBuffs() {
	if g.party == nil {
		return
	}
	hp, mana := 0, 0
	for _, b := range g.combatBuffs {
		if b.Clock.Active() {
			hp, mana = max(hp, b.HPRegenPct), max(mana, b.ManaRegenPct)
		}
	}
	for _, member := range g.party.Members {
		if member != nil {
			member.BuffHPRegenPct, member.BuffManaRegenPct = hp, mana
		}
	}
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
	CombatClock        bool `json:"combat_clock,omitempty"`
	MasteryTier        int  `json:"technique_tier,omitempty"`
	DeferFirstTurnTick bool `json:"defer_first_turn_tick,omitempty"`

	ID              string `json:"spell_id"` // source id; the key predates non-spell buffs
	SourceID        string `json:"source_id,omitempty"`
	Frames          int    `json:"frames" ticks:"frames"`
	OutBonus        int    `json:"out_bonus,omitempty"`
	InReduce        int    `json:"in_reduce,omitempty"`
	ResistPct       int    `json:"resist_pct,omitempty"`
	ResistSchoolPct int    `json:"resist_school_pct,omitempty"` // school itself re-derived from spells.yaml
	// ResistSchool and ArmorBonus remain for migration of older draught saves.
	ResistSchool string `json:"resist_school,omitempty"`
	ArmorBonus   int    `json:"armor_bonus,omitempty"`
	Charges      int    `json:"charges,omitempty"`
	Absorb       int    `json:"absorb,omitempty"`
}

// buildCombatBuffSaves serializes the active buff list for saving.
func buildCombatBuffSaves(buffs []TimedCombatBuff) []CombatBuffSave {
	if len(buffs) == 0 {
		return nil
	}
	out := make([]CombatBuffSave, len(buffs))
	for i, b := range buffs {
		out[i] = CombatBuffSave{
			Charges: b.Charges, Absorb: b.Absorb,
			ID:          b.ID,
			CombatClock: b.CombatClock, MasteryTier: b.MasteryTier, DeferFirstTurnTick: b.DeferFirstTurnTick,
			SourceID:        b.SourceID,
			Frames:          b.Clock.Frames,
			OutBonus:        b.OutBonus,
			InReduce:        b.InReduce,
			ResistPct:       b.ResistPct,
			ResistSchoolPct: b.ResistSchoolPct,
			ResistSchool:    b.ResistSchool,
			ArmorBonus:      b.ArmorBonus,
		}
		if def, ok := config.GetItemDefinition(b.ID); ok && def.HasTimedBuff() {
			out[i].ResistSchoolPct = 0
			out[i].ResistSchool = ""
			out[i].ArmorBonus = 0
		}
	}
	return out
}

func savedCombatBuffItem(s CombatBuffSave) (*config.ItemDefinitionConfig, string, bool) {
	if def, ok := config.GetItemDefinition(s.ID); ok && def.HasTimedBuff() {
		return def, s.ID, true
	}
	if config.GlobalItems == nil {
		return nil, "", false
	}
	var matched *config.ItemDefinitionConfig
	var matchedKey string
	for key, def := range config.GlobalItems.Items {
		if !def.HasTimedBuff() || def.BrewedFrom != "" {
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
		if _, err := spells.GetSpellDefinitionByID(spells.SpellID(s.ID)); err != nil && config.Technique(s.ID) != nil {
			s.ID = config.TechniqueID(s.ID) // older saves stored the bare technique key
		}
		b := TimedCombatBuff{
			Charges: max(0, s.Charges), Absorb: max(0, s.Absorb),
			ID:          s.ID,
			CombatClock: s.CombatClock, MasteryTier: s.MasteryTier, DeferFirstTurnTick: s.DeferFirstTurnTick,
			SourceID:        s.SourceID,
			Clock:           status.FromFrames(s.Frames, config.GetTargetTPS()),
			OutBonus:        s.OutBonus,
			InReduce:        s.InReduce,
			ResistPct:       s.ResistPct,
			ResistSchoolPct: s.ResistSchoolPct,
			ResistSchool:    s.ResistSchool, // draughts: not spell-backed, school rides the save
			ArmorBonus:      s.ArmorBonus,
		}
		if d := config.TechniqueFromID(s.ID); d != nil {
			b.CombatClock = true
			applyTechniqueBuff(&b, d, s.MasteryTier)
		}
		// Static source data is re-derived so a YAML rebalance reaches old saves.
		if def, err := spells.GetSpellDefinitionByID(spells.SpellID(s.ID)); err == nil {
			if def.Mechanic != nil {
				restoreAdvancedBuff(&b, def)
			}
			b.OutDamageType = def.OutgoingDamageType
			b.ResistSchool = def.ResistBuffSchool
		} else if def, key, ok := savedCombatBuffItem(s); ok {
			if itemBuff, valid := timedCombatBuffFromItem(key, def, s.Frames); valid {
				b.ID = itemBuff.ID
				b.ResistSchool = itemBuff.ResistSchool
				b.ResistSchoolPct = itemBuff.ResistSchoolPct
				b.ArmorBonus = itemBuff.ArmorBonus
				b.DodgePct = itemBuff.DodgePct
				b.OutPercent = itemBuff.OutPercent
				b.OutDamageType = itemBuff.OutDamageType
				b.HPRegenPct = itemBuff.HPRegenPct
				b.ManaRegenPct = itemBuff.ManaRegenPct
			}
		}
		if def := config.CrateBuff(s.ID); def != nil {
			applyCrateBuff(&b, def)
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

// tickCombatBuffs spends a real-time frame of every active buff; a turn-based
// round is spent at the party turn. Called once per frame from
// updateSpecialEffects.
func (g *MMGame) tickCombatBuffs() {
	if g.turnBasedMode {
		return
	}
	for i := range g.combatBuffs {
		g.combatBuffs[i].DeferFirstTurnTick = false
	}
	g.advanceCombatBuffs(false)
}

func (g *MMGame) combatBuffDodgePct() int {
	total := 0
	for _, b := range g.combatBuffs {
		total += b.DodgePct
	}
	return total
}

// advanceCombatBuffs spends one tick of every combat buff, refreshes its HUD
// status, and drops the expired ones.
func (g *MMGame) advanceCombatBuffs(turn bool) {
	var expired bool
	g.combatBuffs, expired = tickBuffList(g, g.combatBuffs, turn, func(b *TimedCombatBuff, turn bool) int {
		if !b.Clock.Active() {
			return b.statusDuration()
		}
		if turn && b.DeferFirstTurnTick {
			b.DeferFirstTurnTick = false
		} else {
			g.tickBuffClock(b.SourceID, &b.Clock, turn)
		}
		if !b.Clock.Active() {
			b.Charges, b.Absorb = 0, 0
		}
		return b.statusDuration()
	})
	if expired {
		g.applyPartyRegenBuffs()
	}
}

func applyTechniqueBuff(b *TimedCombatBuff, d *config.TechniqueDefinition, tier int) {
	switch d.Kind {
	case config.TechniqueDodge:
		b.DodgePct = config.TierValue(d.Power, tier)
	case config.TechniqueHaste:
		b.RecoveryPct, b.ExtraActions = config.TierValue(d.Power, tier), config.TierValue(d.TBPower, tier)
	}
}
func applyCrateBuff(b *TimedCombatBuff, d *config.CrateBuffConfig) {
	if limit := d.DurationSeconds * config.GetTargetTPS(); b.Clock.Frames > limit {
		b.Clock = status.FromFrames(limit, config.GetTargetTPS())
	}
	b.HPRegenPct, b.ManaRegenPct = d.HPRegenPct, d.ManaRegenPct
}
