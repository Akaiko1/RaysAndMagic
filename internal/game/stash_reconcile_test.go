package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// The stash is the authority: an item whose nonzero instance id a chest or card
// vault slot holds is stripped on load wherever the party keeps it, so reloading
// a save can't re-deposit that copy. A foreign id survives, and untracked
// (zero-id) items never match - a chest zero must not match a bag zero, or
// legacy items would vanish. A neighbour with its own id always stays.
func TestReconcileAgainstStashStripsOwnedIDs(t *testing.T) {
	type location struct {
		name  string
		place func(g *MMGame, it, neighbour items.Item)
		holds func(g *MMGame, name string) bool
	}
	bagHolds := func(bag []items.Item, name string) bool {
		for _, it := range bag {
			if it.Name == name {
				return true
			}
		}
		return false
	}
	locations := []location{
		{"party bag",
			func(g *MMGame, it, n items.Item) { g.party.Inventory = []items.Item{it, n} },
			func(g *MMGame, name string) bool { return bagHolds(g.party.Inventory, name) }},
		{"member bag",
			func(g *MMGame, it, n items.Item) { g.party.Members[0].Inventory = []items.Item{it, n} },
			func(g *MMGame, name string) bool { return bagHolds(g.party.Members[0].Inventory, name) }},
		{"equipped",
			func(g *MMGame, it, n items.Item) {
				g.party.Members[0].Equipment[items.SlotMainHand] = it
				g.party.Members[0].Equipment[items.SlotArmor] = n
			},
			func(g *MMGame, name string) bool {
				for _, it := range g.party.Members[0].Equipment {
					if it.Name == name {
						return true
					}
				}
				return false
			}},
		{"reserve quick slot",
			func(g *MMGame, it, n items.Item) {
				g.party.Reserve[0].QuickSlots[0], g.party.Reserve[0].QuickSlots[1] = &it, &n
			},
			func(g *MMGame, name string) bool {
				for _, it := range g.party.Reserve[0].QuickSlots {
					if it != nil && it.Name == name {
						return true
					}
				}
				return false
			}},
		{"card collection",
			func(g *MMGame, it, n items.Item) {
				g.cardSlots[0] = cardSlot{key: "fixture_card", item: it}
				g.cardSlots[1] = cardSlot{key: "neighbour_card", item: n}
			},
			func(g *MMGame, name string) bool {
				for _, slot := range g.cardSlots {
					if slot.item.Name == name {
						return slot.key != ""
					}
				}
				return false
			}},
	}
	for _, loc := range locations {
		for _, vault := range []bool{false, true} {
			for _, id := range []string{"owned", "foreign", "zero"} {
				t.Run(fmt.Sprintf("%s/vault=%v/%s", loc.name, vault, id), func(t *testing.T) {
					member := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{}}
					reserve := &character.MMCharacter{Equipment: map[items.EquipSlot]items.Item{}}
					g := &MMGame{
						party: &character.Party{Members: []*character.MMCharacter{member}, Reserve: []*character.MMCharacter{reserve}},
						stash: &stash.Stash{},
					}
					itemType := items.ItemWeapon
					if vault {
						itemType = items.ItemCard
					}
					partyID, stashID := uint64(100), uint64(100)
					switch id {
					case "foreign":
						stashID = 300
					case "zero":
						partyID, stashID = 0, 0
					}
					it := items.Item{Name: "Fixture Item", Type: itemType, InstanceID: partyID}
					neighbour := items.Item{Name: "Neighbour Item", Type: itemType, InstanceID: 200}
					loc.place(g, it, neighbour)
					stashed := items.Item{Name: "Fixture Item", Type: itemType, InstanceID: stashID}
					if vault {
						g.stash.CardSlots[0] = stashed
					} else {
						g.stash.Slots[0] = stashed
					}

					g.reconcilePartyAgainstStash()

					wantKept := id != "owned"
					if got := loc.holds(g, it.Name); got != wantKept {
						t.Fatalf("item kept = %v, want %v", got, wantKept)
					}
					if !loc.holds(g, neighbour.Name) {
						t.Fatal("an item the stash does not own was stripped")
					}
				})
			}
		}
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
