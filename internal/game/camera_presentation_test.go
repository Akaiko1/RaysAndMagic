package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/world"
)

// Case matrix: RT walking/running at independent render rates, TB/pauses,
// loading, save/load and screen picking. Only presentation may interpolate.
func TestCameraPresentationFixedTimeline(t *testing.T) {
	for _, fps := range []int{90, 144, 240} {
		for _, speed := range []float64{2, 4} {
			t.Run(fmt.Sprintf("%dFPS/speed%g", fps, speed), func(t *testing.T) {
				g, _, _ := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = false
				g.appScreen = AppScreenInGame
				start := time.Unix(100, 0)
				step := time.Second / time.Duration(g.config.GetTPS())
				g.camera.X, g.camera.Y, g.camera.Angle = 0, 0, 0
				g.finishCameraTick(g.cameraPose(), g.cameraPresentation.epoch, start)
				tick := 0
				previous := 0.0
				for frame := 1; frame < fps; frame++ {
					now := start.Add(time.Duration(frame) * time.Second / time.Duration(fps))
					for time.Duration(tick+1)*step <= now.Sub(start) {
						before := g.cameraPose()
						g.camera.X += speed
						tick++
						g.finishCameraTick(before, g.cameraPresentation.epoch, now)
					}
					logical := g.cameraPose()
					restore := g.beginRenderCameraSwap(now)
					shown := g.cameraPose()
					restore()
					if g.cameraPose() != logical {
						t.Fatal("Draw changed the logical pose")
					}
					if frame > 3 && math.Abs((shown.x-previous)-speed*float64(time.Second)/float64(step)/float64(fps)) > 1e-5 {
						t.Fatal("catch-up Updates caused a visible step")
					}
					previous = shown.x
				}
			})
		}
	}
}

func TestCameraPresentationUpdateGates(t *testing.T) {
	for _, state := range []string{"RT", "TB", "menu", "loading"} {
		t.Run(state, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			g := h.g
			g.turnBasedMode, g.menuOpen = false, false
			g.world.Monsters = nil
			g.snapFacing(0)
			switch state {
			case "TB":
				g.turnBasedMode = true
			case "menu":
				g.menuOpen = true
			case "loading":
				h.loop.loading = &gameLoadingState{awaitingFrame: true}
			}
			if state == "loading" {
				// The loading fixture owns additional streaming state; exercise this gate
				// without inventing a second loader in the modal fixture.
				g.finishCameraTick(g.cameraPose(), g.cameraPresentation.epoch, time.Now())
			} else if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if g.cameraPresentation.valid != (state == "RT") {
				t.Fatal("Update published interpolation in the wrong state")
			}
		})
	}
}

func TestCameraPresentationSaveLoadAndDiscontinuities(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			cfg := loadTestConfig(t)
			w := newTestWorld(cfg)
			wm := world.NewWorldManager(cfg)
			wm.LoadedMaps = map[string]*world.World3D{"forest": w}
			wm.CurrentMapKey = "forest"
			setTestWorldManager(t, wm)
			g := newTestGame(cfg, w)
			g.appScreen = AppScreenInGame
			g.turnBasedMode = tb
			g.snapFacing(2.5)
			logical := g.cameraPose()
			save := g.buildSave(wm)
			g.cameraPresentation = cameraPresentation{valid: true, current: logical, previous: cameraPose{10, 10, 0}, tickStart: time.Now()}
			restore := g.beginRenderCameraSwap(g.cameraPresentation.tickStart)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			restore()
			if g.cameraPose() != logical || g.cameraPresentation.valid || g.cameraPresentation.presentedValid {
				t.Fatal("render restore overwrote the loaded pose or retained old history")
			}
			before, epoch := g.cameraPose(), g.cameraPresentation.epoch
			g.setPartyPosition(300, 400)
			g.snapFacing(1)
			g.finishCameraTick(before, epoch, time.Now())
			if g.cameraPresentation.valid {
				t.Fatal("teleport interpolated from the departure point")
			}
		})
	}
	g, _, _ := tbBehaviorGame(t, 40, 40)
	g.turnBasedMode = false
	g.appScreen = AppScreenInGame
	start := time.Unix(100, 0)
	before := cameraPose{10, 20, 2*math.Pi - 0.1}
	g.camera.X, g.camera.Y, g.camera.Angle = 14, 28, 0.1
	g.finishCameraTick(before, g.cameraPresentation.epoch, start)
	mid := g.renderCameraPose(start.Add(time.Second / time.Duration(2*g.config.GetTPS())))
	if math.Abs(mid.x-12) > 1e-5 || math.Abs(math.Remainder(mid.angle, 2*math.Pi)) > 1e-5 {
		t.Fatal("interpolation took the long rotation arc")
	}
	if got := g.renderCameraPose(start.Add(time.Second)); got.x != 14 || got.y != 28 {
		t.Fatal("stall extrapolated beyond collision-resolved position")
	}
}

