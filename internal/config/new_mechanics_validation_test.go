package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validMechanicTestWeapon() *WeaponDefinitionConfig {
	return &WeaponDefinitionConfig{
		Name:       "Test Blade",
		Category:   "sword",
		DamageType: "physical",
		Range:      1,
		Melee:      &MeleeAttackConfig{ArcType: 1},
		Graphics:   &WeaponGraphicsConfig{SlashWidth: 1, SlashLength: 1},
	}
}

func TestValidateWeaponConfigRejectsIncompleteNewMechanics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WeaponDefinitionConfig)
	}{
		{"ignite chance without duration", func(w *WeaponDefinitionConfig) { w.IgniteChance = 0.5 }},
		{"poison duration without chance", func(w *WeaponDefinitionConfig) { w.PoisonSeconds = 5 }},
		{"slow over one hundred", func(w *WeaponDefinitionConfig) { w.SlowPct, w.SlowSeconds = 101, 5 }},
		{"weaken without duration", func(w *WeaponDefinitionConfig) { w.WeakenPct = 25 }},
		{"partial death burst", func(w *WeaponDefinitionConfig) { w.DeathBurstDamage = 10 }},
		{"negative true damage", func(w *WeaponDefinitionConfig) { w.TrueDamage = -1 }},
		{"echo over one hundred", func(w *WeaponDefinitionConfig) { w.SpellEchoPct = 101 }},
		{"ricochet without range", func(w *WeaponDefinitionConfig) { w.RicochetTargets = 1 }},
		{"ricochet on melee", func(w *WeaponDefinitionConfig) {
			w.RicochetTargets, w.RicochetRangeTiles = 1, 6
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weapon := validMechanicTestWeapon()
			tt.mutate(weapon)
			cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": weapon}}
			if err := validateWeaponConfig(cfg); err == nil {
				t.Fatal("invalid mechanic passed validation")
			}
		})
	}
}

func TestValidateWeaponConfigAcceptsCompleteNewMechanics(t *testing.T) {
	weapon := validMechanicTestWeapon()
	weapon.IgniteChance, weapon.IgniteSeconds = 0.25, 5
	weapon.PoisonChance, weapon.PoisonSeconds = 0.2, 10
	weapon.ExecuteBelowPct = 15
	weapon.DeathBurstDamage, weapon.DeathBurstRadiusTiles = 20, 2
	weapon.TrueDamage = 5
	weapon.SlowPct, weapon.SlowSeconds = 20, 4
	weapon.WeakenPct, weapon.WeakenSeconds = 25, 5
	weapon.SpellEchoPct = 10
	weapon.TBActionsPerRound = 3

	cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": weapon}}
	if err := validateWeaponConfig(cfg); err != nil {
		t.Fatalf("complete mechanics rejected: %v", err)
	}
}

func TestWeaponStatusTurnsMatchesSharedTooltipFormula(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	if _, err := LoadConfig("../../config.yaml"); err != nil {
		t.Fatal(err)
	}
	if got := WeaponStatusTurns(5); got != 3 {
		t.Fatalf("WeaponStatusTurns(5) = %d, want 3", got)
	}
	weapon := &WeaponDefinitionConfig{SlowPct: 30, SlowSeconds: 5}
	lines := strings.Join(weapon.EffectLines(), "\n")
	if !strings.Contains(lines, "5s RT / 3 turns TB") {
		t.Fatalf("weapon status tooltip does not use shared duration formula:\n%s", lines)
	}
	if want := fmt.Sprintf("by 30%% for 5s RT / 3 turns TB (TB: skips one turn in %d)", SlowSkipEveryTurns()); !strings.Contains(lines, want) {
		t.Fatalf("slow tooltip does not state the TB skip cadence %q:\n%s", want, lines)
	}
}

func TestValidateItemConfigRejectsIncompleteNewMechanics(t *testing.T) {
	tests := []struct {
		name string
		item *ItemDefinitionConfig
	}{
		{"reflect over one hundred", &ItemDefinitionConfig{ProjectileReflectPct: 101}},
		{"status below floor", &ItemDefinitionConfig{StatusDurationPct: MinHostileStatusDurationPct - 1}},
		{"scale stack without cap", &ItemDefinitionConfig{ScaleStackAC: 1}},
		{"spell ward key on item", &ItemDefinitionConfig{Type: "consumable", DeprecatedResistBuffPct: 50}},
		{"ward without school", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60, StatusIcon: "ward"}},
		{"ward without duration", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "fire", ResistBuffSchoolPct: 50, StatusIcon: "ward"}},
		{"ward without icon", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "fire", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60}},
		{"physical ward", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "physical", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60, StatusIcon: "ward"}},
		{"buff on armor", &ItemDefinitionConfig{Type: "armor", BuffArmorClass: 10, BuffDurationSeconds: 60, StatusIcon: "stone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": tt.item}}
			if err := validateItemConfig(cfg); err == nil {
				t.Fatal("invalid mechanic passed validation")
			}
		})
	}
}

func TestValidateItemConfigCanonicalizesTimedWard(t *testing.T) {
	item := &ItemDefinitionConfig{
		Type:                "consumable",
		ResistBuffSchool:    " FIRE ",
		ResistBuffSchoolPct: 50,
		BuffDurationSeconds: 60,
		StatusIcon:          " fire_shield ",
	}
	cfg := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"ward": item}}
	if err := validateItemConfig(cfg); err != nil {
		t.Fatalf("valid ward rejected: %v", err)
	}
	if item.ResistBuffSchool != "fire" || item.StatusIcon != "fire_shield" {
		t.Fatalf("ward metadata not canonicalized: %+v", item)
	}
}

