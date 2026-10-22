package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stats"
	"ugataima/internal/status"
)

// Mana regeneration tunables. SP regenerates by (1 + Personality/divisor +
// the Meditation bonus) every regeneration period (TickRegen). Kept in
// this package because game's balance.go can't be imported from
// internal/character (circular).
const (
	ManaRegenPersonalityDivisor = 10
	MaxSPPersonalityDivisor     = 3
	// MeditationRegenPerTier: extra SP restored per regen tick per Meditation
	// mastery tier (Novice=0 -> bonus from Expert up), making mana recovery faster.
	MeditationRegenPerTier = 3
	// BodybuildingHPPerTier: bonus Max HP per Bodybuilding mastery tier.
	BodybuildingHPPerTier = 8
	// BodybuildingGMMaxHPPct: a Grandmaster bodybuilder gets this percent of extra
	// Max HP (on the pre-bonus base) on top of the flat per-tier amount.
	BodybuildingGMMaxHPPct = 10
)

// bodybuildingBonusHP returns the Bodybuilding contribution to Max HP for a
// given base (pre-bonus) Max HP: a flat per-tier amount plus, at Grandmaster, a
// percentage of the base. Single source for CalculateDerivedStats and
// RecalculateMaxStatsKeepingCurrent so the two can't drift.
func (c *MMCharacter) bodybuildingBonusHP(baseMaxHP int) int {
	tier := c.SkillTier(SkillBodybuilding)
	bonus := tier * BodybuildingHPPerTier
	if tier >= int(MasteryGrandMaster) {
		bonus += baseMaxHP * BodybuildingGMMaxHPPct / 100
	}
	return bonus
}

// SkillTier returns a character's mastery level for a skill as an int
// (Novice=0, Expert=1, Master=2, Grandmaster=3), or 0 if they lack the skill.
// Used to scale skill effects, mirroring weapon/armor/magic mastery bonuses.
// HasSkill reports whether the character has opened a skill (any mastery).
func (c *MMCharacter) HasSkill(skill SkillType) bool {
	_, ok := c.Skills[skill]
	return ok
}

func (c *MMCharacter) SkillTier(skill SkillType) int {
	if s, ok := c.Skills[skill]; ok && s != nil {
		return int(s.Mastery)
	}
	return 0
}

// Convenience tier accessors for misc skills (the game package scales these by
// its own per-tier constants). Methods avoid the package-name shadowing in
// combat functions whose parameter is literally named `character`.
func (c *MMCharacter) ArmsMasterTier() int { return c.SkillTier(SkillArmsMaster) }
func (c *MMCharacter) DisarmTrapTier() int { return c.SkillTier(SkillDisarmTrap) }
func (c *MMCharacter) MerchantTier() int   { return c.SkillTier(SkillMerchant) }

// QuickSlotsPerRow is one quick-slot bar (the 5-cell frame art); a hero has
// QuickSlotRows of them, the second shown only with QuickSecondRow.
const (
	QuickSlotsPerRow = 5
	QuickSlotRows    = 2
	QuickSlotCount   = QuickSlotsPerRow * QuickSlotRows
)

// QuickRows is how many quick-slot rows the hero shows: 1, or 2 with the
// second row on.
func (c *MMCharacter) QuickRows() int {
	if c != nil && c.QuickSecondRow {
		return QuickSlotRows
	}
	return 1
}

// AdventureExperience follows the hero through roster swaps and saves.
type AdventureExperience struct {
	Generation int `json:"generation"`
	Amount     int `json:"amount"`
}

type MMCharacter struct {
	// ActorID is the hero's stable identity for marks it leaves on monsters
	// (Catalysis): assigned on first use, saved.
	ActorID        uint64
	ReactiveCombat ReactiveCombatState
	Clockwork      ClockworkGearState
	AdventureXP    map[string]AdventureExperience
	Inventory      []items.Item // Personal bag; travels with this hero through roster changes.
	RareClass      RareClassState
	SpellState     SpellState
	Name           string
	Class          CharacterClass
	Promotion      Promotion // elite status (Archmage/Lich); PromotionNone by default
	Race           string    // config.yaml race key; persisted because racial traits are gameplay state

	// Core stats
	Level          int
	Experience     int
	HitPoints      int
	MaxHitPoints   int
	SpellPoints    int
	MaxSpellPoints int

	// BuffBonuses is the aggregate of active temporary stat buffs (Bless, ...),
	// pushed by the game whenever a buff is applied/expires/loads. Effective
	// stats AND derived MaxHP/MaxSP read it, so buffs behave like real stats.
	// Runtime-only: rebuilt from buff state on load, never saved per character.
	BuffBonuses StatBonuses
	// PermanentBonuses are one-time permanent stat gains (stat barrels): part of
	// the EFFECTIVE stats (green delta on the sheet), never of the base stats,
	// persisted in the save.
	PermanentBonuses StatBonuses

	// BonusMaxHP is a flat MaxHP addition from an external source (the Jungle
	// Idol Card), pushed the same way as BuffBonuses. Runtime-only.
	BonusMaxHP int
	// BonusWeaponBaseDamage is the active party card bonus, rebuilt on load.
	BonusWeaponBaseDamage int

	// Primary attributes
	Might       int // Physical strength and melee damage
	Intellect   int // Spell points and spell damage
	Personality int // Spell points and magic resistance
	Endurance   int // Hit points and resistances
	Accuracy    int // Ranged attack accuracy
	Speed       int // Recovery time and initiative
	Luck        int // Critical hits and various bonuses

	// Skills (each has a level and mastery)
	Skills map[SkillType]*Skill

	// Magic schools
	MagicSchools map[MagicSchoolID]*MagicSkill

	// Equipment slots
	Equipment map[items.EquipSlot]items.Item

	// QuickSlots is a small per-character container (like a 5-cell pocket) for
	// mouse-driven quick use: weapons to swap to, potions to drink, spells to
	// cast - all by double-click. An item dragged here LEAVES the shared party
	// inventory and lives in the slot (nil = empty). Physical slot items still
	// count as carried stock for quests, crafting and exchanges. Spells hold a temporary
	// spell item (spellbook-owned; never returned to inventory). Independent of
	// the F/Space quick action (Equipment[SlotQuickAction]). Row I is
	// QuickSlots[:QuickSlotsPerRow]; row II, the rest, holds items only while
	// QuickSecondRow is on.
	QuickSlots     [QuickSlotCount]*items.Item
	QuickSecondRow bool

	// Status effects
	Conditions []Condition
	// Consumable and target-mark cooldowns on the one RT/TB clock.
	AutoDrinkCD        status.DualClock
	DesignatedTargetID string
	Designation        status.DualClock
	// AutocastCD is the hero's one autocast budget, shared by spellbook and
	// technique autocasts.
	AutocastCD status.DualClock

	// Poison status timer and tick accumulator (frames).
	PoisonFramesRemaining int
	poisonTickTimer       int
	// Ignite (burn): a separate DoT 3x as strong as poison that STACKS with it.
	BurnFramesRemaining int
	burnTickTimer       int
	// Stun: skips the character's actions.
	Stun status.DualClock
	// ArmorMelt: a melting monster's hit leaves the hero's armor blunting a
	// physical hit only as far as an elemental one (ArmorMelted).
	ArmorMelt status.DualClock
	// Stun diminishing returns (status.StunDRChain): chain step plus the
	// stun-free time left before the chain resets.
	StunDRStacks int
	StunDRMemory status.DualClock
	// ScaleStacks: equipped scale-growth armor accumulated this combat.
	// Runtime-only - stacks shed the moment combat ends (never saved).
	ScaleStacks int

	// SPRegen and HPRegen run to the next regeneration payouts (TickRegen);
	// SPRegenFight is the SP period's kind, the fight one or the calm one.
	SPRegen      status.DualClock
	HPRegen      status.DualClock
	SPRegenFight bool

	// BonusRegenPct is a % of max HP regenerated every regeneration period,
	// pushed by the game (Troll Cards) the same way as BonusMaxHP. Runtime-only.
	BonusRegenPct int
	// Timed party draught bonuses are derived from active buffs, never saved here.
	BuffHPRegenPct   int
	BuffManaRegenPct int

	// Free stat points to distribute on level-up
	FreeStatPoints int

	// OwedLevelChoices are levels at which this character earned a class
	// level-up choice that hasn't been made yet. Active members queue choices
	// immediately; benched members (no party slot) bank them here until swapped
	// in. Persisted so they survive save/load.
	OwedLevelChoices []int

	// ActionsRemaining tracks how many attack/spell actions this character
	// has left in the current turn-based round. Refilled at party-turn start
	// (1 base action plus any party-wide Speed bonus assigned to this member),
	// decremented on each attack/spell, set to 0 on party movement (which
	// immediately ends the round). Unused in real-time.
	ActionsRemaining int

	// TBRoundActionFloor is the personal action floor actually credited at the
	// start of the current TB round. Equipment changes may withdraw that
	// credited portion, but equipping an action-floor weapon mid-round cannot
	// mint fresh actions. Persisted through CharacterSave for suspended rounds.
	TBRoundActionFloor int

	// RTCooldown is this character's remaining real-time action cooldown in
	// frames. While > 0 the member is "busy" (grayed in the HUD, skipped by
	// auto-select); set on each attack/cast from the weapon/spell cooldown and
	// ticked down once per frame. Unused in turn-based mode (which uses
	// ActionsRemaining instead).
	RTCooldown int

	// OffHandRTCooldown mirrors RTCooldown for the off-hand weapon (Dual
	// Wielding only) - the two cooldowns run independently, so a dual-wielder
	// stays "ready" (rtCharReady) as long as EITHER hand is off cooldown. Unused
	// by anyone without a weapon in SlotOffHand.
	OffHandRTCooldown int

	// NextTBAttackOffHand is the Dual Wielding hand cursor: false = the next
	// attack prefers the main hand, true = the off hand. TB flips after every
	// weapon action; RT sets it to the opposite hand after each weapon swing but
	// still lets a ready hand act if the preferred hand is cooling down. Reset
	// false at the start of every party turn. Unused without Dual Wielding.
	NextTBAttackOffHand bool
}

