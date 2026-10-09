//go:build debug

package game

// The volume shader claims to composite "the exact shell stack in one fragment
// invocation". This test holds it to that: the SAME crossed-tree standee is
// drawn twice - once through the one-pass volume compositor, once through the
// per-surface material path - and the two frames are compared pixel by pixel.
// A broken volume path (missing layer, wrong shade, flipped U, ignored wall
// clip) shows up here as a coverage or brightness gap.
//
// Run with:
//
//	RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestStandeeVolumeMatchesMaterialPath -v
import (
	"bytes"
	"math"
	"os"
	"slices"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNaturalCrossLayerBudgetReachesBothRenderPaths(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw")
	}
	t.Chdir("../..")
	g, _, cfg := bootOpenWorldGame(t, false)
	r := g.gameLoop.renderer
	cfg.Graphics.Standee.CrossedStandeeLayers = config.CrossedStandeeLayersConfig{
		DistanceBands: []config.StandeeLayerBand{
			{DistanceTiles: 0, Layers: 16}, {DistanceTiles: 1, Layers: 8},
			{DistanceTiles: 2, Layers: 4}, {DistanceTiles: 3, Layers: 2}, {DistanceTiles: 5, Layers: 0},
		},
		ForwardHalfWidthTiles: 1.5, OffAxisMaxDistanceTiles: 1,
	}
	if !tileIsNaturalCross(world.TileTree) {
		t.Fatal("fixture must be a natural cross")
	}
	w, h := g.worldWidth(), g.worldHeight()
	g.depthBuffer, g.wallTopBuffer = make([]float64, w), make([]float64, w)
	for i := range g.depthBuffer {
		g.depthBuffer[i] = 100000
	}
	ts := float64(cfg.GetTileSize())
	x, y := TileCenterFromTile(10, 10, ts)
	for _, tc := range []struct {
		name                    string
		forward, lateral, angle float64
		want                    int
	}{
		{"point blank", .5, 0, 0, 16}, {"one", 1, 0, 0, 8}, {"two", 2, 0, 0, 4},
		{"three", 3, 0, 0, 2}, {"four", 4, 0, 0, 2}, {"five", 5, 0, 0, 0},
		{"far cross", 30, 0, 0, 0}, {"side lane", 3, 1.4, 0, 2}, {"outside lanes", 3, 1.6, 0, 0},
		{"rotated lane", 2.1, 1.2, .7, 4}, {"rotated outside", 2.1, -1.6, .7, 0},
		{"authored budget", 2.25, 0, 0, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "authored budget" {
				cfg.Graphics.Standee.CrossedStandeeLayers.DistanceBands[2].Layers = 6
			}
			g.camera.Angle = tc.angle
			dx, dy := math.Cos(tc.angle), math.Sin(tc.angle)
			g.camera.X = x - ts*(tc.forward*dx-tc.lateral*dy)
			g.camera.Y = y - ts*(tc.forward*dy+tc.lateral*dx)
			size := 2 * ts * g.viewFocal() / (tc.forward * ts)
			entry := UnifiedSpriteRenderData{spriteType: SpriteTypeTree, tileType: world.TileTree, tileX: 10, tileY: 10, spriteName: "tree", sizeF: size, bottomF: r.game.renderHelper.calculateFloorScreenYF(tc.forward * ts), depthPerp: tc.forward * ts, distance: math.Hypot(tc.forward, tc.lateral) * ts}
			parts := r.splitCrossedTreesForPainterOrder([]UnifiedSpriteRenderData{entry}, 0, 1)
			if len(parts) != 4 {
				t.Fatalf("cross collapsed to %d painter entries", len(parts))
			}
			var split, whole []byte
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(w, h)
				defer dst.Deallocate()
				r.crossedGeometry.begin()
				for _, p := range parts {
					r.drawCrossedTreeStandees(dst, p)
				}
				for i := 0; i < r.crossedGeometry.used; i++ {
					slab := r.crossedGeometry.slabs[i].slab
					if got := len(slab.surfaces) - 2; got != tc.want {
						t.Errorf("slab %d: %d interior layers, want %d", i, got, tc.want)
					}
					if len(slab.surfaces) < 2 || slab.surfaces[0].mipKey.layer != standeeMipSticker || slab.surfaces[len(slab.surfaces)-1].mipKey.layer != standeeMipSticker {
						t.Error("outer faces lost")
					}
				}
				if r.crossedGeometry.used != 2 {
					t.Errorf("prepared %d slabs, want two", r.crossedGeometry.used)
				}
				r.crossedGeometry.end()
				// Compare in painter order, just like the production collector.
				dst.Clear()
				slices.SortStableFunc(parts, compareUnifiedSprites)
				for _, p := range parts {
					r.drawCrossedTreeStandees(dst, p)
				}
				split = make([]byte, w*h*4)
				dst.ReadPixels(split)
				dst.Clear()
				r.drawCrossedTreeStandees(dst, entry)
				whole = make([]byte, w*h*4)
				dst.ReadPixels(whole)
			})
			if !bytes.Equal(split, whole) {
				t.Error("split and whole-cross paths disagree")
			}
		})
	}
}

