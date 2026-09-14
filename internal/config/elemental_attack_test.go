package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestElementalAttackConfigValidation(t *testing.T) {
	for _, tt := range []struct {
		name         string
		chance, mult float64
		ok           bool
	}{
		{"enabled", .2, 2, true}, {"disabled", 0, 0, true}, {"always", 1, 2, true},
		{"negative", -.1, 2, false}, {"over_one", 1.01, 2, false}, {"nan", math.NaN(), 2, false},
		{"infinite", math.Inf(1), 2, false}, {"zero_multiplier", .2, 0, false}, {"nan_multiplier", .2, math.NaN(), false}, {"infinite_multiplier", .2, math.Inf(1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := (ElementalAttackConfig{Chance: tt.chance, DamageMultiplier: tt.mult}).Validate(); (err == nil) != tt.ok {
				t.Fatalf("validation=%v, want valid=%t", err, tt.ok)
			}
		})
	}
}

func TestElementalAttackLoadConfigValidation(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, old, replacement string }{
		{"chance", "chance: 0.20", "chance: 1.2"},
		{"multiplier", "damage_multiplier: 2.0", "damage_multiplier: .nan"},
		{"fx_duration", "duration_seconds: 0.48", "duration_seconds: 0"},
		{"fx_count", "particle_count: 9", "particle_count: 0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			changed := strings.Replace(string(data), tt.old, tt.replacement, 1)
			if changed == string(data) {
				t.Fatal("fixture setting absent")
			}
			if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatal("invalid elemental setting accepted")
			}
		})
	}
}

func TestElementalBiomeValidation(t *testing.T) {
	for _, school := range []string{"fire", "water", "air", "earth", "body", "mind", "spirit", "light", "dark", "", "physical", "firre"} {
		t.Run(school, func(t *testing.T) {
			cfg := MapConfigs{Biomes: map[string]BiomeConfig{"test": {ElementalAttackSchool: school}}}
			valid := school != "" && school != "physical" && school != "firre"
			if err := cfg.ValidateElementalSchools(true); (err == nil) != valid {
				t.Fatalf("validation=%v, want valid=%t", err, valid)
			}
		})
	}
}
