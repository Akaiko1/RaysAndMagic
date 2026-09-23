//go:build debug

package game

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"ugataima/internal/storage"
)

// The frame caches preserve the full compass, including minimap layering and
// live RT/TB heading. Radius changes rebuild; pose/world changes reuse. Images
// are derived after load and never enter saves.
func TestCompassFrameCache(t *testing.T) {
	requireStandeeGPU(t)
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	ui := g.gameLoop.ui
	for _, tc := range []struct {
		name         string
		w, h         int
		angle        float64
		tb, newWorld bool
	}{
		{"1280x720-RT", 1280, 720, 0, false, false},
		{"1280x720-RT-turned", 1280, 720, 1, false, false},
		{"1280x720-TB", 1280, 720, 2, true, false},
		{"1280x720-world-change", 1280, 720, 2, false, true},
		{"1920x1080-RT", 1920, 1080, 0.5, false, false},
		{"800x600-RT", 800, 600, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h := g.gameLoop.Layout(tc.w, tc.h)
			captureGameplayPreviewFrame(t, g, w, h)
			g.camera.Angle = tc.angle
			g.turnBasedMode = tc.tb
			g.viewTurnFramesLeft = 0
			if tc.tb {
				g.viewTurnFramesLeft = 3
				g.viewAngleRender = tc.angle + 0.3
			}
			oldWorld := g.world
			if tc.newWorld {
				copyWorld := *g.world
				g.world = &copyWorld
			}
			defer func() { g.world = oldWorld }()
			prior, priorRadius := ui.compassFrameBackground, ui.compassFrameRadius
			radius := ui.compassRadius()
			var sum, changed, peak int
			runOnDrawFrame(func(*ebiten.Image) {
				actual, reference := ebiten.NewImage(w, h), ebiten.NewImage(w, h)
				defer actual.Deallocate()
				defer reference.Deallocate()
				// Nonblack backing exercises premultiplied-alpha composition.
				backing := color.RGBA{43, 67, 91, 255}
				actual.Fill(backing)
				reference.Fill(backing)
				ui.drawCompass(actual)
				x, y := ui.getCompassCenter()
				ui.drawCompassUncachedReference(reference, x, y)
				a, b := make([]byte, w*h*4), make([]byte, w*h*4)
				actual.ReadPixels(a)
				reference.ReadPixels(b)
				if out := os.Getenv("RAM_COMPASS_PREVIEW"); out != "" {
					if err := os.MkdirAll(out, 0755); err != nil {
						t.Error(err)
						return
					}
					crop := image.Rect(x-radius-18, y-radius-20, x+radius+18, y+radius+20)
					for label, pixels := range map[string][]byte{"uncached": b, "cached": a} {
						im := &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
						f, err := os.Create(filepath.Join(out, "compass-"+tc.name+"-"+label+".png"))
						if err != nil {
							t.Error(err)
							continue
						}
						err = png.Encode(f, im.SubImage(crop))
						closeErr := f.Close()
						if err != nil {
							t.Error(err)
						}
						if closeErr != nil {
							t.Error(closeErr)
						}
					}
				}
				for i := range a {
					d := absInt(int(a[i]) - int(b[i]))
					sum += d
					peak = max(peak, d)
					// The old absolute-coordinate stroke has a coordinate-dependent
					// join at the eastern closure of each circle. Only those tiny
					// seam patches may differ beyond alpha rounding tolerance.
					px, py := (i/4)%w-x, (i/4)/w-y
					seam := absInt(py) <= 1 && (absInt(px-radius) <= 1 || absInt(px-radius-4) <= 1)
					// The thin outer stroke also has coordinate-dependent AA at
					// its boundary. Bound that rounding separately from interiors.
					edgeRounding := math.Abs(math.Hypot(float64(px)+0.5, float64(py)+0.5)-float64(radius+4)) < 1 && d <= 12
					if d > 2 && !seam && !edgeRounding {
						t.Errorf("compass changed outside stroke seam at (%d,%d): %d/%d", px, py, a[i], b[i])
					}
					if d > 1 {
						changed++
					}
				}
				if ui.compassFrameBackground == nil || ui.compassFrameOutline == nil || ui.compassMapMask == nil {
					t.Error("production compass did not populate both frame layers")
				}
				if prior != nil && priorRadius == radius && prior != ui.compassFrameBackground {
					t.Error("pose/world change rebuilt the static frame")
				}
				cached := ui.compassFrameBackground
				ui.drawCompass(actual)
				if cached != ui.compassFrameBackground {
					t.Error("repeated draw rebuilt the frame")
				}
			})
			t.Logf("radius=%d diff sum=%d channels>1=%d peak=%d", radius, sum, changed, peak)
			// Local-coordinate tessellation and one intermediate alpha blend
			// can differ at antialiased edges; interiors must stay unchanged.
			if changed > radius*8 || sum > radius*80 {
				t.Fatalf("compass image changed: channels>1=%d sum=%d", changed, sum)
			}
		})
	}
	runOnDrawFrame(func(*ebiten.Image) {
		ui.releaseCompassFrame()
		if ui.compassFrameBackground != nil || ui.compassFrameOutline != nil || ui.compassMapMask != nil {
			t.Error("frame images retained after release")
		}
		target := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
		defer target.Deallocate()
		ui.drawCompass(target)
		if ui.compassFrameBackground == nil || ui.compassFrameOutline == nil || ui.compassMapMask == nil {
			t.Error("released frame was not reconstructed on draw")
		}
	})

}

