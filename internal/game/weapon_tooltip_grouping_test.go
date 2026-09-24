package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Invariant: each weapon mechanic has one result-first group, with its own
// explanations adjacent, and measurement matches the queued renderer.
// Case table:
//   - Content: entire weapon catalog (melee/projectile, single/multi-strike,
//     physical/elemental/true damage, area effects, special effects, sets).
//   - Context: equipped, offered post-equip preview, base shop/editor.
//   - Detail: compact/full; the same hover and held-Shift entry points.
//   - Layout: icon/missing icon, single/comparison, 800x600/1024x768.
//
// Inventory and stash share queueItemTooltip; shops use it with a nil bearer.
// The editor shares GetItemTooltip but retains its own reference-panel layout.
// Persistence: N/A; no gameplay or saved state is changed by presentation.
func TestWeaponTooltipMechanicGroups(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.party.Members[0] = ch
	for key := range config.GlobalWeapons.Weapons {
		for _, context := range []string{"equipped", "offered", "base"} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/full%v", key, context, full), func(t *testing.T) {
					it := items.CreateWeaponFromYAML(key)
					items.EnsureInstanceID(&it)
					ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
					bearer := ch
					if context == "base" {
						bearer = nil
					} else if context == "offered" {
						ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
					}
					card := GetItemTooltip(it, bearer, cs, full)
					lines := strings.Split(card, "\n")
					group := ""
					for _, line := range lines {
						if tooltipSectionHeading(line) {
							group = line
						}
						for _, expected := range []struct{ prefix, section string }{
							{"Total Damage:", "DAMAGE"}, {"Normal Damage:", "DAMAGE"},
							{"Strikes per attack:", "DAMAGE"}, {"Reduced by target Armor", "DAMAGE"},
							{"True Damage", "DAMAGE"}, {"Grandmaster: this strike", "DAMAGE"},
							{"Chance:", "CRITICAL"}, {"Critical Damage:", "CRITICAL"},
							{"Critical hits", "CRITICAL"}, {"Capped at", "CRITICAL"},
							{"RT Cooldown:", "ATTACK"}, {"Base weapon cooldown:", "ATTACK"},
							{"Range:", "ATTACK"}, {"Projectile Speed:", "ATTACK"},
						} {
							if strings.HasPrefix(line, expected.prefix) && group != expected.section {
								t.Fatalf("%q separated from its %s group:\n%s", line, expected.section, card)
							}
						}
					}
					if strings.Contains(card, "\nRULES\n") {
						t.Fatal("weapon rules must stay with their mechanic")
					}
					if ttIndexOf(card, "Total Damage:") > ttIndexOf(card, "ATTACK") {
						t.Fatal("damage result must precede attack handling")
					}
					if full {
						if ttIndexOf(card, "Total Damage:") >= ttIndexOf(card, "Base:") {
							t.Fatal("damage result must precede its calculation")
						}
						if bearer != nil && ttIndexOf(card, "RT Cooldown:") >= ttIndexOf(card, "Base weapon cooldown:") {
							t.Fatal("cooldown result must precede its calculation")
						}
					}
					ui := &UISystem{game: cs.game}
					ui.queueItemTooltip(lines, it, bearer, 0, 0)
					for _, size := range [][2]int{{800, 600}, {1024, 768}} {
						for _, columns := range []int{1, 2} {
							if columns == 2 && context != "offered" {
								continue // Only offered items queue comparisons.
							}
							for _, icon := range []bool{false, true} {
								// Exercise the measurement path with and without an available sprite.
								ui.tooltipIcon = ""
								if icon {
									ui.tooltipIcon = itemTooltipIconName(it)
								}
								cap := tooltipColumnWidth(size[0], columns)
								if columns == 2 {
									ui.queueTooltipComparison(strings.Split(GetItemComparisonTooltip(it, bearer, cs), "\n"), nil)
									pair := ui.queuedTooltipPairLayout(size[0], size[1])
									cap = pair.mainCap
									if pair.mainW+pair.compareW+tooltipCompareGap > size[0]-2*tooltipScreenMargin || pair.compareH > size[1]-2*tooltipScreenMargin {
										t.Fatalf("comparison pair leaves the viewport: %+v", pair)
									}
								}
								w, h := ui.mainTooltipSize(cap, size[1])
								layout := layoutTooltip(lines, icon, cap, size[1])
								if w != layout.w || h != layout.h || w > cap || h > size[1]-2*tooltipScreenMargin {
									t.Fatalf("%v/columns%d/icon%v: card %dx%d exceeds viewport or differs from renderer", size, columns, icon, w, h)
								}
								reconstructed := make([]string, len(lines))
								for _, row := range layout.rows {
									if debugTextWidth(row.text) > row.w || row.x+row.w > w-6 || row.y+layout.lineHeight > h-6 {
										t.Fatalf("painted row leaves its measured bounds: %+v", row)
									}
									if icon && row.y < 6+tooltipIconSize+tooltipIconGap && row.x+row.w > w-tooltipIconSize-tooltipIconGap-6 {
										t.Fatal("text overlaps the icon")
									}
									reconstructed[row.source] += row.text
								}
								for i, line := range lines {
									if strings.Join(strings.Fields(line), "") != strings.Join(strings.Fields(reconstructed[i]), "") {
										t.Fatalf("layout lost text from %q", line)
									}
								}
							}
						}
					}
				})
			}
		}
	}
}
