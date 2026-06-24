package monster

import "testing"

func TestMonsterSizeMultiplierPrefersCanonicalKey(t *testing.T) {
	def := MonsterDefinition{
		SizeMultiplier: 4.5,
		SizeGame:       9.0,
	}

	if got := def.GetSizeGameMultiplier(); got != 4.5 {
		t.Fatalf("expected size_multiplier to win over legacy size_game, got %f", got)
	}
}

func TestMonsterSizeMultiplierFallsBackToSizeGame(t *testing.T) {
	def := MonsterDefinition{SizeGame: 3.5}

	if got := def.GetSizeGameMultiplier(); got != 3.5 {
		t.Fatalf("expected legacy size_game fallback 3.5, got %f", got)
	}
}

func TestMonsterSizeMultiplierDefault(t *testing.T) {
	var def MonsterDefinition

	if got := def.GetSizeGameMultiplier(); got != 1.0 {
		t.Fatalf("expected default size multiplier 1.0, got %f", got)
	}
}