func TestStandeeVolumeMatchesMaterialPath(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("needs a live Draw frame; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	g, _, cfg := bootOpenWorldGame(t, false)
	r := g.gameLoop.renderer
	w, h := cfg.GetScreenWidth(), cfg.GetScreenHeight()
	g.depthBuffer, g.wallTopBuffer = make([]float64, w), make([]float64, w)
	tile := float64(cfg.GetTileSize())

	sprite := g.sprites.GetSprite("tree")
	if sprite == nil {
		t.Fatal("tree sprite missing")
	}
	key := makeStandeeCoreKey(r.prefixedStandeeKeyName("tree", "tree"), sprite, true)

	// Close enough that the slab carries many core shells (the volume path only
	// engages past standeeVolumeMinShells).
	g.camera.Angle = 0
	g.camera.X, g.camera.Y = 4*tile, 10.5*tile
	const yawA, yawB = math.Pi / 4, 3 * math.Pi / 4
	// Walk in until the slab carries enough core shells for the volume path
	// (shell count grows as the token fills the screen).
	var depth, worldX, worldY, sizeF, heightF, bottomF, footprint float64
	engaged := false
	for _, tiles := range []float64{2.2, 1.6, 1.2, 0.9, 0.7, 0.5, 0.35} {
		depth = tiles * tile
		worldX, worldY = g.camera.X+depth, g.camera.Y
		sizeF = tile * float64(w) / (2 * math.Tan(g.camera.FOV/2) * depth)
		heightF = sizeF * float64(sprite.Bounds().Dy()) / float64(sprite.Bounds().Dx())
		bottomF = r.game.renderHelper.calculateFloorScreenYF(depth)
		footprint = r.spriteFootprintWorld(sizeF, depth)
		probe, ok := r.prepareStandeeSlab(sprite, key, worldX, worldY, yawA, depth,
			heightF, bottomF, 1, 1, 1, true, false, footprint, r.standeeSurfaces[:0], -1)
		r.standeeSurfaces = probe.surfaces[:0]
		if !ok {
			continue
		}
		probe.volumeComposite = true
		if canUseStandeeVolume(probe) {
			t.Logf("volume path engages at %.2f tiles (%d surfaces)", tiles, len(probe.surfaces))
			engaged = true
			break
		}
	}
	if !engaged {
		t.Fatal("no tested distance engaged the volume path - the threshold or shell math changed")
	}

	draw := func(volume, walls bool) []byte {
		img := ebiten.NewImage(w, h)
		defer img.Deallocate()
		runOnDrawFrame(func(_ *ebiten.Image) {
			for i := range g.depthBuffer {
				g.depthBuffer[i], g.wallTopBuffer[i] = g.camera.ViewDist, 0
				if walls {
					offset := float64(i-w/2) + 0.5
					g.depthBuffer[i] = 1 / (1/(depth*0.6) + offset*1e-6)
					g.wallTopBuffer[i] = bottomF - heightF*0.5 + offset*0.12
				}
			}
			img.Clear()
			r.drawCrossedSlabs(img, sprite, key, worldX, worldY, yawA, yawB,
				footprint, depth, heightF, bottomF, 1, volume, -1)
		})
		pix := make([]byte, 4*w*h)
		img.ReadPixels(pix)
		return pix
	}

	clearCovered := 0
	for _, walls := range []bool{false, true} {
		name := "clear"
		if walls {
			name = "sloped foreground wall"
		}
		t.Run(name, func(t *testing.T) {
			volumePix := draw(true, walls)
			materialPix := draw(false, walls)

			var volumeCovered, materialCovered, both, diffSum int
			for i := 0; i < len(volumePix); i += 4 {
				va, ma := volumePix[i+3], materialPix[i+3]
				if va > 8 {
					volumeCovered++
				}
				if ma > 8 {
					materialCovered++
				}
				if va > 8 && ma > 8 {
					both++
					for c := 0; c < 3; c++ {
						d := int(volumePix[i+c]) - int(materialPix[i+c])
						if d < 0 {
							d = -d
						}
						diffSum += d
					}
				}
			}
			if volumeCovered == 0 || materialCovered == 0 {
				t.Fatalf("setup drew nothing: volume=%d material=%d covered pixels", volumeCovered, materialCovered)
			}
			if !walls {
				clearCovered = volumeCovered
			} else if volumeCovered >= clearCovered*9/10 {
				t.Fatalf("wall fixture did not hide enough of the tree: clear=%d clipped=%d", clearCovered, volumeCovered)
			}
			coverage := float64(both) / math.Max(float64(volumeCovered), float64(materialCovered))
			meanDiff := float64(diffSum) / float64(both*3)
			t.Logf("volume=%d material=%d shared=%d pixels; silhouette overlap %.3f; mean RGB delta %.2f/255",
				volumeCovered, materialCovered, both, coverage, meanDiff)

			if coverage < 0.97 {
				t.Errorf("silhouette overlap %.3f - the volume path draws a different shape", coverage)
			}
			if meanDiff > 6 {
				t.Errorf("mean RGB delta %.2f - the volume path shades the stack differently", meanDiff)
			}
		})
	}
}
