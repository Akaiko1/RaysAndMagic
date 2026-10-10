package game

import (
	"math"
	"testing"
	"ugataima/internal/monster"
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

// A view preference is presentation only, including the pixel-pick geometry
// and impacts. A camera crossing must release the correction instead of pinning
// the creature to the HUD while its projected size grows without bound.
func TestMonsterViewPresentation(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	g.renderHelper = NewRenderingHelper(g)
	g.showPartyStats = true
	g.camera.X, g.camera.Y, g.camera.Angle = 320, 320, 0
	g.camera.FOV = squareProjectionFOV(g.worldWidth(), g.worldHeight())
	g.camera.ViewDist = cfg.GetViewDistance()
	ts := cfg.GetTileSize()
	logical := *g.camera
	for _, wide := range []bool{false, true} {
		g.combatPreferences.WideView = wide
		for _, key := range []string{"bat", "rat", "goblin", "troll", "mountain_troll", "orc_hero_boss", "dragon"} {
			m := monster.NewMonster3DFromConfig(320+ts, 320, key, cfg)
			a := visualAnchorFor(m)
			_, ground, size, ok := g.renderHelper.CalculateMonsterSpriteMetricsF(m.X, m.Y, ts, a.sizeTiles)
			if !ok {
				t.Fatal("one-tile monster not projected")
			}
			bottom := a.bottom(g, ts, ground, size)
			if wide && (bottom > float64(worldViewportBottom(g)) || bottom-size < 0) {
				t.Fatalf("Wide clips %s at one tile: %.2f..%.2f", key, bottom-size, bottom)
			}
			if !wide && size < float64(2*worldViewportBottom(g)-g.worldHeight()) && !m.Flying && bottom > float64(worldViewportBottom(g))+.01 {
				t.Fatalf("Classic did not clear HUD for %s", key)
			}
			if math.Abs(a.centerOffset(g, ts)-(bottom-size/2-g.viewHorizon())) > 1e-6 {
				t.Fatal("impact anchor disagrees with sprite")
			}
			c := &monsterCorpse{sizeTiles: a.sizeTiles, flying: a.flying, arborealHeight: a.heightTiles, started: g.frameCount}
			if got := g.corpseBottom(c, ts, ground, size); math.Abs(got-bottom) > 1e-6 {
				t.Fatal("death moved the live anchor")
			}
			g.frameCount += int64(math.Ceil(g.monsterDeathSettings().FallSeconds * float64(cfg.GetTPS())))
			groundAnchor := a
			groundAnchor.flying = false
			groundAnchor.heightTiles = 0
			if got := g.corpseBottom(c, ts, ground, size); math.Abs(got-groundAnchor.bottom(g, ts, ground, size)) > 1e-6 {
				t.Fatal("corpse landed behind the grounded body anchor")
			}

		}
		if *g.camera != logical {
			t.Fatal("view altered logical camera")
		}
	}
	g.combatPreferences.WideView = false
	lastLift := math.Inf(1)
	for depth := ts; depth >= ts*.2; depth -= ts * .01 {
		ground := g.renderHelper.calculateFloorScreenYF(depth)
		size := g.viewFocal() * .35 * ts / depth
		lift := g.monsterHUDLift(depth, ground, size)
		if lift < 0 || lift > float64(g.worldHeight()-worldViewportBottom(g)) || lift > lastLift+1e-6 {
			t.Fatal("camera crossing increased the lift")
		}
		if depth <= cfg.Graphics.View.LiftFadeTiles*ts && lift != 0 {
			t.Fatal("lift survives near camera")
		}
		lastLift = lift
	}
	for _, depth := range []float64{0, -ts} {
		if g.monsterHUDLift(depth, 1000, 800) != 0 {
			t.Fatal("lift exists behind camera")
		}
	}
}
