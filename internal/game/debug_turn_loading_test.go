//go:build debug

package game

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/world"
)

func TestDebugSim_TurnLoadingUsesLogicalCamera(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	gl := g.gameLoop
	wm := world.GlobalWorldManager
	save := g.buildSave(wm)
	save.TurnBased = true
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if !g.turnBasedMode {
		t.Fatal("fixture did not restore turn-based mode")
	}
	g.mainMenuOpen = false
	w, h := gl.Layout(1280, 720)
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	ready := false
	deadline := time.Now().Add(60 * time.Second)
	for !ready && time.Now().Before(deadline) {
		runOnDrawFrame(func(*ebiten.Image) {
			gl.Update()
			g.Draw(frame)
			ready = gl.loading != nil && !gl.loading.awaitingFrame && gl.loading.front != nil
		})
	}
	if !ready {
		t.Fatal("fixture did not finish loading")
	}
	// Both regions are in range, but only the snapped destination view sees
	// the eastern one. Its upload represents a worker that has not finished.
	wm.OpenWorld = g.world
	wm.OpenWorldRegions = []world.OpenWorldRegion{
		{MapKey: "arena", OffsetX: 0, OffsetY: 0, Width: 5, Height: 5},
		{MapKey: "church", OffsetX: 10, OffsetY: 2, Width: 1, Height: 1},
	}
	r.mapRenderResidentMapKeys = []string{"arena", "church"}
	r.mapRenderResourcesByMap["church"] = &mapRenderRegionResources{}
	ts := g.config.GetTileSize()
	g.setPartyPosition(2.5*ts, 2.5*ts)
	g.camera.ViewDist = 20 * ts
	g.camera.FOV = math.Pi / 2
	for _, tb := range []bool{false, true} {
		for _, direction := range []int{-1, 1} {
			for _, age := range []time.Duration{50 * time.Millisecond, time.Second} {
				t.Run(fmt.Sprintf("tb=%v/dir=%d/age=%s", tb, direction, age), func(t *testing.T) {
					runOnDrawFrame(func(*ebiten.Image) {
						g.turnBasedMode = tb
						g.snapFacing(-float64(direction) * math.Pi / 2)
						if tb {
							gl.inputHandler.rotateTurnBased(direction)
						} else {
							g.snapFacing(0)
						}
						// Allow the ordinary short delay, but never restart it while pending.
						gl.loading.started = time.Now().Add(-age)
						gl.loading.finished = time.Time{}
					})
					started := gl.loading.started
					previous := gl.loading.front
					framesLeft := g.viewTurnFramesLeft
					for n := 0; n < 3; n++ {
						runOnDrawFrame(func(*ebiten.Image) {
							r.mapRenderUploadQueue = []mapRenderUpload{{task: &mapRenderPrewarmTask{mapKey: "church"}}}
							if !gl.loadingBarrier() {
								t.Error("Update missed pending destination region")
							}
							gl.tickLoadingPause()
							g.Draw(frame)
							if !gl.loading.awaitingFrame || !gl.loading.finished.IsZero() || gl.loading.front != previous {
								t.Error("Draw completed an old-facing frame while destination region was pending")
							}
							if !gl.loading.started.Equal(started) {
								t.Error("pending turn restarted the loading banner delay")
							}
							if age >= time.Second && gl.loading.bannerAlpha(time.Now()) <= 0 {
								t.Error("long turn load hid its loading banner")
							}
							if g.camera.Angle != 0 || g.viewTurnFramesLeft != framesLeft {
								t.Error("loading changed logical heading or advanced turn")
							}
						})
					}
					runOnDrawFrame(func(*ebiten.Image) { r.mapRenderUploadQueue = nil; g.Draw(frame) })
					if gl.loading.awaitingFrame {
						t.Fatal("completed upload did not resume rendering")
					}
					if tb {
						if len(g.turnBlurUniform) != 1 || g.turnBlurUniform[0] <= 0 {
							t.Error("resumed turn lost motion blur")
						}
						before := g.viewAngleRender
						g.advanceViewTurn()
						if g.viewAngleRender == before {
							t.Error("turn did not resume after loading")
						}
					}
				})
			}
		}
	}
}
