package game

import (
	"math"
	"testing"
)

// A spell/AoE impact burst must anchor on the SAME horizontal projection as the
// monster sprite it lands on. The renderer draws sprites via projectToScreenX
// (camera-plane, screenW/(2*tan(fov/2)) lateral coefficient); the hit-effect
// anchor used to use centerX + relX*(screenH/(depth*fov)), a different lateral
// coefficient that agreed only at screen center and pulled off-axis impacts
// toward the middle (an AoE splash on a mob to the side drew off the mob).
func TestHitEffectAnchor_MatchesSpriteProjection(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	rh := NewRenderingHelper(g)
	g.renderHelper = rh

	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = 10*ts, 10*ts
	g.camera.Angle = 0 // facing +X

	// A mob well off to the side (6 tiles ahead, 3 tiles lateral): the case the
	// player reported, where a splash burst missed the side puma.
	mobX, mobY := g.camera.X+6*ts, g.camera.Y+3*ts

	spriteX, _, _, visible := rh.CalculateMonsterSpriteMetrics(mobX, mobY, Distance(g.camera.X, g.camera.Y, mobX, mobY), 1.0)
	if !visible {
		t.Fatal("test mob should be visible")
	}

	anchorX, _, ok := rh.projectToScreenX(mobX, mobY)
	if !ok {
		t.Fatal("projectToScreenX failed for a visible mob")
	}
	if anchorX != spriteX {
		t.Errorf("hit-effect anchor screenX %d != sprite screenX %d (FX would draw off the mob)", anchorX, spriteX)
	}

	// Guard: the OLD particle formula disagrees off-axis, proving the mismatch
	// this test protects against was real (and that the case is genuinely
	// off-center, not a trivially-passing relX~0).
	dx := mobX - g.camera.X
	dy := mobY - g.camera.Y
	cosA, sinA := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	relY := dx*cosA + dy*sinA
	relX := -dx*sinA + dy*cosA
	oldScale := float64(g.config.GetScreenHeight()) / (relY * g.camera.FOV)
	oldAnchorX := float64(g.config.GetScreenWidth())/2 + relX*oldScale
	if math.Abs(oldAnchorX-float64(spriteX)) < 4 {
		t.Fatalf("test point not off-axis enough: old formula %.1f already ~matches sprite %d", oldAnchorX, spriteX)
	}
}
