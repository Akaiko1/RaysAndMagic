package config

import "testing"

func TestValidateSpellAuthoring_Category(t *testing.T) {
	for _, tc := range []struct {
		name      string
		category  string
		utility   bool
		duration  int
		wantError bool
	}{
		{name: "default"},
		{name: "buff", category: "buff", utility: true, duration: 1},
		{name: "case-insensitive", category: "BuFf", utility: true, duration: 1},
		{name: "unknown", category: "enchantment", utility: true, duration: 1, wantError: true},
		{name: "instant utility cannot be a buff", category: "buff", utility: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := &SpellDefinitionConfig{Category: tc.category, IsUtility: tc.utility, Duration: tc.duration}
			cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{
				"test": def,
			}}
			err := validateSpellAuthoring(cfg)
			if (err != nil) != tc.wantError {
				t.Fatalf("validate category %q: err=%v, wantError=%v", tc.category, err, tc.wantError)
			}
		})
	}
}

func TestValidateSpellAuthoring_MasteryDamageRequiresNova(t *testing.T) {
	for _, tc := range []struct {
		name      string
		def       *SpellDefinitionConfig
		wantError bool
	}{
		{name: "map wide", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, MapWide: true}},
		{name: "party radius", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, PartyAoeRadiusTiles: 2}},
		{name: "negative", def: &SpellDefinitionConfig{MasteryDamagePerTier: -1, MapWide: true}, wantError: true},
		{name: "unsupported projectile", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, IsProjectile: true}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"test": tc.def}}
			err := validateSpellAuthoring(cfg)
			if (err != nil) != tc.wantError {
				t.Fatalf("validate mastery damage: err=%v, wantError=%v", err, tc.wantError)
			}
		})
	}
}
