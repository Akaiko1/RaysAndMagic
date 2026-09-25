//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestDebugSim_RegionLoadingFeedback(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	gl := g.gameLoop
	wm := world.GlobalWorldManager
	owc, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wm.SetOpenWorldConfig(owc)
	// The gallery boots split maps. Rebuild with the production stitching
	// configuration and discard the old split entries before resolving worlds.
	wm.LoadedMaps = make(map[string]*world.World3D)
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatal(err)
	}
	if err := g.switchToMap("forest"); err != nil {
		t.Fatal(err)
	}
	if !g.openWorldActive() {
		t.Fatal("test must use the real stitched world")
	}
	g.mainMenuOpen = false
	w, h := gl.Layout(1280, 720)
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	ts := g.config.GetTileSize()
	for _, tb := range []bool{false, true} {
		g.turnBasedMode = tb
		for _, route := range []struct {
			name, from, to       string
			start, end, y, angle float64
		}{
			{"forest_desert", "forest", "desert", 94.5, 106.5, 50.5, 0},
			{"highlands_forest", "highlands", "forest", 44.5, 55.5, 72.5, 0},
		} {
			t.Run(fmt.Sprintf("%s/tb=%v", route.name, tb), func(t *testing.T) {
				x := route.start
				runOnDrawFrame(func(*ebiten.Image) {
					gl.renderer.resetMapRenderResourceResidency()
					gl.closeResourceLoading()
					g.setPartyPosition(x*ts, route.y*ts)
					g.camera.Angle = route.angle
					g.syncOpenWorldRegion()
				})
				if currentMapKey() != route.from {
					t.Fatalf("start region = %s, want %s", currentMapKey(), route.from)
				}
				checkedPlate := false
				deadline := time.Now().Add(90 * time.Second)
				wasPaused := false
				var pauseStart time.Time
				for time.Now().Before(deadline) {
					complete := false
					runOnDrawFrame(func(*ebiten.Image) {
						before := time.Now()
						if gl.loading != nil && !gl.loading.awaitingFrame {
							x = math.Min(route.end, x+0.125)
							g.setPartyPosition(x*ts, route.y*ts)
						}
						if err := gl.Update(); err != nil {
							t.Error(err)
						}
						updated := time.Now()
						gl.Draw(frame)
						drawn := time.Now()
						l := gl.loading
						paused := l != nil && l.awaitingFrame
						if paused != wasPaused {
							if paused {
								pauseStart = before
							}
							t.Logf("x=%.3f paused=%v span=%v alpha=%.2f", x, paused, drawn.Sub(pauseStart), l.bannerAlpha(drawn))
							wasPaused = paused
						}
						if updated.Sub(before) > 100*time.Millisecond || drawn.Sub(updated) > 100*time.Millisecond {
							t.Logf("slow x=%.3f update=%v draw=%v paused=%v alpha=%.2f", x, updated.Sub(before), drawn.Sub(updated), paused, l.bannerAlpha(drawn))
						}
						if paused && (!l.area || l.bannerAlpha(drawn) != 1) {
							t.Error("first and subsequent area frames must show the banner immediately")
						}
						if paused && !checkedPlate {
							expected := ebiten.NewImage(w, h)
							defer expected.Deallocate()
							expected.Fill(color.RGBA{12, 15, 18, 255})
							gl.ui.drawScreenBannerContent(expected, "Loading area...", bannerQuestProgress, 1, 0)
							geo := screenBannerLayout(w, "Loading area...", 0)
							bounds := image.Rect(geo.plateX, geo.plateY, geo.plateX+geo.plateW, geo.plateY+geo.plateH-8)
							a, b := make([]byte, bounds.Dx()*bounds.Dy()*4), make([]byte, bounds.Dx()*bounds.Dy()*4)
							frame.SubImage(bounds).(*ebiten.Image).ReadPixels(a)
							expected.SubImage(bounds).(*ebiten.Image).ReadPixels(b)
							if !bytes.Equal(a, b) {
								t.Error("first loading frame did not actually render the area label and plate")
							}
							checkedPlate = true
						}
						complete = x >= route.end && !paused
					})
					if complete {
						if !checkedPlate {
							t.Error("cold route never exercised area feedback")
						}
						if currentMapKey() != route.to {
							t.Fatalf("end region = %s, want %s", currentMapKey(), route.to)
						}
						return
					}
				}
				t.Fatal("route did not finish loading")
			})
		}
	}
}
