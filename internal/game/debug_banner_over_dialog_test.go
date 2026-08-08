//go:build debug

package game

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"

	"github.com/hajimehoshi/ebiten/v2"
)

// Quest news and legendary drops are allowed through while a conversation is
// open (visibleScreenBanner), and the dialog fills the screen with a 50% dim -
// so the banner has to be painted ABOVE it. Drawn with the rest of the HUD it
// still appears, at half brightness, which reads as a rendering bug rather than
// a heading.
//
// Draw order can only be checked by drawing, and a draw frame needs the debug
// harness; the plain suite has no way to see this.
func TestDebugSim_BannerIsNotDimmedByTheDialog(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.gameLoop.inputHandler.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))

	const bannerText = "Reward claimed - The Missing Volumes"
	npc := &character.NPC{
		Name: "Archivist Ilthaea", RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{Greeting: "The shelves are yours."},
	}
	geo := screenBannerLayout(g.config.GetScreenWidth(), bannerText, 0)

	logicalW, logicalH := g.gameLoop.Layout(1600, 900)
	logical := ebiten.NewImage(logicalW, logicalH)

	// The heading's own gold, at its brightest: pixels with little blue in them,
	// which the plate (near-black) and the world showing through its translucent
	// edge can never be. Dimming the banner halves this number.
	frame := func(dialog bool) int {
		gold := 0
		runOnDrawFrame(func(_ *ebiten.Image) {
			g.dialogActive = dialog
			if dialog {
				g.dialogNPC = npc
			} else {
				g.dialogNPC = nil
			}
			g.screenBannerQueue = []screenBanner{{
				text:  bannerText,
				kind:  bannerQuestPaid,
				frame: g.bannerInFrames() + g.bannerHoldFrames(bannerQuestPaid)/2, // parked mid-hold, fully faded in
			}}
			logical.Clear()
			renderer.RenderFirstPersonView(logical)
			g.gameLoop.ui.Draw(logical)

			for y := geo.plateY; y < geo.plateY+geo.plateH; y++ {
				for x := geo.plateX; x < geo.plateX+geo.plateW; x++ {
					cr, _, cb, _ := logical.At(x, y).RGBA()
					if r8, b8 := int(cr>>8), int(cb>>8); b8 < 150 && r8 > gold {
						gold = r8
					}
				}
			}
			t.Logf("dialog=%v brightest heading gold = %d", dialog, gold)
		})
		return gold
	}

	open := frame(false)
	withDialog := frame(true)
	if open < 200 {
		t.Fatalf("the banner's gold reads %d with no dialog open - the fixture is wrong", open)
	}
	if withDialog != open {
		t.Fatalf("the heading reads %d with the dialog open and %d without: it is being painted under the dialog's dim",
			withDialog, open)
	}

	// QA shot of the frame that was measured.
	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_banner_dialog")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, "banner_over_dialog.png"))
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
	t.Logf("banner over dialog -> %s", out)
}