// Uncached frame is the image oracle, independent of cache construction.
func (ui *UISystem) drawCompassUncachedReference(screen *ebiten.Image, compassX, compassY int) {
	compassRadius := ui.compassRadius()

	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+5), color.RGBA{66, 48, 24, 245}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+3), color.RGBA{194, 153, 66, 255}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), color.RGBA{8, 14, 23, 235}, true)

	ui.drawCompassMinimap(screen, compassX, compassY, compassRadius)

	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), 2, color.RGBA{98, 140, 181, 245}, true)
	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+4), 1, color.RGBA{255, 218, 115, 245}, true)

	// A single north-up map and a rotating player pointer avoid the ambiguity of
	// the old red line, which looked like either a heading or a target marker.
	angle := ui.game.camera.Angle
	if ui.game.viewTurnFramesLeft > 0 {
		angle = ui.game.viewAngleRender
	}
	tipRadius := float64(compassRadius - 9)
	tipX := float64(compassX) + math.Cos(angle)*tipRadius
	tipY := float64(compassY) + math.Sin(angle)*tipRadius
	rearX := float64(compassX) - math.Cos(angle)*5
	rearY := float64(compassY) - math.Sin(angle)*5
	perpX := -math.Sin(angle) * 5
	perpY := math.Cos(angle) * 5
	verts := []ebiten.Vertex{
		{DstX: float32(tipX), DstY: float32(tipY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.35, ColorG: 0.85, ColorB: 1, ColorA: 1},
		{DstX: float32(rearX + perpX), DstY: float32(rearY + perpY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.08, ColorG: 0.35, ColorB: 0.8, ColorA: 1},
		{DstX: float32(rearX - perpX), DstY: float32(rearY - perpY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.08, ColorG: 0.35, ColorB: 0.8, ColorA: 1},
	}
	screen.DrawTriangles(verts, []uint16{0, 1, 2}, hudWhiteImg, nil)
	vector.StrokeLine(screen, float32(tipX), float32(tipY), float32(rearX+perpX), float32(rearY+perpY), 1, color.RGBA{215, 244, 255, 245}, true)
	vector.StrokeLine(screen, float32(tipX), float32(tipY), float32(rearX-perpX), float32(rearY-perpY), 1, color.RGBA{215, 244, 255, 245}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), 4, color.RGBA{220, 245, 255, 255}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), 2, color.RGBA{32, 124, 220, 255}, true)

	cardinalColor := color.RGBA{236, 214, 156, 255}
	drawDebugTextColored(screen, "N", compassX-3, compassY-compassRadius-17, rarityGold)
	drawDebugTextColored(screen, "E", compassX+compassRadius+8, compassY-8, cardinalColor)
	drawDebugTextColored(screen, "S", compassX-3, compassY+compassRadius+3, cardinalColor)
	drawDebugTextColored(screen, "W", compassX-compassRadius-14, compassY-8, cardinalColor)
}
