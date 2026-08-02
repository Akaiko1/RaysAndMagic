package game

import (
	"testing"

	"ugataima/internal/config"
)

// TestImpassableAuraIsAuthoredOptIn: the ground bubble is reserved for blockers
// geometry cannot express - ground that looks walkable and is not - and an
// author opts in by name. Bubbles are meant to be RARE.
//
// No render type earns one implicitly, crossed classes least of all: a cross
// already reads as an impassable volume, so a bubble under it is exactly the
// clutter the crossed classes exist to remove. Do not reintroduce a render-type
// rule here.
func TestImpassableAuraIsAuthoredOptIn(t *testing.T) {
	renderTypes := []string{
		config.TileRenderStandee,
		config.TileRenderCrossedStandee,
		config.TileRenderLandmarkStandee,
		config.TileRenderWall,
		config.TileRenderFloor,
		"",
	}
	for _, rt := range renderTypes {
		if tileShowsImpassableAura(&config.TileData{RenderType: rt, Solid: true}) {
			t.Errorf("render_type %q earned an aura without authoring impassable_aura", rt)
		}
		if !tileShowsImpassableAura(&config.TileData{RenderType: rt, ImpassableAura: true}) {
			t.Errorf("render_type %q authored impassable_aura but got none", rt)
		}
	}
	if tileShowsImpassableAura(nil) {
		t.Error("unknown tile earned an aura")
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
