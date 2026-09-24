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

// tooltipDetailHeld reports whether the player is holding Shift to expand a
// tooltip to include calculations and mechanic-specific exceptions.
func tooltipDetailHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// GetItemTooltip is shared by inventory, shops and editor catalogs. A nil
// character requests the item's base values, independent of any active game.
func GetItemTooltip(item items.Item, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	if char == nil {
		combatSystem = nil
	}
	wearer := char
	alreadyEquipped := false
	// A bag/shop item's own card uses the same post-equip context as the
	// comparison. Equipped items retain their actual slot (especially rings).
	if char != nil && combatSystem != nil && combatSystem.game != nil &&
		(item.Type == items.ItemWeapon || item.Type == items.ItemArmor || item.Type == items.ItemAccessory) {
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

	// Every category uses result-first mechanic sections. Compact is the
	// default; full=true (Shift held) adds calculations and exceptions within
	// their sections. Pure formatter - no input read.
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
		core = fmt.Sprintf("%s\n%s", item.Name, item.DisplayKind())
	}

	if setLines := equipmentSetTooltipLines(item.Set, wearer); len(setLines) > 0 {
		core += "\n\n" + equipmentSetSectionTitle + "\n" + strings.Join(setLines, "\n")
	}
	var tail []string
	if val, ok := item.Attributes["value"]; ok && val > 0 {
		tail = append(tail, fmt.Sprintf("Value: %d gold", val))
	}
	tail = append(tail, itemProseLines(item)...)
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
	spellID := spells.SpellID(item.SpellEffect)
	_, err := spells.GetSpellDefinitionByID(spellID)
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

	return joinTooltipLines(lines)
}

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