// Turn-based party bonus-action thresholds on effective Speed. Single source
// shared by MMGame.startPartyTurn (mechanic) and the Speed stat tooltip.
const (
	SpeedBonusAction1Threshold = 25 // Any living Speed > this -> +1 party bonus action
	SpeedBonusAction2Threshold = 50 // Any living Speed > this -> +2 party bonus actions
)

// SpeedBonusActionTier returns how many party-wide bonus action slots this
// character unlocks if they are the fastest living member this turn.
func (c *MMCharacter) SpeedBonusActionTier() int {
	speed := c.GetEffectiveSpeed()
	switch {
	case speed > SpeedBonusAction2Threshold:
		return 2
	case speed > SpeedBonusAction1Threshold:
		return 1
	default:
		return 0
	}
}

// CanDualWield reports whether c is ALLOWED an off-hand weapon at all: the ONE
// "an off-hand weapon requires the Dual Wielding skill" rule, shared by
// IsDualWielding, the equip overflow, and the slot-fit check so the rule can't
// drift between them.
func (c *MMCharacter) CanDualWield() bool {
	return c != nil && c.HasSkill(SkillDualWielding)
}

// IsDualWielding reports whether c has the Dual Wielding skill AND an actual
// weapon (not a shield or nothing) in the off-hand. Dual Wielding's perks -
// the personal extra TB action, the independent off-hand RT cooldown - only
// kick in while both hold. Loading returns gear that no longer fits its slot
// to the bag (ItemBelongsInSlot); the skill check still keeps the perks off
// whatever else fills the slot.
func (c *MMCharacter) IsDualWielding() bool {
	if !c.CanDualWield() {
		return false
	}
	off, ok := c.Equipment[items.SlotOffHand]
	return ok && off.Type == items.ItemWeapon
}

// AnyWeaponHandReady reports whether at least one hand that ACTUALLY carries
// a weapon is off cooldown in real time - an unequipped main hand can't be
// "ready" just because its stale cooldown happens to have cleared. For most
// characters that's just RTCooldown; a Dual Wielding character with a weapon
// in the off-hand is ALSO ready once that hand's own cooldown clears, even
// while the main hand is still cycling.
func (c *MMCharacter) AnyWeaponHandReady() bool {
	if c == nil {
		return false
	}
	if _, ok := c.Equipment[items.SlotMainHand]; ok && c.RTCooldown <= 0 {
		return true
	}
	return c.IsDualWielding() && c.OffHandRTCooldown <= 0
}

// MainHandArmed reports whether the main hand carries a weapon. Only weapons
// ever occupy that slot, so its occupancy IS the test - and it can be empty:
// a caster or an unarmed brawler fights with nothing there.
func (c *MMCharacter) MainHandArmed() bool {
	if c == nil {
		return false
	}
	_, ok := c.Equipment[items.SlotMainHand]
	return ok
}

// HasWeaponInEitherHand reports whether a weapon (not a shield/nothing) is
// equipped in the main hand or, for a Dual Wielding character, the off-hand.
func (c *MMCharacter) HasWeaponInEitherHand() bool {
	return c.MainHandArmed() || c.IsDualWielding()
}

// CanAct reports whether this character is alive and conscious. It is used for
// targeting and round bookkeeping; actor-specific gates additionally account
// for statuses such as stun.
// IsLiving reports a hero with HP left who is neither dead nor eradicated.
// Allied spells and shared healing pools target living heroes.
func (c *MMCharacter) IsLiving() bool {
	return c != nil && c.HitPoints > 0 && !c.HasCondition(ConditionDead) && !c.HasCondition(ConditionEradicated)
}

// Revive is the one return to life: Unconscious and Dead clear (Eradicated
// too when eradicated is set) and HP rises to hp, at least 1, never lowered.
func (c *MMCharacter) Revive(hp int, eradicated bool) {
	c.RemoveCondition(ConditionUnconscious)
	c.RemoveCondition(ConditionDead)
	if eradicated {
		c.RemoveCondition(ConditionEradicated)
	}
	c.HitPoints = max(c.HitPoints, hp, 1)
}

func (c *MMCharacter) CanAct() bool {
	if c == nil || c.HitPoints <= 0 {
		return false
	}
	for _, cond := range c.Conditions {
		if cond == ConditionUnconscious {
			return false
		}
	}
	return true
}

// CanUseCombatAction reports whether this character may be the actor for an
// attack, spell, or trap. CanAct deliberately remains the broader "living
// target" predicate: stunned heroes can still be hit, healed, and revived.
func (c *MMCharacter) CanUseCombatAction() bool {
	return c.CanAct() && !c.IsStunned()
}

type CharacterClass int

const (
	ClassKnight CharacterClass = iota
	ClassPaladin
	ClassArcher
	ClassCleric
	ClassSorcerer
	ClassDruid
	ClassThief
	ClassArmsMaster
	ClassMonk
	ClassBattleMage
	ClassSniper
	ClassAlchemist
	ClassWayfarer
)

// Promotion is a mutually-exclusive elite status a spellcaster can earn:
// Archmage (Light magic) via the Mage Tower trial, or Lich (Dark magic) via a
// phylactery. PromotionNone is the default.
type Promotion int

const (
	PromotionNone Promotion = iota
	PromotionArchmage
	PromotionLich
)

// promotionKeys is the authored vocabulary of promotions (a quest's
// rewards.promotion, promoted sprite suffixes).
var promotionKeys = map[Promotion]string{PromotionArchmage: "archmage", PromotionLich: "lich"}

// PromotionByKey resolves an authored promotion key.
func PromotionByKey(key string) (Promotion, bool) {
	for p, k := range promotionKeys {
		if k == key {
			return p, true
		}
	}
	return PromotionNone, false
}

// Key is the promotion's authored key, "" for none.
func (p Promotion) Key() string { return promotionKeys[p] }

// Title is the promoted class name, "" for none.
func (p Promotion) Title() string {
	switch p {
	case PromotionArchmage:
		return "Archmage"
	case PromotionLich:
		return "Lich"
	}
	return ""
}

// RejectsUndead: the Archmage's Light is refused to a party holding a lich.
func (p Promotion) RejectsUndead() bool { return p == PromotionArchmage }

// IsLich is the promotion predicate behind party traits (hates.yaml aggro).
func (c *MMCharacter) IsLich() bool { return c.Promotion == PromotionLich }

// ClassDisplayName returns the promoted title if any, else the base class name.
func (c *MMCharacter) ClassDisplayName() string {
	if t := c.Promotion.Title(); t != "" {
		return t
	}
	return c.Class.String()
}

func CreateCharacter(name string, class CharacterClass, cfg *config.Config) *MMCharacter {
	char := &MMCharacter{
		Name:         name,
		Class:        class,
		Level:        1,
		Experience:   0,
		Skills:       make(map[SkillType]*Skill),
		MagicSchools: make(map[MagicSchoolID]*MagicSkill),
		Equipment:    make(map[items.EquipSlot]items.Item),
		Conditions:   make([]Condition, 0),
	}

	char.applyClassKit(cfg)

	char.CalculateDerivedStats(cfg)
	return char
}

