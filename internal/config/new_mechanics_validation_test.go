package config

import (
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
	if got := WeaponStatusTurns(5); got != 3 {
		t.Fatalf("WeaponStatusTurns(5) = %d, want 3", got)
	}
	weapon := &WeaponDefinitionConfig{SlowPct: 30, SlowSeconds: 5}
	lines := strings.Join(weapon.EffectLines(), "\n")
	if !strings.Contains(lines, "5s RT / 3 turns TB") {
		t.Fatalf("weapon status tooltip does not use shared duration formula:\n%s", lines)
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
	tests := []struct {
		name          string
		source        CrateRollSource
		requireWeight bool
	}{
		{"weight above readable range", CrateRollSource{Pool: "nothing", Weight: 101}, true},
		{"chance on weighted source", CrateRollSource{Pool: "nothing", Weight: 50, ChancePct: 5}, true},
		{"weight on special source", CrateRollSource{Pool: "gold", Weight: 5, Amount: 10}, false},
		{"legendary chance on map pool", CrateRollSource{Pool: "map", LegendaryPct: 5}, false},
		{"legendary chance above one hundred", CrateRollSource{Pool: "rare", LegendaryPct: 101}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCrateRollSource("test", "sources", 0, tt.source, tt.requireWeight); err == nil {
				t.Fatal("ambiguous crate source passed validation")
			}
		})
	}
	if err := validateCrateRollSource(
		"test",
		"roll_sources",
		0,
		CrateRollSource{Pool: "nothing", Weight: 50},
		true,
	); err != nil {
		t.Fatalf("valid weighted empty source rejected: %v", err)
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
