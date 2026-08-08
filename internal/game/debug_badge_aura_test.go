//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Badge aura QA: three phases of the pulse on one portrait that owns BOTH
// progression badges (unspent stat points AND a pending skill choice), so the
// green and gold frames can be compared side by side at peak, trough and midway.
// This is the shot to re-take whenever the aura's geometry or tint changes.
func TestDebugSim_BadgeAuraPhases(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.gameLoop.inputHandler.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	g.camera.Angle = 45 * math.Pi / 180
	g.party.Members[0].FreeStatPoints = 3
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0}}

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_badge_aura")
	// A stale directory left behind would mix last session's frames into the QA
	// comparison, so both steps are checked (as the HUD gallery does).
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	period := int64(g.framesForSeconds(badgeAuraPeriodSeconds))
	for _, frame := range []int64{0, period / 2, period * 3 / 4} {
		logicalW, logicalH := g.gameLoop.Layout(1600, 900)
		logical := ebiten.NewImage(logicalW, logicalH)
		runOnDrawFrame(func(_ *ebiten.Image) {
			// Pin both clocks INSIDE the draw callback: Update keeps running
			// between iterations. The aura follows the modal-safe UI clock; the
			// world clock stays fixed so only the requested aura phase changes.
			g.frameCount = 0
			g.uiFrameCount = frame
			if got := g.gameLoop.ui.cardAnimClock(); got != frame {
				t.Fatalf("badge aura UI clock = %d, want gallery phase %d", got, frame)
			}
			logical.Clear()
			renderer.RenderFirstPersonView(logical)
			g.gameLoop.ui.Draw(logical)
		})
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("badges_frame_%02d.png", frame)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, logical); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("badge aura -> %s", out)
}