// applyClassKit applies the YAML class kit (config.yaml characters.classes):
// base attributes, starting skills, magic schools with known spells, main-hand
// weapon and quick-slot spell. Unknown keys panic at character creation -
// content bugs must surface immediately, not as a silently gimped hero.
func (c *MMCharacter) applyClassKit(cfg *config.Config) {
	key := c.Class.Key()
	stats, ok := cfg.Characters.Classes[key]
	if !ok {
		panic(fmt.Sprintf("class %q missing from config.yaml characters.classes", key))
	}

	c.SetBaseStats(stats.Base)

	c.ensureClassKitSkills(stats, key, cfg)

	for _, entry := range stats.Magic {
		school := MagicSchoolID(entry.School)
		if !isKnownMagicSchool(school) {
			panic(fmt.Sprintf("class %q: unknown magic school %q in config.yaml", key, entry.School))
		}
		known := make([]spells.SpellID, len(entry.Spells))
		for i, sp := range entry.Spells {
			known[i] = spells.SpellID(sp)
		}
		c.MagicSchools[school] = &MagicSkill{Mastery: MasteryNovice, KnownSpells: known}
	}

	if stats.MainHand != "" {
		c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(stats.MainHand)
	}
	// Each starting item routes by its own equip_slot (helmet/ring/boots land in
	// their real slots; body armor stays in SlotArmor). Rings resolve through
	// the shared two-slot ring rule like any player equip.
	for _, itemKey := range stats.Equipment {
		it := items.CreateItemFromYAML(itemKey)
		slot := it.PreferredSlot(items.SlotArmor)
		if slot == items.SlotRing1 {
			if _, taken := c.Equipment[items.SlotRing1]; taken {
				slot = items.SlotRing2
			}
		}
		c.Equipment[slot] = it
	}
	if stats.QuickTrap != "" {
		// The starting trap occupies the SAME quick slot as quick spells.
		// Fail fast on a bad key, but only when traps.yaml is loaded -
		// config-less unit tests build parties without the trap catalog.
		if it, ok := config.TrapItem(stats.QuickTrap); ok {
			c.Equipment[items.SlotQuickAction] = it
		} else if config.GlobalTrapConfig != nil {
			panic(fmt.Sprintf("class %q: unknown quick_trap %q in config.yaml", key, stats.QuickTrap))
		}
	}
	if stats.QuickSpell != "" {
		if spellItem, err := spells.CreateSpellItem(spells.SpellID(stats.QuickSpell)); err == nil {
			c.Equipment[items.SlotQuickAction] = spellItem
		}
	}
}

func (c *MMCharacter) ensureClassKitSkills(stats config.ClassStats, key string, cfg *config.Config) bool {
	if c.Skills == nil {
		c.Skills = make(map[SkillType]*Skill)
	}
	added := false
	for _, sk := range stats.Skills {
		st, ok := SkillTypeFromKey(sk)
		if !ok {
			panic(fmt.Sprintf("class %q: unknown skill key %q in config.yaml", key, sk))
		}
		// A race may swap this kit skill (the half-orc Knight's Orcish Fury).
		// Do not re-add the replaced skill on every save migration.
		if replaced, _, ok := c.racialSkillReplacement(cfg); ok && st == replaced {
			continue
		}
		mastery := MasteryNovice
		if startTier, ok := stats.SkillStartMastery[sk]; ok {
			m, ok := masteryFromKey(startTier)
			if !ok {
				panic(fmt.Sprintf("class %q: unknown skill_start_mastery value %q for %q in config.yaml", key, startTier, sk))
			}
			mastery = m
		}
		if _, exists := c.Skills[st]; !exists {
			c.Skills[st] = &Skill{Mastery: mastery}
			added = true
		}
	}
	return added
}

// EnsureClassKitSkills migrates an existing character to the current
// data-authored class kit. It only adds absent skills; earned mastery remains
// untouched.
func (c *MMCharacter) EnsureClassKitSkills(cfg *config.Config) bool {
	if c == nil || cfg == nil {
		return false
	}
	key := c.Class.Key()
	stats, ok := cfg.Characters.Classes[key]
	if !ok {
		panic(fmt.Sprintf("class %q missing from config.yaml characters.classes", key))
	}
	return c.ensureClassKitSkills(stats, key, cfg)
}

func isKnownMagicSchool(id MagicSchoolID) bool {
	for _, s := range AllMagicSchools {
		if s == id {
			return true
		}
	}
	return false
}

// ApplyRace shifts base attributes by the race's additive modifiers
// (config.yaml characters.races; human is the empty baseline). Call before
// CalculateDerivedStats so HP/SP reflect the shifted stats.
func (c *MMCharacter) ApplyRace(race string, cfg *config.Config) {
	if race == "" {
		return
	}
	mods, ok := cfg.Characters.Races[race]
	if !ok {
		panic(fmt.Sprintf("race %q missing from config.yaml characters.races", race))
	}
	for i, p := range c.baseStatFields() {
		*p += *mods.Shifts.At(i)
	}
	c.Race = race
	c.EnsureRacialTraits(cfg)
}

// EnsureRacialTraits applies the race-owned skill kit without touching racial
// stat modifiers. It is safe on restored characters: absent passives are added,
// fixed passives are normalized, and a half-orc Knight's replaced class skill
// migrates its earned mastery to Orcish Fury.
func (c *MMCharacter) EnsureRacialTraits(cfg *config.Config) bool {
	if c == nil {
		return false
	}
	changed := false
	if c.Race == "" && cfg != nil {
		for _, roster := range [][]config.RosterEntry{
			cfg.Characters.StartingParty,
			cfg.Characters.Captives,
			cfg.Characters.TavernRecruits,
		} {
			for _, entry := range roster {
				if entry.Name == c.Name && entry.Race != "" {
					c.Race = entry.Race
					changed = true
					break
				}
			}
			if c.Race != "" {
				break
			}
		}
	}
	if c.Skills == nil {
		c.Skills = make(map[SkillType]*Skill)
	}
	ensureFixed := func(skillType SkillType) {
		if skill := c.Skills[skillType]; skill == nil {
			c.Skills[skillType] = &Skill{Mastery: MasteryNovice}
			changed = true
		} else if skill.Mastery != MasteryNovice {
			skill.Mastery = MasteryNovice
			changed = true
		}
	}
	if race, ok := c.raceConfig(cfg); ok && race.PassiveSkill != "" {
		if st, ok := SkillTypeFromKey(race.PassiveSkill); ok {
			ensureFixed(st)
		}
	}
	if replaced, replacement, ok := c.racialSkillReplacement(cfg); ok {
		mastery := MasteryNovice
		if old := c.Skills[replaced]; old != nil {
			mastery = old.Mastery
			delete(c.Skills, replaced)
			changed = true
		}
		if skill := c.Skills[replacement]; skill == nil {
			c.Skills[replacement] = &Skill{Mastery: mastery}
			changed = true
		} else if mastery > skill.Mastery {
			skill.Mastery = mastery
			changed = true
		}
	}
	return changed
}

// raceConfig is the hero's race entry (cfg, else the loaded config).
func (c *MMCharacter) raceConfig(cfg *config.Config) (config.RaceStats, bool) {
	if cfg == nil {
		cfg = config.GlobalConfig
	}
	if cfg == nil || c.Race == "" {
		return config.RaceStats{}, false
	}
	race, ok := cfg.Characters.Races[c.Race]
	return race, ok
}

// racialSkillReplacement is shared by class-kit initialization and racial
// migration, so a replaced skill cannot be reintroduced on the next load.
func (c *MMCharacter) racialSkillReplacement(cfg *config.Config) (SkillType, SkillType, bool) {
	race, ok := c.raceConfig(cfg)
	if !ok || race.Replaces == nil || race.Replaces.Class != c.Class.Key() {
		return 0, 0, false
	}
	replaced, okSkill := SkillTypeFromKey(race.Replaces.Skill)
	with, okWith := SkillTypeFromKey(race.Replaces.With)
	return replaced, with, okSkill && okWith
}

// derivedStatMultipliers returns the HP/SP formula multipliers, falling back to
// the shipped config.yaml values when no config is reachable (equip paths in
// minimal tests).
func derivedStatMultipliers(cfg *config.Config) (endurMult, levelHPMult, levelSPMult int) {
	if cfg == nil {
		cfg = config.GlobalConfig
	}
	if cfg == nil {
		return 2, 3, 2
	}
	return cfg.Characters.HitPoints.EnduranceMultiplier,
		cfg.Characters.HitPoints.LevelMultiplier,
		cfg.Characters.SpellPoints.LevelMultiplier
}

// recomputeMaxFromEffective derives MaxHP/MaxSP from the EFFECTIVE stats
// (base + equipment + buffs) - the single formula every recalc path shares:
//
//	MaxHP = effEndurancexendurMult + LevelxlevelHPMult (+ Bodybuilding)
//	MaxSP = effIntellect + effPersonality/MaxSPPersonalityDivisor + LevelxlevelSPMult
func (c *MMCharacter) recomputeMaxFromEffective(cfg *config.Config) {
	endurMult, levelHPMult, levelSPMult := derivedStatMultipliers(cfg)
	_, effInt, effPers, effEnd, _, _, _ := c.GetEffectiveStats()
	baseMaxHP := effEnd*endurMult + c.Level*levelHPMult
	c.MaxHitPoints = baseMaxHP + c.bodybuildingBonusHP(baseMaxHP) + c.BonusMaxHP
	c.MaxSpellPoints = effInt + effPers/MaxSPPersonalityDivisor + c.Level*levelSPMult
}

// CalculateDerivedStats recomputes MaxHP/MaxSP and FULLY RESTORES current
// HP/SP for character creation. Level-ups use RecalculateMaxStatsKeepingCurrent;
// explicit stat/skill gains use RecalculateMaxStatsGrantingGain.
func (c *MMCharacter) CalculateDerivedStats(cfg *config.Config) {
	c.recomputeMaxFromEffective(cfg)
	c.HitPoints = c.MaxHitPoints
	c.SpellPoints = c.MaxSpellPoints
}

