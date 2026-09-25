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
	"ugataima/internal/character"
)

// Uses the production Update/Draw/loading pipeline at native Layout sizes.
func TestDebugSim_DervishGallery(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	out := os.Getenv("RAM_DERVISH_QA_DIR")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	var npc *character.NPC
	runOnDrawFrame(func(*ebiten.Image) {
		if err := g.switchToMap("nomad_city"); err != nil {
			t.Error(err)
			return
		}
		for _, ch := range g.party.Members {
			ch.Level = 20
		}
		g.updatePartyLevelUnlocks()
		for _, n := range g.world.NPCs {
			if n.Key == "nomad_city_safiya" {
				npc = n
				break
			}
		}
		if npc == nil {
			return
		}
		g.setPartyPosition(npc.X, npc.Y+2*g.config.GetTileSize())
		g.snapFacing(-math.Pi / 2)
	})
	if npc == nil {
		t.Fatal("Safiya missing from Dunehold")
	}
	for _, res := range [][2]int{{800, 600}, {1280, 720}, {1920, 1080}} {
		w, h := g.gameLoop.Layout(res[0], res[1])
		for _, dialog := range []bool{false, true} {
			runOnDrawFrame(func(*ebiten.Image) {
				if dialog {
					(&InputHandler{game: g}).openNPCInteraction(npc)
				} else {
					g.closeConversation()
				}
			})
			shot := captureGameplayPreviewFrame(t, g, w, h)
			path := filepath.Join(out, fmt.Sprintf("safiya_%dx%d_dialog_%v.png", w, h, dialog))
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, shot); err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Capture all idle phases through the real world renderer after streaming
	// has settled. Select only the animation clock, not scene or sizing rules.
	w, h := g.gameLoop.Layout(1280, 720)
	runOnDrawFrame(func(*ebiten.Image) { g.closeConversation() })
	captureGameplayPreviewFrame(t, g, w, h)
	for i := 0; i < SpriteSheetFrameCount; i++ {
		runOnDrawFrame(func(*ebiten.Image) {
			g.frameCount = int64(i * animationTicksPerFrame(g.config.GetTPS(), NPCIdleAnimationFPS))
		})
		shot := captureGameplayPreviewFrame(t, g, w, h)
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("safiya_game_idle_%d.png", i+1)))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("idle capture write: %v %v", err, closeErr)
		}
	}
}
