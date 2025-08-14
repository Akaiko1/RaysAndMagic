package game

import (
    "fmt"
    "strings"
    "ugataima/internal/character"
    "ugataima/internal/config"
    "ugataima/internal/items"
    "ugataima/internal/spells"
)

// GetItemTooltip returns a comprehensive tooltip string for any item type
func GetItemTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
    var tooltip []string

	// Item name and type
	tooltip = append(tooltip, fmt.Sprintf("=== %s ===", item.Name))
	tooltip = append(tooltip, getItemTypeString(item.Type))

    // Do not rely on YAML description for mechanics; we generate human-readable text from attributes and type.

	switch item.Type {
	case items.ItemWeapon:
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, getWeaponTooltip(item, char, combatSystem)...)

    case items.ItemArmor:
        tooltip = append(tooltip, "")
        armorLine := getArmorSummary(item, char)
        if armorLine != "" {
            tooltip = append(tooltip, armorLine)
        }

    case items.ItemAccessory:
        tooltip = append(tooltip, "")
        accLine := getAccessorySummary(item, char)
        if accLine != "" {
            tooltip = append(tooltip, accLine)
        }

	case items.ItemBattleSpell, items.ItemUtilitySpell:
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, getSpellItemTooltip(item, char)...)

    case items.ItemConsumable:
        tooltip = append(tooltip, "")
        consLine := getConsumableSummary(item, char)
        if consLine != "" {
            tooltip = append(tooltip, consLine)
        }

	case items.ItemQuest:
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, "Quest Item - Cannot be sold or dropped")
	}

    // Append short flavor text (from YAML description) if present
    if item.Description != "" {
        tooltip = append(tooltip, "")
        tooltip = append(tooltip, fmt.Sprintf("\"%s\"", item.Description))
    }

    // Do not dump raw attributes; type-specific sections above produce a human-friendly summary.

	return joinTooltipLines(tooltip)
}

// getItemTypeString returns a readable string for the item type
func getItemTypeString(itemType items.ItemType) string {
	switch itemType {
	case items.ItemWeapon:
		return "Weapon"
	case items.ItemArmor:
		return "Armor"
	case items.ItemAccessory:
		return "Accessory"
	case items.ItemConsumable:
		return "Consumable"
	case items.ItemQuest:
		return "Quest Item"
	case items.ItemBattleSpell:
		return "Battle Spell"
	case items.ItemUtilitySpell:
		return "Utility Spell"
	default:
		return "Unknown"
	}
}

// getWeaponTooltip returns weapon-specific tooltip information using real combat formulas
func getWeaponTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	var lines []string

	// Use the centralized weapon damage calculation
	base, bonus, total := combatSystem.CalculateWeaponDamage(item, char)

	lines = append(lines, fmt.Sprintf("Base Damage: %d", base))

	// Show primary stat scaling
	bonusStatName := item.BonusStat
	if bonusStatName == "" {
		bonusStatName = "Might" // Fallback
	}

	// Show dual-stat scaling if weapon has secondary stat
	if item.BonusStatSecondary != "" {
		lines = append(lines, fmt.Sprintf("Scales with %s + %s", bonusStatName, item.BonusStatSecondary))
	} else {
		lines = append(lines, fmt.Sprintf("Scales with %s", bonusStatName))
	}

	lines = append(lines, fmt.Sprintf("Stat Bonus: +%d", bonus))
	lines = append(lines, fmt.Sprintf("Total Damage: %d", total))

	if item.Range > 0 {
		lines = append(lines, fmt.Sprintf("Range: %d tiles", item.Range))
	}

	// Get weapon definition for additional stats
	weaponKey := items.GetWeaponKeyByName(item.Name)
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		panic("weapon '" + item.Name + "' not found in weapons.yaml - system misconfigured")
	}

	// Use weapon definition for hit chance calculation
	baseHitBonus := weaponDef.HitBonus
	accuracyBonus := combatSystem.CalculateAccuracyBonus(char)
	totalHitBonus := baseHitBonus + accuracyBonus
	lines = append(lines, fmt.Sprintf("Hit Chance: +%d%% (Base: +%d, Accuracy: +%d)", totalHitBonus, baseHitBonus, accuracyBonus))

	// Add critical hit chance from weapon definition
	if weaponDef.CritChance > 0 {
		critBonus := combatSystem.CalculateCriticalChance(char)
		totalCrit := weaponDef.CritChance + critBonus
		lines = append(lines, fmt.Sprintf("Critical Chance: %d%% (Base: %d, Luck: +%d)", totalCrit, weaponDef.CritChance, critBonus))
	}

	// Add weapon category and rarity
	lines = append(lines, fmt.Sprintf("Type: %s (%s)", weaponDef.Category, weaponDef.Rarity))

	return lines
}

    // getArmorTooltip returns armor-specific tooltip information (YAML-driven)
