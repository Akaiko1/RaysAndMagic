package game

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

func TestEquippedSetTooltipActivation(t *testing.T) {
	for _, keys := range [][]string{
		{"padded_cap", "padded_vest", "padded_gloves", "padded_boots"},
		{"golden_armor", "gold_sword"},
	} {
		for _, state := range []string{"complete", "incomplete", "bag", "restored", "duplicates"} {
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
						if state != "incomplete" || i != len(keys)-1 {
							ch.Equipment[it.PreferredSlot(items.SlotMainHand)] = it
						}
					}
					want := state == "complete" || state == "restored"
					switch state {
					case "bag":
						// An identical loose item must not inherit an equipped instance's badge.
						target.InstanceID = 0
						items.EnsureInstanceID(&target)
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
					lines := strings.Split(text, "\n")
					ui := &UISystem{game: cs.game}
					ui.queueTitledTooltipIcon(lines, nil, color.White, nil, "", 0, 0)
					marked := 0
					for i, line := range lines {
						if strings.HasPrefix(strings.TrimSpace(line), activeSetPrefix) {
							marked++
							if ui.tooltipColors[i] != (color.RGBA{120, 225, 135, 255}) {
								t.Fatal("active bonus was not colored in the queued tooltip")
							}
						}
					}
					wantLines := 0
					if want {
						wantLines = len(config.EquipmentSetLines(target.Set))
					}
					if marked != wantLines {
						t.Fatalf("marked %d lines, want %d:\n%s", marked, wantLines, text)
					}
				})
			}
		}
	}
}
