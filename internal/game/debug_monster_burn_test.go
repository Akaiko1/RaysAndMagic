//go:build debug

package game

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	monsterPkg "ugataima/internal/monster"

	"github.com/hajimehoshi/ebiten/v2"
)

// QA shot for the burning-monster overlay (Drakefang's ignite, a fire spell, a
// Firewall tick): one lit mob beside an unlit one, three frames of the flicker.
// The overlay can only be judged by drawing it - retake this whenever the mob
// flame tuning changes.
//
//	RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_BurningMonster
func TestDebugSim_BurningMonster(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.gameLoop.inputHandler.switchToMap("forest")
	ts := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, ts)
	g.camera.Angle = 0

	g.world.Monsters = nil
	// One burning mob dead ahead, one unlit beside it for comparison.
	for i, key := range []string{"wolf", "wolf"} {
		m := monsterPkg.NewMonster3DFromConfig(g.camera.X+3.0*ts, g.camera.Y+float64(i)*1.1*ts-0.55*ts, key, g.config)
		if m == nil {
			continue
		}
		m.MaxHitPoints, m.HitPoints = 100000, 100000
		if i == 0 {
			m.ApplyBurn(20 * g.config.GetTPS())
		}
		g.world.Monsters = append(g.world.Monsters, m)
	}

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_burn")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	logicalW, logicalH := g.gameLoop.Layout(1600, 900)
	logical := ebiten.NewImage(logicalW, logicalH)
	for _, frame := range []int64{0, 40, 80} {
		runOnDrawFrame(func(_ *ebiten.Image) {
			g.frameCount = frame
			logical.Clear()
			renderer.RenderFirstPersonView(logical)
		})
		f, err := os.Create(filepath.Join(out, "burn_frame_"+string(rune('0'+frame/40))+".png"))
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
	t.Logf("burning monsters -> %s", out)
}
