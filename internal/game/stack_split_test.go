package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestStackSplitPickerStartsExactInventoryFragment(t *testing.T) {
	member := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{}}
	g := &MMGame{
		menuOpen: true,
		party: &character.Party{
			Members: []*character.MMCharacter{member},
			Inventory: []items.Item{{
				Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 42,
			}},
		},
	}
	ui := &UISystem{game: g}
	ui.openStackSplitPicker(stackSplitPickerInventory, 0, g.party.Inventory[0])
	ui.stackSplitPicker.quantity = 3
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		t.Fatal("picker could not resolve its inventory source")
	}
	ui.beginPickedUpStackSplit(item)

	if ui.stackSplitPicker.open || !g.dragPickedUp || g.dragSplitQuantity != 3 || g.dragItem.Count() != 3 {
		t.Fatalf("picker drag state = open:%v picked:%v quantity:%d item:%+v",
			ui.stackSplitPicker.open, g.dragPickedUp, g.dragSplitQuantity, g.dragItem)
	}
	if got := g.party.Inventory[0]; got.Count() != 5 || got.InstanceID != 42 {
		t.Fatalf("picker mutated the source before a drop: %+v", got)
	}

	g.resolveQuickSlotDrop(0, 0)
	if member.QuickSlots[0] == nil || member.QuickSlots[0].Count() != 3 || member.QuickSlots[0].InstanceID != 42 {
		t.Fatalf("quick slot fragment = %+v, want three original-lineage units", member.QuickSlots[0])
	}
	if got := g.party.Inventory[0]; got.Count() != 2 || got.InstanceID == 42 {
		t.Fatalf("bag remainder = %+v, want two rekeyed units", got)
	}
}

func TestCancelPickedUpStackSplitKeepsInventorySource(t *testing.T) {
	g := &MMGame{
		menuOpen: true,
		party: &character.Party{Inventory: []items.Item{{
			Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 42,
		}}},
	}
	ui := &UISystem{game: g}
	g.gameLoop = &GameLoop{ui: ui}
	ui.openStackSplitPicker(stackSplitPickerInventory, 0, g.party.Inventory[0])
	ui.stackSplitPicker.quantity = 2
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		t.Fatal("picker could not resolve its inventory source")
	}
	ui.beginPickedUpStackSplit(item)
	g.cancelStackSplitInteraction()

	if g.stackSplitInteractionActive() || g.dragActive || g.dragPickedUp {
		t.Fatalf("cancel left split interaction active: drag=%v picked=%v", g.dragActive, g.dragPickedUp)
	}
	if got := g.party.Inventory[0]; got.Count() != 5 || got.InstanceID != 42 {
		t.Fatalf("cancel mutated the source: %+v", got)
	}
}

func TestStackSplitPickerStartsExactStashFragment(t *testing.T) {
	g := stashTestGame(t)
	g.stash.Slots[0] = items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 42}
	ui := &UISystem{game: g}
	ui.openStackSplitPicker(stackSplitPickerStash, 0, g.stash.Slots[0])
	ui.stackSplitPicker.quantity = 2
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		t.Fatal("picker could not resolve its stash source")
	}
	ui.beginPickedUpStackSplit(item)

	if ui.stackSplitPicker.open || !g.stashDragPickedUp || g.stashDragSplitQuantity != 2 || g.stashDragItem.Count() != 2 {
		t.Fatalf("picker stash drag = open:%v picked:%v quantity:%d item:%+v",
			ui.stackSplitPicker.open, g.stashDragPickedUp, g.stashDragSplitQuantity, g.stashDragItem)
	}
	if got := g.stash.Slots[0]; got.Count() != 5 || got.InstanceID != 42 {
		t.Fatalf("picker mutated stash source before a drop: %+v", got)
	}

	g.resolveStashDrop(stashAddr{stashKindBag, 0})
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 2 || g.party.Inventory[0].InstanceID != 42 {
		t.Fatalf("bag fragment = %+v, want two original-lineage units", g.party.Inventory)
	}
	if got := g.stash.Slots[0]; got.Count() != 3 || got.InstanceID == 42 {
		t.Fatalf("stash remainder = %+v, want three rekeyed units", got)
	}
}
