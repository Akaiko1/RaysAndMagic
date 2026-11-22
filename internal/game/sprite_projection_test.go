package game

import (
	"math"
	"testing"
)

// Sprite depth and size come from the PERPENDICULAR camera distance, not the
// Euclidean one: Euclidean sizing shrinks off-axis sprites, so they drift off
// the floor point their screen X anchors to.
// Reference: https://lodev.org/cgtutor/raycasting3.html
func TestSpriteUsesPerpendicularDistance(t *testing.T) {
	cfg := loadTestConfig(t)
	game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	game.camera.FOV = squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	game.camera.ViewDist = cfg.GetViewDistance()
	game.renderHelper = NewRenderingHelper(game)
	rh := game.renderHelper
	ts := float64(cfg.GetTileSize())
	camX, camY := 5*ts, 10*ts
	game.camera.X, game.camera.Y = camX, camY

	tests := []struct {
		name            string
		angle           float64
		ahead, lateral  float64 // tiles along / across the view direction
		wantRightOfMidX bool
	}{
		{name: "directly ahead", angle: 0, ahead: 3},
		{name: "ahead and to the right", angle: 0, ahead: 3, lateral: 2, wantRightOfMidX: true},
		{name: "steep angle", angle: 0, ahead: 3, lateral: 1.7, wantRightOfMidX: true},
		{name: "rotated camera", angle: math.Pi / 3, ahead: 4, lateral: 2.5, wantRightOfMidX: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game.camera.Angle = tt.angle
			dirX, dirY := math.Cos(tt.angle), math.Sin(tt.angle)
			// Screen-right is +lateral along (-dirY, dirX) in this y-down world.
			ex := camX + (tt.ahead*dirX-tt.lateral*dirY)*ts
			ey := camY + (tt.ahead*dirY+tt.lateral*dirX)*ts
			euclid := Distance(camX, camY, ex, ey)

			screenX, depth, ok := rh.projectToScreenXF(ex, ey)
			if !ok {
				t.Fatal("entity in front of the camera did not project")
			}
			if want := tt.ahead * ts; math.Abs(depth-want) > 1e-6 {
				t.Fatalf("depth = %.4f, want perpendicular %.4f (euclidean %.4f)", depth, want, euclid)
			}
			mid := float64(game.worldWidth()) / 2
			if tt.wantRightOfMidX != (screenX > mid+0.5) {
				t.Fatalf("screenX = %.2f vs center %.2f, want right of center %v", screenX, mid, tt.wantRightOfMidX)
			}

			// Same perpendicular depth straight ahead: same size and floor anchor.
			ax, ay := camX+tt.ahead*dirX*ts, camY+tt.ahead*dirY*ts
			_, bottom, size, visible := rh.CalculateMonsterSpriteMetricsF(ex, ey, euclid, 1)
			_, wantBottom, wantSize, aheadVisible := rh.CalculateMonsterSpriteMetricsF(ax, ay, tt.ahead*ts, 1)
			if !visible || !aheadVisible {
				t.Fatalf("sprite visibility = %v, on-axis control = %v", visible, aheadVisible)
			}
			if math.Abs(size-wantSize) > 1e-6 || math.Abs(bottom-wantBottom) > 1e-6 {
				t.Fatalf("size/bottom = %.3f/%.3f, want %.3f/%.3f from the perpendicular depth", size, bottom, wantSize, wantBottom)
			}
		})
	}
}
