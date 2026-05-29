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
	if item.Type == items.ItemBattleSpell || item.Type == items.ItemUtilitySpell {
		return buildSpellItemTooltipFromDefinition(item, char, combatSystem)
	}

	fields := map[string]string{}
	order := []string{}

	// Header
	fields["header"] = fmt.Sprintf("=== %s ===", item.Name)
	fields["type"] = item.Type.String()
	order = append(order, "header", "type", "__sep__")

	switch item.Type {
	case items.ItemWeapon:
		// All weapon stats come from weapons.yaml via config — items.Item only
		// carries the name. Look up once and use the def throughout.
		weaponKey := items.GetWeaponKeyByName(item.Name)
		weaponDef, _ := config.GetWeaponDefinition(weaponKey)

		// Core weapon mechanics
		base, bonus, total := combatSystem.CalculateWeaponDamage(item, char)
		fields["w_base"] = fmt.Sprintf("Base Damage: %d", base)
		fields["w_scaling"] = weaponScalingLine(item, char, combatSystem)
		fields["w_bonus"] = fmt.Sprintf("Stat Bonus: +%d", bonus)
		fields["w_total"] = fmt.Sprintf("Total Damage: %d", total)
		if weaponDef != nil && weaponDef.Range > 0 {
			fields["w_range"] = fmt.Sprintf("Range: %d tiles", weaponDef.Range)
		}
		if cooldown := formatCooldownLine(char, combatSystem); cooldown != "" {
			fields["w_cd"] = cooldown
		}
		effectKeys := []string{}
		if weaponDef != nil {
			totalCrit := combatSystem.CalculateWeaponCritChance(item, char)
			if totalCrit > 0 {
				critBonus := combatSystem.CalculateCriticalChance(char)
				fields["w_crit"] = fmt.Sprintf("Critical Chance: %d%% (Base: %d, Luck: +%d)", totalCrit, weaponDef.CritChance, critBonus)
			}
			for i, line := range weaponEffectLines(weaponDef) {
				key := fmt.Sprintf("w_eff_%d", i)
				fields[key] = line
				effectKeys = append(effectKeys, key)
			}
			fields["w_type"] = fmt.Sprintf("Type: %s (%s)", weaponDef.Category, weaponDef.Rarity)
		}
		order = append(order, "w_base", "w_scaling", "w_bonus", "w_total", "w_range", "w_cd", "w_crit")
		order = append(order, effectKeys...)
		order = append(order, "w_type", "__sep__")

	case items.ItemArmor:
		if line := getArmorCategoryLine(item); line != "" {
			fields["a_type"] = line
		}
		if line := getArmorRequirementLine(item, char); line != "" {
			fields["a_req"] = line
		}
		if line := getArmorSummary(item); line != "" {
			fields["a_line"] = line
		}
		order = append(order, "a_type", "a_req", "a_line", "__sep__")

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

	case items.ItemQuest:
		fields["q_line"] = "Quest Item - Cannot be sold or dropped"
		order = append(order, "q_line", "__sep__")

	case items.ItemTrinket:
		fields["t_line"] = "Trinket - Collectible; sell to merchants"
		order = append(order, "t_line", "__sep__")
	}

	// Value
	if val, ok := item.Attributes["value"]; ok && val > 0 {
		fields["value"] = fmt.Sprintf("Value: %d gold", val)
		order = append(order, "value")
	}

	// Flavor/description. BonusVs is intentionally NOT repeated here — it is
	// already surfaced via EffectLines in the effects section above.
	var flavorLines []string
	if item.Description != "" {
		flavorLines = append(flavorLines, fmt.Sprintf("\"%s\"", item.Description))
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
	if len(flavorLines) > 0 {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, flavorLines...)
	}
	return joinTooltipLines(out)
}

