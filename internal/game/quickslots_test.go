package game

import (
	"testing"

	"ugataima/internal/items"
)

// Quick slots: drink/equip dispatch, inventory↔slot drag resolution, and the
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
		t.Fatalf("item should have left the bag (len %d → %d)", bagLen, len(game.party.Inventory))
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

	// Cooldown gate: a character on RT cooldown cannot fire a quick slot.
	game.turnBasedMode = false
	gated := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[4] = &gated
	ch.HitPoints = 1
	ch.RTCooldown = 10
	game.useQuickSlot(0, 4)
	if ch.HitPoints != 1 || ch.QuickSlots[4] == nil {
		t.Fatalf("quick slot must be inert while on cooldown (HP=%d, slot nil=%v)", ch.HitPoints, ch.QuickSlots[4] == nil)
	}
	ch.RTCooldown = 0 // now ready
	game.useQuickSlot(0, 4)
	if ch.HitPoints <= 1 {
		t.Fatalf("quick slot should work once off cooldown")
	}

	// Save round-trip preserves a populated slot.
	keep := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[2] = &keep
	rc := restoreCharacterSave(buildCharacterSave(ch))
	if rc.QuickSlots[2] == nil || rc.QuickSlots[2].Name != keep.Name {
		t.Fatalf("quick slot not preserved across save/load")
	}
}
