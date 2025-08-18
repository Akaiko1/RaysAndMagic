package game

import (
	"fmt"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// GetItemTooltip returns a comprehensive tooltip string for any item type.
// It collects fields in a simple map and glues them together in a stable order
// to keep the function compact and easy to extend.
func GetItemTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
	fields := map[string]string{}
	order := []string{}

	// Header
	fields["header"] = fmt.Sprintf("=== %s ===", item.Name)
	fields["type"] = getItemTypeString(item.Type)
	order = append(order, "header", "type", "__sep__")

	switch item.Type {
	case items.ItemWeapon:
		// Core weapon mechanics
		base, bonus, total := combatSystem.CalculateWeaponDamage(item, char)
		fields["w_base"] = fmt.Sprintf("Base Damage: %d", base)
		fields["w_scaling"] = weaponScalingLine(item)
		fields["w_bonus"] = fmt.Sprintf("Stat Bonus: +%d", bonus)
		fields["w_total"] = fmt.Sprintf("Total Damage: %d", total)
		if item.Range > 0 {
			fields["w_range"] = fmt.Sprintf("Range: %d tiles", item.Range)
		}
		// From YAML weapon definition
		weaponKey := items.GetWeaponKeyByName(item.Name)
		if weaponDef, exists := config.GetWeaponDefinition(weaponKey); exists {
			baseHit := weaponDef.HitBonus
			acc := combatSystem.CalculateAccuracyBonus(char)
			fields["w_hit"] = fmt.Sprintf("Hit Chance: +%d%% (Base: +%d, Accuracy: +%d)", baseHit+acc, baseHit, acc)
			if weaponDef.CritChance > 0 {
				critBonus := combatSystem.CalculateCriticalChance(char)
				fields["w_crit"] = fmt.Sprintf("Critical Chance: %d%% (Base: %d, Luck: +%d)", weaponDef.CritChance+critBonus, weaponDef.CritChance, critBonus)
			}
			fields["w_type"] = fmt.Sprintf("Type: %s (%s)", weaponDef.Category, weaponDef.Rarity)
		}
		order = append(order, "w_base", "w_scaling", "w_bonus", "w_total", "w_range", "w_hit", "w_crit", "w_type", "__sep__")

	case items.ItemArmor:
		if line := getArmorSummary(item); line != "" {
			fields["a_line"] = line
		}
		order = append(order, "a_line", "__sep__")

	case items.ItemAccessory:
		if line := getAccessorySummary(item); line != "" {
			fields["acc_line"] = line
		}
		order = append(order, "acc_line", "__sep__")

	case items.ItemConsumable:
		if line := getConsumableSummary(item); line != "" {
			fields["c_line"] = line
		}
		order = append(order, "c_line", "__sep__")

	case items.ItemBattleSpell, items.ItemUtilitySpell:
		lines := getSpellItemTooltip(item, char)
		if len(lines) > 0 {
			fields["s_block"] = strings.Join(lines, "\n")
			order = append(order, "s_block", "__sep__")
		}

	case items.ItemQuest:
		fields["q_line"] = "Quest Item - Cannot be sold or dropped"
		order = append(order, "q_line", "__sep__")
	}

	// Value
	if val, ok := item.Attributes["value"]; ok && val > 0 {
		fields["value"] = fmt.Sprintf("Value: %d gold", val)
		order = append(order, "value")
	}

	// Flavor/description
	if item.Description != "" {
		order = append(order, "__sep__")
		fields["flavor"] = fmt.Sprintf("\"%s\"", item.Description)
		order = append(order, "flavor")
	}

	// Glue everything together following the order list
	var out []string
	for _, k := range order {
		if k == "__sep__" {
			// Add a blank line only if last emitted wasn't blank and we have content ahead
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if v := fields[k]; v != "" {
			out = append(out, v)
		}
	}
	return joinTooltipLines(out)
}

// weaponScalingLine builds the human-friendly scaling text for a weapon
func weaponScalingLine(item items.Item) string {
	primary := item.BonusStat
	if primary == "" {
		primary = "Might"
	}
	if sec := item.BonusStatSecondary; sec != "" {
		return fmt.Sprintf("Scales with %s + %s", primary, sec)
	}
	return fmt.Sprintf("Scales with %s", primary)
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

// getArmorTooltip returns armor-specific tooltip information (YAML-driven)
func getArmorSummary(item items.Item) string {
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
func getAccessorySummary(item items.Item) string {
	intDiv := item.Attributes["intellect_scaling_divisor"]
	perDiv := item.Attributes["personality_scaling_divisor"]
	mightFlat := item.Attributes["bonus_might"]
	luckFlat := item.Attributes["bonus_luck"]

	var parts []string
	if mightFlat > 0 {
		parts = append(parts, fmt.Sprintf("Might +%d", mightFlat))
	}
	if luckFlat > 0 {
		parts = append(parts, fmt.Sprintf("Luck +%d", luckFlat))
	}
	if intDiv > 0 {
		parts = append(parts, fmt.Sprintf("Spell Power +Intellect/%d", intDiv))
	}
	if perDiv > 0 {
		parts = append(parts, fmt.Sprintf("Spell Points +Personality/%d", perDiv))
	}
	if len(parts) == 0 {
		return "An accessory with minor benefits"
	}
	return strings.Join(parts, ", ")
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
func getConsumableSummary(item items.Item) string {
	// Attribute-driven summary: single source of truth with gameplay
	if item.Attributes["revive"] > 0 {
		if item.Attributes["full_heal"] > 0 {
			return "Revives from Dead/Unconscious and fully restores HP"
		}
		return "Revives from Dead/Unconscious"
	}
	if base, ok := item.Attributes["heal_base"]; ok {
		div := item.Attributes["heal_endurance_divisor"]
		if base > 0 && div > 0 {
			return fmt.Sprintf("Restores %d + Endurance/%d HP on use", base, div)
		}
		return "Heals (misconfigured)"
	}
	if dist, ok := item.Attributes["summon_distance_tiles"]; ok {
		if dist > 0 {
			return fmt.Sprintf("Summons a random monster ~%d tiles away", dist)
		}
		return "Summons (misconfigured)"
	}
	return "Single-use consumable"
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
