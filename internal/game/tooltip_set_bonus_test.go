package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Cases: count-based armor and exact-piece weapon sets; complete, incomplete,
// loose copy, duplicate pieces, shop, missing identity, and save/load; compact/full.
// All cards use plain set text; only worn complete sets receive green styling.
func TestEquippedSetTooltipActivation(t *testing.T) {
	for _, keys := range [][]string{
		{"padded_cap", "padded_vest", "padded_gloves", "padded_boots"},
		{"golden_armor", "gold_sword"},
	} {
		for _, state := range []string{"complete", "incomplete", "bag", "bag-completes", "restored", "duplicates", "shop", "unidentified"} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/full=%v", keys[0], state, full), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					fillTestParty(t, cs.game)
					ch := cs.game.party.Members[0]
					ch.Equipment = map[items.EquipSlot]items.Item{}
					var target items.Item
					for i, key := range keys {
						var it items.Item
						var err error
						if key == "gold_sword" {
							it, err = items.TryCreateWeaponFromYAML(key)
						} else {
							it, err = items.TryCreateItemFromYAML(key)
						}
						if err != nil {
							t.Fatal(err)
						}
						items.EnsureInstanceID(&it)
						if i == 0 {
							target = it
						}
						if (state != "incomplete" || i != len(keys)-1) && (state != "bag-completes" || i != 0) {
							ch.Equipment[it.PreferredSlot(items.SlotMainHand)] = it
						}
					}
					want := state == "complete" || state == "restored"
					switch state {
					case "bag":
						// An identical loose item must not inherit an equipped instance's badge.
						target.InstanceID = 0
						items.EnsureInstanceID(&target)
					case "shop":
						ch = nil
					case "unidentified":
						target.InstanceID = 0
					case "restored":
						data, err := json.Marshal(ch.Equipment)
						if err != nil {
							t.Fatal(err)
						}
						ch.Equipment = nil
						if err := json.Unmarshal(data, &ch.Equipment); err != nil {
							t.Fatal(err)
						}
					case "duplicates":
						if len(keys) != 2 {
							t.Skip("exact-piece rule only")
						}
						target = ch.Equipment[items.SlotMainHand]
						ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: target, items.SlotOffHand: target}
					}
					text := GetItemTooltip(target, ch, cs, full)
					if ch != nil {
						count := len(keys)
						if state == "incomplete" || state == "bag-completes" {
							count--
						}
						if state == "duplicates" {
							count = 1
						}
						if !strings.Contains(text, fmt.Sprintf("(%d/%d equipped)", count, len(keys))) {
							t.Fatalf("wrong set progress: %s", text)
						}
					}
					lines := strings.Split(text, "\n")
					ui := &UISystem{game: cs.game}
					ui.queueItemTooltip(lines, target, ch, 0, 0)
					if strings.Contains(text, "[ACTIVE]") || strings.Contains(strings.Join(ui.tooltipLines, "\n"), "[ACTIVE]") {
						t.Fatalf("set activity leaked into visible text: %s", text)
					}
					found := 0
					for i, line := range ui.tooltipLines {
						isSetLine := false
						for _, setLine := range equipmentSetTooltipLines(target.Set, ch) {
							if strings.TrimSpace(line) == setLine {
								isSetLine = true
								found++
								break
							}
						}
						green := i < len(ui.tooltipColors) && ui.tooltipColors[i] == equipmentBenefitColor
						if green != (want && isSetLine) {
							t.Fatalf("line %q green=%v, active=%v setLine=%v", line, green, want, isSetLine)
						}
					}
					if found != len(equipmentSetTooltipLines(target.Set, ch)) {
						t.Fatalf("missing set description: %s", text)
					}
				})
			}
		}
	}
}

func TestSetHighlightSurvivesLongAuthoredText(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	armor := items.CreateItemFromYAML("golden_armor")
	sword := items.CreateWeaponFromYAML("gold_sword")
	items.EnsureInstanceID(&armor)
	items.EnsureInstanceID(&sword)
	ch.Equipment = map[items.EquipSlot]items.Item{items.SlotArmor: armor, items.SlotMainHand: sword}
	set := config.GetItemSet(armor.Set)
	original := *set
	t.Cleanup(func() { *set = original })
	set.Name = strings.Repeat("Long authored set name ", 5)
	for _, full := range []bool{false, true} {
		lines := strings.Split(GetItemTooltip(armor, ch, cs, full), "\n")
		colors := activeSetBonusColors(lines, nil, armor, ch)
		wrapped, colors := wrapTooltipLines(lines, colors, 0, 300, 0)
		active := false
		greenRows := 0
		for i, line := range wrapped {
			if line == equipmentSetSectionTitle {
				active = true
				continue
			}
			if line == "" {
				active = false
			}
			green := colors[i] == equipmentBenefitColor
			if green != active {
				t.Fatalf("wrapped row %q green=%v, want %v", line, green, active)
			}
			if green {
				greenRows++
			}
		}
		if greenRows < 3 {
			t.Fatal("fixture did not exercise wrapped set rows")
		}
	}
}
