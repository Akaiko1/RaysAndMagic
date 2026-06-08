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
	// has left in the current turn-based round. Refilled by ActionSlotsForTurn
	// at party-turn start, decremented on each attack/spell, set to 0 on
	// party movement (which immediately ends the round). Unused in real-time.
	ActionsRemaining int

	// RTCooldown is this character's remaining real-time action cooldown in
	// frames. While > 0 the member is "busy" (grayed in the HUD, skipped by
	// auto-select); set on each attack/cast from the weapon/spell cooldown and
	// ticked down once per frame. Unused in turn-based mode (which uses
	// ActionsRemaining instead).
	RTCooldown int
}

// Turn-based action-slot thresholds on effective Speed. Single source shared by
// ActionSlotsForTurn (mechanic) and the Speed stat tooltip (description).
const (
	SpeedActionSlot2Threshold = 25 // Speed > this → 2 actions/turn
	SpeedActionSlot3Threshold = 50 // Speed > this → 3 actions/turn
)

// ActionSlotsForTurn returns the number of attack/spell slots this character
// gets per turn-based round, based on effective Speed (see the threshold
// constants above). statBonus is the global Bless-style stat buff (0 if none).
func (c *MMCharacter) ActionSlotsForTurn(statBonus int) int {
	speed := c.GetEffectiveSpeed(statBonus)
	switch {
	case speed > SpeedActionSlot3Threshold:
		return 3
	case speed > SpeedActionSlot2Threshold:
		return 2
	default:
		return 1
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

	// Set base attributes based on class
	switch class {
	case ClassKnight:
		char.setupKnight(cfg)
	case ClassPaladin:
		char.setupPaladin(cfg)
	case ClassArcher:
		char.setupArcher(cfg)
	case ClassCleric:
		char.setupCleric(cfg)
	case ClassSorcerer:
		char.setupSorcerer(cfg)
	case ClassDruid:
		char.setupDruid(cfg)
	}

	char.CalculateDerivedStats(cfg)
	return char
}

func (c *MMCharacter) setupKnight(cfg *config.Config) {
	// Knights: High might and endurance, masters of weapons and armor
	stats := cfg.Characters.Classes["knight"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills
	c.Skills[SkillSword] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillSpear] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillPlate] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillBodybuilding] = &Skill{Mastery: MasteryNovice}

	// Starting equipment - YAML weapons only
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
}

func (c *MMCharacter) setupSorcerer(cfg *config.Config) {
	// Sorcerers: High intellect, masters of elemental magic
	stats := cfg.Characters.Classes["sorcerer"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills
	c.Skills[SkillDagger] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillStaff] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLearning] = &Skill{Mastery: MasteryNovice}

	// Starting magic - give Sorcerer fire and water spells
	c.MagicSchools[MagicSchoolFire] = &MagicSkill{
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("torch_light"),
			spells.SpellID("firebolt"),
		},
	}
	c.MagicSchools[MagicSchoolWater] = &MagicSkill{
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("ice_bolt"),
		},
	}
	c.MagicSchools[MagicSchoolAir] = &MagicSkill{
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("sparks"),
		},
	}

	// Starting equipment - give Sorcerer FireBolt
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("magic_dagger")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("firebolt")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
}

func (c *MMCharacter) setupCleric(cfg *config.Config) {
	// Clerics: High personality, masters of self magic
	stats := cfg.Characters.Classes["cleric"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills
	c.Skills[SkillMace] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLearning] = &Skill{Mastery: MasteryNovice}

	// Starting magic - give Cleric healing spells and spirit magic
	c.MagicSchools[MagicSchoolBody] = &MagicSkill{
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("heal_other"), // Heal
			spells.SpellID("harm"),       // offensive body magic
		},
	}
	// Clerics are the masters of self magic — open Mind and Spirit too (alongside
	// Body) so they can learn/cast the full self-magic catalog (all of which scales
	// with their high Personality). Spirit starts with Spirit Lash.
	c.MagicSchools[MagicSchoolMind] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("mind_blast")},
	}
	c.MagicSchools[MagicSchoolSpirit] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("spirit_lash")},
	}

	// Starting equipment - give Cleric Heal
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("holy_mace")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("heal_other")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
}

func (c *MMCharacter) setupArcher(cfg *config.Config) {
	// Archers: High accuracy, bow masters with some elemental magic
	stats := cfg.Characters.Classes["archer"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills
	c.Skills[SkillBow] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillDagger] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillDisarmTrap] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillBodybuilding] = &Skill{Mastery: MasteryNovice}

	// Starting magic - give Archer Wizard's Eye
	c.MagicSchools[MagicSchoolAir] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("wizard_eye")},
	}

	// Starting equipment - give Archer Wizard's Eye
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("wizard_eye")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
}

