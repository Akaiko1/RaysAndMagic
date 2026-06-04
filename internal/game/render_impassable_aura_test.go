package game

import "testing"

// TestAuraBillboardRenderType: the aura targets ambiguous impassable billboards
// (rocks/cliffs) and skips trees, textured walls, and non-blocking floor tiles.
func TestAuraBillboardRenderType(t *testing.T) {
	cases := map[string]bool{
		"environment_sprite": true,
		"flooring_object":    true,
		"tree_sprite":        false,
		"textured_wall":      false,
		"floor_only":         false,
		"":                   false,
	}
	for rt, want := range cases {
		if got := isAuraBillboardRenderType(rt); got != want {
			t.Errorf("isAuraBillboardRenderType(%q) = %v, want %v", rt, got, want)
		}
	}
}

// TestAuraHash_DeterministicAndBounded: phases are reproducible per particle and
// stay in [0,1); different particles get different phases.
func TestAuraHash_DeterministicAndBounded(t *testing.T) {
	a := auraHash(3, 7, 1, 2)
	if a != auraHash(3, 7, 1, 2) {
		t.Error("auraHash should be deterministic for the same inputs")
	}
	if a < 0 || a >= 1 {
		t.Errorf("auraHash out of range: %v", a)
	}
	if auraHash(3, 7, 1, 2) == auraHash(3, 7, 1, 3) {
		t.Error("different particle index should usually yield a different phase")
	}
}
