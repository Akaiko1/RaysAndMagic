package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestPageCount(t *testing.T) {
	cases := []struct {
		n, size, want int
	}{
		{0, 12, 1}, // empty list still has a valid page 0
		{1, 12, 1},
		{12, 12, 1}, // exactly one full page
		{13, 12, 2}, // one over spills to a second page
		{25, 12, 3},
	}
	for _, c := range cases {
		if got := pageCount(c.n, c.size); got != c.want {
			t.Errorf("pageCount(%d,%d) = %d, want %d", c.n, c.size, got, c.want)
		}
	}
}

func TestClampPage(t *testing.T) {
	cases := []struct {
		page, total, want int
	}{
		{0, 1, 0},
		{5, 3, 2},  // page past the end snaps to the last page
		{-1, 3, 0}, // negative snaps to 0
		{1, 3, 1},  // already valid, unchanged
	}
	for _, c := range cases {
		p := c.page
		clampPage(&p, c.total)
		if p != c.want {
			t.Errorf("clampPage(%d, total=%d) = %d, want %d", c.page, c.total, p, c.want)
		}
	}
}

// merchantCellRect must map each slot to a unique, non-overlapping cell laid out
// in row-major order, so buy/sell clicks resolve to the right item index.
func TestMerchantCellRectLayout(t *testing.T) {
	const baseX, gridTop = 100, 200
	type cell struct{ x, y, w, h int }
	cells := make([]cell, 0, merchantPageSize)
	for slot := 0; slot < merchantPageSize; slot++ {
		x, y, w, h := merchantCellRect(baseX, gridTop, slot)
		if w != merchantIconSize || h != merchantIconSize {
			t.Fatalf("slot %d: size = %dx%d, want %d square", slot, w, h, merchantIconSize)
		}
		for other, c := range cells {
			if x < c.x+c.w && c.x < x+w && y < c.y+c.h && c.y < y+h {
				t.Fatalf("slot %d (%d,%d) overlaps slot %d (%d,%d)", slot, x, y, other, c.x, c.y)
			}
		}
		// Row-major: x grows with column, y with row; both anchored at the grid origin.
		col, row := slot%merchantGridCols, slot/merchantGridCols
		if wantX := baseX + col*(merchantIconSize+merchantIconGapX); x != wantX {
			t.Errorf("slot %d: x = %d, want %d", slot, x, wantX)
		}
		if row == 0 && y != gridTop {
			t.Errorf("slot %d: first-row y = %d, want the grid top %d", slot, y, gridTop)
		}
		if row > 0 && y <= cells[slot-merchantGridCols].y {
			t.Errorf("slot %d: y = %d not below the cell above (%d)", slot, y, cells[slot-merchantGridCols].y)
		}
		cells = append(cells, cell{x, y, w, h})
	}
}

// Filtered views must retain item identity for both click and drag trades.
// A potion precedes the weapon in each underlying list, so a view-relative
// sell index would silently trade the wrong object.
func TestMerchantCategoriesTradeDisplayedItem(t *testing.T) {
	for _, buy := range []bool{false, true} {
		for _, drag := range []bool{false, true} {
			t.Run(fmt.Sprintf("buy=%v/drag=%v", buy, drag), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, ui := h.g, h.ui
				g.dialogActive = true
				g.party.Gold = 1000
				sword := items.CreateWeaponFromYAML("iron_sword")
				sword.Attributes["value"] = 30
				g.party.Inventory = append(g.party.Inventory, sword)
				stock := &character.MerchantStockItem{Item: sword, Cost: 50, Quantity: 4}
				g.dialogNPC.MerchantStock = []*character.MerchantStockItem{potionStock(10, 5), stock}
				dlg := npcDialogLayout(g)
				left, right, top, _ := merchantGridLayout(dlg.x, dlg.y)
				source, target := right, left
				if buy {
					source, target = left, right
				}
				category := inventoryTabOwning(sword)
				filter := inventoryTabRects(source, top-48, merchantGridW)[category]
				before := ui.topModalSnapshot()
				h.clicks(false, filter.x+filter.w/2, filter.y+filter.h/2, 1)
				if ui.topModalSnapshot() == before {
					t.Fatal("category change left the displayed modal identity unchanged")
				}
				if buy && (g.merchantBuyCategory != category || g.merchantSellCategory != 0) ||
					!buy && (g.merchantSellCategory != category || g.merchantBuyCategory != 0) {
					t.Fatal("category click did not filter only its own counter")
				}
				if drag {
					fp := installFakePointer(t)
					fp.moveTo(source+5, top+5)
					fp.press()
					h.pointerStep()
					fp.hold()
					fp.moveTo(target+5, top+5)
					h.pointerStep()
					fp.release()
					h.pointerStep()
					if !ui.stackSplitPicker.open || ui.stackSplitPicker.name != sword.Name || g.party.Gold != 1000 {
						t.Fatal("filtered drag did not stage the displayed weapon without trading")
					}
					confirm := stackSplitPickerLayout(stackSplitPickerRect(1024, 768)).take
					h.clicks(false, confirm.Min.X+5, confirm.Min.Y+5, 1)
				} else {
					h.clicks(false, source+5, top+5, 2)
				}
				wantGold, wantCount := 1030, 0
				if buy {
					wantGold, wantCount = 950, 2
				}
				if g.party.Gold != wantGold || g.party.CountItemsByName(sword.Name) != wantCount || g.party.CountItemsByName("Health Potion") != 5 {
					t.Fatalf("wrong filtered trade: gold=%d weapons=%d potions=%d", g.party.Gold, g.party.CountItemsByName(sword.Name), g.party.CountItemsByName("Health Potion"))
				}
			})
		}
	}
}

func TestMerchantCategoryWithinAuthoredCollection(t *testing.T) {
	g, _ := merchantDragGame(t)
	g.dialogNPC.MerchantStock = []*character.MerchantStockItem{
		{Item: items.Item{Type: items.ItemWeapon}, Tab: "First"},
		{Item: items.Item{Type: items.ItemConsumable}, Tab: "First"},
		{Item: items.Item{Type: items.ItemWeapon}, Tab: "Second"},
	}
	g.switchDialogTab(1)
	g.merchantBuyPage, g.merchantSellPage = 3, 2
	g.dialogLastClickZone, g.dialogLastClickedIdx = "merchant_buy", 0
	g.setMerchantCategory(true, inventoryTabOwning(items.Item{Type: items.ItemWeapon}))
	if got := g.merchantVisibleStock(); len(got) != 1 || got[0] != g.dialogNPC.MerchantStock[2] {
		t.Fatal("category crossed the authored collection boundary")
	}
	if g.merchantBuyPage != 0 || g.merchantSellPage != 2 || g.dialogLastClickedIdx != -1 {
		t.Fatal("category failed to reset its page/click identity or changed the other page")
	}
	g.setMerchantCategory(true, inventoryTabOwning(items.Item{Type: items.ItemArmor}))
	if len(g.merchantVisibleStock()) != 0 {
		t.Fatal("empty category leaked another type")
	}
}
