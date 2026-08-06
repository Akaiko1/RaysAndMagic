package game

import (
	"testing"

	"ugataima/internal/items"
)

// Every item type must land on exactly one tab. A type nobody claims would be
// reachable only from All, which is how a filter quietly hides loot.
func TestInventoryTabs_EveryItemTypeIsClaimedOnce(t *testing.T) {
	for typ := items.ItemWeapon; typ <= items.ItemCard; typ++ {
		item := items.Item{Name: "probe", Type: typ}
		owner := inventoryTabOwning(item)
		if owner == inventoryTabAll {
			t.Fatalf("%v (type %d) resolved to the All tab, which owns nothing", typ, typ)
		}
		claims := 0
		for i, tab := range inventoryTabs {
			for _, t2 := range tab.types {
				if t2 == typ {
					claims++
					if i != owner {
						t.Fatalf("%v claimed by tab %q but resolved to %q", typ, tab.label, inventoryTabs[owner].label)
					}
				}
			}
		}
		if claims > 1 {
			t.Fatalf("%v claimed by %d tabs, want at most 1 (catch-all covers the rest)", typ, claims)
		}
		if claims == 0 && owner != inventoryTabCatchAll {
			t.Fatalf("%v is unclaimed but did not fall to the catch-all", typ)
		}
	}
}

// The tabs the user asked for exist, All leads and the catch-all trails.
func TestInventoryTabs_StripShape(t *testing.T) {
	if inventoryTabs[inventoryTabAll].label != "All" {
		t.Fatalf("first tab = %q, want All", inventoryTabs[inventoryTabAll].label)
	}
	if len(inventoryTabs[inventoryTabCatchAll].types) != 0 {
		t.Fatal("the catch-all tab must not pin a type set")
	}
	want := map[string]bool{"All": true, "Weapons": true, "Armor": true, "Consumables": true, "Quest": true, "Loot": true}
	for _, tab := range inventoryTabs {
		if !want[tab.label] {
			t.Fatalf("unexpected tab %q", tab.label)
		}
		delete(want, tab.label)
	}
	if len(want) != 0 {
		t.Fatalf("missing tabs: %v", want)
	}
}

// A filtered cell must carry the item's ABSOLUTE bag index: equip, use, discard,
// drag and the context menu all address party.Inventory directly, so a
// view-relative index would act on the wrong item.
func TestInventoryViewIndices_AreAbsoluteBagPositions(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.party.Inventory = []items.Item{
		{Name: "Sword", Type: items.ItemWeapon},
		{Name: "Potion", Type: items.ItemConsumable},
		{Name: "Axe", Type: items.ItemWeapon},
		{Name: "Gem", Type: items.ItemTrinket},
		{Name: "Plate", Type: items.ItemArmor},
		{Name: "Ring", Type: items.ItemAccessory},
		{Name: "Idol", Type: items.ItemQuest},
		{Name: "Card", Type: items.ItemCard},
		{Name: "Trap", Type: items.ItemTrap},
	}
	byLabel := map[string]int{}
	for i, tab := range inventoryTabs {
		byLabel[tab.label] = i
	}
	for _, tc := range []struct {
		tab  string
		want []int
	}{
		{"All", []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"Weapons", []int{0, 2}},
		{"Armor", []int{4, 5}},
		{"Consumables", []int{1, 8}},
		{"Quest", []int{6}},
		{"Loot", []int{3, 7}},
	} {
		got := g.inventoryViewIndices(byLabel[tc.tab])
		if len(got) != len(tc.want) {
			t.Fatalf("%s view = %v, want %v", tc.tab, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s view = %v, want %v", tc.tab, got, tc.want)
			}
		}
		// Every index must resolve, and to an item the tab really owns.
		for _, idx := range got {
			if idx < 0 || idx >= len(g.party.Inventory) {
				t.Fatalf("%s view index %d is out of the bag", tc.tab, idx)
			}
			if tc.tab != "All" && inventoryTabOwning(g.party.Inventory[idx]) != byLabel[tc.tab] {
				t.Fatalf("%s view holds %q, which belongs to %q",
					tc.tab, g.party.Inventory[idx].Name, inventoryTabs[inventoryTabOwning(g.party.Inventory[idx])].label)
			}
		}
	}
}

