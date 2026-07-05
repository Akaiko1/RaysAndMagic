package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// The chest is the authority: a bag item whose instance id the chest already
// holds is stripped on load, so reloading a save can't re-deposit that copy.
func TestReconcileAgainstStash_StripsOwnedInstance(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{
			{Name: "Muramasa", InstanceID: 100},
			{Name: "Shield", InstanceID: 200},
		}},
		stash: &stash.Stash{},
	}
	g.stash.Slots[0] = items.Item{Name: "Muramasa", InstanceID: 100} // chest owns #100

	g.reconcilePartyAgainstStash()

	if len(g.party.Inventory) != 1 || g.party.Inventory[0].InstanceID != 200 {
		t.Fatalf("expected only #200 kept, got %+v", g.party.Inventory)
	}
}

// A save written while the item was still EQUIPPED (or in a quick slot) must
// not smuggle a chest-owned copy back in — the sweep covers all rosters'
// slots, not just the bag.
func TestReconcileAgainstStash_StripsEquippedAndQuickSlot(t *testing.T) {
	active := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{
		items.SlotMainHand: {Name: "Muramasa", InstanceID: 100},
		items.SlotArmor:    {Name: "Leather", InstanceID: 300},
	}}
	reserve := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{}}
	reserve.QuickSlots[0] = &items.Item{Name: "Potion", InstanceID: 400}
	g := &MMGame{
		party: &character.Party{
			Members: []*character.MMCharacter{active},
			Reserve: []*character.MMCharacter{reserve},
		},
		stash: &stash.Stash{},
	}
	g.stash.Slots[0] = items.Item{Name: "Muramasa", InstanceID: 100}
	g.stash.Slots[1] = items.Item{Name: "Potion", InstanceID: 400}

	g.reconcilePartyAgainstStash()

	if _, has := active.Equipment[items.SlotMainHand]; has {
		t.Error("chest-owned equipped weapon should be stripped")
	}
	if _, has := active.Equipment[items.SlotArmor]; !has {
		t.Error("unrelated equipment must stay")
	}
	if reserve.QuickSlots[0] != nil {
		t.Error("chest-owned quick-slot item on a RESERVE member should be stripped")
	}
}

// Untracked (zero-id) items are never stripped — a chest zero must not match a
// bag zero, or legacy items would wrongly vanish.
func TestReconcileAgainstStash_IgnoresZeroIDs(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{
			{Name: "Old Sword"}, // id 0
			{Name: "Old Shield"}, // id 0
		}},
		stash: &stash.Stash{},
	}
	g.stash.Slots[0] = items.Item{Name: "Old Relic"} // id 0 in the chest too

	g.reconcilePartyAgainstStash()

	if len(g.party.Inventory) != 2 {
		t.Fatalf("zero-id bag items must never be stripped, got %+v", g.party.Inventory)
	}
}

// Card-vault slots count as owned too (cards deposit into a separate vault).
func TestReconcileAgainstStash_CardVaultOwns(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{
			{Name: "Medusa Card", Type: items.ItemCard, InstanceID: 77},
		}},
		stash: &stash.Stash{},
	}
	g.stash.CardSlots[0] = items.Item{Name: "Medusa Card", Type: items.ItemCard, InstanceID: 77}

	g.reconcilePartyAgainstStash()

	if len(g.party.Inventory) != 0 {
		t.Fatalf("card owned by the vault should be stripped, got %+v", g.party.Inventory)
	}
}

// Migration: legacy zero-id party items get stamped, and the change is reported
// so the loader can persist the slot once.
func TestStampPartyInstanceIDs(t *testing.T) {
	m := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{
		items.SlotMainHand: {Name: "Blade"}, // id 0
	}}
	m.QuickSlots[0] = &items.Item{Name: "Potion"} // id 0
	g := &MMGame{party: &character.Party{
		Inventory: []items.Item{{Name: "Gem"}}, // id 0
		Members:   []*character.MMCharacter{m},
	}}

	if !g.stampPartyInstanceIDs() {
		t.Fatal("stamping legacy items should report a change")
	}
	if g.party.Inventory[0].InstanceID == 0 {
		t.Error("bag item not stamped")
	}
	if m.Equipment[items.SlotMainHand].InstanceID == 0 {
		t.Error("equipment not stamped")
	}
	if m.QuickSlots[0].InstanceID == 0 {
		t.Error("quick slot not stamped")
	}

	// Idempotent: a second pass finds nothing to stamp.
	if g.stampPartyInstanceIDs() {
		t.Error("second stamp pass should report no change")
	}
}
