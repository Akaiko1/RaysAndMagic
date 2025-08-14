package character

import (
	"fmt"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
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

	// Free stat points to distribute on level-up
	FreeStatPoints int
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
	c.Skills[SkillDagger] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic - give Sorcerer fire and water spells
	c.MagicSchools[MagicFire] = &MagicSkill{
		Level:   1,
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("torch_light"),
			spells.SpellID("firebolt"),
			spells.SpellID("fireball"),
		},
	}
	c.MagicSchools[MagicWater] = &MagicSkill{
		Level:   1,
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("ice_bolt"),
			spells.SpellID("water_breathing"),
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
	c.Skills[SkillMace] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillMeditation] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic - give Cleric healing spells and spirit magic
	c.MagicSchools[MagicBody] = &MagicSkill{
		Level:   1,
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("heal"),       // First Aid
			spells.SpellID("heal_other"), // Heal
		},
	}

	// Add Spirit magic for divine spells like Bless
	c.MagicSchools[MagicSpirit] = &MagicSkill{
		Level:   1,
		Mastery: MasteryNovice,
		KnownSpells: []spells.SpellID{
			spells.SpellID("bless"), // Bless
		},
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
	c.Skills[SkillBow] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillLeather] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillDagger] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic - give Archer Wizard's Eye
	c.MagicSchools[MagicAir] = &MagicSkill{
		Level:       1,
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

	// Starting skills
	c.Skills[SkillSword] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillChain] = &Skill{Level: 1, Mastery: MasteryNovice}
	c.Skills[SkillShield] = &Skill{Level: 1, Mastery: MasteryNovice}

	// Starting magic - give Paladin Bless
	c.MagicSchools[MagicSpirit] = &MagicSkill{
		Level:       1,
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("bless")},
	}

	// Starting equipment - give Paladin Bless
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("silver_sword")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("bless")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
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

	// Starting magic - give Druid water and mind magic
	c.MagicSchools[MagicWater] = &MagicSkill{
		Level:       1,
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("ice_bolt")},
	}
	c.MagicSchools[MagicMind] = &MagicSkill{
		Level:       1,
		Mastery:     MasteryNovice,
		KnownSpells: []spells.SpellID{spells.SpellID("awaken")},
	}

	// Starting equipment - give Druid Awaken
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("oak_staff")
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("awaken")); err == nil {
		c.Equipment[items.SlotSpell] = spellItem
	}
}

func (c *MMCharacter) CalculateDerivedStats(cfg *config.Config) {
	// Calculate hit points (Endurance based)
	c.MaxHitPoints = c.Endurance*cfg.Characters.HitPoints.EnduranceMultiplier + c.Level*cfg.Characters.HitPoints.LevelMultiplier
	c.HitPoints = c.MaxHitPoints

	// Calculate spell points (Intellect + Personality based + equipment bonuses)
	_, equipmentPersonalityBonus, _ := c.calculateEquipmentBonuses()
	c.MaxSpellPoints = c.Intellect + c.Personality + equipmentPersonalityBonus + c.Level*cfg.Characters.SpellPoints.LevelMultiplier
	c.SpellPoints = c.MaxSpellPoints
}

func (c *MMCharacter) Update() {
	// Regenerate spell points slowly - only every 3 seconds (90 frames at 30 FPS)
	const spellRegenFrames = 90
	c.spellRegenTimer++

	if c.spellRegenTimer >= spellRegenFrames && c.SpellPoints < c.MaxSpellPoints {
		c.SpellPoints++
		c.spellRegenTimer = 0 // Reset timer
	}
}

// UpdateWithMode updates the character with knowledge of the current game mode
func (c *MMCharacter) UpdateWithMode(turnBasedMode bool) {
	// Skip timer-based regeneration in turn-based mode
	if turnBasedMode {
		return
	}

	// Use normal timer-based regeneration in real-time mode
	c.Update()
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
			c.getMasteryName(magicSkill.Mastery), len(magicSkill.KnownSpells))
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

// GetSpellsForSchool returns the spell IDs for a specific magic school
func (c *MMCharacter) GetSpellsForSchool(school MagicSchool) []spells.SpellID {
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

	category := weaponDef.Category

	switch c.Class {
	case ClassKnight:
		// Knights can use all melee weapons
		return category == "sword" || category == "axe" || category == "mace" || category == "spear"
	case ClassPaladin:
		// Paladins can use swords, maces, and spears
		return category == "sword" || category == "mace" || category == "spear"
	case ClassArcher:
		// Archers use bows and light melee weapons
		return category == "bow" || category == "dagger"
	case ClassCleric:
		// Clerics use maces and staffs
		return category == "mace" || category == "staff"
	case ClassSorcerer:
		// Sorcerers use staffs and light weapons
		return category == "staff" || category == "dagger"
	case ClassDruid:
		// Druids use natural weapons: staffs, spears, daggers
		return category == "staff" || category == "spear" || category == "dagger"
	}
	return false
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
		slot = items.SlotArmor
	case items.ItemAccessory:
		// For now, put accessories in ring slot 1
		slot = items.SlotRing1
	default:
		return items.Item{}, false, false
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
	// Calculate equipment bonuses
	equipmentIntellectBonus, equipmentPersonalityBonus, equipmentEnduranceBonus := c.calculateEquipmentBonuses()

	return c.Might + statBonus,
		c.Intellect + statBonus + equipmentIntellectBonus,
		c.Personality + statBonus + equipmentPersonalityBonus,
		c.Endurance + statBonus + equipmentEnduranceBonus,
		c.Accuracy + statBonus,
		c.Speed + statBonus,
		c.Luck + statBonus
}

    // calculateEquipmentBonuses returns stat bonuses from equipped items (YAML-driven)
    func (c *MMCharacter) calculateEquipmentBonuses() (intellectBonus, personalityBonus, enduranceBonus int) {
        // Rings/accessories: apply scaling divisors if present
        if ring, hasRing := c.Equipment[items.SlotRing1]; hasRing {
            if div := ring.Attributes["intellect_scaling_divisor"]; div > 0 {
                intellectBonus += c.Intellect / div
            }
            if div := ring.Attributes["personality_scaling_divisor"]; div > 0 {
                personalityBonus += c.Personality / div
            }
        }

        if ring, hasRing := c.Equipment[items.SlotRing2]; hasRing {
            if div := ring.Attributes["intellect_scaling_divisor"]; div > 0 {
                intellectBonus += c.Intellect / div
            }
            if div := ring.Attributes["personality_scaling_divisor"]; div > 0 {
                personalityBonus += c.Personality / div
            }
        }

        // Armor: endurance scaling divisor if present
        if armor, hasArmor := c.Equipment[items.SlotArmor]; hasArmor {
            if div := armor.Attributes["endurance_scaling_divisor"]; div > 0 {
                enduranceBonus += c.Endurance / div
            }
        }

        return intellectBonus, personalityBonus, enduranceBonus
    }

// updateDerivedStatsForEquipment recalculates max SP while preserving current SP intelligently
func (c *MMCharacter) updateDerivedStatsForEquipment() {
	// Save current values
	oldMaxSP := c.MaxSpellPoints
	currentSP := c.SpellPoints

	// Recalculate max SP with equipment bonuses
	_, equipmentPersonalityBonus, _ := c.calculateEquipmentBonuses()
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
