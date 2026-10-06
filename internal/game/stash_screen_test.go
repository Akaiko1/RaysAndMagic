package game

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

func stashTestGame(t *testing.T) *MMGame {
	t.Helper()
	storage.SetDataRootForTesting(t.TempDir()) // a fresh stash.json per test
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	return &MMGame{party: &character.Party{}, stash: &stash.Stash{}, stashDragFrom: -1}
}

// TestStashTransfer_DepositWithdraw covers the core mutation: a bag item dragged
// into a chest cell leaves the bag and lands in the chest, and dragging it back
// returns it to the bag and empties the cell.
func TestStashTransfer_DepositWithdraw(t *testing.T) {
	g := stashTestGame(t)
	g.party.AddItem(items.Item{Name: "Belt of Strength", Type: items.ItemAccessory})

	// Deposit: bag index 0 -> chest cell 2.
	g.stashDragFrom = stashDragInvBase + 0
	g.resolveStashDrop(stashAddr{stashKindChest, 2})
	if len(g.party.Inventory) != 0 {
		t.Fatalf("bag should be empty after deposit, has %d", len(g.party.Inventory))
	}
	if g.stash.Slots[2].Name != "Belt of Strength" {
		t.Fatalf("chest cell 2 = %q, want Belt of Strength", g.stash.Slots[2].Name)
	}

	// Withdraw: chest cell 2 -> bag.
	g.stashDragFrom = 2
	g.resolveStashDrop(stashAddr{stashKindBag, 0})
	if !stash.IsEmpty(g.stash.Slots[2]) {
		t.Fatalf("chest cell 2 should be empty after withdraw, got %q", g.stash.Slots[2].Name)
	}
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Name != "Belt of Strength" {
		t.Fatalf("bag should hold the withdrawn item, got %+v", g.party.Inventory)
	}
}

// A partial-stack deposit moves the dragged units under their own lineage and
// rekeys the bag remainder, whatever the target cell already holds.
func TestStashTransferPartialStackLineage(t *testing.T) {
	potion := func(quantity int, id uint64) items.Item {
		return items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: quantity, InstanceID: id}
	}
	for _, tc := range []struct {
		name         string
		bag, split   int
		cell         items.Item // the chest cell before the drop
		wantCount    int
		wantID       uint64
		wantLineage  []items.StackLineage // nil = not checked
		wantBagCount int
	}{
		{name: "empty cell", bag: 5, split: 2, wantCount: 2, wantID: 100, wantBagCount: 3},
		{name: "different lineage", bag: 3, split: 1, cell: potion(1, 200), wantCount: 2, wantID: 200,
			wantLineage: []items.StackLineage{{ID: 200, Quantity: 1}, {ID: 100, Quantity: 1}}, wantBagCount: 2},
		{name: "same lineage", bag: 4, split: 2, cell: potion(1, 100), wantCount: 3, wantID: 100, wantBagCount: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := stashTestGame(t)
			g.party.Inventory = []items.Item{potion(tc.bag, 100)}
			g.stash.Slots[0] = tc.cell
			g.stashDragFrom = stashDragInvBase
			g.stashDragSplitQuantity = tc.split
			g.stashDragItem = potion(tc.split, 100)
			g.resolveStashDrop(stashAddr{stashKindChest, 0})

			got := g.stash.Slots[0]
			if got.Count() != tc.wantCount || got.InstanceID != tc.wantID {
				t.Fatalf("stash stack = %+v, want %d units with ID %d", got, tc.wantCount, tc.wantID)
			}
			if tc.wantLineage != nil && !reflect.DeepEqual(got.StackLineageParts(), tc.wantLineage) {
				t.Fatalf("merged stash provenance = %+v, want %+v", got.StackLineageParts(), tc.wantLineage)
			}
			if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != tc.wantBagCount || g.party.Inventory[0].InstanceID == 100 {
				t.Fatalf("bag remainder = %+v, want %d rekeyed units", g.party.Inventory, tc.wantBagCount)
			}
		})
	}
}

// TestStashTransfer_DepositSwap covers dropping a bag item onto an OCCUPIED chest
// cell: the new item takes the cell and the displaced item returns to the bag.
func TestStashTransfer_DepositSwap(t *testing.T) {
	g := stashTestGame(t)
	g.stash.Slots[0] = items.Item{Name: "Old Item", Type: items.ItemAccessory}
	g.party.AddItem(items.Item{Name: "New Item", Type: items.ItemAccessory})

	g.stashDragFrom = stashDragInvBase + 0
	g.resolveStashDrop(stashAddr{stashKindChest, 0})

	if g.stash.Slots[0].Name != "New Item" {
		t.Errorf("chest cell 0 = %q, want New Item", g.stash.Slots[0].Name)
	}
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Name != "Old Item" {
		t.Errorf("displaced item should return to bag, got %+v", g.party.Inventory)
	}
}

