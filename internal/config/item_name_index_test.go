package config

import (
	"strings"
	"testing"
)

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

// TestWeaponNameIndexRoundTrip gives saved weapon instances the same identity
// guarantee as items: their display name must deterministically resolve to the
// YAML key used by exact-piece equipment sets and icon lookups.
func TestWeaponNameIndexRoundTrip(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if len(cfg.Weapons) == 0 {
		t.Fatal("no weapons loaded")
	}
	for key, def := range cfg.Weapons {
		gotDef, gotKey, ok := GetWeaponDefinitionByName(def.Name)
		if !ok {
			t.Errorf("weapon %q (name %q) is not resolvable by name", key, def.Name)
			continue
		}
		if gotKey != key {
			t.Errorf("weapon name %q resolved to key %q, want %q - duplicate weapon name?", def.Name, gotKey, key)
		}
		if gotDef != def {
			t.Errorf("weapon name %q resolved to a different definition than its own", def.Name)
		}
	}
}

// Aliases must resolve deterministically and a failed reload must leave the
// current catalog usable by inventories, icons and saved merchant shelves.
func TestItemLegacyNames(t *testing.T) {
	preserveUsageCatalog(t)
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		aliases []string
		other   []string
		valid   bool
	}{
		{"accepted", []string{"Old Dust"}, nil, true},
		{"empty", []string{""}, nil, false},
		{"untrimmed", []string{" Old Dust"}, nil, false},
		{"current name", []string{"Wolf Pelt"}, nil, false},
		{"own name", []string{"Fairy Dust"}, nil, false},
		{"duplicate", []string{"Old Dust", "Old Dust"}, nil, false},
		{"shared alias", []string{"Old Dust"}, []string{"Old Dust"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := usageFixture(t)
			defs := root["items"].(map[string]any)
			defs["fairy_dust"].(map[string]any)["legacy_names"] = tc.aliases
			defs["wolf_pelt"].(map[string]any)["legacy_names"] = tc.other
			before := GlobalItems
			cfg, err := loadUsageFixture(t, root)
			if !tc.valid {
				if err == nil || !strings.Contains(err.Error(), "legacy") {
					t.Fatalf("expected legacy-name rejection, got %v", err)
				}
				if GlobalItems != before {
					t.Fatal("invalid catalog replaced active definitions")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Fairy Dust", "Old Dust"} {
				def, key, ok := GetItemDefinitionByName(name)
				if !ok || key != "fairy_dust" || def != cfg.Items[key] {
					t.Fatalf("%q did not resolve to the same item", name)
				}
			}
		})
	}
}
