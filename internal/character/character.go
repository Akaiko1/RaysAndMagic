package character

import (
	"fmt"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

type MMCharacter struct {
	Name  string
	Class CharacterClass

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
	MagicSchools map[MagicSchool]*MagicSkill

	// Equipment slots
	Equipment map[items.EquipSlot]items.Item

	// Status effects
	Conditions []Condition

	// Regeneration timer - counts frames until next spell point regeneration
	spellRegenTimer int
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

func CreateCharacter(name string, class CharacterClass, cfg *config.Config) *MMCharacter {
	char := &MMCharacter{
		Name:         name,
		Class:        class,
		Level:        1,
		Experience:   0,
		Skills:       make(map[SkillType]*Skill),
		MagicSchools: make(map[MagicSchool]*MagicSkill),
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
	c.Skills[SkillSword] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillBodybuilding] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting equipment
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Iron Sword", 8, 2, "A sturdy iron sword")
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
	c.Skills[SkillDagger] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic
	c.MagicSchools[MagicFire] = &MagicSkill{Level: 1, Mastery: MasteryNovice}
	c.MagicSchools[MagicFire].Spells = []Spell{
		{Name: "Torch Light", School: MagicFire, Level: 1, SpellPoints: 1},
		{Name: "Fire Bolt", School: MagicFire, Level: 1, SpellPoints: 2},
		{Name: "Fireball", School: MagicFire, Level: 1, SpellPoints: 4},
	}

	// Starting equipment
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Magic Dagger", 4, 2, "A light dagger for mages")
	c.Equipment[items.SlotSpell] = items.CreateBattleSpell("Fire Bolt", items.SpellEffectFireBolt, "fire", 2, "Launches a fast bolt of fire")
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
	c.Skills[SkillMace] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic
	c.MagicSchools[MagicBody] = &MagicSkill{Level: 1, Mastery: MasteryNovice}
	c.MagicSchools[MagicBody].Spells = []Spell{
		{Name: "First Aid", School: MagicBody, Level: 1, SpellPoints: 1},
		{Name: "Heal", School: MagicBody, Level: 1, SpellPoints: 2},
	}

	// Starting equipment
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Holy Mace", 6, 2, "A blessed mace")
	c.Equipment[items.SlotSpell] = items.CreateUtilitySpell("Heal", items.SpellEffectHealOther, "body", 2, "Restores health to self or others")
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
	c.Skills[SkillBow] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillDagger] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic
	c.MagicSchools[MagicAir] = &MagicSkill{Level: 1, Mastery: MasteryNovice}
	c.MagicSchools[MagicAir].Spells = []Spell{
		{Name: "Wizard Eye", School: MagicAir, Level: 1, SpellPoints: 1},
	}

	// Starting equipment for archer
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Hunting Bow", 6, 8, "A well-crafted bow for hunting")
	c.Equipment[items.SlotSpell] = items.CreateUtilitySpell("Wizard Eye", items.SpellEffectWizardEye, "air", 1, "Reveals the surrounding area")
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

	// Starting skills
	c.Skills[SkillSword] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic
	c.MagicSchools[MagicSpirit] = &MagicSkill{Level: 1, Mastery: MasteryNovice}
	c.MagicSchools[MagicSpirit].Spells = []Spell{
		{Name: "Bless", School: MagicSpirit, Level: 1, SpellPoints: 1},
	}

	// Starting equipment for paladin
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Silver Sword", 8, 2, "A blessed silver sword")
	c.Equipment[items.SlotSpell] = items.CreateUtilitySpell("Bless", items.SpellEffectPartyBuff, "spirit", 1, "Increases party's combat effectiveness")
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

	// Starting skills
	c.Skills[SkillStaff] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic
	c.MagicSchools[MagicWater] = &MagicSkill{Level: 1, Mastery: MasteryNovice}
	c.MagicSchools[MagicWater].Spells = []Spell{
		{Name: "Awaken", School: MagicWater, Level: 1, SpellPoints: 1},
	}

	// Starting equipment for druid
	c.Equipment[items.SlotMainHand] = items.CreateWeapon("Oak Staff", 5, 2, "A staff carved from ancient oak")
	c.Equipment[items.SlotSpell] = items.CreateUtilitySpell("Awaken", items.SpellEffectPartyBuff, "water", 1, "Awakens fallen party members")
}

func (c *MMCharacter) CalculateDerivedStats(cfg *config.Config) {
	// Calculate hit points (Endurance based)
	c.MaxHitPoints = c.Endurance*cfg.Characters.HitPoints.EnduranceMultiplier + c.Level*cfg.Characters.HitPoints.LevelMultiplier
	c.HitPoints = c.MaxHitPoints

	// Calculate spell points (Intellect + Personality based)
	c.MaxSpellPoints = c.Intellect + c.Personality + c.Level*cfg.Characters.SpellPoints.LevelMultiplier
	c.SpellPoints = c.MaxSpellPoints
}

func (c *MMCharacter) Update() {
	// Regenerate spell points slowly - only every 3 seconds (180 frames at 60 FPS)
	const spellRegenFrames = 180
	c.spellRegenTimer++

	if c.spellRegenTimer >= spellRegenFrames && c.SpellPoints < c.MaxSpellPoints {
		c.SpellPoints++
		c.spellRegenTimer = 0 // Reset timer
	}
}

func (c *MMCharacter) GetDisplayInfo() string {
	className := c.GetClassName()
	condition := "OK"
	if len(c.Conditions) > 0 {
		condition = c.getConditionName(c.Conditions[0])
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
	info += fmt.Sprintf("Class: %s  Level: %d\n", c.GetClassName(), c.Level)
	info += fmt.Sprintf("Experience: %d\n\n", c.Experience)

	info += "ATTRIBUTES:\n"
	info += fmt.Sprintf("Might: %d  Intellect: %d\n", c.Might, c.Intellect)
	info += fmt.Sprintf("Personality: %d  Endurance: %d\n", c.Personality, c.Endurance)
	info += fmt.Sprintf("Accuracy: %d  Speed: %d  Luck: %d\n\n", c.Accuracy, c.Speed, c.Luck)

	info += "SKILLS:\n"
	for skillType, skill := range c.Skills {
		info += fmt.Sprintf("%s: %d (%s)\n",
			c.getSkillName(skillType), skill.Level, c.getMasteryName(skill.Mastery))
	}

	info += "\nMAGIC SCHOOLS:\n"
	for school, magicSkill := range c.MagicSchools {
		info += fmt.Sprintf("%s: %d (%s) - %d spells\n",
			c.GetMagicSchoolName(school), magicSkill.Level,
			c.getMasteryName(magicSkill.Mastery), len(magicSkill.Spells))
	}

	return info
}

func (c *MMCharacter) GetClassName() string {
	switch c.Class {
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

func (c *MMCharacter) getMasteryName(mastery SkillMastery) string {
	switch mastery {
	case MasteryNovice:
		return "Novice"
	case MasteryExpert:
		return "Expert"
	case MasteryMaster:
		return "Master"
	case MasteryGrandMaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

func (c *MMCharacter) getSkillName(skill SkillType) string {
	names := map[SkillType]string{
		SkillSword: "Sword", SkillDagger: "Dagger", SkillAxe: "Axe",
		SkillSpear: "Spear", SkillBow: "Bow", SkillMace: "Mace", SkillStaff: "Staff",
		SkillLeather: "Leather", SkillChain: "Chain", SkillPlate: "Plate", SkillShield: "Shield",
		SkillBodybuilding: "Bodybuilding", SkillMeditation: "Meditation",
		SkillMerchant: "Merchant", SkillRepair: "Repair", SkillIdentifyItem: "Identify Item",
	}
	if name, exists := names[skill]; exists {
		return name
	}
	return "Unknown"
}

func (c *MMCharacter) GetMagicSchoolName(school MagicSchool) string {
	names := map[MagicSchool]string{
		MagicBody: "Body", MagicMind: "Mind", MagicSpirit: "Spirit",
		MagicFire: "Fire", MagicWater: "Water", MagicAir: "Air", MagicEarth: "Earth",
		MagicLight: "Light", MagicDark: "Dark",
	}
	if name, exists := names[school]; exists {
		return name
	}
	return "Unknown"
}

func (c *MMCharacter) getConditionName(condition Condition) string {
	names := map[Condition]string{
		ConditionNormal: "OK", ConditionPoisoned: "Poisoned", ConditionDiseased: "Diseased",
		ConditionCursed: "Cursed", ConditionAsleep: "Asleep", ConditionFear: "Fear",
		ConditionParalyzed: "Paralyzed", ConditionDead: "Dead", ConditionStone: "Stone",
		ConditionEradicated: "Eradicated",
	}
	if name, exists := names[condition]; exists {
		return name
	}
	return "Unknown"
}

// GetAvailableSchools returns the magic schools available to this character in a consistent order
func (c *MMCharacter) GetAvailableSchools() []MagicSchool {
	// Return schools in a consistent order to prevent UI issues
	allSchools := []MagicSchool{
		MagicFire,
		MagicWater,
		MagicAir,
		MagicEarth,
		MagicBody,
		MagicMind,
		MagicSpirit,
		MagicLight,
		MagicDark,
	}
	
	var availableSchools []MagicSchool
	for _, school := range allSchools {
		if _, exists := c.MagicSchools[school]; exists {
			availableSchools = append(availableSchools, school)
		}
	}
	return availableSchools
}

// GetSpellsForSchool returns the spells for a specific magic school
func (c *MMCharacter) GetSpellsForSchool(school MagicSchool) []Spell {
	if magicSkill, exists := c.MagicSchools[school]; exists {
		return magicSkill.Spells
	}
	return []Spell{}
}
