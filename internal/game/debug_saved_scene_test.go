//go:build debug

package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ugataima/internal/storage"
)

// Load a private copy: neither migrations nor diagnostic Update/Draw may write
// to the user's save/profile. Images are native production game frames.
func TestSavedScenePreview(t *testing.T) {
	requireStandeeGPU(t)
	source, out := os.Getenv("RAM_PREVIEW_SAVE"), os.Getenv("RAM_SCENE_PREVIEW")
	if source == "" || out == "" {
		t.Skip("set RAM_PREVIEW_SAVE and RAM_SCENE_PREVIEW")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	copyPath := filepath.Join(root, "input.json")
	if err := os.WriteFile(copyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	if err := g.LoadGameFromFile(copyPath); err != nil {
		t.Fatal(err)
	}
	w, h := g.gameLoop.Layout(1919, 1080)
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"saved_view", "turn_left", "turn_right"} {
		original := g.camera.Angle
		switch name {
		case "turn_left":
			g.snapFacing(original - 0.01)
		case "turn_right":
			g.snapFacing(original + 0.02)
		}
		shot := captureGameplayPreviewFrame(t, g, w, h)
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	g.turnBasedMode = false
	// Check the production Draw consumer after real collision-resolved motion.
	// Virtual-cadence tests independently check smoothness at differing rates.
	runOnDrawFrame(func(_ *ebiten.Image) {
		before := g.cameraPose()
		g.gameLoop.inputHandler.movePlayer(math.Cos(before.angle)*0.25, math.Sin(before.angle)*0.25)
		after := g.cameraPose()
		if after == before {
			t.Error("preview movement was blocked")
			return
		}
		g.resetCameraPresentation()
		g.finishCameraTick(before, g.cameraPresentation.epoch, time.Now())
		dst := ebiten.NewImage(w, h)
		defer dst.Deallocate()
		g.Draw(dst)
		p := g.cameraPresentation
		if !p.presentedValid || g.cameraPose() != after {
			t.Error("Draw did not consume/restore the camera presentation")
		}
		if p.presented.x < math.Min(before.x, after.x)-1e-6 || p.presented.x > math.Max(before.x, after.x)+1e-6 {
			t.Error("Draw extrapolated outside logical motion")
		}
	})

}
