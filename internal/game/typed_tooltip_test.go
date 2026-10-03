package game

import (
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// Case table: real shared/personal bag, equipped, stash, merchant and spell
// trader hovers x compact/full. Catalog coverage crosses all authored kinds
// and base/live contexts in TestCardSectionRolesAndHint and the layout tests.
// All row roles x misleading wording x wrapping are exercised below and in
// the editor. Existing dwell tests cover quick-slot/party-creation triggers.
// Rows are rebuilt on hover; persistence is N/A.
func TestTypedRowsReachItemAndSpellHovers(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, entry := range []string{"shared bag", "personal bag", "equipped", "stash", "merchant", "spell trader"} {
			t.Run(fmt.Sprintf("%s/full=%v", entry, full), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, ui := h.g, h.ui
				hero := gmReferenceChar(g.config)
				g.party.Members[g.selectedChar] = hero
				g.party.Inventory, hero.Inventory = nil, nil
				hero.Equipment = map[items.EquipSlot]items.Item{}
				item := items.CreateWeaponFromYAML("iron_sword")
				prev := tooltipDetailHeld
				tooltipDetailHeld = func() bool { return full }
				t.Cleanup(func() { tooltipDetailHeld = prev })
				fp := installFakePointer(t)
				prevCursor := rawCursorPosition
				rawCursorPosition = func() (int, int) { return fp.x, fp.y }
				t.Cleanup(func() { rawCursorPosition = prevCursor })
				var want character.CardRows
				switch entry {
				case "shared bag", "personal bag":
					owner := (*character.MMCharacter)(nil)
					g.party.Inventory = []items.Item{item}
					if entry == "personal bag" {
						owner, g.party.Inventory, hero.Inventory = hero, nil, []items.Item{item}
					}
					grid := layoutRect{20, 20, 300, 300}
					x, y, w, cellH := scaleInventorySourceRect(grid.x, grid.y, grid.w, grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
					fp.moveTo(x+w/2, y+cellH/2)
					page := 0
					ui.drawInventoryBagGrid(h.screen, owner, grid, layoutRect{20, 330, 300, 20}, &page)
					want = GetItemTooltipRows(item, hero, g.combat, full)
					if !reflect.DeepEqual(ui.tooltipCompareRows, GetItemComparisonTooltipRows(item, hero, g.combat)) {
						t.Fatal("bag comparison lost its row roles")
					}
				case "equipped":
					hero.Equipment[items.SlotMainHand] = item
					content := computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content
					paper := computeInventoryContentLayout(content).paper
					slot := inventoryPaperdollSlots[0]
					for _, candidate := range inventoryPaperdollSlots {
						if candidate.slot == items.SlotMainHand {
							slot = candidate
							break
						}
					}
					x, y, size := scaleInventorySourceSquare(paper.x, paper.y, paper.w, paper.h, inventoryPaperdollSourceW, inventoryPaperdollSourceH, slot.rect)
					fp.moveTo(x+size/2, y+size/2)
					ui.drawInventoryContent(h.screen, content)
					want = GetItemTooltipRows(item, hero, g.combat, full)
				case "stash":
					ui.stashCellTooltip(item, image.Rect(10, 10, 74, 74), 30, 30)
					want = GetItemTooltipRows(item, hero, g.combat, full)
				case "merchant":
					g.dialogActive = true
					g.dialogNPC = &character.NPC{Name: "Trader", MerchantStock: []*character.MerchantStockItem{{Item: item, Cost: 10, Quantity: 1}}}
					dlg := npcDialogLayout(g)
					left, _, top, _ := merchantGridLayout(dlg.x, dlg.y)
					x, y, w, cellH := merchantCellRect(left, top, 0)
					fp.moveTo(x+w/2, y+cellH/2)
					ui.drawMerchantDialog(h.screen, dlg.x, dlg.y, dlg.w, dlg.h)
					want = GetItemTooltipRows(item, nil, g.combat, full)
				case "spell trader":
					g.dialogActive = true
					g.dialogNPC = &character.NPC{Name: "Trader", SpellData: map[string]*character.NPCSpell{"fireball": {Name: "Fireball", Cost: 10}}}
					dlg := npcDialogLayout(g)
					x, y, w, cellH := spellTraderIconRect(dlg.x, dlg.y, 0)
					fp.moveTo(x+w/2, y+cellH/2)
					ui.drawSpellTraderDialog(h.screen, dlg.x, dlg.y, dlg.w, dlg.h)
					want = ui.spellTraderTooltipRows("fireball", hero)
				}
				if !reflect.DeepEqual(ui.tooltipRows, want) || len(want) == 0 {
					t.Fatalf("%s hover lost row roles: got %+v, want %+v", entry, ui.tooltipRows, want)
				}
				if ui.tooltipRows.String() != strings.Join(ui.tooltipLines, "\n") {
					t.Fatal("hover's text projection diverged")
				}
			})
		}
	}
}