// RecalculateMaxStatsKeepingCurrent recomputes MaxHP/MaxSP for level-ups and
// reversible stat changes (equip/unequip, buff apply/expire): the maxima move, the
// CURRENT values never grow - only get capped. Granting current on a gain
// here would be a pump: equip +End (+HP granted) -> unequip (cap can't take it
// back) -> repeat until full. Irreversible gains (spending a stat point,
// training Bodybuilding) go through RecalculateMaxStatsGrantingGain.
func (c *MMCharacter) RecalculateMaxStatsKeepingCurrent(cfg *config.Config) {
	c.recomputeMaxFromEffective(cfg)

	if c.HitPoints > c.MaxHitPoints {
		c.HitPoints = c.MaxHitPoints
	}
	if c.SpellPoints > c.MaxSpellPoints {
		c.SpellPoints = c.MaxSpellPoints
	}
}

// RecalculateMaxStatsGrantingGain is the IRREVERSIBLE-change variant: a max
// increase is also added to the current value (spending an Endurance point
// immediately gives its HP). Never use for equip/buff changes - those can be
// reverted and would pump current HP/SP through cycles.
func (c *MMCharacter) RecalculateMaxStatsGrantingGain(cfg *config.Config) {
	oldMaxHP := c.MaxHitPoints
	oldMaxSP := c.MaxSpellPoints

	c.recomputeMaxFromEffective(cfg)

	if c.MaxHitPoints > oldMaxHP {
		c.HitPoints += c.MaxHitPoints - oldMaxHP
	}
	if c.HitPoints > c.MaxHitPoints {
		c.HitPoints = c.MaxHitPoints
	}
	if c.MaxSpellPoints > oldMaxSP {
		c.SpellPoints += c.MaxSpellPoints - oldMaxSP
	}
	if c.SpellPoints > c.MaxSpellPoints {
		c.SpellPoints = c.MaxSpellPoints
	}
}

// PartyClock is what one party frame runs on. Turn-based rounds tick their
// cadences in the game's endPartyTurn; real time ticks here, and a fight
// spaces SP regen to the turn-based pace.
type PartyClock struct {
	TurnBased bool
	InCombat  bool
}

// UpdateWithMode advances the character one RT frame on the party clock.
// Optional absorbers reduce pending DoT damage before HP is deducted.
func (c *MMCharacter) UpdateWithMode(clock PartyClock, absorbDoT ...func(int) int) {
	// Turn-based mode: regen and poison/burn advance once per party turn
	// (TickRegen from endPartyTurn, TickPoisonTurn/TickBurnTurn from
	// startPartyTurn), so deliberating over a move doesn't bleed real-time HP.
	if clock.TurnBased {
		return
	}
	c.updateRegenAndPoison(clock.InCombat, absorbDoT...)
}

// A stun is a rated status.DualClock: a mode switch keeps its spent part.

// tickStunFrames counts down a real-time stun, clearing it when the timer ends.
// Rated: the TB clock drains proportionally so toggling combat mode cannot
// hand the stun its frozen turn count back.
func (c *MMCharacter) tickStunFrames() {
	if !c.IsStunned() {
		c.StunDR().Forget(false)
		return
	}
	if c.Stun.TickFrame() {
		c.RemoveCondition(ConditionStunned)
	}
}

// StunDR exposes this hero's diminishing-returns chain to the shared rule.
func (c *MMCharacter) StunDR() status.StunDRChain {
	return status.StunDRChain{Stacks: &c.StunDRStacks, Memory: &c.StunDRMemory}
}

// TickStunTurn counts down a turn-based stun at the start of the party's turn.
func (c *MMCharacter) TickStunTurn() {
	if c.Stun.TickTurn() {
		c.RemoveCondition(ConditionStunned)
	}
}

// TickPoisonTurn advances poison by one TB round: the round consumes
// elapsedFrames of duration and deals the damage that span is worth (one tick
// per second, so a three-second round bites three times). Mirrors
// monster.TickPoisonTurn; called once per party turn from startPartyTurn.
// Optional absorbers use the same damage path as UpdateWithMode.
func (c *MMCharacter) TickPoisonTurn(elapsedFrames, tps int, absorbDoT ...func(int) int) {
	ticks, expired := status.TickDoT(&c.PoisonFramesRemaining, &c.poisonTickTimer, elapsedFrames, tps)
	if expired {
		c.RemoveCondition(ConditionPoisoned)
	}
	c.dotDamage(c.dotTickDamage("poison", ticks), absorbDoT...)
}

// TickBurnTurn advances ignite by one TB round, mirroring TickPoisonTurn.
func (c *MMCharacter) TickBurnTurn(elapsedFrames, tps int, absorbDoT ...func(int) int) {
	ticks, expired := status.TickDoT(&c.BurnFramesRemaining, &c.burnTickTimer, elapsedFrames, tps)
	if expired {
		c.RemoveCondition(ConditionBurning)
	}
	c.dotDamage(c.dotTickDamage("burn", ticks), absorbDoT...)
}

// DoTTickTimers exposes the persisted sub-second cadence phase for poison and
// burn without making the runtime timers mutable outside this package.
func (c *MMCharacter) DoTTickTimers() (poison, burn int) {
	if c == nil {
		return 0, 0
	}
	return c.poisonTickTimer, c.burnTickTimer
}

// RestoreDoTTickTimers restores cadence only for active effects. Validation is
// shared with monsters through the status package.
func (c *MMCharacter) RestoreDoTTickTimers(poison, burn int) {
	if c == nil {
		return
	}
	tps := config.GetTargetTPS()
	c.poisonTickTimer = status.RestoreDoTTickTimer(c.PoisonFramesRemaining, poison, tps)
	c.burnTickTimer = status.RestoreDoTTickTimer(c.BurnFramesRemaining, burn, tps)
}

// dotDamage lands DoT damage on a still-standing character; Unconscious is
// set by the game loop's knockOut sweep, not here. A zero amount (no tick this
// call) is a no-op. Optional game-owned absorbers spend shared barriers without
// importing combat rules or retaining runtime callbacks in saved characters.
func (c *MMCharacter) dotDamage(amount int, absorbDoT ...func(int) int) {
	if c.HitPoints <= 0 || amount <= 0 {
		return
	}
	for _, absorb := range absorbDoT {
		amount = absorb(amount)
	}
	c.HitPoints -= amount
	if c.HitPoints < 0 {
		c.HitPoints = 0
	}
}

// regenPeriod is a fresh regeneration period, RT seconds beside TB rounds
// (config); SP waits the slower fight period while the party fights.
func regenPeriod(sp, inCombat bool) status.DualClock {
	seconds, rounds := config.RegenerationSeconds, config.RegenerationRounds
	if sp && inCombat {
		seconds, rounds = config.CombatSPRegenSeconds, config.CombatSPRegenRounds
	}
	return status.PairClock(float64(seconds), rounds, config.GetTargetTPS(), 1)
}

// regenLeft is period with spentFrames of RT and spentTurns of TB gone, each
// counted in its own mode as the elapsed time it is; a mode whose part is
// spent pays on its next tick.
func regenLeft(period status.DualClock, spentFrames, spentTurns int) status.DualClock {
	left := period
	left.Frames = max(1, period.Frames-max(0, spentFrames))
	left.Turns = max(1, min(period.Turns-max(0, spentTurns), (left.Frames+left.Rate-1)/left.Rate))
	return left
}

// TickRegen spends one tick of the running mode on both regeneration periods
// and pays each one that lapses. When the party starts or stops fighting the
// running SP period takes the other length, keeping the time already spent.
// Cards, carried supplies and draughts share the HP period, including before
// a supply is acquired. An unconscious hero's periods wait.
func (c *MMCharacter) TickRegen(turn, inCombat bool) {
	if c.HasCondition(ConditionUnconscious) {
		return
	}
	if c.SPRegenFight != inCombat {
		if c.SPRegen.Active() {
			from := regenPeriod(true, c.SPRegenFight)
			c.SPRegen = regenLeft(regenPeriod(true, inCombat), from.Frames-c.SPRegen.Frames, from.Turns-c.SPRegen.Turns)
		}
		c.SPRegenFight = inCombat
	}
	if c.SPRegen.EveryClock(func() status.DualClock { return regenPeriod(true, inCombat) }, turn) {
		c.RegenerateSpellPoints()
	}
	if c.HPRegen.EveryClock(func() status.DualClock { return regenPeriod(false, false) }, turn) {
		c.ApplyCardRegenTick()
	}
}