func TestCameraPresentationPicking(t *testing.T) {
	for _, kind := range []string{"NPC", "loot"} {
		for _, inRange := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reachable=%v", kind, inRange), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 800, 600)
				g := h.g
				g.turnBasedMode, g.menuOpen = false, false
				g.world.Monsters = nil
				g.renderHelper = NewRenderingHelper(g)
				g.camera.FOV = squareProjectionFOV(800, 600)
				g.camera.ViewDist = 5000
				g.camera.X, g.camera.Y, g.camera.Angle = 320, 320, 0
				if !inRange {
					g.camera.X = 1000
				}
				logical := g.cameraPose()
				g.cameraPresentation.presented = cameraPose{320, 320, math.Pi / 2}
				g.cameraPresentation.presentedValid = true
				ex, ey := 320.0, 384.0
				if kind == "NPC" {
					n := &character.NPC{Name: "Pick target", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: ex, Y: ey}
					g.world.NPCs = []*character.NPC{n}
					restore := g.beginPresentedCameraSwap()
					sx, sy, size, visible := g.renderHelper.NPCSpriteMetrics(n, ex, ey, 64)
					restore()
					if !visible || size <= 0 {
						t.Fatal("fixture not visible")
					}
					got, reachable := g.findNPCAtScreen(sx, sy+size/2)
					if got != n || reachable != inRange {
						t.Fatal("NPC picking mixed displayed and logical poses")
					}
					g.updateFocusedNPC()
					if (g.focusedNPC == n) != inRange {
						t.Fatal("focus used the wrong pose")
					}
				} else {
					g.groundContainers = []GroundContainer{{X: ex, Y: ey, Sprite: "missing_pick_fixture"}}
					restore := g.beginPresentedCameraSwap()
					info := g.groundContainerRenderInfo(&g.groundContainers[0], -1)
					restore()
					if !info.Visible {
						t.Fatal("fixture not visible")
					}
					if (g.findGroundContainerIndexAtScreen(info.ScreenX, info.ScreenY+info.SpriteSize/2, 100) == 0) != inRange {
						t.Fatal("loot picking changed logical reach")
					}
				}
				if g.cameraPose() != logical {
					t.Fatal("screen picking changed gameplay pose")
				}
			})
		}
	}
}

// Ebitengine 2.10 rounds tick counts to nearest, so Update can arrive up to
// half a tick before its nominal time. Draw must still follow one timeline.
func TestCameraPresentationEarlyTicks(t *testing.T) {
	for _, fps := range []int{90, 120, 144, 240} {
		for _, speed := range []float64{2, 4} {
			t.Run(fmt.Sprintf("%dFPS/speed%g", fps, speed), func(t *testing.T) {
				g, _, _ := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = false
				g.appScreen = AppScreenInGame
				g.camera.X, g.camera.Y, g.camera.Angle = 0, 0, 0
				start := time.Unix(100, 0)
				step := time.Second / time.Duration(g.config.GetTPS())
				g.finishCameraTick(g.cameraPose(), g.cameraPresentation.epoch, start)
				tick := 0
				for frame := 1; frame < fps*2; frame++ {
					// Small frame-time variation crosses both sides of tick boundaries.
					elapsed := time.Duration(frame) * time.Second / time.Duration(fps)
					if frame%3 == 0 {
						elapsed += step / 8
					}
					now := start.Add(elapsed)
					due := int((elapsed + step/2) / step)
					for tick < due {
						before := g.cameraPose()
						g.camera.X += speed
						tick++
						g.finishCameraTick(before, g.cameraPresentation.epoch, now)
					}
					if frame < 4 {
						continue
					}
					want := speed * (float64(elapsed)/float64(step) - 1)
					got := g.renderCameraPose(now).x
					if math.Abs(got-want) > 1e-5 {
						t.Fatalf("early tick reset motion timeline at frame %d: got %.6f want %.6f", frame, got, want)
					}
				}
			})
		}
	}
}
