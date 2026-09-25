package game

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

// These cells exercise actual party restoration and read-only browsing with
// the same save. Physical/legacy precedence and global stash claims must agree.
func TestSavedCardPreviewMatchesRestore(t *testing.T) {
	loadTestConfig(t)
	card := items.CreateItemFromYAML("troll_card")
	card.InstanceID = 123
	other := items.CreateItemFromYAML("puma_card")
	other.InstanceID = 456
	oldStashCard := card
	oldStashCard.Type, oldStashCard.Quantity = items.ItemTrinket, 2
	legacy := card
	legacy.Type = items.ItemTrinket
	legacy.InstanceID = 0
	forged := items.CreateItemFromYAML("granite")
	forged.Type = items.ItemCard
	many := make([]items.Item, MaxCardSlots+2)
	for i := range many {
		many[i] = card
		many[i].InstanceID = uint64(1000 + i)
	}
	legacyMany := make([]string, MaxCardSlots+2)
	for i := range legacyMany {
		legacyMany[i] = "troll_card"
	}
	for _, tc := range []struct {
		name  string
		save  PartySave
		owned items.Item
		want  map[int]string
	}{
		{"empty", PartySave{}, items.Item{}, nil},
		{"physical", PartySave{CardCollectionItems: []items.Item{card}}, items.Item{}, map[int]string{0: card.Name}},
		{"legacy", PartySave{CardCollection: []string{"troll_card"}}, items.Item{}, map[int]string{0: card.Name}},
		{"mixed", PartySave{CardCollectionItems: []items.Item{other}, CardCollection: []string{"troll_card", "troll_card"}}, items.Item{}, map[int]string{0: other.Name, 1: card.Name}},
		{"noncard legacy", PartySave{CardCollection: []string{"granite"}}, items.Item{}, nil},
		{"missing legacy", PartySave{CardCollection: []string{"removed_card"}}, items.Item{}, nil},
		{"forged type", PartySave{CardCollectionItems: []items.Item{forged}}, items.Item{}, nil},
		{"invalid physical fallback", PartySave{CardCollectionItems: []items.Item{forged}, CardCollection: []string{"troll_card"}}, items.Item{}, map[int]string{0: card.Name}},
		{"old card type", PartySave{CardCollectionItems: []items.Item{legacy}}, items.Item{}, map[int]string{0: card.Name}},
		{"physical stash owned", PartySave{CardCollectionItems: []items.Item{card}}, card, nil},
		{"legacy stash owned", PartySave{CardCollection: []string{"troll_card"}}, card, nil},
		{"same key different identity", PartySave{CardCollectionItems: []items.Item{legacy}}, card, map[int]string{0: card.Name}},
		{"old stash type", PartySave{Inventory: []items.Item{card}, CardCollectionItems: []items.Item{card}}, oldStashCard, map[int]string{0: card.Name}},
		{"claims consumed by bag", PartySave{Inventory: []items.Item{card}, CardCollectionItems: []items.Item{card}}, card, map[int]string{0: card.Name}},
		{"physical limit", PartySave{CardCollectionItems: many}, items.Item{}, map[int]string{0: card.Name, 1: card.Name, 2: card.Name, 3: card.Name, 4: card.Name, 5: card.Name, 6: card.Name, 7: card.Name}},
		{"legacy limit", PartySave{CardCollection: legacyMany}, items.Item{}, map[int]string{0: card.Name, 1: card.Name, 2: card.Name, 3: card.Name, 4: card.Name, 5: card.Name, 6: card.Name, 7: card.Name}},
	} {
		for _, bank := range []string{"general", "cards"} {
			t.Run(tc.name+"/"+bank, func(t *testing.T) {
				shared := &stash.Stash{}
				if bank == "general" {
					shared.Slots[0] = tc.owned
				} else {
					shared.CardSlots[0] = tc.owned
				}
				before, _ := json.Marshal(tc.save)
				beforeStash, _ := json.Marshal(shared)
				preview := PreviewSavedCardCollection(tc.save, shared)
				after, _ := json.Marshal(tc.save)
				afterStash, _ := json.Marshal(shared)
				if !bytes.Equal(before, after) || !bytes.Equal(beforeStash, afterStash) {
					t.Fatal("preview mutated save/stash inputs")
				}
				var copySave PartySave
				if err := json.Unmarshal(before, &copySave); err != nil {
					t.Fatal(err)
				}
				g, wm, _ := travelFixture(t)
				g.stash = shared
				saved := g.buildSave(wm)
				saved.Party = copySave
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				for i, it := range preview {
					actual := g.cardSlots[i].item
					if it.Name != tc.want[i] || actual.Name != tc.want[i] {
						t.Fatalf("slot %d preview=%q loaded=%q want=%q", i, it.Name, actual.Name, tc.want[i])
					}
					if it.Name != "" && (it.Type != items.ItemCard || it.InstanceID == 0) {
						t.Fatalf("unresolved card: %+v", it)
					}
					if i < len(tc.save.CardCollectionItems) && tc.save.CardCollectionItems[i].InstanceID != 0 && it.Name == tc.save.CardCollectionItems[i].Name && it.InstanceID != actual.InstanceID {
						t.Fatal("physical identity changed")
					}
				}
			})
		}
	}
}

func TestReadStashSnapshotPreservesPendingTransfer(t *testing.T) {
	loadTestConfig(t)
	for _, state := range []string{"none", "uncommitted", "committed", "bad journal"} {
		t.Run(state, func(t *testing.T) {
			storage.SetDataRootForTesting(t.TempDir())
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			before := stash.Stash{}
			before.CardSlots[0] = items.CreateItemFromYAML("troll_card")
			after := stash.Stash{}
			after.CardSlots[0] = items.CreateItemFromYAML("puma_card")
			if err := stash.Save(&before); err != nil {
				t.Fatal(err)
			}
			journal := &stash.TransferJournal{ID: "preview-transaction", Before: before, After: after}
			if state != "none" {
				if err := stash.SaveTransferJournal(journal); err != nil {
					t.Fatal(err)
				}
			}
			if state == "bad journal" {
				if err := os.WriteFile(storage.AppSavePath("stash-transfer.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if state == "committed" {
				if err := storage.WriteJSONAtomic(saveRowPath(1), &GameSave{StashTransferID: journal.ID}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			paths := []string{storage.AppSavePath("stash.json"), storage.AppSavePath("stash-transfer.json"), saveRowPath(1)}
			contents := make([][]byte, len(paths))
			for i, path := range paths {
				contents[i], _ = os.ReadFile(path)
			}
			snapshot, err := ReadStashSnapshot()
			if state == "bad journal" {
				if err == nil {
					t.Fatal("corrupt journal hidden")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := before.CardSlots[0].Name
				if state == "committed" {
					want = after.CardSlots[0].Name
				}
				if snapshot.CardSlots[0].Name != want {
					t.Fatalf("effective snapshot=%q, want %q", snapshot.CardSlots[0].Name, want)
				}
			}
			for i, path := range paths {
				data, _ := os.ReadFile(path)
				if !bytes.Equal(data, contents[i]) {
					t.Fatalf("preview wrote %s", path)
				}
			}
			if state != "bad journal" {
				g := &MMGame{}
				if !g.ensureStashLoaded() {
					t.Fatal("game recovery failed")
				}
				if g.stash.CardSlots[0].Name != snapshot.CardSlots[0].Name {
					t.Fatal("preview/recovery disagree")
				}
			}
		})
	}
}
