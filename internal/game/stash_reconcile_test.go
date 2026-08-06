package game

import (
	"encoding/json"
	"os"
	"path/filepath"
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
// not smuggle a chest-owned copy back in - the sweep covers all rosters'
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

// Untracked (zero-id) items are never stripped - a chest zero must not match a
// bag zero, or legacy items would wrongly vanish.
func TestReconcileAgainstStash_IgnoresZeroIDs(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{
			{Name: "Old Sword"},  // id 0
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

func TestReconcileAgainstStash_PartialStackRekeysSurvivor(t *testing.T) {
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{{
			Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 100,
		}}},
		stash: &stash.Stash{},
	}
	// This is an older save from before two of the five potions were moved into
	// the stash. The chest owns those two units under the original lineage ID.
	g.stash.Slots[0] = items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 2, InstanceID: 100}

	g.reconcilePartyAgainstStash()
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 3 {
		t.Fatalf("partial dedupe left %+v, want three loose potions", g.party.Inventory)
	}
	if g.party.Inventory[0].InstanceID == 100 {
		t.Fatal("surviving partial stack kept the stash-owned lineage ID")
	}
	if !g.loadNeedsResave {
		t.Fatal("partial dedupe must request a migration save after rekeying")
	}

	// The rekey makes a later load idempotent: the same two chest units cannot
	// be subtracted from the remaining three again.
	g.loadNeedsResave = false
	g.reconcilePartyAgainstStash()
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 3 {
		t.Fatalf("second partial dedupe changed the survivor: %+v", g.party.Inventory)
	}
	if g.loadNeedsResave {
		t.Fatal("already-rekeyed survivor must not request another migration save")
	}
}

// A partial chest withdrawal must not hide the returned fragment's provenance
// inside the current bag stack. Otherwise an older five-stack save would no
// longer see the one unit still in the chest and could recreate all five.
func TestReconcileAgainstStash_PartialWithdrawalStillClaimsOldSaveUnits(t *testing.T) {
	original := items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 100}
	bag := original
	deposited, ok := bag.SplitOff(2)
	if !ok {
		t.Fatal("partial deposit split failed")
	}
	chest := deposited
	withdrawn, ok := chest.SplitOffForStashWithdrawal(1)
	if !ok {
		t.Fatal("partial withdrawal split failed")
	}
	if !bag.MergeStack(withdrawn) {
		t.Fatal("withdrawn potion did not merge into current bag stack")
	}
	if got := chest.StackLineageParts(); len(got) != 1 || got[0] != (items.StackLineage{ID: 100, Quantity: 1}) {
		t.Fatalf("chest provenance = %+v, want one original unit", got)
	}
	if got := bag.StackLineageParts(); len(got) != 2 || got[0].ID == 100 || got[1].ID == 100 {
		t.Fatalf("current bag kept chest-owned lineage after withdrawal: %+v", got)
	}

	// Load the stale pre-transfer save. The chest owns one original unit, so
	// exactly one of its five must disappear; the other four remain legitimate.
	g := &MMGame{
		party: &character.Party{Inventory: []items.Item{original}},
		stash: &stash.Stash{},
	}
	g.stash.Slots[0] = chest
	g.reconcilePartyAgainstStash()
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != 4 {
		t.Fatalf("stale save after partial withdrawal = %+v, want four potions", g.party.Inventory)
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

func TestReconcileAgainstStash_StripsOwnedCardCollectionSlot(t *testing.T) {
	g := &MMGame{
		party: &character.Party{},
		stash: &stash.Stash{},
	}
	g.cardSlots[0].key = "medusa_card"
	g.cardSlots[0].item = items.Item{Name: "Medusa Card", Type: items.ItemCard, InstanceID: 77}
	g.stash.CardSlots[0] = items.Item{Name: "Medusa Card", Type: items.ItemCard, InstanceID: 77}

	g.reconcilePartyAgainstStash()

	if g.cardSlots[0].key != "" || g.cardSlots[0].item.Name != "" {
		t.Fatalf("card collection slot should be stripped when vault owns the card, key=%q item=%+v", g.cardSlots[0].key, g.cardSlots[0].item)
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

func TestRecoverPendingStashTransferUsesAnySaveCommitMarker(t *testing.T) {
	for _, tc := range []struct {
		name      string
		markerRow int
	}{
		{name: "rollback", markerRow: -1},
		{name: "autosave commit", markerRow: 0},
		{name: "manual save commit", markerRow: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := stashTestGame(t)
			g.stash = nil // force recovery through ensureStashLoaded
			before := stash.Stash{}
			before.Slots[0] = items.Item{Name: "Before", Type: items.ItemTrinket}
			after := stash.Stash{}
			after.Slots[0] = items.Item{Name: "After", Type: items.ItemTrinket}
			if err := stash.SaveTransferJournal(&stash.TransferJournal{ID: "tx-1", Before: before, After: after}); err != nil {
				t.Fatalf("write journal: %v", err)
			}
			if tc.markerRow >= 0 {
				path := saveRowPath(tc.markerRow)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("make autosave dir: %v", err)
				}
				data, err := json.Marshal(GameSave{StashTransferID: "tx-1"})
				if err != nil {
					t.Fatalf("encode autosave marker: %v", err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatalf("write autosave marker: %v", err)
				}
			}

			if !g.ensureStashLoaded() {
				t.Fatal("journal recovery prevented stash load")
			}
			want := "Before"
			if tc.markerRow >= 0 {
				want = "After"
			}
			if got := g.stash.Slots[0].Name; got != want {
				t.Fatalf("recovered stash item = %q, want %q", got, want)
			}
			if journal, err := stash.LoadTransferJournal(); err != nil || journal != nil {
				t.Fatalf("journal was not cleared after recovery: journal=%+v err=%v", journal, err)
			}
		})
	}
}
