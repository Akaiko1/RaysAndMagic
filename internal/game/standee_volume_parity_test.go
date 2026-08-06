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
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestStandeeVolumeMatchesMaterialPath(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("needs a live Draw frame; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	g, _, cfg := bootOpenWorldGame(t, false)
	r := g.gameLoop.renderer
	w, h := cfg.GetScreenWidth(), cfg.GetScreenHeight()
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
			heightF, bottomF, 1, 1, 1, true, false, footprint, r.standeeSurfaces[:0])
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

	draw := func(volume bool) []byte {
		img := ebiten.NewImage(w, h)
		runOnDrawFrame(func(_ *ebiten.Image) {
			for i := range g.depthBuffer {
				g.depthBuffer[i] = g.camera.ViewDist
			}
			img.Clear()
			r.drawCrossedSlabs(img, sprite, key, worldX, worldY, yawA, yawB,
				footprint, depth, heightF, bottomF, 1, volume)
		})
		pix := make([]byte, 4*w*h)
		img.ReadPixels(pix)
		return pix
	}

	volumePix := draw(true)
	materialPix := draw(false)

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
}
