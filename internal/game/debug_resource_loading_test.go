//go:build debug

package game

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_LoadingBannerNamesVisibleSurface(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	for _, size := range [][2]int{{1280, 720}, {1600, 900}} {
		for _, tc := range []struct {
			name, want string
			open       bool
			tab        MenuTab
			overlay    string
		}{
			{name: "exploration", want: "Loading area..."},
			{name: "closed_spellbook", tab: TabSpellbook, want: "Loading area..."},
			{name: "inventory", open: true, tab: TabInventory, want: "Loading inventory..."},
			{name: "spellbook", open: true, tab: TabSpellbook, want: "Loading spellbook..."},
			{name: "characters", open: true, tab: TabCharacters, want: "Loading..."},
			{name: "quests", open: true, tab: TabQuests, want: "Loading..."},
			{name: "cards", open: true, tab: TabCards, want: "Loading..."},
			{name: "menu_above_inventory", open: true, tab: TabInventory, overlay: "menu", want: "Loading..."},
			{name: "dialog_above_spellbook", open: true, tab: TabSpellbook, overlay: "dialog", want: "Loading..."},
			{name: "map", overlay: "map", want: "Loading..."},
		} {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size[0], size[1]), func(t *testing.T) {
				gl := loadingFixture(t)
				gl.game.menuOpen, gl.game.currentTab = tc.open, tc.tab
				switch tc.overlay {
				case "menu":
					gl.game.mainMenuOpen = true
				case "dialog":
					gl.game.dialogActive = true
				case "map":
					gl.game.mapOverlayOpen = true
				}
				runOnDrawFrame(func(*ebiten.Image) {
					w, h := gl.Layout(size[0], size[1])
					actual, expected := ebiten.NewImage(w, h), ebiten.NewImage(w, h)
					defer actual.Deallocate()
					defer expected.Deallocate()
					gl.loading.pattern = make(chan struct{})
					gl.loading.begin(time.Now().Add(-time.Second))
					gl.Draw(actual)
					expected.Fill(color.RGBA{12, 15, 18, 255})
					gl.ui.drawScreenBannerContent(expected, tc.want, bannerQuestProgress, 1, 0)
					geo := screenBannerLayout(w, tc.want, 0)
					// Compare the actual rendered label and plate above the moving
					// indicator, whose phase deliberately follows live wall time.
					bounds := image.Rect(geo.plateX, geo.plateY, geo.plateX+geo.plateW, geo.plateY+geo.plateH-8)
					a, b := make([]byte, bounds.Dx()*bounds.Dy()*4), make([]byte, bounds.Dx()*bounds.Dy()*4)
					actual.SubImage(bounds).(*ebiten.Image).ReadPixels(a)
					expected.SubImage(bounds).(*ebiten.Image).ReadPixels(b)
					if !bytes.Equal(a, b) {
						t.Errorf("rendered loading message does not match %q", tc.want)
					}
				})
			})
		}
	}
}

