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
// tooltip to its full Base->Stat->Mastery breakdown + universal RULES.
func tooltipDetailHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

func GetItemTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	// A bag/shop item's own card uses the same post-equip context as the
	// comparison. Equipped items retain their actual slot (especially rings).
	if char != nil && combatSystem != nil && combatSystem.game != nil &&
		(item.Type == items.ItemWeapon || item.Type == items.ItemArmor || item.Type == items.ItemAccessory) {
		alreadyEquipped := false
		for _, equipped := range char.Equipment {
			if item.InstanceID != 0 && equipped.InstanceID == item.InstanceID {
				alreadyEquipped = true
				break
			}
		}
		if !alreadyEquipped {
			if slot, ok := char.EquipDestination(item); ok {
				char, combatSystem = previewEquippedItem(combatSystem, char, item, slot)
			}
		}
	}
	if item.Type == items.ItemBattleSpell || item.Type == items.ItemUtilitySpell {
		return buildSpellItemTooltipFromDefinition(item, char, combatSystem, full)
	}

	// Every category renders through the unified template (=== Name ===,
	// Category - Rarity, sections, RULES). Compact by default; the UI passes
	// full=true (Shift held) to reveal the Base->Stat->Mastery decomposition +
	// universal RULES (keeps tall cards on screen). Pure formatter - no input read.
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
	case items.ItemConsumable, items.ItemQuest, items.ItemTrinket, items.ItemCard:
		core = buildSimpleItemTooltipUnified(item, full, char)
	}
	if core == "" {
		core = fmt.Sprintf("%s\n%s", item.Name, itemKindLabel(item))
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
// for the default equip destination, including empty slots and mixed wearable types.
func GetItemComparisonTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem) string {
	if char == nil || combatSystem == nil || combatSystem.game == nil {
		return ""
	}

	slot, ok := char.EquipDestination(item)
	if !ok {
		return ""
	}
	equipped, hasEquipped := char.Equipment[slot]
	if item.Type == items.ItemWeapon || item.Type == items.ItemArmor || item.Type == items.ItemAccessory {
		return joinTooltipLines(buildEquipmentComparisonLines(item, char, combatSystem, slot))
	}
	if !hasEquipped {
		return ""
	}

	switch item.Type {
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
			item.Name,
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

// getArmorTooltip returns armor-specific tooltip information (YAML-driven)

func getArmorRequirementLine(item items.Item, char *character.MMCharacter) string {
	category := strings.ToLower(item.ArmorCategory)
	if category == "cloth" {
		return "Requires: None"
	}
	// Category -> required skill via the character package's authoritative map.
	skill, ok := character.ArmorSkillForCategory(category)
	if !ok {
		return ""
	}
	display := strings.ToUpper(category[:1]) + category[1:]
	if char == nil {
		return fmt.Sprintf("Requires: %s Skill", display)
	}
	if _, hasSkill := char.Skills[skill]; hasSkill {
		return fmt.Sprintf("Requires: %s Skill", display)
	}
	return fmt.Sprintf("Requires: %s Skill (Missing)", display)
}

// getConsumableTooltip returns consumable-specific tooltip information

// joinTooltipLines joins tooltip lines with newlines
func joinTooltipLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// itemKindLabel names the item for the player: wearable pieces are labeled by
// their SLOT (Belt / Amulet / Cloak / Ring ...) instead of the internal type -
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

func buildWeaponComparisonLines(item, equipped items.Item, char *character.MMCharacter, combatSystem *CombatSystem, after *character.MMCharacter, afterCombat *CombatSystem) []string {
	var lines []string

	nextDamage := afterCombat.calculateWeaponDamagePreview(item, after)
	oldDamage := combatSystem.calculateWeaponDamagePreview(equipped, char)
	lines = append(lines, fmt.Sprintf("Damage / hit: %d -> %d (%+d)", oldDamage.Total, nextDamage.Total, nextDamage.Total-oldDamage.Total))
	lines = append(lines, fmt.Sprintf("Critical damage: %d -> %d (%+d)", oldDamage.CriticalTotal, nextDamage.CriticalTotal, nextDamage.CriticalTotal-oldDamage.CriticalTotal))
	if nextDamage.True != oldDamage.True {
		lines = append(lines, fmt.Sprintf("True damage / hit: %d -> %d (%+d)", oldDamage.True, nextDamage.True, nextDamage.True-oldDamage.True))
	}
	oldFrames, nextFrames := combatSystem.WeaponCooldownFramesFor(char, equipped.Name), afterCombat.WeaponCooldownFramesFor(after, item.Name)
	lines = append(lines, fmt.Sprintf("RT recovery: %s -> %s", cooldownSeconds(combatSystem, oldFrames), cooldownSeconds(afterCombat, nextFrames)))
	oldDef, _, _ := config.GetWeaponDefinitionByName(equipped.Name)
	nextDef, _, _ := config.GetWeaponDefinitionByName(item.Name)
	if oldDef != nil && nextDef != nil {
		oldCount, nextCount := character.WeaponStrikeCount(oldDef), character.WeaponStrikeCount(nextDef)
		if oldCount != nextCount {
			lines = append(lines, fmt.Sprintf("Strikes / attack: %d -> %d", oldCount, nextCount))
		}
		if oldDef.Volley != nextDef.Volley {
			lines = append(lines, fmt.Sprintf("Projectiles / shot: %d -> %d", max(1, oldDef.Volley), max(1, nextDef.Volley)))
		}
	}

	itemRange, eqRange := 0, 0
	if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok && def != nil {
		itemRange = def.Range
	}
	if def, _, ok := config.GetWeaponDefinitionByName(equipped.Name); ok && def != nil {
		eqRange = def.Range
	}
	if itemRange != eqRange {
		lines = append(lines, fmt.Sprintf("Range: %d -> %d (%+d) tiles", eqRange, itemRange, itemRange-eqRange))
	}

	itemArc, eqArc := "", ""
	if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok {
		itemArc = character.MeleeArcShortLabel(def)
	}
	if def, _, ok := config.GetWeaponDefinitionByName(equipped.Name); ok {
		eqArc = character.MeleeArcShortLabel(def)
	}
	if itemArc != eqArc {
		lines = append(lines, fmt.Sprintf("Swing: %s -> %s", effectOrNone(eqArc), effectOrNone(itemArc)))
	}

	itemCrit := afterCombat.CalculateWeaponCritChance(item, after)
	eqCrit := combatSystem.CalculateWeaponCritChance(equipped, char)
	if itemCrit > 0 || eqCrit > 0 {
		lines = append(lines, fmt.Sprintf("Critical Chance: %d%% -> %d%% (%+d%%)", eqCrit, itemCrit, itemCrit-eqCrit))
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
		fmt.Sprintf("Equipped: %s", equippedDef.Name),
		fmt.Sprintf("Spell Points: %d vs %d (%+d)", itemCost, eqCost, itemCost-eqCost),
	}

	if itemDef.IsProjectile || equippedDef.IsProjectile {
		if rng, ok := combatSystem.CalculateSpellRangeTiles(itemDef.ID); ok {
			if eqRng, eqOK := combatSystem.CalculateSpellRangeTiles(equippedDef.ID); eqOK {
				lines = append(lines, fmt.Sprintf("Range: %.1f vs %.1f (%+.1f) tiles", rng, eqRng, rng-eqRng))
			}
		}
		// Both sides quote the packet combat fires (spellDamageParts), so Strong
		// Magic and mastery splits weigh into the comparison exactly as in play.
		_, _, itemTotal := combatSystem.CalculateSpellDamage(itemDef.ID, char)
		_, _, eqTotal := combatSystem.CalculateSpellDamage(equippedDef.ID, char)
		itemParts := combatSystem.spellDamageParts(itemDef.ID, char, itemTotal)
		itemParts, _ = combatSystem.spellPartsWithOutgoingBuff(itemParts, itemDef.School)
		eqParts := combatSystem.spellDamageParts(equippedDef.ID, char, eqTotal)
		eqParts, _ = combatSystem.spellPartsWithOutgoingBuff(eqParts, equippedDef.School)
		itemDmg := itemParts.Total()
		eqDmg := eqParts.Total()
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

// spellEffectsSummary compresses the spell's mechanics into one comparison
// cell - the SAME EffectLines the tooltip and editor print (a hand-picked
// field list here once showed only Disintegrate and lost AoE/stun/buffs).
func spellEffectsSummary(def spells.SpellDefinition) string {
	return strings.Join(def.EffectLines(), "; ")
}

// GetSpellTooltip returns a comprehensive tooltip for spells in the spellbook using centralized spell definitions
func GetSpellTooltip(spellID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return fmt.Sprintf("Unknown Spell (%s)", spellID)
	}
	out := buildSpellTooltipUnified(def, char, combatSystem, full)
	if def.Description != "" {
		out += "\n\n\"" + def.Description + "\""
	}
	return out
}

// spellSchoolForChar is the school a card SCORES this spell under: the one the
// character actually holds (SpellSchoolFor), falling back to the spell's primary
// school when there is no character to ask. A dual-school page must not be
// scored - or labelled - against a school its caster never opened.
func spellSchoolForChar(char *character.MMCharacter, def spells.SpellDefinition) string {
	if char == nil {
		return def.School
	}
	return string(char.SpellSchoolFor(def))
}

// spellSchoolsLabel names EVERY school a spell belongs to ("Earth / Air"), so a
// dual-school page does not read as the one school its definition happens to
// list first - the shop sells it to either caster.
func spellSchoolsLabel(def spells.SpellDefinition) string {
	schools := def.SchoolList()
	names := make([]string, 0, len(schools))
	for _, s := range schools {
		if n := formatSchoolName(s); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, " / ")
}

func formatSchoolName(school string) string {
	if school == "" {
		return ""
	}
	return strings.ToUpper(school[:1]) + school[1:]
}
