//go:build debug

package game

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Fixed camera samples for visual before/after videos, not FPS measurement.
// Encode the numbered native frames at 30 FPS. Production loading, Update and
// Draw remain active; scripted positioning keeps the camera route identical.
func TestRiverVisualFrames(t *testing.T) {
	out := os.Getenv("RAM_RIVER_VIDEO_FRAMES")
	if out == "" {
		t.Skip("RAM_RIVER_VIDEO_FRAMES required")
	}
	requireStandeeGPU(t)
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	w, h := g.gameLoop.Layout(1920, 1080)
	tile := float64(g.config.GetTileSize())
	place := func(x, angle float64) {
		wx, wy := world.GlobalWorldManager.ProjectWorldPos("forest", x*tile, 36.5*tile)
		g.setPartyPosition(wx, wy)
		g.snapFacing(world.GlobalWorldManager.ProjectAngle("forest", angle))
	}
	place(13.5, 0)
	captureGameplayPreviewFrame(t, g, w, h)
	target := ebiten.NewImage(w, h)
	defer target.Deallocate()
	frame := image.NewRGBA(image.Rect(0, 0, w, h))
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	for i := 0; i < 240; i++ {
		x, angle := 13.5+4*float64(min(i, 119))/119, 0.0
		if i >= 120 {
			angle = math.Pi * float64(i-120) / 119
		}
		place(x, angle)
		var updateErr error
		runOnDrawFrame(func(*ebiten.Image) {
			updateErr = g.Update()
			g.resetCameraPresentation()
			g.Draw(target)
			target.ReadPixels(frame.Pix)
		})
		if updateErr != nil {
			t.Fatal(updateErr)
		}
		if g.gameLoop.loading.awaitingFrame {
			t.Fatal("visual route left its preloaded regions")
		}
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("%04d.png", i)))
		if err != nil {
			t.Fatal(err)
		}
		err = encoder.Encode(f, frame)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	t.Logf("240 native %dx%d frames at 30 FPS -> %s", w, h, out)
}