func (c *MMCharacter) setupPaladin(cfg *config.Config) {
	// Paladins: Balanced fighter/cleric hybrid
	stats := cfg.Characters.Classes["paladin"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills — paladins wield axes and swords, wear chain, bear shields,
	// and train their bodies for the front line.
	c.Skills[SkillAxe] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillSword] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillBodybuilding] = &Skill{Mastery: MasteryNovice}

	// No starting magic — the paladin can choose to learn Heal Other at level 3.

	// Starting equipment — a heavy steel axe.
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("steel_axe")
}

func (c *MMCharacter) setupDruid(cfg *config.Config) {
	// Druids: Nature-focused hybrid of sorcerer/cleric
	stats := cfg.Characters.Classes["druid"]
	c.Might = stats.Might
	c.Intellect = stats.Intellect
	c.Personality = stats.Personality
	c.Endurance = stats.Endurance
	c.Accuracy = stats.Accuracy
	c.Speed = stats.Speed
	c.Luck = stats.Luck

	// Starting skills — wilderness hardiness and nature lore round out the druid.
	c.Skills[SkillStaff] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillBodybuilding] = &Skill{Mastery: MasteryNovice}
	c.Skills[SkillLearning] = &Skill{Mastery: MasteryNovice}

	// Starting magic - give Druid water and mind magic
	c.MagicSchools[MagicSchoolWater] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("ice_bolt")},
	}
	c.MagicSchools[MagicSchoolMind] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("awaken")},
	}
	c.MagicSchools[MagicSchoolEarth] = &MagicSkill{
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("deadly_swarm")},
	}

	// Starting equipment - give Druid Awaken
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("oak_staff")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("awaken")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
}

func (c *MMCharacter) CalculateDerivedStats(cfg *config.Config) {
	// Calculate hit points (Endurance based) + Bodybuilding bonus
	baseMaxHP := c.Endurance*cfg.Characters.HitPoints.EnduranceMultiplier + c.Level*cfg.Characters.HitPoints.LevelMultiplier
	c.MaxHitPoints = baseMaxHP + c.bodybuildingBonusHP(baseMaxHP)
	c.HitPoints = c.MaxHitPoints

	// Calculate spell points (Intellect + Personality based + equipment bonuses)
	_, _, equipmentPersonalityBonus, _, _, _, _ := c.calculateEquipmentBonuses()
	c.MaxSpellPoints = c.Intellect + c.Personality + equipmentPersonalityBonus + c.Level*cfg.Characters.SpellPoints.LevelMultiplier
	c.SpellPoints = c.MaxSpellPoints
}

// RecalculateMaxStatsKeepingCurrent recomputes MaxHP/MaxSP after a stat change
// (e.g. spending a single stat point) WITHOUT fully healing: any increase in a
// max is added to the current value and current is capped at the new max, so a
// hurt character only gains the stat's bonus, not a free full heal. Use this
// instead of CalculateDerivedStats, which fully restores (intended only at
// character creation and on level-up).
func (c *MMCharacter) RecalculateMaxStatsKeepingCurrent(cfg *config.Config) {
	oldMaxHP := c.MaxHitPoints
	oldMaxSP := c.MaxSpellPoints

	baseMaxHP := c.Endurance*cfg.Characters.HitPoints.EnduranceMultiplier + c.Level*cfg.Characters.HitPoints.LevelMultiplier
	c.MaxHitPoints = baseMaxHP + c.bodybuildingBonusHP(baseMaxHP)
	_, _, equipmentPersonalityBonus, _, _, _, _ := c.calculateEquipmentBonuses()
	c.MaxSpellPoints = c.Intellect + c.Personality + equipmentPersonalityBonus + c.Level*cfg.Characters.SpellPoints.LevelMultiplier

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
	c.UpdateWithStatBonus(0)
}

// UpdateWithMode updates the character with knowledge of the current game mode
func (c *MMCharacter) UpdateWithMode(turnBasedMode bool, statBonus int) {
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
	c.UpdateWithStatBonus(statBonus)
}

// UpdateWithStatBonus updates the character and applies stat-based regen using the provided bonus.
func (c *MMCharacter) UpdateWithStatBonus(statBonus int) {
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
		c.RegenerateSpellPoints(statBonus)
		c.spellRegenTimer = 0 // Reset timer
	}
}

