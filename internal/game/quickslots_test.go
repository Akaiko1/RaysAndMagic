package game

import (
	"testing"

	"ugataima/internal/items"
)

// Quick slots: drink/equip dispatch, inventory<->slot drag resolution, and the
// save round-trip. Casting is exercised by the spellbook tests (shared path).
func TestQuickSlots_UseDropAndPersist(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	ch.ActionsRemaining = 9 // turn-based: quick slots require an available action

	// Consumable: double-click drinks one and empties the slot.
	pot := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[0] = &pot
	ch.HitPoints = 1
	game.useQuickSlot(0, 0)
	if ch.HitPoints <= 1 {
		t.Fatalf("potion in quick slot did not heal (HP=%d)", ch.HitPoints)
	}
	if ch.QuickSlots[0] != nil {
		t.Fatalf("consumed potion should leave the slot empty")
	}

	// Weapon: equip from the slot. Stash the starting weapon, clear the hand,
	// then re-equip it via the quick slot.
	cur, had := ch.Equipment[items.SlotMainHand]
	if !had {
		t.Skip("class has no starting main-hand weapon")
	}
	w := cur
	ch.QuickSlots[1] = &w
	delete(ch.Equipment, items.SlotMainHand)
	game.useQuickSlot(0, 1)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatalf("weapon was not equipped from the quick slot")
	}
	if ch.QuickSlots[1] != nil {
		t.Fatalf("equipping into an empty hand should empty the slot")
	}

	// Drag an inventory item into a slot: it leaves the shared bag.
	game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	idx := len(game.party.Inventory) - 1
	bagLen := len(game.party.Inventory)
	game.dragSrc = dragFromInventory
	game.dragInvIndex = idx
	game.resolveQuickSlotDrop(0, 3)
	if ch.QuickSlots[3] == nil {
		t.Fatalf("inventory item was not moved into the quick slot")
	}
	if len(game.party.Inventory) != bagLen-1 {
		t.Fatalf("item should have left the bag (len %d -> %d)", bagLen, len(game.party.Inventory))
	}

	// Drag it back out onto the inventory: slot empties, bag regrows.
	ui := &UISystem{game: game}
	game.dragSrc = dragFromQuickSlot
	game.dragQuickChar = 0
	game.dragQuickSlot = 3
	game.menuOpen = true
	game.dragDropAt = 1
	game.dragCurX, game.dragCurY = 5, 5
	ui.quickInvDropZone(0, 0, 100, 100)
	if ch.QuickSlots[3] != nil {
		t.Fatalf("dragging out should empty the slot")
	}
	if len(game.party.Inventory) != bagLen {
		t.Fatalf("item should be back in the bag")
	}

	// Potions are passive: a quick-slot potion works even on RT cooldown and
	// never spends an action (free, by design - unlike weapons/spells/traps).
	game.turnBasedMode = false
	gated := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[4] = &gated
	ch.HitPoints = 1
	ch.RTCooldown = 10
	game.useQuickSlot(0, 4)
	if ch.HitPoints <= 1 || ch.QuickSlots[4] != nil {
		t.Fatalf("potion quick slot must work even on cooldown (HP=%d, slot nil=%v)", ch.HitPoints, ch.QuickSlots[4] == nil)
	}

	// Quest item (map): opens the overlay even on cooldown, and is NOT consumed.
	mapItem := items.Item{Name: "Old Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}}
	ch.QuickSlots[4] = &mapItem
	ch.RTCooldown = 10 // would block a combat action; a map is not one
	game.mapOverlayOpen = false
	game.useQuickSlot(0, 4)
	if !game.mapOverlayOpen {
		t.Fatalf("map quick slot should open the map overlay")
	}
	if ch.QuickSlots[4] == nil {
		t.Fatalf("a map must not be consumed on use")
	}
	game.mapOverlayOpen = false
	ch.QuickSlots[4] = nil
	ch.RTCooldown = 0

	// Save round-trip preserves a populated slot.
	keep := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[2] = &keep
	rc := restoreCharacterSave(buildCharacterSave(ch))
	if rc.QuickSlots[2] == nil || rc.QuickSlots[2].Name != keep.Name {
		t.Fatalf("quick slot not preserved across save/load")
	}
}

func TestQuickSlotDrop_TakesOnlyDraggedStackUnits(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	game.party.Inventory = []items.Item{{
		Name: "Health Potion", Type: items.ItemConsumable, Quantity: 3, InstanceID: 42,
	}}

	// Shift-drag is represented by a positive partial quantity. It must leave
	// the two remaining potions in the bag and place only one in the quick slot.
	game.dragSrc = dragFromInventory
	game.dragInvIndex = 0
	game.dragSplitQuantity = 1
	game.dragItem = game.party.Inventory[0]
	game.dragItem.Quantity = 1
	game.resolveQuickSlotDrop(0, 0)

	if ch.QuickSlots[0] == nil || ch.QuickSlots[0].Count() != 1 || ch.QuickSlots[0].InstanceID != 42 {
		t.Fatalf("quick slot fragment = %+v, want one original-lineage potion", ch.QuickSlots[0])
	}
	if len(game.party.Inventory) != 1 || game.party.Inventory[0].Count() != 2 || game.party.Inventory[0].InstanceID == 42 {
		t.Fatalf("bag remainder = %+v, want two rekeyed potions", game.party.Inventory)
	}

	// A second partial drop onto the same quick slot merges the quantity there,
	// while leaving one potion in the bag.
	game.dragSrc = dragFromInventory
	game.dragInvIndex = 0
	game.dragSplitQuantity = 1
	game.dragItem = game.party.Inventory[0]
	game.dragItem.Quantity = 1
	game.resolveQuickSlotDrop(0, 0)
	if ch.QuickSlots[0].Count() != 2 || game.party.Inventory[0].Count() != 1 {
		t.Fatalf("second partial drop quick=%+v bag=%+v, want 2 and 1", ch.QuickSlots[0], game.party.Inventory)
	}
}

// Equipping gear via a quick slot is a free swap - it must not spend a turn-based
// action or set a real-time cooldown (only spells/traps are combat actions).
func TestQuickSlot_EquipIsFree(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	cur, had := ch.Equipment[items.SlotMainHand]
	if !had {
		t.Skip("class has no starting main-hand weapon")
	}

	// Turn-based: equipping from a slot must NOT consume an action.
	game.turnBasedMode = true
	ch.ActionsRemaining = 1
	w := cur
	delete(ch.Equipment, items.SlotMainHand)
	ch.QuickSlots[0] = &w
	game.useQuickSlot(0, 0)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatal("weapon was not equipped from the quick slot")
	}
	if ch.ActionsRemaining != 1 {
		t.Errorf("equipping spent a TB action (ActionsRemaining=%d, want 1)", ch.ActionsRemaining)
	}

	// Real-time: equipping must NOT set a cooldown, and works even while on one.
	game.turnBasedMode = false
	w2 := ch.Equipment[items.SlotMainHand]
	delete(ch.Equipment, items.SlotMainHand)
	ch.QuickSlots[1] = &w2
	ch.RTCooldown = 30 // already busy from a swing
	game.useQuickSlot(0, 1)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatal("weapon was not equipped while on cooldown (equip should be free)")
	}
	if ch.RTCooldown != 30 {
		t.Errorf("equipping changed the RT cooldown (RTCooldown=%d, want 30 unchanged)", ch.RTCooldown)
	}
}
