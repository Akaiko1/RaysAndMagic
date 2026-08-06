package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
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
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 2 || g.party.Inventory[0].InstanceID == 42 {
		t.Fatalf("bag fragment = %+v, want two rekeyed withdrawal units", g.party.Inventory)
	}
	if got := g.stash.Slots[0]; got.Count() != 3 || got.InstanceID != 42 {
		t.Fatalf("stash remainder = %+v, want three original-lineage units", got)
	}
}

// escKeyboard builds a key source where only ESC reads as just-pressed, so
// HandleInput's keyboard paths run headlessly.
func escKeyboard() func(ebiten.Key) bool {
	return func(k ebiten.Key) bool { return k == ebiten.KeyEscape }
}

// A picked-up split fragment is input-owning transient state, not a rendered
// modal layer: ESC must cancel the fragment and leave its parent hub open.
// Only a second press (fragment gone) closes the hub as usual.
func TestEscCancelsHubSplitFragmentWithoutClosingHub(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	ih := NewInputHandler(g)
	ih.keys = keytracker.NewWithSource(escKeyboard())

	g.menuOpen = true
	g.party.Inventory = []items.Item{{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 6, InstanceID: 3}}
	g.dragActive, g.dragPickedUp = true, true
	g.dragSrc = dragFromInventory
	g.dragInvIndex = 0
	g.dragSplitQuantity = 2

	ih.HandleInput()
	if g.dragPickedUp {
		t.Fatal("ESC did not cancel the picked-up fragment")
	}
	if !g.menuOpen {
		t.Fatal("ESC closed the hub instead of only cancelling the fragment")
	}

	ih.HandleInput()
	if g.menuOpen {
		t.Fatal("control failed: ESC no longer closes the hub after the fragment is gone")
	}
}

// Same contract on the stash screen: first ESC drops the fragment and keeps the
// screen, the next one closes the screen through its modal-layer case.
func TestEscCancelsStashSplitFragmentBeforeClosingStash(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	ih := NewInputHandler(g)
	ih.keys = keytracker.NewWithSource(escKeyboard())

	g.stashScreenOpen = true
	g.stashDragActive, g.stashDragPickedUp = true, true
	g.stashDragSplitQuantity = 2

	ih.HandleInput()
	if g.stashDragPickedUp {
		t.Fatal("ESC did not cancel the picked-up stash fragment")
	}
	if !g.stashScreenOpen {
		t.Fatal("ESC closed the stash screen instead of only cancelling the fragment")
	}

	ih.HandleInput()
	if g.stashScreenOpen {
		t.Fatal("control failed: ESC no longer closes the stash screen after the fragment is gone")
	}
}