func getArmorSummary(item items.Item, char *character.MMCharacter) string {
    // Calculate armor bonuses based on item attributes
    baseArmor := item.Attributes["armor_class_base"]
    enduranceDiv := item.Attributes["endurance_scaling_divisor"]
    if baseArmor == 0 && enduranceDiv == 0 {
        return "Provides basic protection"
    }
    if enduranceDiv > 0 {
        return fmt.Sprintf("Armor: %d base, +1 per %d Endurance (reduces damage by AC/2)", baseArmor, enduranceDiv)
    }
    return fmt.Sprintf("Armor: %d base (reduces damage by AC/2)", baseArmor)
}

    // getAccessoryTooltip returns accessory-specific tooltip information (YAML-driven)
func getAccessorySummary(item items.Item, char *character.MMCharacter) string {
    intDiv := item.Attributes["intellect_scaling_divisor"]
    perDiv := item.Attributes["personality_scaling_divisor"]
    if intDiv == 0 && perDiv == 0 {
        return "An accessory with minor benefits"
    }
    if intDiv > 0 && perDiv > 0 {
        return fmt.Sprintf("Spell Power +Intellect/%d, Spell Points +Personality/%d", intDiv, perDiv)
    }
    if intDiv > 0 {
        return fmt.Sprintf("Spell Power +Intellect/%d", intDiv)
    }
    return fmt.Sprintf("Spell Points +Personality/%d", perDiv)
}

// getSpellItemTooltip returns spell item-specific tooltip information
func getSpellItemTooltip(item items.Item, char *character.MMCharacter) []string {
	var lines []string

	lines = append(lines, fmt.Sprintf("School: %s", item.SpellSchool))
	lines = append(lines, fmt.Sprintf("Spell Points: %d", item.SpellCost))

	// Check if character can cast this spell
	if char.SpellPoints >= item.SpellCost {
		lines = append(lines, "✓ Can cast")
	} else {
		lines = append(lines, "✗ Insufficient spell points")
	}

	// Add spell effect description
	effectDesc := getSpellEffectDescription(item.SpellEffect)
	if effectDesc != "" {
		lines = append(lines, "")
		lines = append(lines, effectDesc)
	}

	return lines
}

// getConsumableTooltip returns consumable-specific tooltip information
func getConsumableSummary(item items.Item, char *character.MMCharacter) string {
    switch item.Name {
    case "Health Potion":
        base := item.Attributes["heal_base"]
        if base <= 0 { base = 25 }
        div := item.Attributes["heal_endurance_divisor"]
        if div <= 0 { div = 4 }
        return fmt.Sprintf("Restores %d + Endurance/%d HP on use", base, div)
    case "Dead Branch":
        dist := item.Attributes["summon_distance_tiles"]
        if dist <= 0 { dist = 2 }
        return fmt.Sprintf("Summons a random monster ~%d tiles away", dist)
    default:
        return "Single-use consumable"
    }
}

