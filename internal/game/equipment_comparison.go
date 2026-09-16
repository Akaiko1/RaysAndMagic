package game

import (
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/stats"
)

// equipmentComparisonContext isolates the equipment mutation while retaining
// party membership, cards, auras and active buffs for the real combat formulas.
// Other members and read-only skill/buff definitions are shared, never mutated.
func equipmentComparisonContext(cs *CombatSystem, original *character.MMCharacter) (*character.MMCharacter, *CombatSystem) {
	copy := *original
	copy.Equipment = maps.Clone(original.Equipment)
	if copy.Equipment == nil {
		copy.Equipment = make(map[items.EquipSlot]items.Item)
	}
	source := cs.game
	preview := &MMGame{config: source.config, cardSlots: source.cardSlots, combatBuffs: source.combatBuffs, turnBasedMode: source.turnBasedMode}
	if source.party != nil {
		preview.party = &character.Party{Members: slices.Clone(source.party.Members)}
		for i, member := range preview.party.Members {
			if member == original {
				preview.party.Members[i] = &copy
			}
		}
	}
	copy.RecalculateMaxStatsKeepingCurrent(source.config)
	return &copy, &CombatSystem{game: preview}
}

// previewEquippedItem is shared by the item's card and the comparison panel.
func previewEquippedItem(cs *CombatSystem, original *character.MMCharacter, item items.Item, slot items.EquipSlot) (*character.MMCharacter, *CombatSystem) {
	candidate, preview := equipmentComparisonContext(cs, original)
	candidate.Equipment[slot] = item
	candidate.RecalculateMaxStatsKeepingCurrent(cs.game.config)
	return candidate, preview
}

