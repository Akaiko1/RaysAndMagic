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

func TestLoadSpellConfigRejectsRetiredDurationFields(t *testing.T) {
	for _, field := range []string{"duration: 0", "duration: 180", "mechanic:\n      duration_by_mastery: [6, 9, 12, 15]"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "spells.yaml")
			source := "spells:\n  test:\n    cooldown_seconds: 1\n    " + field + "\n"
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSpellConfig(path); err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("retired duration field must fail strict decoding: %v", err)
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
		if len(GetSpellsBySchool(school)) == 0 {
			t.Errorf("%s has no player spells", school)
		}
	}
	for _, tc := range []struct {
		name, spell string
		edit        func(*SpellDefinitionConfig)
	}{
		{"missing personal buff category", "fire_aura", func(d *SpellDefinitionConfig) { d.Category, d.CooldownSeconds = "", 2 }},
		{"missing party buff category", "shield", func(d *SpellDefinitionConfig) { d.Category, d.CooldownSeconds = "", 2 }},
		{"personal combat buff without recovery", "fire_aura", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"personal timed buff recovery", "vampiric_weapon", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 2 }},
		{"party timed buff recovery", "shield", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 2 }},
		{"party combat buff without recovery", "preservation", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"technique combat buff without recovery", "haste", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"buff without utility semantics", "fate", func(d *SpellDefinitionConfig) { d.IsUtility = false }},
		{"instant utility needs recovery", "body_purify", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"beacon charges are not buffs", "vvoid_beacon", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"offensive duration is not a buff", "slow", func(d *SpellDefinitionConfig) { d.CooldownSeconds = 0 }},
		{"category cannot override mechanic", "body_purify", func(d *SpellDefinitionConfig) {
			d.Category, d.DurationByMastery, d.CooldownSeconds = "buff", []int{60, 60, 60, 60}, 0
		}},
		{"missing charges", "preservation", func(d *SpellDefinitionConfig) { d.Mechanic.ChargesByMastery = nil }},
		{"short tier ladder", "fire_aura", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery = []int{20} }},
		{"zero duration", "slow", func(d *SpellDefinitionConfig) { d.DurationByMastery[0] = 0 }},
		{"missing control duration", "slow", func(d *SpellDefinitionConfig) { d.DurationByMastery = nil }},
		{"missing technique duration", "haste", func(d *SpellDefinitionConfig) { d.DurationByMastery = nil }},
		{"missing legacy buff duration", "bless", func(d *SpellDefinitionConfig) { d.DurationByMastery = nil }},
		{"missing field duration", "firewall", func(d *SpellDefinitionConfig) { d.DurationByMastery = nil }},
		{"short duration ladder", "shield", func(d *SpellDefinitionConfig) { d.DurationByMastery = []int{60, 90} }},
		{"negative duration", "vampiric_weapon", func(d *SpellDefinitionConfig) { d.DurationByMastery[0] = -1 }},
		{"decreasing duration", "bless", func(d *SpellDefinitionConfig) { d.DurationByMastery[1] = 1 }},
		{"charged buff cannot expire", "preservation", func(d *SpellDefinitionConfig) { d.DurationByMastery = []int{60, 60, 60, 60} }},
		{"instant mechanic has no timer", "body_purify", func(d *SpellDefinitionConfig) { d.DurationByMastery = []int{60, 60, 60, 60} }},
		{"ordinary projectile has no timer", "fireball", func(d *SpellDefinitionConfig) { d.DurationByMastery = []int{60, 60, 60, 60} }},
		{"unrelated water timer", "shield", func(d *SpellDefinitionConfig) { d.Mechanic.WaterFreezeSeconds = 180 }},
		{"invalid percentage", "fate", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery[3] = 101 }},
		{"armor shred above all armor", "acid_burst", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery[3] = 101 }},
		{"haste without bonus actions", "haste", func(d *SpellDefinitionConfig) { d.Mechanic.ExtraActionsByMastery = nil }},
		{"timed mechanic summary without duration", "shield", func(d *SpellDefinitionConfig) { d.Mechanic.Summary = "Wards the party." }},
		{"stationary slow", "slow", func(d *SpellDefinitionConfig) { d.Mechanic.PowerByMastery[3] = 100 }},
		{"absent freeze duration", "frozen_ground", func(d *SpellDefinitionConfig) { d.Mechanic.WaterFreezeSeconds = 0 }},
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
			load := func(edit func(*SpellDefinitionConfig)) error {
				data, err := yaml.Marshal(catalog.Spells[tc.spell])
				if err != nil {
					t.Fatal(err)
				}
				var definition SpellDefinitionConfig
				if err := yaml.Unmarshal(data, &definition); err != nil {
					t.Fatal(err)
				}
				edit(&definition)
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
				_, err = LoadSpellConfig(path)
				return err
			}
			// The unedited copy loads, so a rejection is caused by the edit itself.
			if err := load(func(*SpellDefinitionConfig) {}); err != nil {
				t.Fatalf("shipped %s rejected: %v", tc.spell, err)
			}
			if load(tc.edit) == nil {
				t.Fatal("invalid gameplay authoring was accepted")
			}
		})
	}
}
