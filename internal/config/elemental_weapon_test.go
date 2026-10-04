package config

import (
	"math"
	"testing"
)

func TestElementalWeaponAuthoringRejectsIncompleteRules(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*WeaponDefinitionConfig)
	}{
		{"unknown", func(w *WeaponDefinitionConfig) { w.ElementalAbility.Kind = "burn" }},
		{"wrong_school", func(w *WeaponDefinitionConfig) { w.DamageType = "water" }},
		{"zero_damage", func(w *WeaponDefinitionConfig) { w.ElementalAbility.Damage = 0 }},
		{"missing_duration", func(w *WeaponDefinitionConfig) { w.ElementalAbility.DurationSeconds = 0 }},
		{"single_hit", func(w *WeaponDefinitionConfig) { w.ElementalAbility.Hits = 1 }},
		{"irrelevant_range", func(w *WeaponDefinitionConfig) { w.ElementalAbility.RangeTiles = 2 }},
		{"not_finite", func(w *WeaponDefinitionConfig) { w.ElementalAbility.RangeTiles = math.NaN() }},
		{"wrong_delivery", func(w *WeaponDefinitionConfig) { w.Melee = nil }},
		{"missing_stat", func(w *WeaponDefinitionConfig) { w.ElementalAbility.ScalingStat = "" }},
		{"wrong_stat", func(w *WeaponDefinitionConfig) { w.ElementalAbility.ScalingStat = "Intellect" }},
		{"zero_divisor", func(w *WeaponDefinitionConfig) { w.ElementalAbility.StatDivisor = 0 }},
		{"negative_divisor", func(w *WeaponDefinitionConfig) { w.ElementalAbility.StatDivisor = -3 }},
		{"irrelevant_stacks", func(w *WeaponDefinitionConfig) { w.ElementalAbility.MaxStacks = 10 }},
		{"irrelevant_chain_cap", func(w *WeaponDefinitionConfig) { w.ElementalAbility.HitsPerTarget = 2 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			w := validMechanicTestWeapon()
			w.DamageType = "fire"
			w.Melee = &MeleeAttackConfig{}
			w.Physics = nil
			w.ElementalAbility = &ElementalWeaponAbility{Kind: "pressure", Damage: 24, DurationSeconds: 6, Hits: 3, ScalingStat: "Speed", StatDivisor: 3}
			if err := w.ElementalAbility.validate(w); err != nil {
				t.Fatal(err)
			}
			change.edit(w)
			if err := validateWeaponConfig(&WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": w}}); err == nil {
				t.Fatal("invalid ability accepted")
			}
		})
	}
}

func TestElementalWeaponSpecializedContracts(t *testing.T) {
	for _, kind := range []string{"backwash", "anchor", "crosswind"} {
		for _, invalid := range []string{"valid", "missing_budget", "wrong_range", "irrelevant_scaling", "wrong_duration"} {
			t.Run(kind+"/"+invalid, func(t *testing.T) {
				w := validMechanicTestWeapon()
				w.Melee, w.Physics = &MeleeAttackConfig{}, nil
				a := &ElementalWeaponAbility{Kind: kind, Damage: 12, DurationSeconds: 6}
				w.ElementalAbility = a
				switch kind {
				case "backwash":
					w.DamageType, w.ProjectileSchool = "water", "water"
					w.Melee, w.Physics = nil, &ProjectilePhysicsConfig{}
					a.DurationSeconds, a.HitsPerTarget, a.RangeTiles, a.ScalingStat, a.StatDivisor = 0, 2, 2, "Intellect", 3
				case "anchor":
					w.DamageType, a.MaxStacks = "earth", 10
				case "crosswind":
					w.DamageType, a.Hits, a.RangeTiles = "air", 3, 3
				}
				switch invalid {
				case "missing_budget":
					a.Hits, a.MaxStacks, a.HitsPerTarget = 0, 0, 0
				case "wrong_range":
					if kind == "anchor" {
						a.RangeTiles = 2
					} else {
						a.RangeTiles = 0
					}
				case "irrelevant_scaling":
					a.ScalingStat, a.StatDivisor = "Speed", 3
				case "wrong_duration":
					if kind == "backwash" {
						a.DurationSeconds = 6
					} else {
						a.DurationSeconds = 0
					}
				}
				err := a.validate(w)
				if (err != nil) != (invalid != "valid") {
					t.Fatalf("validation=%v", err)
				}
			})
		}
	}
}
