package monster

import (
	"strings"
	"testing"
)

func TestMonsterSizeMultiplier(t *testing.T) {
	def := MonsterDefinition{SizeMultiplier: 4.5}

	if got := def.GetSizeGameMultiplier(); got != 4.5 {
		t.Fatalf("expected size_multiplier 4.5, got %f", got)
	}
}

func TestMonsterSizeMultiplierDefault(t *testing.T) {
	var def MonsterDefinition

	if got := def.GetSizeGameMultiplier(); got != 1.0 {
		t.Fatalf("expected default size multiplier 1.0, got %f", got)
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