// TestStashTransfer_ChestToChestSwap covers reordering within the chest.
func TestStashTransfer_ChestToChestSwap(t *testing.T) {
	g := stashTestGame(t)
	g.stash.Slots[1] = items.Item{Name: "A", Type: items.ItemAccessory}
	g.stash.Slots[4] = items.Item{Name: "B", Type: items.ItemAccessory}

	g.stashDragFrom = 1
	g.resolveStashDrop(stashAddr{stashKindChest, 4})

	if g.stash.Slots[1].Name != "B" || g.stash.Slots[4].Name != "A" {
		t.Errorf("swap failed: slot1=%q slot4=%q, want B / A", g.stash.Slots[1].Name, g.stash.Slots[4].Name)
	}
}

// TestStashCardSlot_OnlyCards covers the card-only bank: a monster card deposits
// into a card slot, but a non-card is refused (stays in the bag, slot untouched).
func TestStashCardSlot_OnlyCards(t *testing.T) {
	g := stashTestGame(t)
	g.party.AddItem(items.Item{Name: "Medusa Card", Type: items.ItemCard})
	g.party.AddItem(items.Item{Name: "Belt of Strength", Type: items.ItemAccessory})

	// Card -> card slot 0: accepted.
	g.stashDragFrom = stashDragInvBase + 0
	g.resolveStashDrop(stashAddr{stashKindCard, 0})
	if g.stash.CardSlots[0].Name != "Medusa Card" {
		t.Fatalf("card slot 0 = %q, want Medusa Card", g.stash.CardSlots[0].Name)
	}

	// Non-card -> card slot 1: rejected. The belt is now bag index 0 (the card left).
	g.stashDragFrom = stashDragInvBase + 0
	g.resolveStashDrop(stashAddr{stashKindCard, 1})
	if !stash.IsEmpty(g.stash.CardSlots[1]) {
		t.Errorf("card slot 1 should stay empty for a non-card, got %q", g.stash.CardSlots[1].Name)
	}
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Name != "Belt of Strength" {
		t.Errorf("rejected non-card must stay in the bag, got %+v", g.party.Inventory)
	}

	// Withdraw the card back to the bag.
	g.stashDragFrom = stashCardDragBase + 0
	g.resolveStashDrop(stashAddr{stashKindBag, 0})
	if !stash.IsEmpty(g.stash.CardSlots[0]) {
		t.Errorf("card slot 0 should be empty after withdraw, got %q", g.stash.CardSlots[0].Name)
	}
}

// TestSaveRowModel pins the save-row table: the Autosave and Quicksave rows
// are written by the game and load-only in the menus; manual rows keep their
// old saveN.json files and slot numbers.
func TestSaveRowModel(t *testing.T) {
	for _, tc := range []struct {
		row      int
		label    string
		file     string
		loadOnly bool
	}{
		{0, "Autosave", "autosave.json", true},
		{1, "Quicksave", "quicksave.json", true},
		{2, "Slot 1", "save1.json", false},
		{3, "Slot 2", "save2.json", false},
		{21, "Slot 20", "save20.json", false},
		{saveRowCount - 1, fmt.Sprintf("Slot %d", saveRowCount-2), fmt.Sprintf("save%d.json", saveRowCount-2), false},
	} {
		if got := saveRowLabel(tc.row); got != tc.label {
			t.Errorf("row %d label = %q, want %q", tc.row, got, tc.label)
		}
		if got, want := saveRowPath(tc.row), storage.AppSavePath(tc.file); got != want {
			t.Errorf("row %d path = %q, want %q", tc.row, got, want)
		}
		if got := saveRowIsLoadOnly(tc.row); got != tc.loadOnly {
			t.Errorf("row %d load-only = %v, want %v", tc.row, got, tc.loadOnly)
		}
		if !saveRowIsSlot(tc.row) {
			t.Errorf("row %d is not a menu row", tc.row)
		}
	}
	// Every existing save1..save21 file stays reachable from a menu row.
	for n := 1; n <= 21; n++ {
		found := false
		for row := 0; row < saveRowCount; row++ {
			found = found || saveRowFileName(row) == fmt.Sprintf("save%d.json", n)
		}
		if !found {
			t.Errorf("save%d.json has no menu row", n)
		}
	}
	if saveRowIsSlot(saveRowCount) || saveRowIsSlot(-1) {
		t.Error("rows outside the menus count as slots")
	}
	g := &MMGame{menuState: menuState{savePage: 2, slotSelection: 1}}
	if got, want := g.selectedSaveRow(), 2*saveRowsPerPage+1; got != want {
		t.Errorf("selectedSaveRow = %d, want %d", got, want)
	}
}