// RestoreLegacyRegen reads an old save's progress since the last payouts -
// RT frames per hero, TB rounds per party, each counted in its own mode - into
// the periods. SP progress past the calm period was made in a fight.
func (c *MMCharacter) RestoreLegacyRegen(spellFrames, hpFrames, spellRounds, hpRounds int) {
	calm := regenPeriod(true, false)
	c.SPRegenFight = spellFrames >= calm.Frames || spellRounds >= calm.Turns
	if spellFrames > 0 || spellRounds > 0 {
		c.SPRegen = regenLeft(regenPeriod(true, c.SPRegenFight), spellFrames, spellRounds)
	}
	if hpFrames > 0 || hpRounds > 0 {
		c.HPRegen = regenLeft(regenPeriod(false, false), hpFrames, hpRounds)
	}
}

// updateRegenAndPoison ticks poison and the RT regen periods (buffs flow in
// via BuffBonuses).
func (c *MMCharacter) updateRegenAndPoison(inCombat bool, absorbDoT ...func(int) int) {
	tps := config.GetTargetTPS()
	if tps <= 0 {
		tps = config.DefaultTPS
	}
	c.updatePoison(tps, absorbDoT...)
	c.updateBurn(tps, absorbDoT...)
	c.tickStunFrames()
	c.ArmorMelt.TickFrame()

	c.TickRegen(false, inCombat)
}

// CalculateManaRegenAmount returns SP regen per tick based on effective Personality.
func (c *MMCharacter) CalculateManaRegenAmount() int {
	effectivePersonality := c.GetEffectivePersonality()
	// Meditation speeds recovery: extra SP per regen tick per mastery tier.
	regen := 1 + (effectivePersonality / ManaRegenPersonalityDivisor) + c.SkillTier(SkillMeditation)*MeditationRegenPerTier
	if regen < 1 {
		return 1
	}
	return regen
}

// RegenerateSpellPoints adds Personality-derived SP to the character, capped
// at MaxSpellPoints. No-op for KO/unconscious characters and for those
// already at max SP. Used by both the real-time tick timer and the
// turn-based round counter so both paths share the same skip/cap rules.
func (c *MMCharacter) RegenerateSpellPoints() {
	if !c.CanAct() {
		return
	}
	if c.SpellPoints >= c.MaxSpellPoints {
		return
	}
	c.SpellPoints += c.CalculateManaRegenAmount() + c.MaxSpellPoints*c.BuffManaRegenPct/100
	if c.SpellPoints > c.MaxSpellPoints {
		c.SpellPoints = c.MaxSpellPoints
	}
}

// ApplyCardRegenTick heals card and timed-draught percentages of max HP, capped at
// max. Called on its own frame-timer cadence in RT (updateRegenAndPoison) and
// on the TB round counter (endPartyTurn), matching RegenerateSpellPoints' two
// cadences for the two modes.
func (c *MMCharacter) ApplyCardRegenTick() {
	if !c.CanAct() {
		return
	}
	percent := c.BonusRegenPct + c.BuffHPRegenPct + c.PersonalInventoryRegenPct()
	if percent <= 0 || c.HitPoints >= c.MaxHitPoints {
		return
	}
	c.HitPoints += c.MaxHitPoints * percent / 100
	if c.HitPoints > c.MaxHitPoints {
		c.HitPoints = c.MaxHitPoints
	}
}

// ApplyPoison applies or refreshes a poison effect for the given duration in frames.
func (c *MMCharacter) ApplyPoison(frames int) {
	if c.HasSkill(SkillFieldMedicine) && frames > 0 {
		frames = max(1, frames*(100-FieldMedicinePoisonReductionPct(c.SkillTier(SkillFieldMedicine)))/100)
	}
	if frames <= 0 {
		return
	}
	if status.Refresh(&c.PoisonFramesRemaining, frames) {
		c.AddCondition(ConditionPoisoned)
	}
}

// CurePoison ends an active poison outright - zeroing the frame timer, not just
// the Poisoned condition/icon. RemoveCondition alone leaves PoisonFramesRemaining
// ticking in the background (updatePoison keys off the timer, not the
// condition), so a cure item would look like it worked while 1 HP/sec kept
// draining until the poison's natural expiry.
func (c *MMCharacter) CurePoison() {
	status.Clear(&c.PoisonFramesRemaining, &c.poisonTickTimer)
	c.RemoveCondition(ConditionPoisoned)
}

// CureDamageOverTime ends poison and burning without removing other controls.
func (c *MMCharacter) CureDamageOverTime() {
	c.CurePoison()
	status.Clear(&c.BurnFramesRemaining, &c.burnTickTimer)
	c.RemoveCondition(ConditionBurning)
}

// clearAfflictionClocks stops every running affliction timer. Callers own the
// condition tags, so a tag and its clock always change together.
func (c *MMCharacter) clearAfflictionClocks() {
	status.Clear(&c.PoisonFramesRemaining, &c.poisonTickTimer)
	status.Clear(&c.BurnFramesRemaining, &c.burnTickTimer)
	c.Stun = status.DualClock{}
	c.ArmorMelt = status.DualClock{}
}

// clearRestState drops everything a full rest (or death's end) wipes: the
// affliction clocks and the stun diminishing-returns chain.
func (c *MMCharacter) clearRestState() {
	c.clearAfflictionClocks()
	c.StunDR().Reset()
}

// CureRestConditions clears living heroes' afflictions and their simulation
// timers together. Death and eradication require a separate revival action.
func (c *MMCharacter) CureRestConditions() {
	if c.HasCondition(ConditionDead) || c.HasCondition(ConditionEradicated) {
		return
	}
	c.clearRestState()
	c.Conditions = nil
}

// Eradicate ends the hero outright: 0 HP, Eradicated as the only state, and no
// poison, burn or stun left running to resurface after Resurrect.
func (c *MMCharacter) Eradicate() {
	c.ClearReactiveCombat()
	c.clearRestState()
	c.HitPoints = 0
	c.Conditions = []Condition{ConditionEradicated}
}

// ApplyBurn applies or refreshes ignite (fire DoT). It is INDEPENDENT of poison -
// both can run at once. The tick is desynced (starts half a second in) so burn
// and poison ticks don't land on the same frame. The banked half second is
// measured on the SAME clock the ticks run on (GetTargetTPS == the game config's
// TPS); a shorter ticking clock would make the bank worth a full second and buy
// the burn an extra tick.
func (c *MMCharacter) ApplyBurn(frames int) {
	if frames <= 0 {
		return
	}
	wasActive := c.BurnFramesRemaining > 0
	if status.Refresh(&c.BurnFramesRemaining, frames) {
		if !wasActive {
			c.burnTickTimer = config.GetTargetTPS() / 2 // desync the first tick from poison
		}
		c.AddCondition(ConditionBurning)
	}
}

// dotTickDamage is the hero's loss for ticks once-per-second ticks of a DoT
// status: the status table's percent of max HP per tick, at least its minimum.
func (c *MMCharacter) dotTickDamage(key string, ticks int) int {
	if ticks <= 0 {
		return 0
	}
	return ticks * config.DoTDamage(key, c.MaxHitPoints, false)
}

func (c *MMCharacter) updateBurn(tps int, absorbDoT ...func(int) int) {
	ticks, expired := status.TickDoT(&c.BurnFramesRemaining, &c.burnTickTimer, 1, tps)
	c.dotDamage(c.dotTickDamage("burn", ticks), absorbDoT...)
	if expired {
		c.RemoveCondition(ConditionBurning)
	}
}

// ApplyCharStun stuns the character for the given RT frames / TB turns (max with
// any existing stun). A stunned character takes no action until it wears off.
func (c *MMCharacter) ApplyCharStun(frames, turns int) {
	if c.Stun.Refresh(frames, turns) {
		c.AddCondition(ConditionStunned)
	}
}

// MeltArmor refreshes the melted-armor debuff; it never stacks.
func (c *MMCharacter) MeltArmor(frames, turns int) { c.ArmorMelt.Refresh(frames, turns) }

// ArmorMelted reports armor that blunts physical hits only as far as elemental ones.
func (c *MMCharacter) ArmorMelted() bool { return c != nil && c.ArmorMelt.Active() }

// IsStunned reports whether the character currently skips actions.
func (c *MMCharacter) IsStunned() bool {
	return c != nil && (c.Stun.Active())
}

func (c *MMCharacter) updatePoison(tps int, absorbDoT ...func(int) int) {
	ticks, expired := status.TickDoT(&c.PoisonFramesRemaining, &c.poisonTickTimer, 1, tps)
	c.dotDamage(c.dotTickDamage("poison", ticks), absorbDoT...)
	if expired {
		c.RemoveCondition(ConditionPoisoned)
	}
}

// String returns the display name of the class (Stringer interface).
func (c CharacterClass) String() string {
	switch c {
	case ClassKnight:
		return "Knight"
	case ClassPaladin:
		return "Paladin"
	case ClassArcher:
		return "Archer"
	case ClassCleric:
		return "Cleric"
	case ClassSorcerer:
		return "Sorcerer"
	case ClassDruid:
		return "Druid"
	case ClassThief:
		return "Thief"
	case ClassArmsMaster:
		return "Arms Master"
	case ClassMonk:
		return "Monk"
	case ClassBattleMage:
		return "Battle Mage"
	case ClassSniper:
		return "Sniper"
	case ClassAlchemist:
		return "Alchemist"
	case ClassWayfarer:
		return "Pilgrim"
	default:
		return "Unknown"
	}
}

