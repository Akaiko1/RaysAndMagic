package config

import (
	"os"
	"path/filepath"
	"testing"
)

// LoadLootTables must validate BEFORE publishing to the process global, so a
// malformed file can't leave GlobalLoots pointing at invalid data (stale-global
// footgun on reload).
func TestLoadLootTables_InvalidLeavesGlobalUnchanged(t *testing.T) {
	prev := GlobalLoots
	sentinel := &LootTablesConfig{}
	GlobalLoots = sentinel
	t.Cleanup(func() { GlobalLoots = prev })

	bad := filepath.Join(t.TempDir(), "bad_loots.yaml")
	// rolls:0 fails validation before any key lookup, so no weapon/item config needed.
	body := "loot_tables:\n  broken:\n    rolls: 0\n    gold_min: 1\n    gold_max: 2\n    entries: []\n"
	if err := os.WriteFile(bad, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadLootTables(bad); err == nil {
		t.Fatal("expected a validation error for rolls:0")
	}
	if GlobalLoots != sentinel {
		t.Error("LoadLootTables must not overwrite GlobalLoots when validation fails")
	}
}

func TestLoadLootTables_InvalidBossDeathLootLeavesGlobalUnchanged(t *testing.T) {
	prev := GlobalLoots
	sentinel := &LootTablesConfig{}
	GlobalLoots = sentinel
	t.Cleanup(func() { GlobalLoots = prev })

	bad := filepath.Join(t.TempDir(), "bad_boss_death_loot.yaml")
	// Chance validation precedes item lookup, so this remains a focused loader
	// test without depending on the full item catalog.
	body := "boss_death_loot:\n  - type: item\n    key: skeleton_key\n    chance: 1.01\n"
	if err := os.WriteFile(bad, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadLootTables(bad); err == nil {
		t.Fatal("expected invalid boss_death_loot chance to fail")
	}
	if GlobalLoots != sentinel {
		t.Error("LoadLootTables must not overwrite GlobalLoots on invalid boss_death_loot")
	}
}
