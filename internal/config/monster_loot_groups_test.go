package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMonsterLootGroupsValidateBeforePublication(t *testing.T) {
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	prior := GlobalLoots
	t.Cleanup(func() { GlobalLoots = prior })
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"nested", "loots:\n  wolf: [{type: item, key: wolf_pelt, chance: 1}]\n  strong: [{type: table, key: wolf, rolls: 5}, {type: table, key: herbs}]\nloot_groups:\n  herbs: [{type: harvest, key: mooncap, chance: 0.06}]\n", true},
		{"cycle", "loots:\n  a: [{type: table, key: b}]\n  b: [{type: table, key: a}]\n", false},
		{"unknown", "loots:\n  a: [{type: table, key: absent}]\n", false},
		{"negative rolls", "loots:\n  a: [{type: table, key: a, rolls: -1}]\n", false},
		{"ambiguous group", "loots:\n  a: []\nloot_groups:\n  a: []\n", false},
		{"chance on group", "loots:\n  a: [{type: table, key: b, chance: 0.5}]\n  b: []\n", false},
		{"crafted herb", "loots:\n  a: [{type: harvest, key: brewed_health_potion, chance: 1}]\n", false},
		{"nonherb", "loots:\n  a: [{type: harvest, key: wolf_pelt, chance: 1}]\n", false},
		{"crate herb", "loot_tables:\n  a:\n    rolls: 1\n    entries: [{type: harvest, key: mooncap, weight: 1}]\n", false},
		{"ordinary herb", "loots:\n  a: [{type: item, key: mooncap, chance: 1}]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := &LootTablesConfig{}
			GlobalLoots = sentinel
			p := filepath.Join(t.TempDir(), "loots.yaml")
			if err := os.WriteFile(p, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadLootTables(p)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if !tc.valid && GlobalLoots != sentinel {
				t.Fatal("failed loader changed published tables")
			}
			if tc.valid {
				entries := got.Loots["strong"]
				if len(entries) != 2 || entries[0].RollCount() != 5 || entries[1].Type != "harvest" {
					t.Fatalf("expanded loot=%v", entries)
				}
			}
		})
	}
}