// GetClassKey returns the lowercase class key used in config.
func (c *MMCharacter) GetClassKey() string {
	return c.Class.Key()
}

// ClassFromKey resolves a lowercase class key (knight/paladin/...) to its
// CharacterClass. Returns false for an unknown key.
func ClassFromKey(key string) (CharacterClass, bool) {
	switch key {
	case "knight":
		return ClassKnight, true
	case "paladin":
		return ClassPaladin, true
	case "archer":
		return ClassArcher, true
	case "cleric":
		return ClassCleric, true
	case "sorcerer":
		return ClassSorcerer, true
	case "druid":
		return ClassDruid, true
	case "thief":
		return ClassThief, true
	case "arms_master":
		return ClassArmsMaster, true
	case "monk":
		return ClassMonk, true
	case "battle_mage":
		return ClassBattleMage, true
	case "sniper":
		return ClassSniper, true
	case "alchemist":
		return ClassAlchemist, true
	case "wayfarer":
		return ClassWayfarer, true
	default:
		return 0, false
	}
}

// HasCondition checks if the character has a specific condition
func (c *MMCharacter) HasCondition(cond Condition) bool {
	for _, existing := range c.Conditions {
		if existing == cond {
			return true
		}
	}
	return false
}

// IsIncapacitated returns true if the character is unconscious or has 0 HP.
// Use this instead of checking HasCondition(ConditionUnconscious) || HitPoints <= 0.
func (c *MMCharacter) IsIncapacitated() bool {
	return c.HasCondition(ConditionUnconscious) || c.HitPoints <= 0
}

// AddCondition adds a condition if not already present
func (c *MMCharacter) AddCondition(cond Condition) {
	if !c.HasCondition(cond) {
		c.Conditions = append(c.Conditions, cond)
	}
}

// RemoveCondition removes a condition if present
func (c *MMCharacter) RemoveCondition(cond Condition) {
	for i, existing := range c.Conditions {
		if existing == cond {
			c.Conditions = append(c.Conditions[:i], c.Conditions[i+1:]...)
			return
		}
	}
}

// GetAvailableSchools returns the magic schools available to this character in
// the canonical order defined by AllMagicSchools.
func (c *MMCharacter) GetAvailableSchools() []MagicSchoolID {
	var available []MagicSchoolID
	for _, school := range AllMagicSchools {
		if _, exists := c.MagicSchools[school]; exists {
			available = append(available, school)
		}
	}
	return available
}

// spellSchoolForLearner resolves WHICH school a spell files into for this
// character: the first of its schools they already hold, else its primary.
//
// Dual-school spells (Town Portal is earth AND air) go into whichever of their
// schools the learner already holds, so learning one through Air never silently
// opens Earth. THE one place that choice is made: LearnSpell files by it and
// HasSchoolOpenFor gates on the school it names being open, so a shop can never
// refuse a spell the spellbook would happily accept.
func (c *MMCharacter) spellSchoolForLearner(def spells.SpellDefinition) MagicSchoolID {
	for _, s := range def.SchoolList() {
		if c.MagicSchools[MagicSchoolID(s)] != nil {
			return MagicSchoolID(s)
		}
	}
	return MagicSchoolID(def.School)
}

// SpellSchoolFor is the school this character casts the spell under. For a
// dual-school page that means WHERE IT IS FILED, not merely which of its schools
// happens to be open: a sorcerer who bought Town Portal through Air keeps
// casting it as Air even after a promotion opens Earth. An unlearned spell (a
// shop preview) answers with the school it WOULD be filed in.
func (c *MMCharacter) SpellSchoolFor(def spells.SpellDefinition) MagicSchoolID {
	schools := def.SchoolList()
	if len(schools) == 1 {
		return MagicSchoolID(schools[0]) // every spell but the dual-school ones
	}
	if school, filed := c.spellFiledUnder(def); filed {
		return school
	}
	return c.spellSchoolForLearner(def)
}

// spellFiledUnder is the school this character's spellbook actually holds the
// spell in, if any. THE one "is it learned, and where" walk: KnowsSpell asks it
// and SpellSchoolFor files a dual-school page by it.
func (c *MMCharacter) spellFiledUnder(def spells.SpellDefinition) (MagicSchoolID, bool) {
	for _, s := range def.SchoolList() {
		ms := c.MagicSchools[MagicSchoolID(s)]
		if ms == nil {
			continue
		}
		for _, known := range ms.KnownSpells {
			if known == def.ID {
				return MagicSchoolID(s), true
			}
		}
	}
	return MagicSchoolID(def.School), false
}

// SpellMasterySkill is the mastery that applies when this character casts the
// spell - the skill in SpellSchoolFor's school, or nil if they hold none of it.
// THE one lookup: damage, duration, resist pierce and the tooltip that explains
// all three read it, so the card can never name a mastery the fight ignores.
func (c *MMCharacter) SpellMasterySkill(def spells.SpellDefinition) *MagicSkill {
	return c.MagicSchools[c.SpellSchoolFor(def)]
}

// LearnSpell adds a spell to the school its DEFINITION declares (spells.yaml
// is the source of truth, not the caller), opening that school at Novice if
// needed. Reports whether the spellbook changed (false: unknown spell or
// already known) - the ONE learn path for level-ups, promotions and shops.
func (c *MMCharacter) LearnSpell(spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	if _, known := c.spellFiledUnder(def); known {
		return false // already filed under one of its schools
	}
	school := c.spellSchoolForLearner(def)
	if c.MagicSchools[school] == nil {
		c.MagicSchools[school] = &MagicSkill{
			Mastery:     MasteryNovice,
			KnownSpells: make([]spells.SpellID, 0),
		}
	}
	c.MagicSchools[school].KnownSpells = append(c.MagicSchools[school].KnownSpells, spellID)
	return true
}

// KnowsSpell reports whether the spell is filed under any of its schools.
func (c *MMCharacter) KnowsSpell(spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	_, known := c.spellFiledUnder(def)
	return known
}

// HasSchoolOpenFor reports whether ANY of the spell's schools is already open
// on this character - THE learn gate, shared by the lectern and by the shop
// counter (a shop never opens a school; level-ups and promotions do).
func (c *MMCharacter) HasSchoolOpenFor(spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	// The learner's own choice, asked as a question: the school it would file
	// into has to be one this character already holds.
	return c.MagicSchools[c.spellSchoolForLearner(def)] != nil
}

// GetSpellsForSchool returns the spell IDs this character knows in one school.
func (c *MMCharacter) GetSpellsForSchool(school MagicSchoolID) []spells.SpellID {
	if magicSkill, exists := c.MagicSchools[school]; exists {
		return magicSkill.KnownSpells
	}
	return []spells.SpellID{}
}

// CanEquipWeapon checks if this character class can wield a weapon.
func (c *MMCharacter) CanEquipWeapon(weapon items.Item) bool {
	return c.canWieldWeapon(weapon, true)
}

// canWieldWeapon is the weapon gate. statGates false skips stat thresholds
// (equip_personality_min), which are checked when a weapon is put on only.
func (c *MMCharacter) canWieldWeapon(weapon items.Item, statGates bool) bool {
	_, weaponKey, ok := config.WeaponDefOf(weapon)
	if !ok {
		return false // Unknown weapon cannot be equipped
	}
	weaponDef, exists := getWeaponDefinitionFromGlobal(weaponKey)
	if !exists {
		return false
	}

	// Personality-gated weapons (Lanista's Scepter): force of presence replaces
	// weapon training entirely.
	if weaponDef.EquipPersonalityMin > 0 && (!statGates || c.GetEffectivePersonality() >= weaponDef.EquipPersonalityMin) {
		return true
	}
	// Firearms need no training to point and shoot: anyone with real weapon
	// training can fire a blaster untrained (the Blaster skill only makes it
	// better - mastery true damage and the Grandmaster crit). See
	// weaponCategorySkillOptional.
	if WeaponCategorySkillOptional(weaponDef.Category) && c.HasAnyWeaponSkill() {
		return true
	}
	requiredSkill, ok := WeaponSkillForCategory(weaponDef.Category)
	if !ok {
		return false
	}
	_, hasSkill := c.Skills[requiredSkill]
	return hasSkill
}

func (c *MMCharacter) CanEquipArmor(item items.Item) bool {
	if !c.itemClassAllowed(item) {
		return false
	}
	category := strings.ToLower(item.ArmorCategory)
	if category == "" {
		return false
	}
	if category == "cloth" {
		return true // cloth carries no skill requirement - wearable by anyone (a Monk's robes)
	}
	requiredSkill, ok := ArmorSkillForCategory(category)
	if !ok {
		return false
	}
	_, hasSkill := c.Skills[requiredSkill]
	return hasSkill
}