// getSpellEffectDescription returns a description for spell effects
func getSpellEffectDescription(effect items.SpellEffect) string {
	switch effect {
	case items.SpellEffectFireball:
		return "Launches a powerful fireball at enemies"
	case items.SpellEffectFireBolt:
		return "Quick fire attack with moderate damage"
	case items.SpellEffectTorchLight:
		return "Creates magical light to illuminate dark areas"
	case items.SpellEffectLightning:
		return "Strikes enemies with electrical damage"
	case items.SpellEffectIceShard:
		return "Hurls ice projectiles that may slow enemies"
	case items.SpellEffectHealSelf:
		return "Restores your own health"
	case items.SpellEffectHealOther:
		return "Heals a selected party member"
	case items.SpellEffectPartyBuff:
		return "Provides beneficial effects to the entire party"
	case items.SpellEffectShield:
		return "Creates magical protection against attacks"
	case items.SpellEffectBless:
		return "Blesses party with improved combat abilities"
	case items.SpellEffectWizardEye:
		return "Reveals hidden enemies and secrets"
	case items.SpellEffectAwaken:
		return "Cures sleep and paralysis effects"
	case items.SpellEffectWalkOnWater:
		return "Allows party to walk on water surfaces"
	default:
		return ""
	}
}

// joinTooltipLines joins tooltip lines with newlines
func joinTooltipLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// GetSpellTooltip returns a comprehensive tooltip for spells in the spellbook using centralized spell definitions
func GetSpellTooltip(spellID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem) string {
	var tooltip []string

	// Get centralized spell definition
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		tooltip = append(tooltip, fmt.Sprintf("=== Unknown Spell (%s) ===", spellID))
		return strings.Join(tooltip, "\n")
	}

	// Spell name and school
	tooltip = append(tooltip, fmt.Sprintf("=== %s ===", def.Name))
	tooltip = append(tooltip, fmt.Sprintf("%s Magic (Level %d)", def.School, def.Level))

	// Spell cost and availability
	tooltip = append(tooltip, "")
	tooltip = append(tooltip, fmt.Sprintf("Spell Points: %d", def.SpellPointsCost))

	if char.SpellPoints >= def.SpellPointsCost {
		tooltip = append(tooltip, "✓ Can cast")
	} else {
		needed := def.SpellPointsCost - char.SpellPoints
		tooltip = append(tooltip, fmt.Sprintf("✗ Need %d more SP", needed))
	}

	// Character's skill level in this school (convert string to MagicSchool)
	school := getSchoolFromString(def.School)
	if magicSkill, exists := char.MagicSchools[school]; exists {
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, fmt.Sprintf("Your %s Skill:", def.School))
		tooltip = append(tooltip, fmt.Sprintf("Level %d (%s)", magicSkill.Level, getMasteryString(magicSkill.Mastery)))

		// Calculate effectiveness based on skill level
		effectiveness := calculateSpellEffectivenessFromID(spellID, magicSkill)
		if effectiveness != 100 {
			tooltip = append(tooltip, fmt.Sprintf("Effectiveness: %d%%", effectiveness))
		}
	}

	// Description
	if def.Description != "" {
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, def.Description)
	}

	// Add spell-specific calculations using centralized definitions
	spellDetails := getSpellMechanicsFromDefinition(def, char, combatSystem)
	if len(spellDetails) > 0 {
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, "--- Spell Effects ---")
		tooltip = append(tooltip, spellDetails...)
	}

	return joinTooltipLines(tooltip)
}

