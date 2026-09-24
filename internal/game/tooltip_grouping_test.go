package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Invariant: every tooltip uses the same measured renderer; structured cards
// lead each mechanic with its results and keep explanations in that group.
// Case table: full catalog of item/spell/trap kinds x base/live x compact/full;
// plain/icon/titled queues x icon present/missing x 800x600/1024x768;
// equipment/spell comparisons; all stat/skill/school references; direct menu
// tooltips are covered by the GPU margin/gallery harness. Inventory, stash,
// shops and books share these queues. Editor cards share the result formatter
// and already style section headings in their native catalog renderer.
// Hover, Shift and card-art triggers are unchanged. Persistence: N/A.
func TestTooltipCatalogUsesSharedLayout(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.party.Members[0] = ch
	for _, full := range []bool{false, true} {
		for _, base := range []bool{false, true} {
			bearer := ch
			if base {
				bearer = nil
			}
			check := func(kind, key, card string, queue func(*UISystem, []string)) {
				t.Helper()
				t.Run(fmt.Sprintf("%s/%s/full%v/base%v", kind, key, full, base), func(t *testing.T) {
					lines := strings.Split(card, "\n")
					ui := &UISystem{game: cs.game}
					queue(ui, lines)
					if strings.Contains(card, "\nRULES\n") {
						t.Fatal("exceptions are separated from their mechanic")
					}
					assertSharedTooltipLayout(t, ui)
				})
			}
			for key := range config.GlobalItems.Items {
				it := items.CreateItemFromYAML(key)
				check("item", key, GetItemTooltip(it, bearer, cs, full), func(ui *UISystem, lines []string) {
					ui.queueItemTooltip(lines, it, bearer, 0, 0)
				})
			}
			for key := range config.GlobalSpells.Spells {
				check("spell", key, GetSpellTooltip(spells.SpellID(key), bearer, cs, full), func(ui *UISystem, lines []string) {
					ui.queueTooltipIcon(lines, "", 0, 0)
				})
			}
			for _, key := range config.TrapKeysOrdered() {
				def, _ := config.GetTrapDefinition(key)
				check("trap", key, buildTrapTooltipUnified(key, def, bearer, cs, full), func(ui *UISystem, lines []string) {
					ui.queueTitledTooltipIcon(lines, nil, woodPlateColor, nil, def.Icon, 0, 0)
				})
			}
		}
	}
}

func assertSharedTooltipLayout(t *testing.T, ui *UISystem) {
	t.Helper()
	for _, size := range [][2]int{{800, 600}, {1024, 768}} {
		for _, hasIcon := range []bool{false, true} {
			ui.tooltipIcon = ""
			if hasIcon {
				ui.tooltipIcon = "available-test-icon"
			}
			cap := tooltipColumnWidth(size[0], 1)
			if ui.tooltipCompareLines != nil {
				pair := ui.queuedTooltipPairLayout(size[0], size[1])
				cap = pair.mainCap
				if pair.mainW+pair.compareW+tooltipCompareGap > size[0]-2*tooltipScreenMargin || pair.compareH > size[1]-2*tooltipScreenMargin {
					t.Fatalf("%v/icon%v: comparison exceeds viewport: %+v", size, hasIcon, pair)
				}
			}
			w, h := ui.mainTooltipSize(cap, size[1])
			measuredW, measuredH := tooltipBoxSizeForScreen(ui.tooltipLines, ui.tooltipColors, hasIcon, 0, cap, size[1])
			layout := layoutTooltip(ui.tooltipLines, hasIcon, cap, size[1])
			if w != measuredW || h != measuredH || w != layout.w || h != layout.h || w > cap || h > size[1]-2*tooltipScreenMargin {
				t.Fatalf("%v/icon%v: card %dx%d does not fit or differs from renderer", size, hasIcon, w, h)
			}
			reconstructed := make([]string, len(ui.tooltipLines))
			for _, row := range layout.rows {
				if debugTextWidth(row.text) > row.w || row.x+row.w > w-6 || row.y+layout.lineHeight > h-6 {
					t.Fatalf("row outside measured bounds: %+v", row)
				}
				if hasIcon && row.y < 6+tooltipIconSize+tooltipIconGap && row.x+row.w > w-tooltipIconSize-tooltipIconGap-6 {
					t.Fatal("text overlaps icon")
				}
				reconstructed[row.source] += row.text
			}
			for i, line := range ui.tooltipLines {
				if strings.Join(strings.Fields(line), "") != strings.Join(strings.Fields(reconstructed[i]), "") {
					t.Fatalf("layout lost text from %q", line)
				}
			}
		}
	}
}