// CalculateManaRegenAmount returns SP regen per tick based on effective Personality.
func (c *MMCharacter) CalculateManaRegenAmount(statBonus int) int {
	effectivePersonality := c.GetEffectivePersonality(statBonus)
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
func (c *MMCharacter) RegenerateSpellPoints(statBonus int) {
	if !c.CanAct() {
		return
	}
	if c.SpellPoints >= c.MaxSpellPoints {
		return
	}
	c.SpellPoints += c.CalculateManaRegenAmount(statBonus)
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
func (c *MMCharacter) GetEffectiveStats(statBonus int) (might, intellect, personality, endurance, accuracy, speed, luck int) {
	// Calculate equipment bonuses (YAML-driven)
	eqMight, eqIntellect, eqPersonality, eqEndurance, eqAccuracy, eqSpeed, eqLuck := c.calculateEquipmentBonuses()

	return c.Might + statBonus + eqMight,
		c.Intellect + statBonus + eqIntellect,
		c.Personality + statBonus + eqPersonality,
		c.Endurance + statBonus + eqEndurance,
		c.Accuracy + statBonus + eqAccuracy,
		c.Speed + statBonus + eqSpeed,
		c.Luck + statBonus + eqLuck
}

// GetEffectiveMight returns effective Might with bonuses applied
func (c *MMCharacter) GetEffectiveMight(statBonus int) int {
	eqBonus, _, _, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Might + statBonus + eqBonus
}

// GetEffectiveIntellect returns effective Intellect with bonuses applied
func (c *MMCharacter) GetEffectiveIntellect(statBonus int) int {
	_, eqBonus, _, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Intellect + statBonus + eqBonus
}

// GetEffectivePersonality returns effective Personality with bonuses applied
func (c *MMCharacter) GetEffectivePersonality(statBonus int) int {
	_, _, eqBonus, _, _, _, _ := c.calculateEquipmentBonuses()
	return c.Personality + statBonus + eqBonus
}

// GetEffectiveEndurance returns effective Endurance with bonuses applied
func (c *MMCharacter) GetEffectiveEndurance(statBonus int) int {
	_, _, _, eqBonus, _, _, _ := c.calculateEquipmentBonuses()
	return c.Endurance + statBonus + eqBonus
}

// GetEffectiveAccuracy returns effective Accuracy with bonuses applied
func (c *MMCharacter) GetEffectiveAccuracy(statBonus int) int {
	_, _, _, _, eqBonus, _, _ := c.calculateEquipmentBonuses()
	return c.Accuracy + statBonus + eqBonus
}

// GetEffectiveSpeed returns effective Speed with bonuses applied
func (c *MMCharacter) GetEffectiveSpeed(statBonus int) int {
	_, _, _, _, _, eqBonus, _ := c.calculateEquipmentBonuses()
	return c.Speed + statBonus + eqBonus
}

// GetEffectiveLuck returns effective Luck with bonuses applied
func (c *MMCharacter) GetEffectiveLuck(statBonus int) int {
	_, _, _, _, _, _, eqBonus := c.calculateEquipmentBonuses()
	return c.Luck + statBonus + eqBonus
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
		if div := it.Attributes["endurance_scaling_divisor"]; div > 0 {
			enduranceBonus += c.Endurance / div
		}
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

// GearResistPct returns the character's % damage resistance for a damage school
// (e.g. "fire", "physical") summed from equipped gear — the same per-element
// resist model monsters use. Combat adds any party-wide resist buff on top and
// caps the total. School is matched lowercase.
func (c *MMCharacter) GearResistPct(school string) int {
	key := "resist_" + strings.ToLower(strings.TrimSpace(school))
	total := 0
	for _, it := range c.Equipment {
		total += it.Attributes[key]
	}
	return total
}

// updateDerivedStatsForEquipment recalculates max SP while preserving current SP intelligently
func (c *MMCharacter) updateDerivedStatsForEquipment() {
	// Save current values
	oldMaxSP := c.MaxSpellPoints
	currentSP := c.SpellPoints

	// Recalculate max SP with equipment bonuses
	_, _, equipmentPersonalityBonus, _, _, _, _ := c.calculateEquipmentBonuses()
	newMaxSP := c.Intellect + c.Personality + equipmentPersonalityBonus + c.Level*2 // Level multiplier from config
	c.MaxSpellPoints = newMaxSP

	// Smart SP update: if we gained max SP, grant the bonus to current SP too
	if newMaxSP > oldMaxSP {
		spBonus := newMaxSP - oldMaxSP
		c.SpellPoints = currentSP + spBonus
		// Cap at new maximum
		if c.SpellPoints > c.MaxSpellPoints {
			c.SpellPoints = c.MaxSpellPoints
		}
	}
	// If we lost max SP (unequipping), just cap current SP at new max
	if c.SpellPoints > c.MaxSpellPoints {
		c.SpellPoints = c.MaxSpellPoints
	}
}
