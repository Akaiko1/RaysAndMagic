package config

import (
	"strings"
	"testing"
)

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
			// Only non-buff spells author a cooldown, so each case must satisfy
			// that rule to be judged on the behaviour it actually tests.
			if !strings.EqualFold(strings.TrimSpace(tc.category), "buff") {
				def.CooldownSeconds = 1
			}
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
		{name: "map wide", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, MapWide: true, CooldownSeconds: 1}},
		{name: "party radius", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, PartyAoeRadiusTiles: 2, CooldownSeconds: 1}},
		{name: "negative", def: &SpellDefinitionConfig{MasteryDamagePerTier: -1, MapWide: true, CooldownSeconds: 1}, wantError: true},
		{name: "unsupported projectile", def: &SpellDefinitionConfig{MasteryDamagePerTier: 15, IsProjectile: true, CooldownSeconds: 1}, wantError: true},
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

// Spell level was removed, so nothing can derive a cooldown any more: every
// castable spell authors it, and a buff (zero cooldown by rule) must not.
func TestValidateSpellAuthoring_CooldownRequiredExceptBuffs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		def       *SpellDefinitionConfig
		wantError bool
	}{
		{name: "castable with cooldown", def: &SpellDefinitionConfig{CooldownSeconds: 1.5}},
		{name: "castable without cooldown", def: &SpellDefinitionConfig{}, wantError: true},
		{name: "castable with zero cooldown", def: &SpellDefinitionConfig{CooldownSeconds: 0}, wantError: true},
		{name: "buff without cooldown", def: &SpellDefinitionConfig{Category: "buff", IsUtility: true, Duration: 60}},
		{name: "buff with dead cooldown", def: &SpellDefinitionConfig{Category: "buff", IsUtility: true, Duration: 60, CooldownSeconds: 5}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"test": tc.def}}
			err := validateSpellAuthoring(cfg)
			if (err != nil) != tc.wantError {
				t.Fatalf("validate cooldown: err=%v, wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestValidateSpellAuthoring_StunRequiresBothModeClocks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		def     *SpellDefinitionConfig
		wantErr bool
	}{
		{
			name: "complete chance stun",
			def: &SpellDefinitionConfig{
				CooldownSeconds: 1, StunChance: 0.5,
				StunDurationSeconds: 2, StunDurationTurns: 1,
			},
		},
		{
			name: "complete radius stun",
			def: &SpellDefinitionConfig{
				CooldownSeconds: 1, StunRadiusTiles: 3,
				StunDurationSeconds: 4, StunDurationTurns: 2,
			},
		},
		{
			name: "missing seconds",
			def: &SpellDefinitionConfig{
				CooldownSeconds: 1, StunChance: 0.5, StunDurationTurns: 1,
			},
			wantErr: true,
		},
		{
			name: "missing turns",
			def: &SpellDefinitionConfig{
				CooldownSeconds: 1, StunRadiusTiles: 3, StunDurationSeconds: 4,
			},
			wantErr: true,
		},
		{
			name: "orphan durations",
			def: &SpellDefinitionConfig{
				CooldownSeconds: 1, StunDurationSeconds: 2, StunDurationTurns: 1,
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"test": tc.def}}
			err := validateSpellAuthoring(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validate stun clocks: err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