func TestDebugSim_LoadingIndicatorMovesWithoutSimulationTicks(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	for _, tb := range []bool{false, true} {
		for _, size := range [][2]int{{1280, 720}, {1600, 900}} {
			for _, stage := range []string{"worker", "upload", "floor"} {
				t.Run(fmt.Sprintf("tb=%v/%dx%d/%s", tb, size[0], size[1], stage), func(t *testing.T) {
					gl := loadingFixture(t)
					gl.game.turnBasedMode = tb
					gl.game.frameCount = 77
					var previous []byte
					for _, age := range []time.Duration{400 * time.Millisecond, time.Second} {
						runOnDrawFrame(func(*ebiten.Image) {
							w, h := gl.Layout(size[0], size[1])
							frame := ebiten.NewImage(w, h)
							defer frame.Deallocate()
							gl.loading.started = time.Now().Add(-age)
							switch stage {
							case "worker":
								gl.loading.pattern = make(chan struct{})
							case "upload":
								img := ebiten.NewImage(1, 1)
								defer img.Deallocate()
								gl.loading.uploads = []*ebiten.Image{img}
							case "floor":
								gl.renderer.cancelFloorPreparation()
								_, cancel := context.WithCancel(context.Background())
								gl.renderer.floorPreparation = &floorPreparation{cancel: cancel, result: make(chan preparedFloor)}
							}
							gl.Draw(frame)
							geo := screenBannerLayout(w, "Loading area...", 0)
							strip := image.Rect(geo.plateX+12, geo.plateY+geo.plateH-7, geo.plateX+geo.plateW-12, geo.plateY+geo.plateH-4)
							pixels := make([]byte, strip.Dx()*strip.Dy()*4)
							frame.SubImage(strip).(*ebiten.Image).ReadPixels(pixels)
							if previous != nil && bytes.Equal(previous, pixels) {
								t.Error("loading indicator stayed static while Draw continued without Update")
							}
							previous = pixels
							if !gl.loading.awaitingFrame || gl.game.frameCount != 77 {
								t.Error("indicator animation advanced or resumed simulation")
							}
							if dir := os.Getenv("RAM_LOADING_QA_DIR"); dir != "" && !tb && stage == "worker" && age == time.Second {
								if err := os.MkdirAll(dir, 0755); err != nil {
									panic(err)
								}
								f, err := os.Create(filepath.Join(dir, fmt.Sprintf("indicator_%dx%d.png", w, h)))
								if err != nil {
									panic(err)
								}
								if err := png.Encode(f, frame); err != nil {
									panic(err)
								}
								if err := f.Close(); err != nil {
									panic(err)
								}
							}
						})
					}
				})
			}
		}
	}
}

func TestDebugSim_ResourceLoadingResumesAfterCompleteFrame(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	gl := g.gameLoop
	g.mainMenuOpen = true // Keep the combat fixture stable after readiness.
	w, h := gl.Layout(1280, 720)
	frame := ebiten.NewImage(w, h)
	defer frame.Deallocate()
	settle := func() {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		ready := false
		for !ready && time.Now().Before(deadline) {
			runOnDrawFrame(func(*ebiten.Image) {
				if err := gl.Update(); err != nil {
					panic(err)
				}
				gl.Draw(frame)
				ready = gl.loading != nil && !gl.loading.awaitingFrame && gl.loading.front != nil
			})
		}
		if !ready {
			t.Fatal("cold render never reached a complete frame")
		}
	}
	settle()
	runOnDrawFrame(func(*ebiten.Image) {
		g.mainMenuOpen = false
		g.sprites.EvictResource("party_member_panel", "")
		previous := gl.loading.front
		gl.Draw(frame)
		if !gl.loading.awaitingFrame || gl.loading.front != previous || gl.ui.displayedInput.ready {
			t.Error("cold UI draw replaced the last complete frame or left stale input actionable")
		}
		g.mainMenuOpen = true
	})
	settle()
	for _, mapKey := range []string{"clock_tower", "forest"} {
		runOnDrawFrame(func(*ebiten.Image) { g.switchToMap(mapKey) })
		settle()
	}
	runOnDrawFrame(func(*ebiten.Image) { g.mainMenuOpen = false; gl.Draw(frame) })
	settle()
	// A pending CPU job must retain a complete frame at both logical sizes.
	for _, size := range [][2]int{{1280, 720}, {1600, 900}} {
		runOnDrawFrame(func(*ebiten.Image) {
			w, h := gl.Layout(size[0], size[1])
			shot := ebiten.NewImage(w, h)
			defer shot.Deallocate()
			gl.loading.pattern = make(chan struct{})
			gl.loading.begin(time.Now().Add(-time.Second))
			gl.Draw(shot)
			if !gl.loading.awaitingFrame || gl.loading.front == nil {
				t.Error("resize lost loading barrier or cached frame")
			}
			if dir := os.Getenv("RAM_LOADING_QA_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					panic(err)
				}
				name := "loading_default.png"
				if size[0] == 1600 {
					name = "loading_resized.png"
				}
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					panic(err)
				}
				if err := png.Encode(f, shot); err != nil {
					panic(err)
				}
				if err := f.Close(); err != nil {
					panic(err)
				}
			}
			gl.loading.pattern = nil
		})
	}
	settle()
}
