package game

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

type itemTooltipCard struct {
	name           string
	lines, compare character.CardRows
	icon           string
}

// tooltipCatalog is every shipped tooltip as a player can open it: items and
// spells brief and with the Shift breakdown (items beside their equipment
// comparison when the hero could wear them), skills, stats, traps and cards.
func tooltipCatalog(t *testing.T) (*CombatSystem, []itemTooltipCard) {
	t.Helper()
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	comparisonTestHero(ch)
	ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")
	ch.RecalculateMaxStatsKeepingCurrent(cs.game.config)
	var catalog []items.Item
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		catalog = append(catalog, items.CreateItemFromYAML(key))
	}
	for _, key := range slices.Sorted(maps.Keys(config.GlobalWeapons.Weapons)) {
		catalog = append(catalog, items.CreateWeaponFromYAML(key))
	}
	var cards []itemTooltipCard
	for _, it := range catalog {
		var compare character.CardRows
		if _, ok := ch.EquipDestination(it); ok {
			if text := GetItemComparisonTooltipRows(it, ch, cs); len(text) > 0 {
				compare = text
			}
		}
		for _, full := range []bool{false, true} {
			cards = append(cards, itemTooltipCard{
				name:    fmt.Sprintf("item %s (full=%v)", it.Name, full),
				lines:   GetItemTooltipRows(it, ch, cs, full),
				compare: compare,
				icon:    itemTooltipIconName(it),
			})
		}
		if key := itemCardKey(it); key != "" {
			cards = append(cards, itemTooltipCard{name: "card " + key, lines: cardItemTooltipRows(key, uitext.Text("dialog.double_click_to_remove"))})
		}
	}
	for _, key := range slices.Sorted(maps.Keys(config.GlobalSpells.Spells)) {
		id := spells.SpellID(key)
		var compare character.CardRows
		if text := GetSpellComparisonTooltipRows(id, ch, cs); len(text) > 0 {
			compare = text
		}
		for _, full := range []bool{false, true} {
			cards = append(cards, itemTooltipCard{
				name:    fmt.Sprintf("spell %s (full=%v)", key, full),
				lines:   GetSpellTooltipRows(id, ch, cs, full),
				compare: compare,
				icon:    spellTooltipIconName(id),
			})
		}
	}
	for _, skill := range character.AllSkills {
		cards = append(cards, itemTooltipCard{name: "skill " + skill.String(), lines: masteryTooltipRowsForSkill(skill)})
	}
	for _, stat := range []string{"might", "intellect", "personality", "endurance", "accuracy", "speed", "luck"} {
		cards = append(cards, itemTooltipCard{name: "stat " + stat, lines: statTooltipRows(stat)})
	}
	for _, key := range slices.Sorted(maps.Keys(config.GlobalTrapConfig.Traps)) {
		text := trapTooltipRows(key, config.GlobalTrapConfig.Traps[key], ch, cs)
		cards = append(cards, itemTooltipCard{name: "trap " + key, lines: text})
	}
	for _, key := range slices.Sorted(maps.Keys(config.GlobalSpells.Spells)) {
		status := &UtilitySpellStatus{SpellID: spells.SpellID(key), Duration: 300 * cs.game.config.GetTPS()}
		lines, _ := cs.game.buffStatusCardRows(status)
		cards = append(cards, itemTooltipCard{name: "status " + key, lines: lines, icon: spellTooltipIconName(status.SpellID)})
	}
	campLines, _ := cs.game.campStatusCardRows()
	cards = append(cards, itemTooltipCard{name: "camp", lines: campLines, icon: campHUDSprite})
	return cs, cards
}

// Every tooltip - brief and the Shift breakdown, alone and beside its
// comparison - stays inside every interface frame, wherever the cursor is, in
// every font. The longest card (tallest at the UI minimum) is logged by name.
func TestEveryTooltipFitsEveryInterfaceFrame(t *testing.T) {
	cs, cards := tooltipCatalog(t)
	ui := NewUISystem(cs.game)
	if len(cards) == 0 {
		t.Fatal("no item tooltips")
	}
	forEachUIFont(t, func(t *testing.T) { tooltipsFitEveryFrame(t, ui, cards) })
}

func tooltipsFitEveryFrame(t *testing.T, ui *UISystem, cards []itemTooltipCard) {
	measure := func(c itemTooltipCard, w, h int) (single [2]int, pair tooltipPairGeometry) {
		ui.tooltipRows, ui.tooltipLines, ui.tooltipIcon = c.lines, c.lines.Lines(), c.icon
		ui.tooltipCompareRows, ui.tooltipCompareLines, ui.tooltipCompareColors = c.compare, c.compare.Lines(), nil
		single[0], single[1] = ui.mainTooltipSize(tooltipColumnWidth(w, 1), h)
		if c.compare != nil {
			pair = ui.queuedTooltipPairLayout(w, h)
		}
		return single, pair
	}
	minW, minH := MinimumWindowSize()
	longest, longestH := "", 0
	for _, c := range cards {
		single, pair := measure(c, minW, minH)
		if hh := max(single[1], pair.mainH, pair.compareH); hh > longestH {
			longest, longestH = c.name, hh
		}
	}
	t.Logf("%d tooltips; longest: %s, %dpx tall at %dx%d", len(cards), longest, longestH, minW, minH)
	for _, f := range interfaceFrameSizes(t) {
		w, h := f[0], f[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			inside := func(x, y, bw, bh int) bool {
				return x >= tooltipScreenMargin && y >= tooltipScreenMargin &&
					x+bw <= w-tooltipScreenMargin && y+bh <= h-tooltipScreenMargin
			}
			for _, c := range cards {
				single, pair := measure(c, w, h)
				for _, cursor := range [][2]int{{0, 0}, {w / 2, h / 2}, {w, h}} {
					if r := positionTooltipBox(cursor[0], cursor[1], single[0], single[1], w, h); !inside(r.x, r.y, r.w, r.h) {
						t.Fatalf("%s: tooltip %+v leaves the frame", c.name, r)
					}
					if c.compare == nil {
						continue
					}
					ph := max(pair.mainH, pair.compareH)
					y := flipTooltipY(cursor[1], ph, h)
					mainX, compareX := tooltipPairX(cursor[0], pair.mainW, pair.compareW, tooltipCompareGap, w)
					if !inside(mainX, y, pair.mainW, ph) || !inside(compareX, y, pair.compareW, ph) {
						t.Fatalf("%s: comparison pair (%d,%d %dx%d | %d %dx%d) leaves the frame", c.name, mainX, y, pair.mainW, ph, compareX, pair.compareW, ph)
					}
				}
			}
		})
	}
}
