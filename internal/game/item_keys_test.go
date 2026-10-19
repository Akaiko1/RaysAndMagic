package game

import (
	"testing"

	"ugataima/internal/items"
)

// A save from before item keys loads keyed: the backfill finds the entry by
// key, renamed key, display name or former name, and the stock record by key
// or by the (possibly former) name it held.
func TestItemKeysBackfillOnLoad(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads the catalogs
	for _, tc := range []struct {
		name      string
		saved     items.Item
		wantKey   string
		wantName  string
		wantType  items.ItemType
		unchanged bool
	}{
		{name: "keyless weapon", saved: items.Item{Name: "Iron Sword", Type: items.ItemWeapon}, wantKey: "iron_sword", wantName: "Iron Sword", wantType: items.ItemWeapon},
		{name: "former weapon name", saved: items.Item{Name: "Escapement Mace", Type: items.ItemWeapon}, wantKey: "escapement_staff", wantType: items.ItemWeapon},
		{name: "former item name", saved: items.Item{Name: "Medusa Card", Type: items.ItemCard}, wantKey: "jellyfish_card", wantType: items.ItemCard},
		{name: "former item key", saved: items.Item{Key: "medusa_card", Name: "Medusa Card", Type: items.ItemCard}, wantKey: "jellyfish_card", wantType: items.ItemCard},
		{name: "keyed item", saved: items.Item{Key: "health_potion", Name: "Health Potion", Type: items.ItemConsumable}, wantKey: "health_potion", wantName: "Health Potion", wantType: items.ItemConsumable},
		{name: "weapon key gone, name known", saved: items.Item{Key: "retired_iron_sword", Name: "Iron Sword", Type: items.ItemWeapon}, wantKey: "iron_sword", wantName: "Iron Sword", wantType: items.ItemWeapon},
		{name: "item key gone, name known", saved: items.Item{Key: "retired_health_potion", Name: "Health Potion", Type: items.ItemConsumable}, wantKey: "health_potion", wantName: "Health Potion", wantType: items.ItemConsumable},
		{name: "unknown item", saved: items.Item{Name: "No Such Thing", Type: items.ItemTrinket}, unchanged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := tc.saved
			normalizeItemFromConfig(&it)
			if tc.unchanged {
				if it.Key != "" || it.Name != tc.saved.Name {
					t.Fatalf("unknown item rewritten to %+v", it)
				}
				return
			}
			if it.Key != tc.wantKey || it.Type != tc.wantType || (tc.wantName != "" && it.Name != tc.wantName) {
				t.Fatalf("loaded %q/%q/%v, want %q/%q/%v", it.Key, it.Name, it.Type, tc.wantKey, tc.wantName, tc.wantType)
			}
			if fresh, err := items.FromKey(tc.wantKey); err != nil || fresh.Name != it.Name {
				t.Fatalf("backfilled name %q, catalog %q (%v)", it.Name, fresh.Name, err)
			}
		})
	}

	for _, tc := range []struct {
		saved NPCStockSave
		want  string
	}{
		{NPCStockSave{Key: "iron_sword"}, "iron_sword"},
		{NPCStockSave{Key: "medusa_card"}, "jellyfish_card"},
		{NPCStockSave{Name: "Escapement Mace"}, "escapement_staff"},
		{NPCStockSave{Name: "Health Potion"}, "health_potion"},
		{NPCStockSave{Name: "No Such Thing"}, ""},
	} {
		if got := savedStockKey(tc.saved); got != tc.want {
			t.Errorf("savedStockKey(%+v) = %q, want %q", tc.saved, got, tc.want)
		}
	}
}