func TestTooltipResultsAndExceptionsStayTogether(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	for _, tc := range []struct {
		kind, key, group, result, detail, exception string
	}{
		{"item", "golden_armor", "DEFENSE", "Item Armor Class:", "Base Armor Class:", "Typed true damage"},
		{"item", "health_potion", "RECOVERY", "Current recovery:", "Base recovery:", "Field Medicine:"},
		{"spell", "fireball", "DAMAGE", "Total Damage:", "Base (", "Resistance reduces damage"},
		{"spell", "fireball", "CRITICAL", "Chance:", "Luck:", "Critical Damage:"},
		{"spell", "firewall", "DAMAGE PER TICK", "Total per tick:", "Base (", "Resistance reduces damage"},
		{"spell", "heal_other", "HEALING", "Total Healing:", "Base:", "Natural Healer:"},
		{"spell", "hour_of_power", "DURATION", "Current Duration:", "Base Duration:", "Light Mastery -"},
		{"trap", "cleave_trap", "DAMAGE", "Total Damage:", "Base:", "Reduced by target Armor"},
		{"trap", "bear_trap", "CONTROL", "Total Root:", "Base Root:", "Prevents movement but not attacks"},
		{"trap", "stasis_trap", "CONTROL", "Total Stun:", "Base Stun:", "Trapper -"},
	} {
		t.Run(tc.kind+"/"+tc.key+"/"+tc.group, func(t *testing.T) {
			var card string
			switch tc.kind {
			case "item":
				card = GetItemTooltip(items.CreateItemFromYAML(tc.key), ch, cs, true)
			case "spell":
				card = GetSpellTooltip(spells.SpellID(tc.key), ch, cs, true)
			case "trap":
				def, _ := config.GetTrapDefinition(tc.key)
				card = buildTrapTooltipUnified(tc.key, def, ch, cs, true)
			}
			section := ""
			var body []string
			for _, line := range strings.Split(card, "\n") {
				if tooltipSectionHeading(line) {
					section = line
				} else if section == tc.group {
					body = append(body, line)
				}
			}
			text := strings.Join(body, "\n")
			result, detail := strings.Index(text, tc.result), strings.Index(text, tc.detail)
			if result < 0 || detail < result || !strings.Contains(text, tc.exception) {
				t.Fatalf("result/calculation/exception missing or separated:\n%s", card)
			}
		})
	}
}

func TestReferenceTooltipsKeepCanonicalDescriptions(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	check := func(name, description, card string) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(card, "\n")
			var body []string
			for _, line := range lines[1:] {
				if !tooltipSectionHeading(line) {
					body = append(body, line)
				}
			}
			if strings.Join(strings.Fields(strings.Join(body, " ")), " ") != strings.Join(strings.Fields(description), " ") {
				t.Fatal("reference formatting changed the canonical facts")
			}
			ui := &UISystem{game: cs.game}
			ui.queueTooltip(lines, 0, 0)
			assertSharedTooltipLayout(t, ui)
		})
	}
	for _, stat := range []string{"Might", "Intellect", "Personality", "Endurance", "Accuracy", "Speed", "Luck"} {
		check(stat, character.StatDescription(stat), statTooltipText(stat))
	}
	for _, skill := range character.AllSkills {
		check(skill.String(), skill.Description(), masteryTooltipTextForSkill(skill))
	}
	for _, school := range character.AllMagicSchools {
		check(school.DisplayName(), character.MagicMasteryDescription(school), magicMasteryTooltipText(school))
	}
}

func TestGroupedTooltipComparisonsFit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.party.Members[0] = ch
	ch.Equipment = map[items.EquipSlot]items.Item{
		items.SlotArmor:    items.CreateItemFromYAML("leather_armor"),
		items.SlotMainHand: items.CreateWeaponFromYAML("hunting_bow"),
	}
	for _, full := range []bool{false, true} {
		for key := range config.GlobalItems.Items {
			it := items.CreateItemFromYAML(key)
			compare := GetItemComparisonTooltip(it, ch, cs)
			if compare == "" {
				continue
			}
			t.Run(fmt.Sprintf("item/%s/full%v", key, full), func(t *testing.T) {
				ui := &UISystem{game: cs.game}
				ui.queueItemTooltip(strings.Split(GetItemTooltip(it, ch, cs, full), "\n"), it, ch, 0, 0)
				ui.queueTooltipComparison(strings.Split(compare, "\n"), nil)
				assertSharedTooltipLayout(t, ui)
			})
		}
		for key := range config.GlobalSpells.Spells {
			t.Run(fmt.Sprintf("spell/%s/full%v", key, full), func(t *testing.T) {
				ui := &UISystem{game: cs.game}
				ui.queueTooltipIcon(strings.Split(GetSpellTooltip(spells.SpellID(key), ch, cs, full), "\n"), "", 0, 0)
				ui.queueTooltipComparison(buildSpellComparisonLinesByID(spells.SpellID(key), "fireball", ch, cs), nil)
				assertSharedTooltipLayout(t, ui)
			})
		}
	}
}

func TestTooltipComparisonClearsPreviousTitleStyle(t *testing.T) {
	ui := &UISystem{}
	ui.queueTitledTooltipComparison([]string{"Equipment"}, nil, woodPlateColor, equipmentBenefitColor)
	ui.queueTooltipComparison([]string{"Plain comparison"}, nil)
	if ui.tooltipCompareTitle != nil || ui.tooltipCompareText != nil {
		t.Fatal("plain comparison inherited a previous card's title style")
	}
}