func TestTypedTooltipColorsAndWrappingIgnoreWording(t *testing.T) {
	rows := character.CardRows{
		{Text: "DAMAGE", Kind: character.CardRowTitle},
		{Text: "DAMAGE", Kind: character.CardRowCategory},
		{Text: "A new unregistered mechanic", Kind: character.CardRowSection},
		{Text: "Answer without a known prefix", Kind: character.CardRowResult},
		{Text: "Range: this is only a calculation", Kind: character.CardRowDetail},
		{Text: "DAMAGE", Kind: character.CardRowDescription},
		{Text: "Unquoted flavor", Kind: character.CardRowFlavor},
		{Text: "DAMAGE", Kind: character.CardRowBody},
	}
	colors := tooltipBodyColors(rows, woodPlateColor)
	if colors[0] != colors[3] || colors[0] == colors[4] || colors[2] == colors[7] || colors[6] == colors[7] {
		t.Fatal("tooltip colors rediscovered roles from wording")
	}
	for _, icon := range []bool{false, true} {
		long := append(character.CardRows(nil), rows...)
		for i := range long {
			long[i].Text = strings.Repeat(long[i].Text+" ", 5)
		}
		ui := &UISystem{}
		ui.queueCardTooltip(long, nil, nil, nil, "", 0, 0)
		ui.queueCardComparison(long, nil, nil, nil)
		for _, queued := range []character.CardRows{ui.mainTooltipRows(), ui.compareTooltipRows()} {
			layout := layoutCardTooltip(queued, icon, 400, 768)
			seen := make([]int, len(queued))
			for _, row := range layout.rows {
				seen[row.source]++
			}
			for i, count := range seen {
				if count == 0 || queued[i].Kind != rows[i].Kind {
					t.Fatalf("wrapped row %d lost its role", i)
				}
			}
		}
	}
	// Generic hover copy must never acquire section/result/flavor roles from text.
	ui := &UISystem{}
	ui.queueTooltip([]string{"Plain", "DAMAGE", "Range: 2", "\"quote\""}, 0, 0)
	for _, row := range ui.tooltipRows[1:] {
		if row.Kind != character.CardRowBody {
			t.Fatalf("plain hover reclassified %q", row.Text)
		}
	}
}

