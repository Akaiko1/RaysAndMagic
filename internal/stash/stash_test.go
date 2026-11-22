package stash_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"ugataima/internal/config"
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

// A chest records the tick rate its items' clocks were written at; read at
// another rate they keep their seconds (a chest without a rate ran at the
// shipped one).
func TestStash_TickRateRescalesItemClocks(t *testing.T) {
	var s stash.Stash
	s.Slots[0] = items.Item{Name: "Translocator", DeviceCooldown: items.RoundClock{Frames: 400, Turns: 2, Rate: 360}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	live := config.GetTargetTPS()
	for _, tc := range []struct {
		name, rate string
		want       items.RoundClock
	}{
		{"same rate", fmt.Sprintf(`"tick_rate":%d`, live), items.RoundClock{Frames: 400, Turns: 2, Rate: 360}},
		{"written at twice the rate", fmt.Sprintf(`"tick_rate":%d`, 2*live), items.RoundClock{Frames: 200, Turns: 2, Rate: 180}},
		{"no recorded rate", `"tick_rate":0`, items.RoundClock{Frames: 400, Turns: 2, Rate: 360}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := strings.Replace(string(raw), fmt.Sprintf(`"tick_rate":%d`, live), tc.rate, 1)
			var got stash.Stash
			if err := json.Unmarshal([]byte(data), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Slots[0].DeviceCooldown != tc.want {
				t.Fatalf("device recharge %+v, want %+v", got.Slots[0].DeviceCooldown, tc.want)
			}
		})
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

// A full chest, the legendary collection and the card album all survive a
// reload.
func TestStash_FullChestKeepsEverySlot(t *testing.T) {
	freshSaveDir(t)

	s := &stash.Stash{
		Collection: map[string]items.Item{"amber_talisman": {Name: "Amber Talisman", Type: items.ItemAccessory, InstanceID: 300}},
		Album:      map[string][]items.Item{"rat_card": {{Name: "Sewer Rat Card", Type: items.ItemCard, InstanceID: 400}, {Name: "Sewer Rat Card", Type: items.ItemCard, InstanceID: 401}}},
	}
	for i := range s.Slots {
		s.Slots[i] = items.Item{Name: fmt.Sprintf("Item %d", i), Type: items.ItemAccessory, InstanceID: uint64(100 + i)}
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
	if c := got.Collection["amber_talisman"]; c.InstanceID != 300 {
		t.Errorf("collection stamp = %+v", c)
	}
	if page := got.Album["rat_card"]; len(page) != 2 || page[1].InstanceID != 401 {
		t.Errorf("album page = %+v", page)
	}
}

// The retired 8 + 8 layout (general cells plus a card vault) loads with every
// card moved into the free general cells, from the chest file and from a
// transfer journal written before the change.
func TestStash_OldCardVaultMovesIntoGeneralCells(t *testing.T) {
	freshSaveDir(t)
	old := `{"slots":[{"Name":"Belt of Strength","Type":2,"instance_id":1},{},{"Name":"Health Potion","Type":3,"instance_id":2},{},{},{},{},{}],` +
		`"card_slots":[{"Name":"Sewer Rat Card","Type":9,"instance_id":3},{},{"Name":"Spider Card","Type":9,"instance_id":4},{},{},{},{},{}]}`
	if err := os.WriteFile(storage.AppSavePath("stash.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := stash.Load()
	if err != nil {
		t.Fatalf("load old layout: %v", err)
	}
	var ids []uint64
	got.Each(func(it *items.Item) { ids = append(ids, it.InstanceID) })
	if fmt.Sprint(ids) != "[1 3 2 4]" || got.Slots[0].InstanceID != 1 || got.Slots[1].InstanceID != 3 || got.Slots[2].InstanceID != 2 || got.Slots[3].InstanceID != 4 {
		t.Fatalf("migrated cells = %v (first free cells take the cards in order)", ids)
	}
	journal := `{"id":"7","before":` + old + `,"after":` + old + `}`
	if err := os.WriteFile(storage.AppSavePath("stash-transfer.json"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := stash.LoadTransferJournal()
	if err != nil || j.Before.Slots[1].InstanceID != 3 || j.After.Slots[3].InstanceID != 4 {
		t.Fatalf("old journal did not migrate: %+v, %v", j, err)
	}
}
