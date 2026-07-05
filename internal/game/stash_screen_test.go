package game

import (
	"os"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

func stashTestGame(t *testing.T) *MMGame {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil { // isolate stash.json writes
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
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

// TestSaveRowModel verifies the slot layout: row 0 is the load-only Autosave, the
// rest are manual slots mapped to backward-compatible files, across 3 pages.
func TestSaveRowModel(t *testing.T) {
	if !saveRowIsAutosave(0) {
		t.Error("row 0 must be the autosave slot")
	}
	if saveRowIsAutosave(1) {
		t.Error("row 1 must be a manual slot")
	}
	if got := saveRowLabel(0); got != "Autosave" {
		t.Errorf("row 0 label = %q, want Autosave", got)
	}
	if got := saveRowLabel(3); got != "Slot 3" {
		t.Errorf("row 3 label = %q, want Slot 3", got)
	}
	// Row N (N>=1) keeps the old saveN.json filename so existing saves stay reachable.
	if got, want := saveRowPath(1), slotPath(0); got != want {
		t.Errorf("manual row 1 path = %q, want %q (old slot 0)", got, want)
	}
	// 3 pages x rows-per-page total rows; selected row offsets by page.
	g := &MMGame{savePage: 2, slotSelection: 1}
	if got, want := g.selectedSaveRow(), 2*saveRowsPerPage+1; got != want {
		t.Errorf("selectedSaveRow = %d, want %d", got, want)
	}
	if saveRowCount != saveRowsPerPage*savePageCount {
		t.Errorf("saveRowCount = %d, want %d", saveRowCount, saveRowsPerPage*savePageCount)
	}
}