// Switching tabs drops everything that addressed the old view.
func TestSetInventoryTab_DropsStaleViewState(t *testing.T) {
	ui := &UISystem{
		inventoryTab:          inventoryTabAll,
		inventoryPage:         3,
		lastClickedItem:       7,
		inventoryContextOpen:  true,
		inventoryContextIndex: 7,
	}
	ui.setInventoryTab(2)
	if ui.inventoryTab != 2 {
		t.Fatalf("tab = %d, want 2", ui.inventoryTab)
	}
	if ui.inventoryPage != 0 {
		t.Fatalf("page = %d, want 0 - the old page indexed the old view", ui.inventoryPage)
	}
	if ui.lastClickedItem != -1 {
		t.Fatalf("double-click chain survived the tab switch: %d", ui.lastClickedItem)
	}
	if ui.inventoryContextOpen || ui.inventoryContextIndex != -1 {
		t.Fatalf("context menu survived the tab switch: open=%v idx=%d",
			ui.inventoryContextOpen, ui.inventoryContextIndex)
	}

	// Re-selecting the same tab is a no-op, so a stray click cannot reset the page.
	ui.inventoryPage = 2
	ui.setInventoryTab(2)
	if ui.inventoryPage != 2 {
		t.Fatalf("re-selecting the active tab reset the page to %d", ui.inventoryPage)
	}
}

// The strip spans the grid exactly, in order, without overlaps - the drawn tab
// IS the hitbox.
func TestInventoryTabRects_TileTheStrip(t *testing.T) {
	const x, y, w = 40, 100, inventoryGridSize
	rects := inventoryTabRects(x, y, w)
	if len(rects) != len(inventoryTabs) {
		t.Fatalf("%d rects for %d tabs", len(rects), len(inventoryTabs))
	}
	if rects[0].x != x {
		t.Fatalf("strip starts at %d, want %d", rects[0].x, x)
	}
	if last := rects[len(rects)-1]; last.right() != x+w {
		t.Fatalf("strip ends at %d, want %d", last.right(), x+w)
	}
	for i, r := range rects {
		if r.y != y || r.h != inventoryTabH {
			t.Fatalf("tab %d = (y %d, h %d), want (%d, %d)", i, r.y, r.h, y, inventoryTabH)
		}
		if label := inventoryTabs[i].label; r.w < debugTextWidth(label) {
			t.Fatalf("tab %q is %dpx wide, narrower than its %dpx label", label, r.w, debugTextWidth(label))
		}
		if i > 0 && r.x != rects[i-1].right() {
			t.Fatalf("tab %d starts at %d, previous ends at %d", i, r.x, rects[i-1].right())
		}
	}
}

// The cell a player clicks must resolve to the item drawn in it, on any page of
// any tab. This is the arithmetic that would silently discard the wrong item.
func TestInventoryCellIndex_ResolvesToTheDrawnItem(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	// 20 weapons interleaved with junk, so the Weapons view spans two pages of
	// 16 and its page-1 cells sit deep in the bag. A fresh party already carries
	// starting kit - clear it so the expected indices are exact.
	g.party.Inventory = nil
	var wantWeapons []int
	for i := 0; i < 20; i++ {
		g.party.Inventory = append(g.party.Inventory,
			items.Item{Name: "Junk", Type: items.ItemTrinket},
			items.Item{Name: "Blade", Type: items.ItemWeapon})
		wantWeapons = append(wantWeapons, len(g.party.Inventory)-1)
	}
	weaponsTab := 0
	for i, tab := range inventoryTabs {
		if tab.label == "Weapons" {
			weaponsTab = i
		}
	}
	view := g.inventoryViewIndices(weaponsTab)
	const pageSize = 16
	if len(view) != len(wantWeapons) {
		t.Fatalf("weapons view = %d entries, want %d", len(view), len(wantWeapons))
	}
	for pos := range view {
		page, slot := pos/pageSize, pos%pageSize
		idx := inventoryCellIndex(view, page, pageSize, slot)
		if idx != wantWeapons[pos] {
			t.Fatalf("page %d cell %d -> bag %d, want %d", page, slot, idx, wantWeapons[pos])
		}
		if got := g.party.Inventory[idx]; got.Type != items.ItemWeapon {
			t.Fatalf("page %d cell %d resolved to %q (%v), not a weapon", page, slot, got.Name, got.Type)
		}
	}
	// Cells past the end of the view are empty, never a wrap onto page 0.
	lastPage := (len(view) - 1) / pageSize
	for slot := len(view) % pageSize; slot < pageSize; slot++ {
		if idx := inventoryCellIndex(view, lastPage, pageSize, slot); idx != -1 {
			t.Fatalf("trailing cell %d resolved to bag %d, want empty", slot, idx)
		}
	}
	if idx := inventoryCellIndex(view, lastPage+1, pageSize, 0); idx != -1 {
		t.Fatalf("cell on a page past the view resolved to bag %d, want empty", idx)
	}
}