// getWeaponDefinitionFromGlobal accesses weapon definition without circular imports
func getWeaponDefinitionFromGlobal(weaponKey string) (*items.WeaponDefinitionFromYAML, bool) {
	if items.GlobalWeaponAccessor == nil {
		return nil, false
	}
	return items.GlobalWeaponAccessor(weaponKey)
}

// EquipDestination resolves the default destination used by inventory equip and
// comparison. Eligibility remains in ItemFitsSlot; an untrained hero can still
// inspect the hypothetical result before learning the required skill.
func (c *MMCharacter) EquipDestination(item items.Item) (items.EquipSlot, bool) {
	var slot items.EquipSlot
	switch item.Type {
	case items.ItemWeapon:
		slot = items.SlotMainHand
		// Dual Wielding: a second weapon overflows to the off-hand, mirroring
		// how a second ring overflows to Ring2 below.
		if _, mainTaken := c.Equipment[items.SlotMainHand]; mainTaken && c.CanDualWield() {
			if _, offTaken := c.Equipment[items.SlotOffHand]; !offTaken {
				slot = items.SlotOffHand
			}
		}
	case items.ItemBattleSpell, items.ItemUtilitySpell, items.ItemTechnique, items.ItemThrowable:
		slot = items.SlotQuickAction
	case items.ItemArmor:
		slot = item.PreferredSlot(items.SlotArmor)
	case items.ItemAccessory:
		slot = item.PreferredSlot(items.SlotRing1)
	default:
		return 0, false
	}

	// Rings share two interchangeable slots, but equip_slot resolves every ring
	// to SlotRing1. If that finger is taken, fall to the free SlotRing2 so a
	// second ring can be worn - only overwrite SlotRing1 when both are full.
	if slot == items.SlotRing1 {
		if _, ring1Taken := c.Equipment[items.SlotRing1]; ring1Taken {
			if _, ring2Taken := c.Equipment[items.SlotRing2]; !ring2Taken {
				slot = items.SlotRing2
			}
		}
	}

	return slot, true
}

// EquipItem attempts to equip an item from inventory.
func (c *MMCharacter) EquipItem(item items.Item) (items.Item, bool, bool) {
	slot, ok := c.EquipDestination(item)
	if !ok {
		return items.Item{}, false, false
	}
	return c.EquipItemToSlot(item, slot)
}

