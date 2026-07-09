package monster

import (
	"strings"
	"testing"
)

func TestMonsterSizeClassResolves(t *testing.T) {
	SetSizeClassHeights(map[string]float64{"person": 0.8, "huge": 1.3})
	def := MonsterDefinition{SizeClass: "huge"}

	if got := def.GetSizeGameMultiplier(); got != 1.3 {
		t.Fatalf("expected huge height 1.3, got %f", got)
	}
}

func TestMonsterSizeClassFallsBackToPerson(t *testing.T) {
	SetSizeClassHeights(map[string]float64{"person": 0.8})
	var def MonsterDefinition // no class set

	if got := def.GetSizeGameMultiplier(); got != 0.8 {
		t.Fatalf("expected person fallback 0.8, got %f", got)
	}
}

// A missing or misspelled size_class must fail validation loudly instead of
// silently rendering at the fallback scale.
func TestMonsterSizeClassValidated(t *testing.T) {
	for _, tc := range []struct{ name, class string }{
		{"empty", ""},
		{"typo", "gigantic"},
	} {
		cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
			"blob": {Name: "Blob", SizeClass: tc.class},
		}}
		err := validateMonsterConfiguration(cfg)
		if err == nil || !strings.Contains(err.Error(), "size_class") || !strings.Contains(err.Error(), "blob") {
			t.Fatalf("%s size_class should fail naming key+monster, got: %v", tc.name, err)
		}
	}
}

// Removed size_game must fail validation loudly, not decode into a silent
// 1.0-scale render.
func TestMonsterSizeGameRejected(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"relic": {Name: "Relic", DeprecatedSizeGame: 3.5},
	}}

	err := validateMonsterConfiguration(cfg)
	if err == nil {
		t.Fatal("expected validation error for removed size_game key")
	}
	if !strings.Contains(err.Error(), "size_game") || !strings.Contains(err.Error(), "relic") {
		t.Fatalf("error should name the key and the monster, got: %v", err)
	}
}