// getMasteryString returns a readable string for mastery levels
func getMasteryString(mastery character.SkillMastery) string {
	switch mastery {
	case character.MasteryNovice:
		return "Novice"
	case character.MasteryExpert:
		return "Expert"
	case character.MasteryMaster:
		return "Master"
	case character.MasteryGrandMaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

// getSchoolFromString converts school string to MagicSchool enum
func getSchoolFromString(schoolStr string) character.MagicSchool {
	switch schoolStr {
	case "body":
		return character.MagicBody
	case "mind":
		return character.MagicMind
	case "spirit":
		return character.MagicSpirit
	case "fire":
		return character.MagicFire
	case "water":
		return character.MagicWater
	case "air":
		return character.MagicAir
	case "earth":
		return character.MagicEarth
	case "light":
		return character.MagicLight
	case "dark":
		return character.MagicDark
	default:
		return character.MagicBody // Default fallback
	}
}

// calculateSpellEffectivenessFromID calculates spell effectiveness using SpellID
func calculateSpellEffectivenessFromID(spellID spells.SpellID, skill *character.MagicSkill) int {
	// Base effectiveness depends on spell complexity
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 50 // Default effectiveness if spell not found
	}
	baseEffectiveness := 70 + (skill.Level * 5) // 75% at level 1, 100% at level 6

	// More complex spells (higher level) are harder to cast effectively
	complexity_penalty := (def.Level - 1) * 5
	effectiveness := baseEffectiveness - complexity_penalty

	// Mastery bonuses
	switch skill.Mastery {
	case character.MasteryExpert:
		effectiveness += 10
	case character.MasteryMaster:
		effectiveness += 20
	case character.MasteryGrandMaster:
		effectiveness += 30
	}

	// Cap at 100% for display purposes
	if effectiveness > 100 {
		effectiveness = 100
	}

	return effectiveness
}

// getSpellMechanicsFromDefinition returns detailed spell mechanics using centralized spell definitions
func getSpellMechanicsFromDefinition(def spells.SpellDefinition, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	var details []string

	// Check if this is a damage spell
	if def.School == "fire" || def.School == "air" || def.School == "water" || def.School == "earth" {
		// Use centralized damage calculation with effective stats (same as actual projectile system)
		_, effectiveIntellect, _, _, _, _, _ := char.GetEffectiveStats(combatSystem.game.statBonus)
		baseDamage, intellectBonus, totalDamage := spells.CalculateSpellDamageByID(def.ID, effectiveIntellect)

		details = append(details, fmt.Sprintf("Base Damage: %d", baseDamage))
		details = append(details, fmt.Sprintf("Intellect Bonus: +%d", intellectBonus))
		details = append(details, fmt.Sprintf("Total Damage: %d", totalDamage))
	}

	// Check if this is a healing spell
	if def.School == "body" || (def.Name == "First Aid" || def.Name == "Heal") {
		// Use the same centralized healing calculation as actual combat with effective stats
		_, _, effectivePersonality, _, _, _, _ := char.GetEffectiveStats(combatSystem.game.statBonus)
		baseHeal, personalityBonus, totalHeal := spells.CalculateHealingAmountByID(def.ID, effectivePersonality)
		details = append(details, fmt.Sprintf("Base Healing: %d", baseHeal))
		details = append(details, fmt.Sprintf("Personality Bonus: +%d", personalityBonus))
		details = append(details, fmt.Sprintf("Total Healing: %d", totalHeal))

		switch def.Name {
		case "First Aid":
			details = append(details, "Self-target only")
		case "Heal":
			details = append(details, "Can target any party member")
		}
	}

	// Check if this is a utility spell with duration
	if def.Duration > 0 {
		// Get character's skill level for this school
		school := getSchoolFromString(def.School)
		magicSkill, exists := char.MagicSchools[school]

		// Calculate actual duration with skill bonuses
		duration := def.Duration
		if exists {
			skillMultiplier := 1.0 + (float64(magicSkill.Level) * 0.1)
			duration = int(float64(def.Duration) * skillMultiplier)
		}

		// Display duration appropriately (seconds vs minutes)
		if duration >= 60 {
			details = append(details, fmt.Sprintf("Duration: %d minutes", duration/60))
		} else {
			details = append(details, fmt.Sprintf("Duration: %d seconds", duration))
		}

		// Add spell-specific descriptions
		switch def.Name {
		case "Torch Light":
			details = append(details, "Illuminates dark areas")
		case "Wizard Eye":
			details = append(details, "Reveals monster locations on radar")
		case "Walk on Water":
			details = append(details, "Allows party to walk on water")
		case "Bless":
			// Calculate bless bonuses
			if exists {
				accuracyBonus := 5 + magicSkill.Level
				damageBonus := 2 + (magicSkill.Level / 2)
				details = append(details, fmt.Sprintf("Accuracy Bonus: +%d", accuracyBonus))
				details = append(details, fmt.Sprintf("Damage Bonus: +%d", damageBonus))
			}
			details = append(details, "Affects entire party")
		}
	}

	// Generic information for unknown spells
	if len(details) == 0 {
		switch def.School {
		case "fire", "air", "water", "earth":
			details = append(details, "Offensive elemental spell")
		case "body", "mind", "spirit":
			details = append(details, "Self-magic spell")
			details = append(details, "Provides beneficial effects")
		}
	}

	return details
}
