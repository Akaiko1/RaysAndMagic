package stash_test

import (
	"fmt"
	"os"
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

// freshSaveDir gives each test its own empty save dir.
func freshSaveDir(t *testing.T) {
	t.Helper()
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
}

func TestStash_RoundTrip(t *testing.T) {
	freshSaveDir(t)

	// A fresh load with no file yields an empty stash, not an error.
	s, err := stash.Load()
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	for i, slot := range s.Slots {
		if !stash.IsEmpty(slot) {
			t.Fatalf("slot %d should start empty, got %q", i, slot.Name)
		}
	}

	s.Slots[0] = items.Item{Name: "Belt of Strength", Type: items.ItemAccessory, Attributes: map[string]int{"bonus_might": 5}}
	s.Slots[7] = items.Item{Name: "Health Potion", Type: items.ItemConsumable, Attributes: map[string]int{"heal_base": 20}}
	if err := stash.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	// A new process (fresh Load) sees the same items - the cross-save guarantee.
	got, err := stash.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Slots[0].Name != "Belt of Strength" || got.Slots[0].Attributes["bonus_might"] != 5 {
		t.Errorf("slot 0 = %+v, want Belt of Strength/might 5", got.Slots[0])
	}
	if got.Slots[7].Name != "Health Potion" || got.Slots[7].Attributes["heal_base"] != 20 {
		t.Errorf("slot 7 = %+v, want Health Potion/heal 20", got.Slots[7])
	}
	if !stash.IsEmpty(got.Slots[3]) {
		t.Errorf("slot 3 should still be empty, got %q", got.Slots[3].Name)
	}
}

// A corrupted/partially-written stash.json must return an ERROR, not an empty
// stash - otherwise the UI opens a blank chest and the next save overwrites the
// file, permanently losing the deposited items.
func TestStash_CorruptFileErrors(t *testing.T) {
	freshSaveDir(t)

	s := &stash.Stash{}
	s.Slots[0] = items.Item{Name: "Belt of Strength", Type: items.ItemAccessory}
	if err := stash.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Overwrite with invalid JSON, as an interrupted write would leave behind.
	if err := os.WriteFile(storage.AppSavePath("stash.json"), []byte(`{"slots": [`), 0644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := stash.Load(); err == nil {
		t.Fatal("Load on corrupt stash.json should error, not return an empty stash")
	}
}

// A full chest keeps every general and card slot across a reload.
func TestStash_FullChestKeepsEverySlot(t *testing.T) {
	freshSaveDir(t)

	s := &stash.Stash{}
	for i := range s.Slots {
		s.Slots[i] = items.Item{Name: fmt.Sprintf("Item %d", i), Type: items.ItemAccessory, InstanceID: uint64(100 + i)}
	}
	for i := range s.CardSlots {
		s.CardSlots[i] = items.Item{Name: fmt.Sprintf("Card %d", i), Type: items.ItemCard, InstanceID: uint64(200 + i)}
	}
	if err := stash.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := stash.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for i, want := range s.Slots {
		if got.Slots[i].Name != want.Name || got.Slots[i].InstanceID != want.InstanceID {
			t.Errorf("slot %d = %+v, want %+v", i, got.Slots[i], want)
		}
	}
	for i, want := range s.CardSlots {
		if got.CardSlots[i].Name != want.Name || got.CardSlots[i].Type != want.Type || got.CardSlots[i].InstanceID != want.InstanceID {
			t.Errorf("card slot %d = %+v, want %+v", i, got.CardSlots[i], want)
		}
	}
}