// GetItemComparisonTooltip returns a comparison block against the currently equipped item
// for the same slot/type (weapons, spells, armor). Returns empty string if no comparison applies.
func GetItemComparisonTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
	if char == nil || combatSystem == nil || combatSystem.game == nil {
		return ""
	}

	slot, ok := getEquipSlotForItem(item)
	if !ok {
		return ""
	}
	equipped, hasEquipped := char.Equipment[slot]
	if !hasEquipped {
		return ""
	}

	switch item.Type {
	case items.ItemWeapon:
		if equipped.Type != items.ItemWeapon {
			return ""
		}
		if item.Name == equipped.Name {
			return ""
		}
		return joinTooltipLines(buildWeaponComparisonLines(item, equipped, char, combatSystem))
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		if equipped.Type != items.ItemBattleSpell && equipped.Type != items.ItemUtilitySpell {
			return ""
		}
		itemID := spells.SpellID(item.SpellEffect)
		equippedID := spells.SpellID(equipped.SpellEffect)
		if itemID == "" || equippedID == "" || itemID == equippedID {
			return ""
		}
		if def, err := spells.GetSpellDefinitionByID(itemID); err == nil && def.IsUtility {
			return ""
		}
		if def, err := spells.GetSpellDefinitionByID(equippedID); err == nil && def.IsUtility {
			return ""
		}
		return joinTooltipLines(buildSpellComparisonLines(item, equipped, char, combatSystem))
	case items.ItemArmor:
		if equipped.Type != items.ItemArmor {
			return ""
		}
		if item.Name == equipped.Name {
			return ""
		}
		return joinTooltipLines(buildArmorComparisonLines(item, equipped, char, combatSystem))
	default:
		return ""
	}
}

// GetSpellComparisonTooltip returns a comparison block for a spellbook spell against the equipped spell.
func GetSpellComparisonTooltip(spellID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem) string {
	if char == nil || combatSystem == nil || combatSystem.game == nil {
		return ""
	}
	equipped, hasEquipped := char.Equipment[items.SlotSpell]
	if !hasEquipped {
		return ""
	}
	if equipped.Type != items.ItemBattleSpell && equipped.Type != items.ItemUtilitySpell {
		return ""
	}
	equippedID := spells.SpellID(equipped.SpellEffect)
	if equippedID == "" {
		return ""
	}
	if spellID == equippedID {
		return ""
	}
	if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.IsUtility {
		return ""
	}
	if def, err := spells.GetSpellDefinitionByID(equippedID); err == nil && def.IsUtility {
		return ""
	}
	return joinTooltipLines(buildSpellComparisonLinesByID(spellID, equippedID, char, combatSystem))
}

func buildSpellItemTooltipFromDefinition(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
	if char == nil || combatSystem == nil {
		return ""
	}

	spellID := spells.SpellID(item.SpellEffect)
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		lines := []string{
			fmt.Sprintf("=== %s ===", item.Name),
			"Unknown Spell",
		}
		if item.SpellSchool != "" {
			lines = append(lines, fmt.Sprintf("%s Magic", formatSchoolName(item.SpellSchool)))
		}
		if item.SpellCost > 0 {
			lines = append(lines, fmt.Sprintf("Spell Points: %d", item.SpellCost))
		}
		if item.Description != "" {
			lines = append(lines, "", fmt.Sprintf("\"%s\"", item.Description))
		}
		return joinTooltipLines(lines)
	}

	tooltip := GetSpellTooltip(spellID, char, combatSystem)
	lines := strings.Split(tooltip, "\n")

	if val, ok := item.Attributes["value"]; ok && val > 0 {
		lines = append(lines, "", fmt.Sprintf("Value: %d gold", val))
	}

	if item.Description != "" && item.Description != def.Description {
		lines = append(lines, "", fmt.Sprintf("\"%s\"", item.Description))
	}

	return joinTooltipLines(lines)
}

// weaponScalingLine builds the human-friendly scaling text for a weapon.
// The /N divisors come from balance.go so the displayed weights stay in
// lockstep with the actual damage formula in CalculateMeleeDamage.
func weaponScalingLine(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
	primary := "Might"
	secondary := ""
	if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok && def != nil {
		if def.BonusStat != "" {
			primary = def.BonusStat
		}
		secondary = def.BonusStatSecondary
	}
	primaryValue := getEffectiveStatValue(primary, char, combatSystem)
	if secondary != "" {
		secondaryValue := getEffectiveStatValue(secondary, char, combatSystem)
		return fmt.Sprintf("Scales with %s/%d (Effective: %d) + %s/%d (Effective: %d)",
			primary, WeaponPrimaryStatDivisor, primaryValue,
			secondary, WeaponSecondaryStatDivisor, secondaryValue)
	}
	return fmt.Sprintf("Scales with %s/%d (Effective: %d)",
		primary, WeaponPrimaryStatDivisor, primaryValue)
}

