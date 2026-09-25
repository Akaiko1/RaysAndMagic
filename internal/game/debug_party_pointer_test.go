//go:build debug

package game

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_PartySelectionInRenderedHub(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	gl := g.gameLoop
	fp := installFakePointer(t)
	g.mainMenuOpen = false
	g.menuOpen, g.showPartyStats = true, true
	w, h := gl.Layout(1280, 720)
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	settle := func() {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			ready := false
			runOnDrawFrame(func(*ebiten.Image) {
				if err := gl.Update(); err != nil {
					t.Error(err)
				}
				gl.Draw(frame)
				ready = gl.loading != nil && !gl.loading.awaitingFrame && gl.loading.front != nil
			})
			if ready {
				return
			}
		}
		t.Fatal("hub did not finish rendering")
	}
	for _, tb := range []bool{false, true} {
		for tab := TabInventory; tab <= TabCards; tab++ {
			t.Run(fmt.Sprintf("tb=%v/tab=%d", tb, tab), func(t *testing.T) {
				g.turnBasedMode, g.currentTab = tb, tab
				fp.idle()
				settle()
				for _, member := range []int{1, 0} {
					x, y := partyPointerPosition(g, member)
					fp.moveTo(x, y)
					fp.press()
					runOnDrawFrame(func(*ebiten.Image) {
						if err := gl.Update(); err != nil {
							t.Error(err)
						}
						gl.Draw(frame)
					})
					if g.selectedChar != member {
						t.Fatalf("rendered hub selected %d, want %d", g.selectedChar, member)
					}
					fp.hold()
					settle()
					fp.release()
					settle()
					fp.idle()
					if g.selectedChar != member {
						t.Fatal("one physical click selected more than once")
					}
				}
			})
		}
	}
}
