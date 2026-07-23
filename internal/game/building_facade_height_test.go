package game

import (
	"math"
	"testing"
)

// TestBuildingFacadeHeightDepthInvariance: the facade slab's column height is
// centerSize*centerDepth/t, so the size*depth product must not depend on the
// depth the geometry was computed at. The product breaks below ~mult world
// units of depth, where CalculateWallDimensionsWithHeight's sanity cap
// flattens the height - the draw path's clamp floor (one tile) must sit above
// that zone. Guards the "tower squashes 2x when its center is behind the
// camera" regression.
func TestBuildingFacadeHeightDepthInvariance(t *testing.T) {
	t.Chdir("../..")

	g, _, cfg := bootOpenWorldGame(t, false)
	rh := g.gameLoop.renderer.game.renderHelper
	ts := float64(cfg.GetTileSize())

	for _, mult := range []float64{1, 2, 3, 6} {
		ref, _ := rh.CalculateWallDimensionsWithHeightF(20*ts, mult)
		refProduct := ref * 20 * ts
		for _, depth := range []float64{ts, 2 * ts, 5 * ts} {
			h, _ := rh.CalculateWallDimensionsWithHeightF(depth, mult)
			if p := h * depth; math.Abs(p-refProduct) > refProduct*0.01 {
				t.Errorf("mult %.0f: size*depth at depth %.0f = %.0f, want %.0f (facade squashes by %.2fx)",
					mult, depth, p, refProduct, refProduct/p)
			}
		}
		// Document the hazard the clamp floor avoids: at ~1 world px the cap
		// DOES flatten multi-tile facades - the draw path must never ask for
		// geometry that close.
		if mult >= 2 {
			h, _ := rh.CalculateWallDimensionsWithHeightF(1.0, mult)
			if p := h * 1.0; p >= refProduct*0.99 {
				t.Errorf("mult %.0f: expected the sanity cap to distort the product at depth 1.0; clamp floor test is vacuous", mult)
			}
		}
	}
}