func TestValidateCrateRollSourceRejectsAmbiguousFields(t *testing.T) {
	tables := map[string]*WeightedLootTable{"gear": {Rolls: 1}}
	for _, tt := range []struct {
		name   string
		source CrateRollSource
	}{
		{"weight above one hundred", CrateRollSource{Pool: "nothing", Weight: 101}},
		{"weight below a tenth", CrateRollSource{Pool: "nothing", Weight: 0.05}},
		{"weight finer than a tenth", CrateRollSource{Pool: "nothing", Weight: 2.45}},
		{"removed rare pool", CrateRollSource{Pool: "rare", Weight: 50}},
		{"item type on a map pool", CrateRollSource{Pool: "map", ItemType: "armor", Weight: 50}},
		{"rarity on an empty slot", CrateRollSource{Pool: "nothing", Rarity: "rare", Weight: 50}},
		{"rarity on gold", CrateRollSource{Pool: "gold", Amount: 10, Rarity: "rare", Weight: 50}},
		{"gold without an amount", CrateRollSource{Pool: "gold", Weight: 50}},
		{"amount on a catalog pool", CrateRollSource{Pool: "catalog", ItemType: "armor", Amount: 5, Weight: 50}},
		{"unknown catalog type", CrateRollSource{Pool: "catalog", ItemType: "potion", Weight: 50}},
		{"unknown rarity", CrateRollSource{Pool: "map", Rarity: "ultra", Weight: 50}},
		{"backwards rarity span", CrateRollSource{Pool: "map", Rarity: "rare-common", Weight: 50}},
		{"unknown loot table", CrateRollSource{Pool: "loot_table", LootTable: "missing", Weight: 50}},
		{"loot table pool without a table", CrateRollSource{Pool: "loot_table", Weight: 50}},
		{"rarity on a loot table", CrateRollSource{Pool: "loot_table", LootTable: "gear", Rarity: "rare", Weight: 50}},
		{"amount on a loot table", CrateRollSource{Pool: "loot_table", LootTable: "gear", Amount: 5, Weight: 50}},
		{"loot table on a map pool", CrateRollSource{Pool: "map", LootTable: "gear", Weight: 50}},
		{"loot table on gold", CrateRollSource{Pool: "gold", Amount: 10, LootTable: "gear", Weight: 50}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCrateRollSource(tables, "test", 0, tt.source); err == nil {
				t.Fatal("ambiguous crate source passed validation")
			}
		})
	}
	for _, ok := range []CrateRollSource{
		{Pool: "nothing", Weight: 50},
		{Pool: "map", Rarity: "uncommon+", Weight: 2.4},
		{Pool: "map", Rarity: "common-uncommon", Weight: 66.4},
		{Pool: "arena_points", Amount: 5000, Weight: 1.7},
		{Pool: "loot_table", LootTable: "gear", Weight: 3.5},
	} {
		if err := validateCrateRollSource(tables, "test", 0, ok); err != nil {
			t.Fatalf("valid source %+v rejected: %v", ok, err)
		}
	}
}

// One rarity field reads every gate the crates need.
func TestParseRarityRange(t *testing.T) {
	for _, tc := range []struct {
		in       string
		min, max int
	}{
		{"", 0, 4}, {"rare", 2, 2}, {"common-uncommon", 0, 1}, {"uncommon+", 1, 4}, {"common-rare", 0, 2}, {" Legendary ", 3, 3},
	} {
		got, err := ParseRarityRange(tc.in)
		if err != nil || got.Min != tc.min || got.Max != tc.max {
			t.Fatalf("ParseRarityRange(%q) = %+v, %v; want %d..%d", tc.in, got, err, tc.min, tc.max)
		}
	}
}

// Loot data decodes strictly: a field the crates no longer read fails the load.
func TestLootTablesRejectRemovedCrateFields(t *testing.T) {
	for _, field := range []string{"special_rolls: []", "min_rarity: rare", "legendary_pct: 5"} {
		data := "crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: nothing\n        weight: 100\n    " + field + "\n"
		if field != "special_rolls: []" {
			data = "crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: nothing\n        weight: 100\n        " + field + "\n"
		}
		path := filepath.Join(t.TempDir(), "loot.yaml")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLootTables(path); err == nil {
			t.Fatalf("removed field %q loaded silently", field)
		}
	}
}

func TestValidateCratesRequiresReadablePercentageWeights(t *testing.T) {
	cfg := &LootTablesConfig{Crates: map[string]*CrateConfig{
		"bad_total": {
			Rolls: 1,
			RollSources: []CrateRollSource{
				{Pool: "nothing", Weight: 60},
				{Pool: "nothing", Weight: 30},
			},
		},
	}}
	if err := validateCrates(cfg); err == nil || !strings.Contains(err.Error(), "total 100") {
		t.Fatalf("non-percentage crate weights validation = %v", err)
	}

	cfg.Crates["bad_total"].RollSources[1].Weight = 40
	if err := validateCrates(cfg); err != nil {
		t.Fatalf("percentage crate weights rejected: %v", err)
	}
}
