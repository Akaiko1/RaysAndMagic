//go:build debug

package game

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDebugSim_VictoryOverlayRender writes both interactive states of the live
// Victory overlay to Downloads for visual QA.
func TestDebugSim_VictoryOverlayRender(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()

	g.totalGoldEarned = 18425
	g.totalExperienceEarned = 327860
	g.sessionStartTime = time.Now().Add(-2*time.Hour - 17*time.Minute - 42*time.Second)
	g.victoryTime = time.Now()
	g.victoryNameInput = "Raywalker"
	for i, member := range g.party.Members {
		if member != nil {
			member.Level = 33 + i
		}
	}

	outDir := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_victory_screen")
	if err := os.RemoveAll(outDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	render := func(name string, saved bool) {
		g.victoryScoreSaved = saved
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			renderer.RenderFirstPersonView(screen)
			g.gameLoop.ui.drawVictoryOverlay(screen)
		})
		path := filepath.Join(outDir, name+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}

	render("victory_name_entry", false)
	render("victory_score_saved", true)
}