func buildEquipmentComparisonLines(item items.Item, original *character.MMCharacter, cs *CombatSystem, slot items.EquipSlot) []string {
	before, beforeCS := equipmentComparisonContext(cs, original)
	after, afterCS := previewEquippedItem(cs, original, item, slot)
	equipped, occupied := before.Equipment[slot]
	name := "Empty"
	if occupied {
		name = equipped.Name
	}
	slotLabel := slot.DisplayName()
	if slot == items.SlotRing1 {
		slotLabel = "Ring 1"
	}
	if slot == items.SlotRing2 {
		slotLabel = "Ring 2"
	}
	lines := []string{fmt.Sprintf("%s: %s", slotLabel, name), "After equipping (current -> new)"}
	if !original.ItemFitsSlot(item, slot) {
		lines = append(lines, "Cannot equip: requirements not met")
	}
	if item.Type == items.ItemWeapon && occupied && equipped.Type == items.ItemWeapon {
		// Weapon-specific traits use the resulting set and stats, too.
		lines = append(lines, buildWeaponComparisonLines(item, equipped, before, beforeCS, after, afterCS)...)
	}
	appendDelta := func(label string, old, next int, unit string) {
		if old != next {
			lines = append(lines, fmt.Sprintf("%s: %d%s -> %d%s (%+d%s)", label, old, unit, next, unit, next-old, unit))
		}
	}
	appendDelta("Armor Class", beforeCS.CalculateTotalArmorClass(before), afterCS.CalculateTotalArmorClass(after), "")
	appendDelta("Max HP", before.MaxHitPoints, after.MaxHitPoints, "")
	appendDelta("Max SP", before.MaxSpellPoints, after.MaxSpellPoints, "")
	oldStats, newStats := character.EffectiveCombatStats(before), character.EffectiveCombatStats(after)
	for _, name := range stats.Names {
		appendDelta(config.TitleWords(name), oldStats.ValueByName(name), newStats.ValueByName(name), "")
	}
	appendDelta("Dodge", beforeCS.PerfectDodgeChance(before), afterCS.PerfectDodgeChance(after), "%")
	appendDelta("Spell critical chance", beforeCS.totalCriticalChance(0, before), afterCS.totalCriticalChance(0, after), "%")
	for _, hand := range []items.EquipSlot{items.SlotMainHand, items.SlotOffHand} {
		oldWeapon, oldOK := before.Equipment[hand]
		newWeapon, newOK := after.Equipment[hand]
		if !newOK || newWeapon.Type != items.ItemWeapon {
			if oldOK && oldWeapon.Type == items.ItemWeapon {
				lines = append(lines, "Lose: "+hand.DisplayName()+" weapon attack")
			}
			continue
		}
		if item.Type == items.ItemWeapon && hand == slot && oldOK && oldWeapon.Type == items.ItemWeapon {
			continue
		}
		oldDamage, oldCrit := 0, 0
		if oldOK && oldWeapon.Type == items.ItemWeapon {
			oldDamage = beforeCS.calculateWeaponDamagePreview(oldWeapon, before).Total
			oldCrit = beforeCS.CalculateWeaponCritChance(oldWeapon, before)
		}
		appendDelta(hand.DisplayName()+" damage", oldDamage, afterCS.calculateWeaponDamagePreview(newWeapon, after).Total, "")
		appendDelta(hand.DisplayName()+" critical chance", oldCrit, afterCS.CalculateWeaponCritChance(newWeapon, after), "%")
		if oldOK && oldWeapon.Type == items.ItemWeapon {
			oldRecovery := cooldownSeconds(beforeCS, beforeCS.WeaponCooldownFramesFor(before, oldWeapon.Name))
			newRecovery := cooldownSeconds(afterCS, afterCS.WeaponCooldownFramesFor(after, newWeapon.Name))
			if oldRecovery != newRecovery {
				lines = append(lines, fmt.Sprintf("%s RT recovery: %s -> %s", hand.DisplayName(), oldRecovery, newRecovery))
			}
		}
	}
	// Group equal resistance changes instead of printing nine identical rows.
	type resistChange struct {
		old, next int
		schools   []string
	}
	var resists []resistChange
	for _, school := range damagecalc.Types() {
		old, next := beforeCS.game.schoolResistPct(before, school.String()), afterCS.game.schoolResistPct(after, school.String())
		if old == next {
			continue
		}
		index := slices.IndexFunc(resists, func(r resistChange) bool { return r.old == old && r.next == next })
		if index < 0 {
			resists = append(resists, resistChange{old: old, next: next})
			index = len(resists) - 1
		}
		resists[index].schools = append(resists[index].schools, config.TitleWords(school.String()))
	}
	for _, r := range resists {
		label := strings.Join(r.schools, "/")
		if len(r.schools) == len(damagecalc.Types()) {
			label = "All"
		}
		appendDelta(label+" resist", r.old, r.next, "%")
	}
	// Read all authored effects from the existing formatter. Numerical stat and
	// resistance rows already appear above; conditional mechanics remain explicit.
	oldEffects, newEffects := comparisonEffectLines(equipped), comparisonEffectLines(item)
	for _, pair := range []struct {
		prefix      string
		from, other []string
	}{{"Gain: ", newEffects, oldEffects}, {"Lose: ", oldEffects, newEffects}} {
		for _, line := range pair.from {
			if !slices.Contains(pair.other, line) {
				lines = append(lines, pair.prefix+line)
			}
		}
	}
	keys := []string{equipped.Set, item.Set}
	for i, key := range keys {
		if key == "" || (i > 0 && key == keys[0]) {
			continue
		}
		old, next := before.HasCompletedEquipmentSet(key), after.HasCompletedEquipmentSet(key)
		if old != next {
			label := "Set activated: "
			if !next {
				label = "Set lost: "
			}
			lines = append(lines, label+config.GetItemSet(key).Name)
			for _, line := range config.EquipmentSetLines(key)[1:] {
				lines = append(lines, line)
			}
		}
	}
	if len(lines) == 2 {
		lines = append(lines, "No change to current stats or abilities")
	}
	return lines
}

func comparisonEffectLines(item items.Item) []string {
	if item.Type == items.ItemWeapon {
		if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok {
			var out []string
			for _, line := range weaponEffectLines(def) {
				if !slices.Contains(def.SetLines(), line) {
					out = append(out, line)
				}
			}
			return out
		}
		return nil
	}
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	if !ok || def == nil {
		return nil
	}
	excluded := append(def.StatBonusLines(), def.ResistLines()...)
	excluded = append(excluded, def.SetLines()...)
	var out []string
	for _, line := range character.FilteredItemEffectLines(def) {
		if !slices.Contains(excluded, line) {
			out = append(out, line)
		}
	}
	return out
}

// Numerical gains and losses use readable colors independently of item rarity.
// Recovery stays neutral: a lower duration is better, unlike these stat rows.
func equipmentComparisonColors(lines []string, base []color.Color) []color.Color {
	out := make([]color.Color, len(lines))
	for i, line := range lines {
		out[i] = color.White
		if i < len(base) {
			out[i] = base[i]
		}
		if strings.Contains(line, " -> ") && strings.Contains(line, " (+") {
			out[i] = color.RGBA{120, 225, 135, 255}
		}
		if (strings.Contains(line, " -> ") && strings.Contains(line, " (-")) || strings.HasPrefix(line, "Cannot equip") {
			out[i] = color.RGBA{245, 135, 120, 255}
		}
	}
	return out
}