func TestTypedRowsReachStatusHovers(t *testing.T) {
	for _, entry := range []string{"buff", "camp"} {
		t.Run(entry, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g, ui := h.g, h.ui
			g.menuOpen = false
			fp := installFakePointer(t)
			prevCursor := rawCursorPosition
			rawCursorPosition = func() (int, int) { return fp.x, fp.y }
			t.Cleanup(func() { rawCursorPosition = prevCursor })
			var want character.CardRows
			if entry == "buff" {
				g.showPartyStats = true
				equipSpellAndPrepareCaster(t, g.combat, "bless", 500, 40)
				if !g.combat.CastEquippedSpell() {
					t.Fatal("fixture: Bless failed")
				}
				rail, ok := g.spellStatusRail()
				if !ok || len(rail.statuses) != 1 {
					t.Fatal("fixture: status rail missing")
				}
				want, _ = g.buffStatusCardRows(rail.statuses[0])
				icon := rail.icons[0]
				fp.moveTo(icon.x+icon.w/2, icon.y+icon.h/2)
			} else {
				bar, ok := inGameActionBarLayout(g)
				if !ok {
					t.Fatal("fixture: action bar missing")
				}
				fp.moveTo(bar.camp.x+bar.camp.w/2, bar.camp.y+bar.camp.h/2)
				want, _ = g.campStatusCardRows()
			}
			ui.Draw(h.screen)
			if !reflect.DeepEqual(ui.tooltipRows, want) || len(want) == 0 {
				t.Fatalf("%s status hover discarded authored roles", entry)
			}
		})
	}
}

func TestTypedSetOverridesPreserveOtherRowStyles(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	hero := cs.game.party.Members[0]
	armor := items.CreateItemFromYAML("golden_armor")
	sword := items.CreateWeaponFromYAML("gold_sword")
	items.EnsureInstanceID(&armor)
	items.EnsureInstanceID(&sword)
	hero.Equipment = map[items.EquipSlot]items.Item{items.SlotArmor: armor, items.SlotMainHand: sword}
	rows := GetItemTooltipRows(armor, hero, cs, true)
	rows = append(rows, character.CardRow{Text: "SET", Kind: character.CardRowResult, Section: "DEFENSE"})
	ui := &UISystem{game: cs.game}
	ui.queueItemTooltip(rows, armor, hero, 0, 0)
	highlighted := 0
	for i, row := range rows {
		if len(ui.tooltipColors) != len(rows) {
			t.Fatal("fixture: active set was not highlighted")
		}
		override := ui.tooltipColors[i]
		if row.Section == equipmentSetSectionTitle && row.Kind == character.CardRowResult {
			if override != equipmentBenefitColor {
				t.Fatal("set result lost its benefit ink")
			}
			highlighted++
		} else if override != nil {
			t.Fatalf("set override replaced role %v's default ink on %q", row.Kind, row.Text)
		}
	}
	if highlighted == 0 {
		t.Fatal("fixture: no set results")
	}
}

func TestTypedTooltipFallbackRoles(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	for _, tc := range []struct {
		name string
		item items.Item
		want []character.CardRowKind
	}{
		{"unknown weapon", items.Item{Name: "Lost relic", Type: items.ItemWeapon}, []character.CardRowKind{character.CardRowTitle}},
		{"unknown spell", items.Item{Name: "Lost spell", Type: items.ItemBattleSpell, SpellEffect: "unknown_spell", SpellCost: 5}, []character.CardRowKind{character.CardRowTitle, character.CardRowCategory, character.CardRowResult}},
		{"legacy item type", items.Item{Name: "Lost item", Type: items.ItemType(-1), Description: "DAMAGE"}, []character.CardRowKind{character.CardRowTitle, character.CardRowCategory, character.CardRowSpacer, character.CardRowFlavor}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := GetItemTooltipRows(tc.item, nil, nil, true)
			kinds := make([]character.CardRowKind, len(rows))
			for i, row := range rows {
				kinds[i] = row.Kind
			}
			if !reflect.DeepEqual(kinds, tc.want) {
				t.Fatalf("fallback roles %v, want %v", kinds, tc.want)
			}
			if rows.String() != GetItemTooltip(tc.item, nil, nil, true) {
				t.Fatal("fallback lost its text projection")
			}
		})
	}
	rows := GetSpellTooltipRows("unknown_spell", nil, nil, true)
	if len(rows) != 1 || rows[0].Kind != character.CardRowTitle {
		t.Fatal("unknown spellbook entry lost its title role")
	}
}
