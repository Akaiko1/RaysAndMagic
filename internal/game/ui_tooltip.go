package game

import (
	"fmt"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// GetItemTooltip returns a comprehensive tooltip string for any item type.
// It collects fields in a simple map and glues them together in a stable order
// to keep the function compact and easy to extend.
// tooltipDetailHeld reports whether the player is holding Shift to expand a
// tooltip to its full Base→Stat→Mastery breakdown + universal RULES.
func tooltipDetailHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

func GetItemTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	if item.Type == items.ItemBattleSpell || item.Type == items.ItemUtilitySpell {
		return buildSpellItemTooltipFromDefinition(item, char, combatSystem, full)
	}

	// Every category renders through the unified template (=== Name ===,
	// Category · Rarity, sections, RULES). Compact by default; the UI passes
	// full=true (Shift held) to reveal the Base→Stat→Mastery decomposition +
	// universal RULES (keeps tall cards on screen). Pure formatter — no input read.
	var core string
	switch item.Type {
	case items.ItemTrap:
		if def, ok := config.GetTrapDefinition(string(item.SpellEffect)); ok {
			core = buildTrapTooltipUnified(string(item.SpellEffect), def, char, combatSystem, full)
		}
	case items.ItemWeapon:
		core = buildWeaponTooltipUnified(item, char, combatSystem, full)
	case items.ItemArmor, items.ItemAccessory:
		core = buildArmorTooltipUnified(item, char, combatSystem, full)
	case items.ItemConsumable:
		core = buildSimpleItemTooltipUnified(item, "EFFECT", []string{"Double-click to use", "Single use"}, full)
	case items.ItemQuest:
		// Only ACTIVATABLE quest items get a usage hint — plain story tokens
		// (statuettes etc.) just sit in the inventory.
		usage := []string{"Cannot be sold or dropped"}
		if def, _, ok := config.GetItemDefinitionByName(item.Name); ok && def != nil && (def.OpensMap || def.PromotesLich) {
			usage = append([]string{"Double-click to use"}, usage...)
		}
		core = buildSimpleItemTooltipUnified(item, "EFFECT", usage, full)
	case items.ItemTrinket:
		core = buildSimpleItemTooltipUnified(item, "EFFECT", []string{"Collectible; sell to merchants"}, full)
	}
	if core == "" {
		core = fmt.Sprintf("=== %s ===\n%s", item.Name, itemKindLabel(item))
	}

	var tail []string
	if val, ok := item.Attributes["value"]; ok && val > 0 {
		tail = append(tail, fmt.Sprintf("Value: %d gold", val))
	}
	if item.Description != "" {
		tail = append(tail, fmt.Sprintf("\"%s\"", item.Description))
	}
	if len(tail) > 0 {
		core += "\n\n" + strings.Join(tail, "\n")
	}
	return core
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

func buildSpellItemTooltipFromDefinition(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
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

	tooltip := GetSpellTooltip(spellID, char, combatSystem, full)
	lines := strings.Split(tooltip, "\n")

	if val, ok := item.Attributes["value"]; ok && val > 0 {
		lines = append(lines, "", fmt.Sprintf("Value: %d gold", val))
	}

	if item.Description != "" && item.Description != def.Description {
		lines = append(lines, "", fmt.Sprintf("\"%s\"", item.Description))
	}

	return joinTooltipLines(lines)
}

func getEffectiveStatValue(statName string, char *character.MMCharacter) int {
	might, intellect, personality, endurance, accuracy, speed, luck := char.GetEffectiveStats()
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

func getArmorRequirementLine(item items.Item, char *character.MMCharacter) string {
	category := strings.ToLower(item.ArmorCategory)
	if category == "cloth" {
		return "Requires: None"
	}
	skillName, hasReq := armorRequiredSkillName(item)
	if !hasReq {
		return ""
	}
	display := strings.Title(skillName)
	if char == nil {
		return fmt.Sprintf("Requires: %s Skill", display)
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
		return fmt.Sprintf("Requires: %s Skill", display)
	}
	return fmt.Sprintf("Requires: %s Skill (Missing)", display)
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

// getConsumableTooltip returns consumable-specific tooltip information

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

// armorBonusParts lists flat stat bonuses + resistances via the config-level
// formatter (ItemDefinitionConfig.StatBonusLines/ResistLines) — one source
// with the map editor; a hand-rolled attribute walk here drifted twice.
func armorBonusParts(item items.Item) []string {
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	if !ok || def == nil {
		return nil
	}
	var parts []string
	for _, ln := range def.StatBonusLines() {
		// Divisor lines are accessory-summary territory (getAccessorySummary).
		if strings.Contains(ln, "+base/") {
			continue
		}
		parts = append(parts, ln)
	}
	return append(parts, def.ResistLines()...)
}

// itemKindLabel names the item for the player: wearable pieces are labeled by
// their SLOT (Belt / Amulet / Cloak / Ring …) instead of the internal type —
// "Accessory" told you nothing about where it goes.
func itemKindLabel(item items.Item) string {
	if item.Type == items.ItemArmor || item.Type == items.ItemAccessory {
		if slotCode, ok := item.Attributes["equip_slot"]; ok {
			return items.EquipSlot(slotCode).DisplayName()
		}
		if item.Type == items.ItemAccessory {
			return items.SlotRing1.DisplayName() // accessories default to the ring slot
		}
	}
	return item.Type.String()
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
		// Two lines, not one "X vs Y": the effect summaries are verbose, and a single
		// combined line ran 200+ chars wide (it spanned the screen and buried the card).
		lines = append(lines, fmt.Sprintf("Effects (this): %s", effectOrNone(itemEffects)))
		lines = append(lines, fmt.Sprintf("Effects (equipped): %s", effectOrNone(eqEffects)))
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

	// Costs as actually paid (Meditation GM discount), matching the main tooltip.
	itemCost, eqCost := itemDef.SpellPointsCost, equippedDef.SpellPointsCost
	if combatSystem != nil {
		itemCost = combatSystem.effectiveSpellCost(char, itemCost)
		eqCost = combatSystem.effectiveSpellCost(char, eqCost)
	}
	lines := []string{
		fmt.Sprintf("--- Equipped: %s ---", equippedDef.Name),
		fmt.Sprintf("Spell Points: %d vs %d (%+d)", itemCost, eqCost, itemCost-eqCost),
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
	// Config-computable lines + the game-side combat traits (attack speed,
	// ranged armor pierce) from the shared character helper.
	return append(def.EffectLines(), character.WeaponCombatLines(def)...)
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

// spellEffectsSummary compresses the spell's mechanics into one comparison
// cell — the SAME EffectLines the tooltip and editor print (a hand-picked
// field list here once showed only Disintegrate and lost AoE/stun/buffs).
func spellEffectsSummary(def spells.SpellDefinition) string {
	return strings.Join(def.EffectLines(), "; ")
}

// GetSpellTooltip returns a comprehensive tooltip for spells in the spellbook using centralized spell definitions
func GetSpellTooltip(spellID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return fmt.Sprintf("=== Unknown Spell (%s) ===", spellID)
	}
	out := buildSpellTooltipUnified(def, char, combatSystem, full)
	if def.Description != "" {
		out += "\n\n\"" + def.Description + "\""
	}
	return out
}

func formatSchoolName(school string) string {
	if school == "" {
		return ""
	}
	return strings.ToUpper(school[:1]) + school[1:]
}

// spellGMPierceLine renders the Grandmaster resist-pierce note when the caster
// is GM in the spell's school — shared by the projectile and zone sections
// (the two paths that actually apply spellResistPierce).
func spellGMPierceLine(def spells.SpellDefinition, char *character.MMCharacter) string {
	if char == nil || def.School == "" {
		return ""
	}
	school := char.MagicSchools[character.MagicSchoolID(def.School)]
	if school == nil || school.Mastery < character.MasteryGrandMaster {
		return ""
	}
	return fmt.Sprintf("Grandmaster: ignores %d%% of enemy resistance", MagicGMResistPiercePct)
}
