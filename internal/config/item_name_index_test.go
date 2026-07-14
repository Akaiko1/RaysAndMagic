package config

import "testing"

// TestItemNameIndexRoundTrip guards the O(1) display-name index behind
// GetItemDefinitionByName: for EVERY item, looking it up by its Name must
// return that item's own definition and key. This pins two properties the
// card system (and every by-name lookup) depends on:
//  1. the index is fully populated at load (no missing entries), and
//  2. no two items share a Name - a duplicate would silently collapse to one
//     key in the map and break whichever item lost the race.
func TestItemNameIndexRoundTrip(t *testing.T) {
	cfg, err := LoadItemConfig("../../assets/items.yaml")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	if len(cfg.Items) == 0 {
		t.Fatal("no items loaded")
	}
	for key, def := range cfg.Items {
		gotDef, gotKey, ok := GetItemDefinitionByName(def.Name)
		if !ok {
			t.Errorf("item %q (name %q) is not resolvable by name", key, def.Name)
			continue
		}
		if gotKey != key {
			t.Errorf("name %q resolved to key %q, want %q - duplicate item name?", def.Name, gotKey, key)
		}
		if gotDef != def {
			t.Errorf("name %q resolved to a different definition than its own", def.Name)
		}
	}
}