func getEffectiveStatValue(statName string, char *character.MMCharacter, combatSystem *CombatSystem) int {
	might, intellect, personality, endurance, accuracy, speed, luck := char.GetEffectiveStats(combatSystem.game.statBonus)
	switch statName {
	case "Might":
		return might
	case "Intellect":
		return intellect
	case "Personality":
		return personality
	case "Endurance":
		return endurance
	case "Accuracy":
		return accuracy
	case "Speed":
		return speed
	case "Luck":
		return luck
	default:
		return might
	}
}

// getArmorTooltip returns armor-specific tooltip information (YAML-driven)
func getArmorSummary(item items.Item) string {
	// Calculate armor bonuses based on item attributes
	baseArmor := item.Attributes["armor_class_base"]
	enduranceDiv := item.Attributes["endurance_scaling_divisor"]
	bonusParts := armorBonusParts(item)
	if baseArmor == 0 && enduranceDiv == 0 {
		if len(bonusParts) > 0 {
			return fmt.Sprintf("Bonuses: %s", strings.Join(bonusParts, ", "))
		}
		return "Provides basic protection"
	}
	var armorLine string
	if enduranceDiv > 0 {
		armorLine = fmt.Sprintf("Armor: %d base, +1 per %d Endurance (reduces physical damage by AC/%d)", baseArmor, enduranceDiv, ArmorPhysicalReductionDivisor)
	} else {
		armorLine = fmt.Sprintf("Armor: %d base (reduces physical damage by AC/%d)", baseArmor, ArmorPhysicalReductionDivisor)
	}
	if len(bonusParts) > 0 {
		return fmt.Sprintf("%s\nBonuses: %s", armorLine, strings.Join(bonusParts, ", "))
	}
	return armorLine
}

func getArmorCategoryLine(item items.Item) string {
	category := armorCategoryString(item)
	if category == "" {
		return ""
	}
	label := strings.ToUpper(category[:1]) + category[1:]
	return fmt.Sprintf("Armor Type: %s", label)
}

func getArmorRequirementLine(item items.Item, char *character.MMCharacter) string {
	category := strings.ToLower(item.ArmorCategory)
	if category == "cloth" {
		return "Requires: None"
	}
	skillName, hasReq := armorRequiredSkillName(item)
	if !hasReq {
		return ""
	}
	if char == nil {
		return fmt.Sprintf("Requires: %s Skill", skillName)
	}
	hasSkill := false
	switch strings.ToLower(skillName) {
	case "leather":
		_, hasSkill = char.Skills[character.SkillLeather]
	case "chain":
		_, hasSkill = char.Skills[character.SkillChain]
	case "plate":
		_, hasSkill = char.Skills[character.SkillPlate]
	case "shield":
		_, hasSkill = char.Skills[character.SkillShield]
	}
	if hasSkill {
		return fmt.Sprintf("Requires: %s Skill", skillName)
	}
	return fmt.Sprintf("Requires: %s Skill (Missing)", skillName)
}

func armorCategoryString(item items.Item) string {
	category := strings.ToLower(item.ArmorCategory)
	switch category {
	case "leather":
		return "leather"
	case "chain":
		return "chain"
	case "plate":
		return "plate"
	case "shield":
		return "shield"
	case "cloth":
		return "cloth"
	default:
		return ""
	}
}

func armorRequiredSkillName(item items.Item) (string, bool) {
	category := armorCategoryString(item)
	if category == "" {
		return "", false
	}
	return category, true
}