// joinTooltipLines joins tooltip lines with newlines
func joinTooltipLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func buildWeaponComparisonLines(item, equipped items.Item, char *character.MMCharacter, combatSystem *CombatSystem, after *character.MMCharacter, afterCombat *CombatSystem) []string {
	var lines []string

	nextDamage := afterCombat.calculateWeaponDamagePreview(item, after)
	oldDamage := combatSystem.calculateWeaponDamagePreview(equipped, char)
	if oldDamage.Total != nextDamage.Total {
		lines = append(lines, fmt.Sprintf("Damage / hit: %d -> %d (%+d)", oldDamage.Total, nextDamage.Total, nextDamage.Total-oldDamage.Total))
	}
	if oldDamage.CriticalTotal != nextDamage.CriticalTotal {
		lines = append(lines, fmt.Sprintf("Critical damage: %d -> %d (%+d)", oldDamage.CriticalTotal, nextDamage.CriticalTotal, nextDamage.CriticalTotal-oldDamage.CriticalTotal))
	}
	if nextDamage.True != oldDamage.True {
		lines = append(lines, fmt.Sprintf("True damage / hit: %d -> %d (%+d)", oldDamage.True, nextDamage.True, nextDamage.True-oldDamage.True))
	}
	oldFrames, nextFrames := combatSystem.WeaponCooldownFramesFor(char, equipped.Name), afterCombat.WeaponCooldownFramesFor(after, item.Name)
	oldRecovery, nextRecovery := cooldownSeconds(combatSystem, oldFrames), cooldownSeconds(afterCombat, nextFrames)
	if oldRecovery != nextRecovery {
		lines = append(lines, fmt.Sprintf("RT recovery: %s -> %s", oldRecovery, nextRecovery))
	}
	oldDef, _, _ := config.GetWeaponDefinitionByName(equipped.Name)
	nextDef, _, _ := config.GetWeaponDefinitionByName(item.Name)
	if oldDef != nil && nextDef != nil {
		oldCount, nextCount := character.WeaponStrikeCount(oldDef), character.WeaponStrikeCount(nextDef)
		if oldCount != nextCount {
			lines = append(lines, fmt.Sprintf("Strikes / attack: %d -> %d", oldCount, nextCount))
		}
		if max(1, oldDef.Volley) != max(1, nextDef.Volley) {
			lines = append(lines, fmt.Sprintf("Projectiles / shot: %d -> %d", max(1, oldDef.Volley), max(1, nextDef.Volley)))
		}
	}

	itemRange, itemSpeed := character.EffectiveWeaponFlight(nextDef, after)
	eqRange, eqSpeed := character.EffectiveWeaponFlight(oldDef, char)
	if itemRange != eqRange {
		lines = append(lines, fmt.Sprintf("Range: %.0f -> %.0f (%+.0f) tiles", eqRange, itemRange, itemRange-eqRange))
	}

	if itemSpeed != eqSpeed {
		lines = append(lines, fmt.Sprintf("Projectile Speed: %.1f -> %.1f (%+.1f) tiles/s", eqSpeed, itemSpeed, itemSpeed-eqSpeed))
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
	if itemCrit != eqCrit {
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
	lines := []string{fmt.Sprintf("Equipped: %s", equippedDef.Name), "After equipping (current -> new)"}
	casting := ttSection{Title: "CASTING"}
	damageSection := ttSection{Title: "DAMAGE"}
	healing := ttSection{Title: "HEALING"}
	effects := ttSection{Title: "EFFECTS"}
	if itemCost != eqCost {
		casting.Add("Spell Points: %d -> %d (%+d)", eqCost, itemCost, itemCost-eqCost)
	}

	oldCooldown := cooldownSeconds(combatSystem, combatSystem.SpellCooldownFrames(char, equippedID))
	newCooldown := cooldownSeconds(combatSystem, combatSystem.SpellCooldownFrames(char, itemID))
	if oldCooldown != newCooldown {
		casting.Add("RT recovery: %s -> %s", effectOrNone(oldCooldown), effectOrNone(newCooldown))
	}

	if itemDef.IsProjectile || equippedDef.IsProjectile {
		if rng, ok := combatSystem.CalculateSpellRangeTiles(itemDef.ID); ok {
			if eqRng, eqOK := combatSystem.CalculateSpellRangeTiles(equippedDef.ID); eqOK && fmt.Sprintf("%.1f", rng) != fmt.Sprintf("%.1f", eqRng) {
				casting.Add("Range: %.1f -> %.1f (%+.1f) tiles", eqRng, rng, rng-eqRng)
			}
		}
	}
	// Compare like units: direct hits and zone ticks get separate rows. Control
	// and healing spells do not acquire outgoing damage bonuses in the preview.
	damage := func(def spells.SpellDefinition, perTick bool) int {
		kind := def.DamageFormula().Kind
		if kind == spells.DamageNone || (kind == spells.DamageZone) != perTick {
			return 0
		}
		_, _, total := combatSystem.CalculateSpellDamage(def.ID, char)
		parts := combatSystem.spellDamageParts(def.ID, char, total)
		parts, _ = combatSystem.spellPartsWithOutgoingBuff(parts, def.School)
		return parts.Total()
	}
	for _, perTick := range []bool{false, true} {
		old, next := damage(equippedDef, perTick), damage(itemDef, perTick)
		if old != next {
			label := "Total Damage"
			if perTick {
				label = "Damage per tick"
			}
			damageSection.Add("%s: %d -> %d (%+d)", label, old, next, next-old)
		}
	}

	if itemDef.HealAmount > 0 || equippedDef.HealAmount > 0 {
		_, _, itemHeal := combatSystem.CalculateSpellHealing(itemDef.ID, char)
		_, _, eqHeal := combatSystem.CalculateSpellHealing(equippedDef.ID, char)
		if itemHeal != eqHeal {
			healing.Add("Total Healing: %d -> %d (%+d)", eqHeal, itemHeal, itemHeal-eqHeal)
		}
	}

	if itemDef.IsUtility || equippedDef.IsUtility {
		if itemDef.Duration > 0 || equippedDef.Duration > 0 {
			itemDur := combatSystem.CalculateSpellDurationSeconds(itemDef.ID, char)
			eqDur := combatSystem.CalculateSpellDurationSeconds(equippedDef.ID, char)
			if itemDur != eqDur {
				effects.Add("Duration: %ds -> %ds (%+ds)", eqDur, itemDur, itemDur-eqDur)
			}
		}
	}

	itemEffects := spellEffectsSummary(itemDef, char)
	eqEffects := spellEffectsSummary(equippedDef, char)
	if itemEffects != eqEffects {
		effects.Add("Effects: %s -> %s", effectOrNone(eqEffects), effectOrNone(itemEffects))
	}
	body := character.RenderCardLines([]ttSection{damageSection, healing, effects, casting}, true)
	if len(body) > 0 {
		lines = append(append(lines, ""), body...)
	}
	if len(lines) == 2 {
		lines = append(lines, "No change to current stats or abilities")
	}

	return lines
}

func effectOrNone(s string) string {
	if s == "" {
		return "None"
	}
	return s
}

// spellEffectsSummary uses the card's current effects and delivery geometry.
func spellEffectsSummary(def spells.SpellDefinition, char *character.MMCharacter) string {
	effects := spellCurrentEffects(def, char, true)
	effects.Title = ""
	var lines []string
	for _, line := range character.RenderCardLines([]ttSection{effects}, false) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if kind := def.DamageFormula().Kind; kind == spells.DamageNova || kind == spells.DamageZone {
		lines = append(lines, damageTypeAoELine(def.School, 0))
	}
	lines = append(lines, spellAreaLines(def)...)
	return strings.Join(lines, "; ")
}

// GetSpellTooltip returns a comprehensive tooltip for spells in the spellbook using centralized spell definitions
func GetSpellTooltip(spellID spells.SpellID, char *character.MMCharacter, combatSystem *CombatSystem, full bool) string {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return fmt.Sprintf("Unknown Spell (%s)", spellID)
	}
	var out string
	if authored, ok := config.GetSpellDefinition(string(spellID)); ok && authored.MonsterOnly {
		out = renderTooltip(def.Name, "Monster spell - "+spellSchoolsLabel(def), character.MonsterSpellCardSections(authored, def), full)
	} else {
		out = buildSpellTooltipUnified(def, char, combatSystem, full)
	}
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

// Authored description and flavor have different jobs. Legacy saves may store
// only flavor in Description, so known definitions always supply both texts.
func itemProseLines(item items.Item) []string {
	description, flavor := "", ""
	if item.Type == items.ItemWeapon {
		if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok {
			description, flavor = def.Description, def.Flavor
		} else {
			flavor = item.Description
		}
	} else if def, _, ok := config.GetItemDefinitionByName(item.Name); ok {
		description, flavor = def.Description, def.Flavor
	} else {
		flavor = item.Description
	}
	var lines []string
	if description != "" {
		lines = append(lines, description)
	}
	if flavor != "" && flavor != description {
		lines = append(lines, fmt.Sprintf("\"%s\"", flavor))
	}
	return lines
}
