package game

import "testing"

// A head overlay held at a screen edge must hold it where it is finally shown.
// Panini pushes points away from the horizon, so a clamp applied before the
// projection lost a point-blank tall monster's badges off the top of the frame.
func TestHeadOverlaysStayInsideTheShownFrame(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	g.combatPreferences.WideView = true
	panini := g.widePaniniProjection()
	if panini.distance == 0 {
		t.Fatal("Wide view must project through Panini")
	}
	viewBottom := float64(worldViewportBottom(g))
	shownY := func(p paniniProjection, x, y float64) float64 {
		_, sy := p.presentedPoint(x, y)
		return sy
	}
	const tallest, rowWidth, eps = 14, 60, .01 // badge rows are placed in float32
	for _, proj := range []struct {
		name string
		p    paniniProjection
	}{{"perspective", paniniProjection{}}, {"panini", panini}} {
		// The centre column magnifies most; a side column still magnifies.
		for _, x := range []float64{float64(g.worldWidth()) / 2, float64(g.worldWidth()) * .1} {
			// Point-blank tall monster: head above the frame, feet under the HUD.
			headY, size := -400.0, 1600.0
			y := float64(headBadgeRowY(proj.p, float32(x-rowWidth/2), rowWidth, tallest, float32(headY)))
			for _, col := range []float64{x - rowWidth/2, x, x + rowWidth/2} {
				if top := shownY(proj.p, col, y-tallest); top < 3-eps {
					t.Errorf("%s x=%.0f: badge row top shown at %.2f, above the frame", proj.name, col, top)
				}
			}
			cy, rx, ry := stunStarRingGeometry(proj.p, x, headY, size)
			for _, col := range []float64{x - rx, x, x + rx} {
				if top := shownY(proj.p, col, cy-ry); top < -eps {
					t.Errorf("%s x=%.0f: stun ring top shown at %.2f, above the frame", proj.name, col, top)
				}
			}
			top, bottom := statusCueSpan(proj.p, x, headY, size, viewBottom)
			for _, col := range []float64{x - size*.32, x, x + size*.32} {
				if s := shownY(proj.p, col, top); s < 4-eps {
					t.Errorf("%s x=%.0f: status cue top shown at %.2f, above the frame", proj.name, col, s)
				}
				if s := shownY(proj.p, col, bottom); s > viewBottom-4+eps {
					t.Errorf("%s x=%.0f: status cue bottom shown at %.2f, under the HUD (%.0f)", proj.name, col, s, viewBottom)
				}
			}

			// A monster in the middle of the view keeps its unclamped placement.
			headY, size = 300, 100
			if y := headBadgeRowY(proj.p, float32(x-rowWidth/2), rowWidth, tallest, float32(headY)); y != float32(headY)-tallest-5 {
				t.Errorf("%s x=%.0f: far badge row moved to %.2f", proj.name, x, y)
			}
			if cy, _, _ := stunStarRingGeometry(proj.p, x, headY, size); cy != headY-size*.08 {
				t.Errorf("%s x=%.0f: far stun ring moved to %.2f", proj.name, x, cy)
			}
			if top, bottom := statusCueSpan(proj.p, x, headY, size, viewBottom); top != headY || bottom != headY+size {
				t.Errorf("%s x=%.0f: far status cue moved to %.2f..%.2f", proj.name, x, top, bottom)
			}
		}
	}
}