// getAccessoryTooltip returns accessory-specific tooltip information (YAML-driven)
func getAccessorySummary(item items.Item) string {
	intDiv := item.Attributes["intellect_scaling_divisor"]
	perDiv := item.Attributes["personality_scaling_divisor"]

	parts := armorBonusParts(item)
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

func armorBonusParts(item items.Item) []string {
	var parts []string
	if v := item.Attributes["bonus_might"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Might +%d", v))
	}
	if v := item.Attributes["bonus_intellect"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Intellect +%d", v))
	}
	if v := item.Attributes["bonus_personality"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Personality +%d", v))
	}
	if v := item.Attributes["bonus_endurance"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Endurance +%d", v))
	}
	if v := item.Attributes["bonus_accuracy"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Accuracy +%d", v))
	}
	if v := item.Attributes["bonus_speed"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Speed +%d", v))
	}
	if v := item.Attributes["bonus_luck"]; v > 0 {
		parts = append(parts, fmt.Sprintf("Luck +%d", v))
	}
	return parts
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

func getEquipSlotForItem(item items.Item) (items.EquipSlot, bool) {
	switch item.Type {
	case items.ItemWeapon:
		return items.SlotMainHand, true
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		return items.SlotSpell, true
	case items.ItemArmor:
		if slotCode, ok := item.Attributes["equip_slot"]; ok {
			return items.EquipSlot(slotCode), true
		}
		return items.SlotArmor, true
	case items.ItemAccessory:
		if slotCode, ok := item.Attributes["equip_slot"]; ok {
			return items.EquipSlot(slotCode), true
		}
		return items.SlotRing1, true
	default:
		return 0, false
	}
}

func buildWeaponComparisonLines(item, equipped items.Item, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	lines := []string{
		fmt.Sprintf("--- Equipped: %s ---", equipped.Name),
	}

	_, _, total := combatSystem.CalculateWeaponDamage(item, char)
	_, _, eqTotal := combatSystem.CalculateWeaponDamage(equipped, char)
	lines = append(lines, fmt.Sprintf("Total Damage: %d vs %d (%+d)", total, eqTotal, total-eqTotal))

	itemRange, eqRange := 0, 0
	if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok && def != nil {
		itemRange = def.Range
	}
	if def, _, ok := config.GetWeaponDefinitionByName(equipped.Name); ok && def != nil {
		eqRange = def.Range
	}
	if itemRange > 0 || eqRange > 0 {
		lines = append(lines, fmt.Sprintf("Range: %d vs %d (%+d) tiles", itemRange, eqRange, itemRange-eqRange))
	}

	itemCrit := combatSystem.CalculateWeaponCritChance(item, char)
	eqCrit := combatSystem.CalculateWeaponCritChance(equipped, char)
	if itemCrit > 0 || eqCrit > 0 {
		lines = append(lines, fmt.Sprintf("Critical Chance: %d%% vs %d%% (%+d%%)", itemCrit, eqCrit, itemCrit-eqCrit))
	}

	itemEffects := weaponEffectsSummary(item)
	eqEffects := weaponEffectsSummary(equipped)
	if itemEffects != "" || eqEffects != "" {
		lines = append(lines, fmt.Sprintf("Effects: %s vs %s", effectOrNone(itemEffects), effectOrNone(eqEffects)))
	}

	return lines
}

func buildSpellComparisonLines(item, equipped items.Item, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	itemID := spells.SpellID(item.SpellEffect)
	equippedID := spells.SpellID(equipped.SpellEffect)
	if itemID == "" || equippedID == "" {
		return nil
	}
	return buildSpellComparisonLinesByID(itemID, equippedID, char, combatSystem)
}

func buildSpellComparisonLinesByID(itemID, equippedID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	itemDef, err := spells.GetSpellDefinitionByID(itemID)
	if err != nil {
		return nil
	}
	equippedDef, err := spells.GetSpellDefinitionByID(equippedID)
	if err != nil {
		return nil
	}

	lines := []string{
		fmt.Sprintf("--- Equipped: %s ---", equippedDef.Name),
		fmt.Sprintf("Spell Points: %d vs %d (%+d)", itemDef.SpellPointsCost, equippedDef.SpellPointsCost, itemDef.SpellPointsCost-equippedDef.SpellPointsCost),
	}

	if itemDef.IsProjectile || equippedDef.IsProjectile {
		if rng, ok := combatSystem.CalculateSpellRangeTiles(itemDef.ID); ok {
			if eqRng, eqOK := combatSystem.CalculateSpellRangeTiles(equippedDef.ID); eqOK {
				lines = append(lines, fmt.Sprintf("Range: %.1f vs %.1f (%+.1f) tiles", rng, eqRng, rng-eqRng))
			}
		}
		_, _, itemDmg := combatSystem.CalculateSpellDamage(itemDef.ID, char)
		_, _, eqDmg := combatSystem.CalculateSpellDamage(equippedDef.ID, char)
		if itemDmg > 0 || eqDmg > 0 {
			lines = append(lines, fmt.Sprintf("Total Damage: %d vs %d (%+d)", itemDmg, eqDmg, itemDmg-eqDmg))
		}
	}

	if itemDef.HealAmount > 0 || equippedDef.HealAmount > 0 {
		_, _, itemHeal := combatSystem.CalculateSpellHealing(itemDef.ID, char)
		_, _, eqHeal := combatSystem.CalculateSpellHealing(equippedDef.ID, char)
		if itemHeal > 0 || eqHeal > 0 {
			lines = append(lines, fmt.Sprintf("Total Healing: %d vs %d (%+d)", itemHeal, eqHeal, itemHeal-eqHeal))
		}
	}

	if itemDef.IsUtility || equippedDef.IsUtility {
		if itemDef.Duration > 0 || equippedDef.Duration > 0 {
			itemDur := combatSystem.CalculateSpellDurationSeconds(itemDef.ID, char)
			eqDur := combatSystem.CalculateSpellDurationSeconds(equippedDef.ID, char)
			lines = append(lines, fmt.Sprintf("Duration: %ds vs %ds (%+ds)", itemDur, eqDur, itemDur-eqDur))
		}
	}

	itemEffects := spellEffectsSummary(itemDef)
	eqEffects := spellEffectsSummary(equippedDef)
	if itemEffects != "" || eqEffects != "" {
		lines = append(lines, fmt.Sprintf("Effects: %s vs %s", effectOrNone(itemEffects), effectOrNone(eqEffects)))
	}

	return lines
}

func buildArmorComparisonLines(item, equipped items.Item, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	lines := []string{
		fmt.Sprintf("--- Equipped: %s ---", equipped.Name),
	}

	itemAC := combatSystem.CalculateArmorClassContribution(item, char)
	eqAC := combatSystem.CalculateArmorClassContribution(equipped, char)
	lines = append(lines, fmt.Sprintf("Armor Class: %d vs %d (%+d)", itemAC, eqAC, itemAC-eqAC))

	itemEffects := armorEffectsSummary(item)
	eqEffects := armorEffectsSummary(equipped)
	if itemEffects != "" || eqEffects != "" {
		lines = append(lines, fmt.Sprintf("Effects: %s vs %s", effectOrNone(itemEffects), effectOrNone(eqEffects)))
	}
	return lines
}

func effectOrNone(s string) string {
	if s == "" {
		return "None"
	}
	return s
}

// weaponEffectLines delegates to the canonical formatter on the config
// type so the in-game tooltip, compare-tooltip and map-viewer card stay
// in sync. Add new special-effect rows in config.WeaponDefinitionConfig
// EffectLines and every consumer picks them up automatically.
func weaponEffectLines(def *config.WeaponDefinitionConfig) []string {
	return def.EffectLines()
}

func weaponEffectsSummary(item items.Item) string {
	def, _, ok := config.GetWeaponDefinitionByName(item.Name)
	if !ok || def == nil {
		return ""
	}
	// EffectLines already includes BonusVs entries.
	return strings.Join(weaponEffectLines(def), ", ")
}

func armorEffectsSummary(item items.Item) string {
	parts := armorBonusParts(item)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func spellEffectsSummary(def spells.SpellDefinition) string {
	var parts []string
	if def.DisintegrateChance > 0 {
		parts = append(parts, fmt.Sprintf("Disintegrate: %.0f%%", def.DisintegrateChance*100))
	}
	return strings.Join(parts, ", ")
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
	tooltip = append(tooltip, fmt.Sprintf("%s Magic (Level %d)", formatSchoolName(def.School), def.Level))

	// Spell cost and availability
	tooltip = append(tooltip, "")
	tooltip = append(tooltip, fmt.Sprintf("Spell Points: %d", def.SpellPointsCost))
	if cooldown := formatCooldownLine(char, combatSystem); cooldown != "" {
		tooltip = append(tooltip, cooldown)
	}
	if def.IsProjectile {
		if rangeLine := spellRangeLine(def.ID, combatSystem); rangeLine != "" {
			tooltip = append(tooltip, rangeLine)
		}
		if combatSystem != nil && combatSystem.game != nil && char != nil {
			critBonus := combatSystem.CalculateCriticalChance(char)
			totalCrit := critBonus // Spells have no base crit chance
			if totalCrit < 0 {
				totalCrit = 0
			}
			if totalCrit > 100 {
				totalCrit = 100
			}
			tooltip = append(tooltip, fmt.Sprintf("Critical Chance: %d%% (Luck: +%d)", totalCrit, critBonus))
		}
	}

	if char.SpellPoints >= def.SpellPointsCost {
		tooltip = append(tooltip, "✓ Can cast")
	} else {
		needed := def.SpellPointsCost - char.SpellPoints
		tooltip = append(tooltip, fmt.Sprintf("✗ Need %d more SP", needed))
	}

	// Character's skill level in this school
	school := character.MagicSchoolID(def.School)
	if magicSkill, exists := char.MagicSchools[school]; exists {
		tooltip = append(tooltip, "")
		tooltip = append(tooltip, fmt.Sprintf("Your %s Skill:", formatSchoolName(def.School)))
		tooltip = append(tooltip, fmt.Sprintf("Level %d (%s)", magicSkill.Level(), magicSkill.Mastery))
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

func formatCooldownLine(char *character.MMCharacter, combatSystem *CombatSystem) string {
	if combatSystem == nil || combatSystem.game == nil || char == nil {
		return ""
	}
	cooldownFrames := combatSystem.CalculateActionCooldownFrames(char)
	if cooldownFrames <= 0 {
		return ""
	}
	tps := combatSystem.game.config.GetTPS()
	if tps <= 0 {
		tps = 60
	}
	seconds := float64(cooldownFrames) / float64(tps)
	return fmt.Sprintf("Cooldown: %d frames (%.1fs)", cooldownFrames, seconds)
}

func spellRangeLine(spellID spells.SpellID, combatSystem *CombatSystem) string {
	if combatSystem == nil {
		return ""
	}
	if rng, ok := combatSystem.CalculateSpellRangeTiles(spellID); ok {
		return fmt.Sprintf("Range: %.1f tiles", rng)
	}
	return ""
}

func formatSchoolName(school string) string {
	if school == "" {
		return ""
	}
	return strings.ToUpper(school[:1]) + school[1:]
}

// getSpellMechanicsFromDefinition returns detailed spell mechanics using centralized spell definitions
func getSpellMechanicsFromDefinition(def spells.SpellDefinition, char *character.MMCharacter, combatSystem *CombatSystem) []string {
	var details []string

	// Check if this is a damage spell
	if def.IsProjectile {
		if combatSystem != nil {
			baseDamage, intellectBonus, totalDamage := combatSystem.CalculateSpellDamage(def.ID, char)
			details = append(details, fmt.Sprintf("Base Damage: %d", baseDamage))
			details = append(details, fmt.Sprintf("Intellect Bonus: +%d", intellectBonus))
			details = append(details, fmt.Sprintf("Total Damage: %d", totalDamage))
		}
		if def.AoeRadiusTiles > 0 {
			details = append(details, fmt.Sprintf("AoE radius: %.1f tiles (splashes all nearby monsters)", def.AoeRadiusTiles))
		}
	}

	// Check if this is a healing spell
	if def.HealAmount > 0 {
		if combatSystem != nil {
			baseHeal, personalityBonus, totalHeal := combatSystem.CalculateSpellHealing(def.ID, char)
			details = append(details, fmt.Sprintf("Base Healing: %d", baseHeal))
			details = append(details, fmt.Sprintf("Personality Bonus: +%d", personalityBonus))
			details = append(details, fmt.Sprintf("Total Healing: %d", totalHeal))
		}

		if def.TargetSelf {
			details = append(details, "Self-target only")
		} else {
			details = append(details, "Can target any party member")
		}
	}

	// Check if this is a utility spell
	if def.IsUtility {
		if combatSystem != nil {
			duration := combatSystem.CalculateSpellDurationSeconds(def.ID, char)

			// Display duration appropriately (seconds vs minutes)
			if duration >= 60 {
				details = append(details, fmt.Sprintf("Duration: %d minutes", duration/60))
			} else {
				details = append(details, fmt.Sprintf("Duration: %d seconds", duration))
			}
		}

		if def.StatBonus > 0 && combatSystem != nil {
			if statBonus := combatSystem.CalculateSpellStatBonus(def.ID, char); statBonus > 0 {
				details = append(details, fmt.Sprintf("Stat Bonus: +%d to all stats", statBonus))
				details = append(details, "Affects entire party")
			}
		}
		if def.WaterWalk {
			details = append(details, "Allows party to walk on water")
		}
		if def.WaterBreathing {
			details = append(details, "Allows underwater travel via deep water")
		}
		if def.Awaken {
			details = append(details, "No current gameplay effect")
		}
	}

	// Generic information for unknown spells
	if len(details) == 0 && !def.IsUtility {
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
