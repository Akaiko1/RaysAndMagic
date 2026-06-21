package character

import (
	"fmt"
	"strings"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Mana regeneration tunables. SP regenerates by (1 + Personality/divisor +
// Meditation tier) every ManaRegenIntervalFrames ticks. Kept in this package
// because game's balance.go can't be imported from internal/character (circular).
const (
	ManaRegenIntervalFrames     = 600 // ~5s at 120 TPS
	ManaRegenPersonalityDivisor = 10
	MaxSPPersonalityDivisor     = 3
	// MeditationRegenPerTier: extra SP restored per regen tick per Meditation
	// mastery tier (Novice=0 → bonus from Expert up), making mana recovery faster.
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

type MMCharacter struct {
	Name      string
	Class     CharacterClass
	Promotion Promotion // elite status (Archmage/Lich); PromotionNone by default

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

	// Status effects
	Conditions []Condition
	// Poison status timer and tick accumulator (frames)
	PoisonFramesRemaining int
	poisonTickTimer       int

	// Regeneration timer - counts frames until next spell point regeneration
	spellRegenTimer int

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

	// RTCooldown is this character's remaining real-time action cooldown in
	// frames. While > 0 the member is "busy" (grayed in the HUD, skipped by
	// auto-select); set on each attack/cast from the weapon/spell cooldown and
	// ticked down once per frame. Unused in turn-based mode (which uses
	// ActionsRemaining instead).
	RTCooldown int
}

// Turn-based party bonus-action thresholds on effective Speed. Single source
// shared by MMGame.startPartyTurn (mechanic) and the Speed stat tooltip.
const (
	SpeedBonusAction1Threshold = 25 // Any living Speed > this → +1 party bonus action
	SpeedBonusAction2Threshold = 50 // Any living Speed > this → +2 party bonus actions
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

// CanAct reports whether this character can take actions this turn: alive
// (HP > 0) AND not unconscious. Dead/KO characters are skipped by the
// turn-based scheduler entirely (no gray frame, can't be selected).
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

type CharacterClass int

const (
	ClassKnight CharacterClass = iota
	ClassPaladin
	ClassArcher
	ClassCleric
	ClassSorcerer
	ClassDruid
	ClassThief
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

func (c *MMCharacter) IsArchmage() bool { return c.Promotion == PromotionArchmage }
func (c *MMCharacter) IsLich() bool     { return c.Promotion == PromotionLich }

// ClassDisplayName returns the promoted title if any, else the base class name.
func (c *MMCharacter) ClassDisplayName() string {
	switch c.Promotion {
	case PromotionArchmage:
		return "Archmage"
	case PromotionLich:
		return "Lich"
	default:
		return c.Class.String()
	}
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
// weapon and quick-slot spell. Unknown keys panic at character creation —
// content bugs must surface immediately, not as a silently gimped hero.
func (c *MMCharacter) applyClassKit(cfg *config.Config) {
	key := c.Class.Key()
	stats, ok := cfg.Characters.Classes[key]
	if !ok {
		panic(fmt.Sprintf("class %q missing from config.yaml characters.classes", key))
	}

	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	for _, sk := range stats.Skills {
		st, ok := SkillTypeFromKey(sk)
		if !ok {
			panic(fmt.Sprintf("class %q: unknown skill key %q in config.yaml", key, sk))
		}
		c.Skills[st] = &Skill{Mastery: MasteryNovice}
	}

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
	if stats.Armor != "" {
		c.Equipment[items.SlotArmor] = items.CreateItemFromYAML(stats.Armor)
	}
	if stats.QuickTrap != "" {
		// The starting trap occupies the SAME quick slot as quick spells.
		// Fail fast on a bad key, but only when traps.yaml is loaded —
		// config-less unit tests build parties without the trap catalog.
		if it, ok := config.TrapItem(stats.QuickTrap); ok {
			c.Equipment[items.SlotSpell] = it
		} else if config.GlobalTrapConfig != nil {
			panic(fmt.Sprintf("class %q: unknown quick_trap %q in config.yaml", key, stats.QuickTrap))
		}
	}
	if stats.QuickSpell != "" {
		if spellItem, err := spells.CreateSpellItem(spells.SpellID(stats.QuickSpell)); err == nil {
			c.Equipment[items.SlotSpell] = spellItem
		}
	}
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
	c.Might += mods.Might
	c.Intellect += mods.Intellect
	c.Personality += mods.Personality
	c.Endurance += mods.Endurance
	c.Accuracy += mods.Accuracy
	c.Speed += mods.Speed
	c.Luck += mods.Luck
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
// (base + equipment + buffs) — the single formula every recalc path shares:
//
//	MaxHP = effEndurance×endurMult + Level×levelHPMult (+ Bodybuilding)
//	MaxSP = effIntellect + effPersonality/MaxSPPersonalityDivisor + Level×levelSPMult
func (c *MMCharacter) recomputeMaxFromEffective(cfg *config.Config) {
	endurMult, levelHPMult, levelSPMult := derivedStatMultipliers(cfg)
	_, effInt, effPers, effEnd, _, _, _ := c.GetEffectiveStats()
	baseMaxHP := effEnd*endurMult + c.Level*levelHPMult
	c.MaxHitPoints = baseMaxHP + c.bodybuildingBonusHP(baseMaxHP)
	c.MaxSpellPoints = effInt + effPers/MaxSPPersonalityDivisor + c.Level*levelSPMult
}

// CalculateDerivedStats recomputes MaxHP/MaxSP and FULLY RESTORES current
// HP/SP — character creation and level-up only. Everything else (equip, stat
// spend, buff change) goes through RecalculateMaxStatsKeepingCurrent.
func (c *MMCharacter) CalculateDerivedStats(cfg *config.Config) {
	c.recomputeMaxFromEffective(cfg)
	c.HitPoints = c.MaxHitPoints
	c.SpellPoints = c.MaxSpellPoints
}

// RecalculateMaxStatsKeepingCurrent recomputes MaxHP/MaxSP for REVERSIBLE
// stat changes (equip/unequip, buff apply/expire): the maxima move, the
// CURRENT values never grow — only get capped. Granting current on a gain
// here would be a pump: equip +End (+HP granted) → unequip (cap can't take it
// back) → repeat until full. Irreversible gains (spending a stat point,
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
// immediately gives its HP). Never use for equip/buff changes — those can be
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

func (c *MMCharacter) Update() {
	c.updateRegenAndPoison()
}

// UpdateWithMode updates the character with knowledge of the current game mode
func (c *MMCharacter) UpdateWithMode(turnBasedMode bool) {
	// Skip timer-based regeneration in turn-based mode
	if turnBasedMode {
		tps := config.GetTargetTPS()
		if tps <= 0 {
			tps = 60
		}
		c.updatePoison(tps)
		return
	}

	// Use normal timer-based regeneration in real-time mode
	c.updateRegenAndPoison()
}

// updateRegenAndPoison ticks poison and the SP-regen cadence (buffs flow in
// via BuffBonuses).
func (c *MMCharacter) updateRegenAndPoison() {
	tps := config.GetTargetTPS()
	if tps <= 0 {
		tps = 60
	}
	c.updatePoison(tps)

	// If unconscious, skip regeneration and updates
	if c.HasCondition(ConditionUnconscious) {
		return
	}
	// Regenerate spell points on a fixed cadence.
	c.spellRegenTimer++
	if c.spellRegenTimer >= ManaRegenIntervalFrames {
		c.RegenerateSpellPoints()
		c.spellRegenTimer = 0 // Reset timer
	}
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
	c.SpellPoints += c.CalculateManaRegenAmount()
	if c.SpellPoints > c.MaxSpellPoints {
		c.SpellPoints = c.MaxSpellPoints
	}
}

// ApplyPoison applies or refreshes a poison effect for the given duration in frames.
func (c *MMCharacter) ApplyPoison(frames int) {
	if frames <= 0 {
		return
	}
	if frames > c.PoisonFramesRemaining {
		c.PoisonFramesRemaining = frames
	}
	c.AddCondition(ConditionPoisoned)
}

func (c *MMCharacter) updatePoison(tps int) {
	if c.PoisonFramesRemaining <= 0 {
		return
	}
	if tps <= 0 {
		tps = 60
	}
	c.PoisonFramesRemaining--
	c.poisonTickTimer++

	if c.poisonTickTimer >= tps {
		c.poisonTickTimer = 0
		if c.HitPoints > 0 {
			c.HitPoints--
			if c.HitPoints <= 0 {
				c.HitPoints = 0
				c.AddCondition(ConditionUnconscious)
			}
		}
	}

	if c.PoisonFramesRemaining <= 0 {
		c.PoisonFramesRemaining = 0
		c.poisonTickTimer = 0
		c.RemoveCondition(ConditionPoisoned)
	}
}

func (c *MMCharacter) GetDisplayInfo() string {
	className := c.ClassDisplayName()
	condition := "OK"
	if len(c.Conditions) > 0 {
		condNames := make([]string, 0, len(c.Conditions))
		for _, cond := range c.Conditions {
			condNames = append(condNames, cond.String())
		}
		condition = strings.Join(condNames, ", ")
	}

	// Add equipment info
	weaponInfo := "No weapon"
	if weapon, hasWeapon := c.Equipment[items.SlotMainHand]; hasWeapon {
		weaponInfo = weapon.Name
	}

	spellInfo := "No spell"
	// Check unified spell slot
	if spell, hasSpell := c.Equipment[items.SlotSpell]; hasSpell {
		spellInfo = spell.Name
	}

	return fmt.Sprintf("%s\n%s Lv.%d\nHP: %d/%d\nSP: %d/%d\n%s\nW:%s\nS:%s",
		c.Name, className, c.Level,
		c.HitPoints, c.MaxHitPoints,
		c.SpellPoints, c.MaxSpellPoints,
		condition, weaponInfo, spellInfo)
}

func (c *MMCharacter) GetDetailedInfo() string {
	info := fmt.Sprintf("=== %s ===\n", c.Name)
	info += fmt.Sprintf("Class: %s  Level: %d\n", c.ClassDisplayName(), c.Level)
	info += fmt.Sprintf("Experience: %d\n\n", c.Experience)

	info += "ATTRIBUTES:\n"
	info += fmt.Sprintf("Might: %d  Intellect: %d\n", c.Might, c.Intellect)
	info += fmt.Sprintf("Personality: %d  Endurance: %d\n", c.Personality, c.Endurance)
	info += fmt.Sprintf("Accuracy: %d  Speed: %d  Luck: %d\n\n", c.Accuracy, c.Speed, c.Luck)

	info += "SKILLS:\n"
	for skillType, skill := range c.Skills {
		info += fmt.Sprintf("%s: %d (%s)\n", skillType, skill.Level(), skill.Mastery)
	}

	info += "\nMAGIC SCHOOLS:\n"
	for school, magicSkill := range c.MagicSchools {
		info += fmt.Sprintf("%s: %d (%s) - %d spells\n",
			school.DisplayName(), magicSkill.Level(),
			magicSkill.Mastery, len(magicSkill.KnownSpells))
	}

	return info
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

// GetSpellsForSchool returns the spell IDs for a specific magic school
// LearnSpell adds a spell to the school its DEFINITION declares (spells.yaml
// is the source of truth, not the caller), opening that school at Novice if
// needed. Reports whether the spellbook changed (false: unknown spell or
// already known) — the ONE learn path for level-ups, promotions and shops.
func (c *MMCharacter) LearnSpell(spellID spells.SpellID) bool {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	school := MagicSchoolID(def.School)
	if c.MagicSchools[school] == nil {
		c.MagicSchools[school] = &MagicSkill{
			Mastery:     MasteryNovice,
			KnownSpells: make([]spells.SpellID, 0),
		}
	}
	for _, existing := range c.MagicSchools[school].KnownSpells {
		if existing == spellID {
			return false
		}
	}
	c.MagicSchools[school].KnownSpells = append(c.MagicSchools[school].KnownSpells, spellID)
	return true
}

func (c *MMCharacter) GetSpellsForSchool(school MagicSchoolID) []spells.SpellID {
	if magicSkill, exists := c.MagicSchools[school]; exists {
		return magicSkill.KnownSpells
	}
	return []spells.SpellID{}
}

// CanEquipWeapon checks if this character class can equip a specific weapon by name
func (c *MMCharacter) CanEquipWeaponByName(weaponName string) bool {
	// Get weapon key and definition from YAML
	weaponKey := items.GetWeaponKeyByName(weaponName)
	weaponDef, exists := getWeaponDefinitionFromGlobal(weaponKey)
	if !exists {
		return false // Unknown weapon cannot be equipped
	}

	if weaponDef.Category == "blaster" {
		return true // universally usable
	}
	requiredSkill, ok := WeaponSkillForCategory(weaponDef.Category)
	if !ok {
		return false
	}
	_, hasSkill := c.Skills[requiredSkill]
	return hasSkill
}

func (c *MMCharacter) CanEquipArmor(item items.Item) bool {
	category := strings.ToLower(item.ArmorCategory)
	if category == "" {
		return false
	}
	if category == "cloth" {
		return true // universally wearable
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

// EquipItem attempts to equip an item from inventory, returns (previousItem, hadPreviousItem, success)
func (c *MMCharacter) EquipItem(item items.Item) (items.Item, bool, bool) {
	var previousItem items.Item
	var hadPreviousItem bool
	var slot items.EquipSlot

	switch item.Type {
	case items.ItemWeapon:
		// Check if character can equip this weapon by name
		if !c.CanEquipWeaponByName(item.Name) {
			return items.Item{}, false, false
		}
		slot = items.SlotMainHand
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		slot = items.SlotSpell
	case items.ItemArmor:
		if !c.CanEquipArmor(item) {
			return items.Item{}, false, false
		}
		// Use equip_slot attribute if defined, otherwise default to armor slot
		if equipSlotCode, hasSlot := item.Attributes["equip_slot"]; hasSlot {
			slot = items.EquipSlot(equipSlotCode)
		} else {
			slot = items.SlotArmor
		}
	case items.ItemAccessory:
		// Use equip_slot attribute if defined, otherwise default to ring slot 1
		if equipSlotCode, hasSlot := item.Attributes["equip_slot"]; hasSlot {
			slot = items.EquipSlot(equipSlotCode)
		} else {
			slot = items.SlotRing1
		}
	default:
		return items.Item{}, false, false
	}

	// Rings share two interchangeable slots, but equip_slot resolves every ring
	// to SlotRing1. If that finger is taken, fall to the free SlotRing2 so a
	// second ring can be worn — only overwrite SlotRing1 when both are full.
	if slot == items.SlotRing1 {
		if _, ring1Taken := c.Equipment[items.SlotRing1]; ring1Taken {
			if _, ring2Taken := c.Equipment[items.SlotRing2]; !ring2Taken {
				slot = items.SlotRing2
			}
		}
	}

	// Check if there's already an item equipped in this slot
	if existingItem, exists := c.Equipment[slot]; exists {
		previousItem = existingItem
		hadPreviousItem = true
	}

	// Equip the new item
	c.Equipment[slot] = item

	// Recalculate derived stats to apply equipment bonuses (preserves current HP/SP ratios)
	c.updateDerivedStatsForEquipment()

	return previousItem, hadPreviousItem, true
}

// UnequipItem removes an item from an equipment slot and returns it
func (c *MMCharacter) UnequipItem(slot items.EquipSlot) (items.Item, bool) {
	if item, exists := c.Equipment[slot]; exists {
		delete(c.Equipment, slot)

		// Recalculate derived stats after unequipping
		c.updateDerivedStatsForEquipment()

		return item, true
	}
	return items.Item{}, false
}

// GetEffectiveStats returns character stats with any active bonuses applied (spells + equipment)
func (c *MMCharacter) GetEffectiveStats() (might, intellect, personality, endurance, accuracy, speed, luck int) {
	// Calculate equipment bonuses (YAML-driven); buffs come from BuffBonuses
	// (pushed by the game when spells like Bless apply/expire).
	eqMight, eqIntellect, eqPersonality, eqEndurance, eqAccuracy, eqSpeed, eqLuck := c.calculateEquipmentBonuses()

	return c.Might + c.BuffBonuses.Might + eqMight,
		c.Intellect + c.BuffBonuses.Intellect + eqIntellect,
		c.Personality + c.BuffBonuses.Personality + eqPersonality,
		c.Endurance + c.BuffBonuses.Endurance + eqEndurance,
		c.Accuracy + c.BuffBonuses.Accuracy + eqAccuracy,
		c.Speed + c.BuffBonuses.Speed + eqSpeed,
		c.Luck + c.BuffBonuses.Luck + eqLuck
}

// GetEffectiveMight returns effective Might with bonuses applied
func (c *MMCharacter) GetEffectiveMight() int {
	eqBonus, _, _, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Might + c.BuffBonuses.Might + eqBonus
}

// GetEffectiveIntellect returns effective Intellect with bonuses applied
func (c *MMCharacter) GetEffectiveIntellect() int {
	_, eqBonus, _, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Intellect + c.BuffBonuses.Intellect + eqBonus
}

// GetEffectivePersonality returns effective Personality with bonuses applied
func (c *MMCharacter) GetEffectivePersonality() int {
	_, _, eqBonus, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Personality + c.BuffBonuses.Personality + eqBonus
}

// GetEffectiveEndurance returns effective Endurance with bonuses applied
func (c *MMCharacter) GetEffectiveEndurance() int {
	_, _, _, eqBonus, _, _, _ := c.calculateEquipmentBonuses()
	return c.Endurance + c.BuffBonuses.Endurance + eqBonus
}

// GetEffectiveAccuracy returns effective Accuracy with bonuses applied
func (c *MMCharacter) GetEffectiveAccuracy() int {
	_, _, _, _, eqBonus, _, _ := c.calculateEquipmentBonuses()
	return c.Accuracy + c.BuffBonuses.Accuracy + eqBonus
}

// GetEffectiveSpeed returns effective Speed with bonuses applied
func (c *MMCharacter) GetEffectiveSpeed() int {
	_, _, _, _, _, eqBonus, _ := c.calculateEquipmentBonuses()
	return c.Speed + c.BuffBonuses.Speed + eqBonus
}

// GetEffectiveLuck returns effective Luck with bonuses applied
func (c *MMCharacter) GetEffectiveLuck() int {
	_, _, _, _, _, _, eqBonus := c.calculateEquipmentBonuses()
	return c.Luck + c.BuffBonuses.Luck + eqBonus
}

// calculateEquipmentBonuses returns stat bonuses from all equipped items (YAML-driven)
func (c *MMCharacter) calculateEquipmentBonuses() (mightBonus, intellectBonus, personalityBonus, enduranceBonus, accuracyBonus, speedBonus, luckBonus int) {
	for _, it := range c.Equipment {
		// Scaling divisor bonuses (stat / divisor)
		if div := it.Attributes["intellect_scaling_divisor"]; div > 0 {
			intellectBonus += c.Intellect / div
		}
		if div := it.Attributes["personality_scaling_divisor"]; div > 0 {
			personalityBonus += c.Personality / div
		}
		// endurance_scaling_divisor is deliberately NOT a stat bonus: it is the
		// armor piece's AC formula input (AC = base + effective End / divisor,
		// see CalculateArmorClassContribution). Feeding it back into Endurance
		// made every armor piece inflate every other piece's AC.
		// Flat bonuses
		if bonus := it.Attributes["bonus_might"]; bonus > 0 {
			mightBonus += bonus
		}
		if bonus := it.Attributes["bonus_intellect"]; bonus > 0 {
			intellectBonus += bonus
		}
		if bonus := it.Attributes["bonus_personality"]; bonus > 0 {
			personalityBonus += bonus
		}
		if bonus := it.Attributes["bonus_endurance"]; bonus > 0 {
			enduranceBonus += bonus
		}
		if bonus := it.Attributes["bonus_accuracy"]; bonus > 0 {
			accuracyBonus += bonus
		}
		if bonus := it.Attributes["bonus_speed"]; bonus > 0 {
			speedBonus += bonus
		}
		if bonus := it.Attributes["bonus_luck"]; bonus > 0 {
			luckBonus += bonus
		}
	}
	return mightBonus, intellectBonus, personalityBonus, enduranceBonus, accuracyBonus, speedBonus, luckBonus
}

// GearResistPct sums the character's % resistance to a damage school from equipped gear.
func (c *MMCharacter) GearResistPct(school string) int {
	key := "resist_" + strings.ToLower(strings.TrimSpace(school))
	total := 0
	for _, it := range c.Equipment {
		total += it.Attributes[key]
	}
	return total
}

// updateDerivedStatsForEquipment re-derives MaxHP/MaxSP after an equip change,
// preserving current HP/SP (gain is added, loss only caps — no free healing).
// Same formula as every other recalc path: effective stats drive both maxima,
// so endurance/intellect gear finally shows up in HP/SP.
func (c *MMCharacter) updateDerivedStatsForEquipment() {
	c.RecalculateMaxStatsKeepingCurrent(nil) // nil → config.GlobalConfig / defaults
}
