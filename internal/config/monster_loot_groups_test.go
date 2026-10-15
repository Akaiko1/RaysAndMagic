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
	nested := func(t *testing.T, got *LootTablesConfig) {
		entries := got.Loots["strong"]
		if len(entries) != 2 || entries[0].RollCount() != 5 || entries[1].Type != "harvest" {
			t.Fatalf("expanded loot=%v", entries)
		}
	}
	// A monster's gold leaves its drop list; a table that references it does
	// not inherit the coin.
	gold := func(t *testing.T, got *LootTablesConfig) {
		if g, ok := got.MonsterGold["wolf"]; !ok || g != (GoldRange{2, 5}) {
			t.Fatalf("wolf gold = %v %v", g, ok)
		}
		if _, ok := got.MonsterGold["strong"]; ok {
			t.Fatal("a referencing table inherited gold")
		}
		if w, s := got.Loots["wolf"], got.Loots["strong"]; len(w) != 1 || w[0].Type != "item" || len(s) != 1 || s[0].Type != "item" {
			t.Fatalf("drops wolf=%v strong=%v", w, s)
		}
	}
	quantity := func(t *testing.T, got *LootTablesConfig) {
		if a := got.Loots["a"]; len(a) != 1 || a[0].Min != 2 || a[0].Max != 4 {
			t.Fatalf("item quantity = %+v", a)
		}
	}
	// An ordered group stays one entry holding its flattened group.
	firstOf := func(t *testing.T, got *LootTablesConfig) {
		a := got.Loots["a"]
		if len(a) != 1 || a[0].Type != "first_of" || len(a[0].Of) != 2 || a[0].Of[0].Type != "set" || a[0].Of[1].Type != "harvest" {
			t.Fatalf("first_of table = %+v", a)
		}
	}
	// A kill_only reference makes every drop it brings a kill trophy.
	killOnly := func(t *testing.T, got *LootTablesConfig) {
		if a := got.Loots["a"]; len(a) != 2 || !a[0].KillOnly || !a[1].KillOnly {
			t.Fatalf("trophies = %+v", a)
		}
	}
	for _, tc := range []struct {
		name, body string
		valid      bool
		check      func(*testing.T, *LootTablesConfig)
	}{
		{"kill_only table reference", "loots:\n  a: [{type: table, key: g, kill_only: true}]\nloot_groups:\n  g: [{type: item, key: wolf_pelt, chance: 1}, {type: set, key: padded, chance: 0.1}]\n", true, killOnly},
		{"kill_only gold", "loots:\n  a: [{type: gold, min: 1, max: 2, kill_only: true}]\n", false, nil},
		{"nested", "loots:\n  wolf: [{type: item, key: wolf_pelt, chance: 1}]\n  strong: [{type: table, key: wolf, rolls: 5}, {type: table, key: herbs}]\nloot_groups:\n  herbs: [{type: harvest, key: mooncap, chance: 0.06}]\n", true, nested},
		{"gold", "loots:\n  wolf: [{type: gold, min: 2, max: 5}, {type: item, key: wolf_pelt, chance: 1}]\n  strong: [{type: table, key: wolf}]\n", true, gold},
		{"two gold entries", "loots:\n  a: [{type: gold, min: 1, max: 2}, {type: gold, min: 1, max: 2}]\n", false, nil},
		{"gold max below min", "loots:\n  a: [{type: gold, min: 5, max: 2}]\n", false, nil},
		{"gold without max", "loots:\n  a: [{type: gold, min: 0}]\n", false, nil},
		{"gold with a chance", "loots:\n  a: [{type: gold, min: 1, max: 2, chance: 1}]\n", false, nil},
		{"gold in a group", "loots:\n  a: [{type: table, key: g}]\nloot_groups:\n  g: [{type: gold, min: 1, max: 2}]\n", false, nil},
		{"gold in boss loot", "boss_loot: [{type: gold, min: 1, max: 2}]\n", false, nil},
		{"first_of", "loots:\n  a: [{type: first_of, key: g}]\nloot_groups:\n  g: [{type: set, key: padded, chance: 0.1}, {type: table, key: herbs}]\n  herbs: [{type: harvest, key: mooncap, chance: 0.06}]\n", true, firstOf},
		{"unknown set", "loots:\n  a: [{type: set, key: absent, chance: 1}]\n", false, nil},
		{"item quantity", "loots:\n  a: [{type: item, key: wolf_pelt, chance: 1, min: 2, max: 4}]\n", true, quantity},
		{"quantity max below min", "loots:\n  a: [{type: item, key: wolf_pelt, chance: 1, min: 4, max: 2}]\n", false, nil},
		{"quantity without min", "loots:\n  a: [{type: item, key: wolf_pelt, chance: 1, max: 3}]\n", false, nil},
		{"quantity on a table reference", "loots:\n  a: [{type: table, key: g, min: 1, max: 2}]\nloot_groups:\n  g: [{type: item, key: wolf_pelt, chance: 1}]\n", false, nil},
		{"first_of with a chance", "loots:\n  a: [{type: first_of, key: g, chance: 0.5}]\nloot_groups:\n  g: [{type: set, key: padded, chance: 1}]\n", false, nil},
		{"first_of of a monster table", "loots:\n  a: [{type: first_of, key: b}]\n  b: [{type: set, key: padded, chance: 1}]\n", false, nil},
		{"nested first_of", "loots:\n  a: [{type: first_of, key: g}]\nloot_groups:\n  g: [{type: first_of, key: h}]\n  h: [{type: set, key: padded, chance: 1}]\n", false, nil},
		{"first_of in boss loot", "loot_groups:\n  g: [{type: set, key: padded, chance: 1}]\nboss_loot: [{type: first_of, key: g}]\n", false, nil},
		{"cycle", "loots:\n  a: [{type: table, key: b}]\n  b: [{type: table, key: a}]\n", false, nil},
		{"unknown", "loots:\n  a: [{type: table, key: absent}]\n", false, nil},
		{"negative rolls", "loots:\n  a: [{type: table, key: a, rolls: -1}]\n", false, nil},
		{"ambiguous group", "loots:\n  a: []\nloot_groups:\n  a: []\n", false, nil},
		{"chance on group", "loots:\n  a: [{type: table, key: b, chance: 0.5}]\n  b: []\n", false, nil},
		{"crafted herb", "loots:\n  a: [{type: harvest, key: brewed_health_potion, chance: 1}]\n", false, nil},
		{"nonherb", "loots:\n  a: [{type: harvest, key: wolf_pelt, chance: 1}]\n", false, nil},
		{"crate herb", "loot_tables:\n  a:\n    rolls: 1\n    entries: [{type: harvest, key: mooncap, weight: 1}]\n", false, nil},
		{"ordinary herb", "loots:\n  a: [{type: item, key: mooncap, chance: 1}]\n", false, nil},
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
				tc.check(t, got)
			}
		})
	}
}