func (c *MMCharacter) itemClassAllowed(item items.Item) bool {
	if def, _, ok := config.ItemDefOf(item); ok && len(def.AllowedClasses) > 0 {
		allowed := false
		for _, key := range def.AllowedClasses {
			if key == c.Class.Key() {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

// ItemFitsSlot reports whether item can legally occupy slot for this character:
// the type->slot mapping plus the class/armor gates. Single source of truth for
// the model equip paths and the UI drag highlight / drop validation.
func (c *MMCharacter) ItemFitsSlot(item items.Item, slot items.EquipSlot) bool {
	return c.itemFitsSlot(item, slot, true)
}

// ItemBelongsInSlot is ItemFitsSlot without stat thresholds: worn gear a buff
// once allowed stays worn, as in play. A loaded save revalidates with it.
func (c *MMCharacter) ItemBelongsInSlot(item items.Item, slot items.EquipSlot) bool {
	return c.itemFitsSlot(item, slot, false)
}

func (c *MMCharacter) itemFitsSlot(item items.Item, slot items.EquipSlot, statGates bool) bool {
	if !c.itemClassAllowed(item) {
		return false
	}

	switch item.Type {
	case items.ItemWeapon:
		if !c.canWieldWeapon(item, statGates) {
			return false
		}
		if slot == items.SlotMainHand {
			return true
		}
		// A second weapon only fits the off-hand with Dual Wielding.
		return slot == items.SlotOffHand && c.CanDualWield()
	case items.ItemTechnique:
		d := config.Technique(item.Technique)
		return slot == items.SlotQuickAction && c.Class == ClassWayfarer && d != nil && c.Level >= d.Level
	case items.ItemThrowable:
		return slot == items.SlotQuickAction && c.CanThrowFlask(item.Flask)
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		return slot == items.SlotQuickAction
	case items.ItemArmor:
		return slot == item.PreferredSlot(items.SlotArmor) && c.CanEquipArmor(item)
	case items.ItemAccessory:
		ps := item.PreferredSlot(items.SlotRing1)
		if ps == items.SlotRing1 {
			return slot == items.SlotRing1 || slot == items.SlotRing2 // rings fit either finger
		}
		return slot == ps
	}
	return false
}

// EquipItemToSlot forces item into a specific equipment slot, returning any
// displaced item. Used when the UI drop target is an exact slot (e.g. dragging a
// ring onto the Ring2 finger) - unlike EquipItem, which resolves the slot itself
// and would send a ring to Ring1. Refuses a slot the item can't legally occupy.
func (c *MMCharacter) EquipItemToSlot(item items.Item, slot items.EquipSlot) (items.Item, bool, bool) {
	if !c.ItemFitsSlot(item, slot) {
		return items.Item{}, false, false
	}
	previousItem, hadPreviousItem := c.Equipment[slot]
	c.Equipment[slot] = item
	c.updateDerivedStatsForEquipment()
	return previousItem, hadPreviousItem, true
}

// MoveEquipmentSlot moves the item in srcSlot to dstSlot, swapping with whatever
// already occupies dstSlot. In practice this serves interchangeable slots
// (dragging a ring between the two fingers); both the moved item and any
// displaced occupant must fit their new slots. Returns false if srcSlot is
// empty, the slots are the same, or either item doesn't fit.
func (c *MMCharacter) MoveEquipmentSlot(srcSlot, dstSlot items.EquipSlot) bool {
	if srcSlot == dstSlot {
		return false
	}
	moving, ok := c.Equipment[srcSlot]
	if !ok || !c.ItemFitsSlot(moving, dstSlot) {
		return false
	}
	if occ, occupied := c.Equipment[dstSlot]; occupied {
		if !c.ItemFitsSlot(occ, srcSlot) {
			return false
		}
		c.Equipment[srcSlot] = occ // swap the two fingers
	} else {
		delete(c.Equipment, srcSlot)
	}
	c.Equipment[dstSlot] = moving
	c.updateDerivedStatsForEquipment()
	return true
}

// HasAnyWeaponSkill reports whether c has a REPLACEABLE weapon-category skill -
// deliberately EXCLUDING Martial Arts, which only ever gates the Monk's fists
// (the same weapon already sitting in the slot). Guards UnequipItem: a
// character whose only weapon skill is Martial Arts could never equip anything
// else, so unequipping SlotMainHand would leave them permanently weaponless -
// everyone else can always re-equip some other weapon they know.
//
// Membership goes through Category() (the single source of what's a weapon
// skill) rather than a numeric SkillType range - a range silently drops any
// weapon skill appended past its end, which is exactly where new skills MUST
// go for save compatibility (Martial Arts already had to).
func (c *MMCharacter) HasAnyWeaponSkill() bool {
	for skill := range c.Skills {
		// Martial Arts IS a weapon skill but is excluded here on purpose: it only
		// ever gates the Monk's fists (already in the slot), so it can't rescue an
		// unequipped main hand. Explicit exclusion, not a range accident.
		if skill.IsWeaponSkill() && skill != SkillMartialArts {
			return true
		}
	}
	return false
}

// UnequipItem removes an item from an equipment slot and returns it
func (c *MMCharacter) UnequipItem(slot items.EquipSlot) (items.Item, bool) {
	if slot == items.SlotMainHand && !c.HasAnyWeaponSkill() {
		return items.Item{}, false
	}
	if item, exists := c.Equipment[slot]; exists {
		delete(c.Equipment, slot)

		// Recalculate derived stats after unequipping
		c.updateDerivedStatsForEquipment()

		return item, true
	}
	return items.Item{}, false
}

// baseStatFields are the hero's base stat fields in stats.Names order: the
// one place the character's fields meet the canonical list.
func (c *MMCharacter) baseStatFields() []*int {
	return []*int{&c.Might, &c.Intellect, &c.Personality, &c.Endurance, &c.Accuracy, &c.Speed, &c.Luck}
}

// BaseStats is the hero's unmodified stats as one value.
func (c *MMCharacter) BaseStats() StatBonuses {
	var b StatBonuses
	for i, p := range c.baseStatFields() {
		*b.At(i) = *p
	}
	return b
}

// SetBaseStats overwrites the hero's base stats.
func (c *MMCharacter) SetBaseStats(b StatBonuses) {
	for i, p := range c.baseStatFields() {
		*p = *b.At(i)
	}
}

// BaseStatField points at a base stat by canonical name, nil when unknown.
func (c *MMCharacter) BaseStatField(name string) *int {
	for i, n := range stats.Names {
		if n == name {
			return c.baseStatFields()[i]
		}
	}
	return nil
}

// EffectiveStatBlock is the one effective-stat sum: base, buffs, permanent
// gains and equipment.
func (c *MMCharacter) EffectiveStatBlock() StatBonuses {
	return c.BaseStats().Add(c.BuffBonuses).Add(c.PermanentBonuses).Add(c.calculateEquipmentBonuses())
}

// GetEffectiveStats returns character stats with any active bonuses applied (spells + equipment)
func (c *MMCharacter) GetEffectiveStats() (might, intellect, personality, endurance, accuracy, speed, luck int) {
	b := c.EffectiveStatBlock()
	return b.Might, b.Intellect, b.Personality, b.Endurance, b.Accuracy, b.Speed, b.Luck
}

func (c *MMCharacter) GetEffectiveMight() int       { return c.EffectiveStatBlock().Might }
func (c *MMCharacter) GetEffectiveIntellect() int   { return c.EffectiveStatBlock().Intellect }
func (c *MMCharacter) GetEffectivePersonality() int { return c.EffectiveStatBlock().Personality }
func (c *MMCharacter) GetEffectiveEndurance() int   { return c.EffectiveStatBlock().Endurance }
func (c *MMCharacter) GetEffectiveAccuracy() int    { return c.EffectiveStatBlock().Accuracy }
func (c *MMCharacter) GetEffectiveSpeed() int       { return c.EffectiveStatBlock().Speed }
func (c *MMCharacter) GetEffectiveLuck() int        { return c.EffectiveStatBlock().Luck }

// ItemAttributeScalingBonuses uses base attributes, excluding gear and buffs.
func (c *MMCharacter) ItemAttributeScalingBonuses(item items.Item) (intellect, personality int) {
	if div := item.Attributes["intellect_scaling_divisor"]; div > 0 {
		intellect = c.Intellect / div
	}
	if div := item.Attributes["personality_scaling_divisor"]; div > 0 {
		personality = c.Personality / div
	}
	return
}

// calculateEquipmentBonuses returns stat bonuses from all equipped items
// (YAML-driven). Only a positive flat bonus counts.
func (c *MMCharacter) calculateEquipmentBonuses() StatBonuses {
	var b StatBonuses
	for _, it := range c.Equipment {
		intBonus, perBonus := c.ItemAttributeScalingBonuses(it)
		b.Intellect += intBonus
		b.Personality += perBonus
		// endurance_scaling_divisor is deliberately NOT a stat bonus: it is the
		// armor piece's AC formula input (AC = base + effective End / divisor,
		// see CalculateArmorClassContribution). Feeding it back into Endurance
		// made every armor piece inflate every other piece's AC.
		for i, key := range items.StatBonusAttributes {
			if v := it.Attributes[key]; v > 0 {
				*b.At(i) += v
			}
		}
	}
	// Equipment-set bonuses: this loop is the only place that sees the whole
	// equipped kit together, so completed sets add their bonuses here.
	c.forEachCompletedSet(func(set *config.ItemSetConfig) {
		b = b.Add(set.StatBonus.Stats())
	})
	return b
}

// forEachCompletedSet visits every equipment set completed by the equipped
// items. Count-based sets use pieces_required; exact-piece sets use their
// required_pieces list, so duplicate weapons cannot replace their armor. Zero
// allocations on purpose: this runs inside
// calculateEquipmentBonuses, i.e. inside EVERY effective-stat read. Each set
// is visited once - counted at its lowest-slot piece (the "owner"); the inner
// rescan is bounded by the handful of equipment slots.
func (c *MMCharacter) forEachCompletedSet(fn func(*config.ItemSetConfig)) {
	for slot, it := range c.Equipment {
		if it.Set == "" {
			continue
		}
		owner := true
		count := 0
		for slot2, it2 := range c.Equipment {
			if it2.Set != it.Set {
				continue
			}
			if slot2 < slot {
				owner = false
				break
			}
			count++
		}
		if !owner {
			continue
		}
		if set := config.GetItemSet(it.Set); set != nil && c.hasCompletedSet(it.Set, set, count) {
			fn(set)
		}
	}
}

// SetArmorClassBonus uses the same complete-set rule as attribute bonuses.
func (c *MMCharacter) SetArmorClassBonus() (bonus int) {
	c.forEachCompletedSet(func(set *config.ItemSetConfig) { bonus += set.BonusArmorClass })
	return
}

// HasCompletedEquipmentSet exposes the same completion rule used by combat bonuses.
func (c *MMCharacter) HasCompletedEquipmentSet(key string) bool {
	set := config.GetItemSet(key)
	if set == nil {
		return false
	}
	count := 0
	for _, item := range c.Equipment {
		if item.Set == key {
			count++
		}
	}
	return c.hasCompletedSet(key, set, count)
}

func (c *MMCharacter) hasCompletedSet(setKey string, set *config.ItemSetConfig, count int) bool {
	return c.equipmentSetPieceCount(setKey, set, count) >= set.RequiredPieceCount()
}

// EquipmentSetProgress counts exact required pieces only once, matching combat.
func (c *MMCharacter) EquipmentSetProgress(key string) (int, int) {
	set := config.GetItemSet(key)
	if set == nil {
		return 0, 0
	}
	count := 0
	for _, item := range c.Equipment {
		if item.Set == key {
			count++
		}
	}
	return c.equipmentSetPieceCount(key, set, count), set.RequiredPieceCount()
}

func (c *MMCharacter) equipmentSetPieceCount(key string, set *config.ItemSetConfig, count int) int {
	if len(set.RequiredPieces) == 0 {
		return count
	}
	count = 0
	for _, required := range set.RequiredPieces {
		for _, equipped := range c.Equipment {
			if equipped.Set == key && config.CatalogKey(equipped) == required {
				count++
				break
			}
		}
	}
	return count
}

// SetStunDurationPct sums stun-duration shifts from completed armor sets.
// The combat layer combines this with per-item shifts before applying the
// single global duration floor.
func (c *MMCharacter) SetStunDurationPct() int {
	total := 0
	c.forEachCompletedSet(func(set *config.ItemSetConfig) {
		total += set.StunDurationPct
	})
	return total
}

// SetCritChanceBonus returns the flat critical-chance percentage points granted
// by completed equipment sets. It applies to every player critical-hit roll,
// including weapon attacks and damage spells.
func (c *MMCharacter) SetCritChanceBonus() int {
	total := 0
	c.forEachCompletedSet(func(set *config.ItemSetConfig) {
		total += set.BonusCritChance
	})
	return total
}

// SetFieryRipostePct sums fiery-riposte shares from completed sets (the
// Drakeforged pair): melee attackers take this % of dealt damage back as fire.
func (c *MMCharacter) SetFieryRipostePct() int {
	total := 0
	c.forEachCompletedSet(func(set *config.ItemSetConfig) {
		total += set.FieryRipostePct
	})
	return total
}

// forEachEquippedItemDefinition visits typed items.yaml definitions for the
// current equipment. New item mechanics use this path instead of duplicating
// definition lookups in each combat subsystem.
func (c *MMCharacter) forEachEquippedItemDefinition(fn func(*config.ItemDefinitionConfig)) {
	if c == nil {
		return
	}
	for _, it := range c.Equipment {
		if def, _, ok := config.ItemDefOf(it); ok && def != nil {
			fn(def)
		}
	}
}

// ProjectileReflectPct returns the strongest equipped projectile reflection
// chance. Multiple mirror shields do not stack into guaranteed reflection.
func (c *MMCharacter) ProjectileReflectPct() int {
	best := 0
	c.forEachEquippedItemDefinition(func(def *config.ItemDefinitionConfig) {
		if def.ProjectileReflectPct > best {
			best = def.ProjectileReflectPct
		}
	})
	return best
}

// ItemStatusDurationPct sums per-item shifts applied to every hostile status.
func (c *MMCharacter) ItemStatusDurationPct() int {
	total := 0
	c.forEachEquippedItemDefinition(func(def *config.ItemDefinitionConfig) {
		total += def.StatusDurationPct
	})
	return total
}

// ScaleStackParams returns the strongest scale growth contract on the equipped
// items. Per-stack value and cap always come from the same YAML definition.
func (c *MMCharacter) ScaleStackParams() (perStack, maxStacks int) {
	c.forEachEquippedItemDefinition(func(def *config.ItemDefinitionConfig) {
		if def.ScaleStackAC > perStack ||
			(def.ScaleStackAC == perStack && def.ScaleStackMax > maxStacks) {
			perStack = def.ScaleStackAC
			maxStacks = def.ScaleStackMax
		}
	})
	return perStack, maxStacks
}

// GearResistPct sums the character's % resistance to a damage school from equipped gear.
func (c *MMCharacter) GearResistPct(school string) int {
	damageType, err := damagecalc.ParseType(school)
	if err != nil {
		return 0
	}
	key := "resist_" + damageType.String()
	total := 0
	for _, it := range c.Equipment {
		total += it.Attributes[key]
	}
	return total
}

// updateDerivedStatsForEquipment re-derives MaxHP/MaxSP after an equip change,
// preserving current HP/SP (gain is added, loss only caps - no free healing).
// Same formula as every other recalc path: effective stats drive both maxima,
// so endurance/intellect gear finally shows up in HP/SP.
func (c *MMCharacter) updateDerivedStatsForEquipment() {
	c.RecalculateMaxStatsKeepingCurrent(nil) // nil -> config.GlobalConfig / defaults
}
