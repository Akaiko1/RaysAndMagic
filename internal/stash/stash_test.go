package stash_test

import (
	"os"
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

// chdirTemp points the storage save dir at a throwaway location so tests never
// touch the real saves folder. storage.AppSaveDir falls back to cwd/saves when
// the test binary lives in a go-build temp dir.
func chdirTemp(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestStash_RoundTrip(t *testing.T) {
	chdirTemp(t)

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

	// A new process (fresh Load) sees the same items — the cross-save guarantee.
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
// stash — otherwise the UI opens a blank chest and the next save overwrites the
// file, permanently losing the deposited items.
func TestStash_CorruptFileErrors(t *testing.T) {
	chdirTemp(t)

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

func TestStash_SlotCount(t *testing.T) {
	if stash.SlotCount != 8 {
		t.Errorf("SlotCount = %d, want 8 (per the design)", stash.SlotCount)
	}
}
