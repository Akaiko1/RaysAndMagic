package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfigRejectsInvalidAutocastInterval(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("autocast_interval_rounds: "+value), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "autocast_interval_rounds") {
				t.Fatalf("invalid automatic retry cadence accepted: %v", err)
			}
		})
	}
}

func TestAdvancedSpellAuthoring(t *testing.T) {
	previous := GlobalSpells
	t.Cleanup(func() { GlobalSpells = previous })
	catalog, err := LoadSpellConfig("../../assets/spells.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, school := range []string{"fire", "air", "water", "earth", "spirit", "mind", "body", "light", "dark"} {
		if got := len(GetSpellsBySchool(school)); got != 8 {
			t.Errorf("%s has %d player spells, want eight", school, got)
		}
	}
	for _, tc := range []struct {
		name, spell string
		edit        func(*SpellDefinitionConfig)
	}{
		{"missing charges", "preservation", func(d *SpellDefinitionConfig) { d.Mechanic.ChargesByMastery = nil }},
		{"short tier ladder", "fire_aura", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery = []int{20} }},
		{"zero duration", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.DurationByMastery[0] = 0 }},
		{"invalid percentage", "fate", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery[3] = 101 }},
		{"stationary slow", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery[3] = 100 }},
		{"absent freeze duration", "frozen_ground", func(d *SpellDefinitionConfig) { d.Duration = 0 }},
		{"missing cone", "frozen_ground", func(d *SpellDefinitionConfig) { d.Mechanic.ConeDegrees = 0 }},
		{"missing fragment cone", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.ConeDegrees = 0 }},
		{"missing fragment count", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.ProjectilesByMastery = nil }},
		{"short fragment ladder", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.ProjectilesByMastery = []int{3, 4} }},
		{"zero fragments", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.ProjectilesByMastery[0] = 0 }},
		{"missing fragment hit cap", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.MaxHitsPerTarget = 0 }},
		{"negative fragment hit cap", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.MaxHitsPerTarget = -1 }},
		{"missing execution chance", "destroy_undead", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = 0 }},
		{"non-projectile acid", "acid_burst", func(d *SpellDefinitionConfig) { d.IsProjectile = false }},
		{"non-projectile fist", "flying_fist", func(d *SpellDefinitionConfig) { d.IsProjectile = false }},
		{"missing silence immunity", "feeblemind", func(d *SpellDefinitionConfig) { d.Mechanic.ReapplyImmunitySeconds = 0 }},
		{"non-projectile fragments", "shrapmetal", func(d *SpellDefinitionConfig) { d.IsProjectile = false }},
		{"ignored fragments", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.ProjectilesByMastery = []int{1, 2, 3, 4} }},
		{"ignored hit cap", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.MaxHitsPerTarget = 3 }},
		{"nonfinite radius", "feeblemind", func(d *SpellDefinitionConfig) { d.Mechanic.RadiusTiles = math.Inf(1) }},
		{"wrong technique", "haste", func(d *SpellDefinitionConfig) { d.Mechanic.Technique = "purify" }},
		{"wrong autocast", "haste", func(d *SpellDefinitionConfig) { d.Mechanic.Autocast = "purify" }},
		{"unknown kind", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.Kind = "slwo" }},
		{"ignored charges", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.ChargesByMastery = []int{1, 2, 3, 4} }},
		{"missing damage", "destroy_undead", func(d *SpellDefinitionConfig) { d.DamageBase = 0 }},
		{"negative execution chance", "destroy_undead", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = -0.01 }},
		{"execution chance above one", "destroy_undead", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = 1.01 }},
		{"infinite execution chance", "destroy_undead", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = math.Inf(1) }},
		{"NaN execution chance", "destroy_undead", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = math.NaN() }},
		{"unsupported execution chance", "berserk", func(d *SpellDefinitionConfig) { d.Mechanic.InstantKillChance = 0.25 }},
		{"unsupported execution summary", "berserk", func(d *SpellDefinitionConfig) { d.Mechanic.Summary = "Has a {instant_kill_chance}% execution chance." }},
		{"missing formula source", "prismatic_light", func(d *SpellDefinitionConfig) { d.DamageSource = "absent" }},
		{"self formula source", "prismatic_light", func(d *SpellDefinitionConfig) { d.DamageSource = "prismatic_light" }},
		{"zero formula divisor", "prismatic_light", func(d *SpellDefinitionConfig) { d.DamageDivisor = 0 }},
		{"conflicting base", "prismatic_light", func(d *SpellDefinitionConfig) { d.DamageBase = 30 }},
		{"bad damage school", "fire_aura", func(d *SpellDefinitionConfig) { d.Mechanic.DamageSchool = "flame" }},
		{"unknown text variable", "fate", func(d *SpellDefinitionConfig) { d.Mechanic.Summary = "Grants {charge} hits." }},
		{"empty flight palette", "prismatic_light", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.Palette = nil }},
		{"invalid flight color", "prismatic_light", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.Palette[0][0] = 256 }},
		{"zero flight scale", "souldrinker", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.SizeScale = 0 }},
		{"nonfinite flight scale", "souldrinker", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.SizeScale = math.NaN() }},
		{"missing flight source", "souldrinker", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.Projectile = "absent" }},
		{"missing target cap", "prismatic_light", func(d *SpellDefinitionConfig) { d.Mechanic.MaxTargets = 0 }},
		{"negative target cap", "shrapmetal", func(d *SpellDefinitionConfig) { d.Mechanic.MaxTargets = -1 }},
		{"ignored target cap", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.MaxTargets = 2 }},
		{"invalid flight duration", "prismatic_light", func(d *SpellDefinitionConfig) { d.Graphics.TargetFlights.DurationSeconds = math.Inf(1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := yaml.Marshal(catalog.Spells[tc.spell])
			if err != nil {
				t.Fatal(err)
			}
			var definition SpellDefinitionConfig
			if err := yaml.Unmarshal(data, &definition); err != nil {
				t.Fatal(err)
			}
			tc.edit(&definition)
			defs := map[string]*SpellDefinitionConfig{tc.spell: &definition}
			if graphics := catalog.Spells[tc.spell].Graphics; graphics != nil && graphics.TargetFlights != nil {
				source := graphics.TargetFlights.Projectile
				defs[source] = catalog.Spells[source]
			}
			data, err = yaml.Marshal(SpellSystemConfig{Spells: defs})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "spells.yaml")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSpellConfig(path); err == nil {
				t.Fatal("invalid gameplay authoring was accepted")
			}
		})
	}
}
