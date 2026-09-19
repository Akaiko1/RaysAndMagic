//go:build debug

package game

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// CPU submission/loading diagnostic with production config, world, collision,
// Update and Draw. The outer GPU harness is not a display-pacing benchmark.
func TestRiverMotion(t *testing.T) {
	requireStandeeGPU(t)
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	width := 1920
	if raw := os.Getenv("RAM_RIVER_WIDTH"); raw != "" {
		var err error
		width, err = strconv.Atoi(raw)
		if err != nil || width < 800 {
			t.Fatal("invalid width")
		}
	}
	w, h := g.gameLoop.Layout(width, width*9/16)
	t.Logf("production viewport=%dx%d", w, h)
	x, y := TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	angle := 0.0
	if g.config.OpenWorldEnabled() {
		x, y = world.GlobalWorldManager.ProjectWorldPos("forest", x, y)
		angle = world.GlobalWorldManager.ProjectAngle("forest", angle)
	}
	g.setPartyPosition(x, y)
	g.snapFacing(angle)
	if source := os.Getenv("RAM_RIVER_SAVE"); source != "" {
		bytes, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		saved := filepath.Join(root, "river.json")
		if err := os.WriteFile(saved, bytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err := g.LoadGameFromFile(saved); err != nil {
			t.Fatal(err)
		}
	}
	g.showFPS = os.Getenv("RAM_RIVER_OVERLAY") != ""
	t.Logf("pose=%+v overlay=%v", g.cameraPose(), g.showFPS)
	captureGameplayPreviewFrame(t, g, w, h)
	target := ebiten.NewImage(w, h)
	defer target.Deallocate()
	probe := ebiten.NewImage(1, 1)
	defer probe.Deallocate()
	if path := os.Getenv("RAM_RIVER_CPU"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}
	for _, phase := range []string{"stationary", "running", "turning"} {
		moving := phase == "running"
		var updates, draws, intervals, fences []float64
		frames := 600
		if os.Getenv("RAM_RIVER_FENCE") != "" {
			frames = 180
		}
		var previous time.Time
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		loadingFrames, moved := 0, 0
		for i := 0; i < frames; i++ {
			runOnDrawFrame(func(*ebiten.Image) {
				started := time.Now()
				before, epoch := g.cameraPose(), g.cameraPresentation.epoch
				presentation := g.cameraPresentation
				if phase == "turning" && !g.gameLoop.loading.awaitingFrame {
					g.camera.Angle += g.config.GetRotSpeed() * g.gameLoop.inputHandler.movementScale()
				}
				if moving && !g.gameLoop.loading.awaitingFrame {
					speed := g.config.GetMoveSpeed() * g.gameLoop.inputHandler.movementScale() * g.config.Movement.RunMultiplier
					if (i/64)%2 != 0 {
						speed = -speed
					}
					g.gameLoop.inputHandler.movePlayer(g.camera.GetForwardX()*speed, g.camera.GetForwardY()*speed)
					if g.cameraPose() != before {
						moved++
					}
				}
				if err := g.Update(); err != nil {
					t.Error(err)
				}
				if g.cameraPresentation.epoch == epoch {
					g.cameraPresentation = presentation
				}
				g.finishCameraTick(before, epoch, started)
				drawStart := time.Now()
				g.Draw(target)
				drawn := time.Now()
				if os.Getenv("RAM_RIVER_FENCE") != "" {
					var pixel [4]byte
					view := target.RecyclableSubImage(image.Rect(0, 0, 1, 1))
					probe.DrawImage(view, nil)
					probe.ReadPixels(pixel[:])
					view.Recycle()
					fences = append(fences, float64(time.Since(drawn))/float64(time.Millisecond))
				}
				if i > 0 {
					updates = append(updates, float64(drawStart.Sub(started))/float64(time.Millisecond))
					draws = append(draws, float64(drawn.Sub(drawStart))/float64(time.Millisecond))
					intervals = append(intervals, float64(started.Sub(previous))/float64(time.Millisecond))
				}
				previous = started
				if g.gameLoop.loading.awaitingFrame {
					loadingFrames++
				}
			})
		}
		runtime.ReadMemStats(&b)
		for name, values := range map[string][]float64{"Update": updates, "Draw": draws, "interval": intervals, "GPU fence": fences} {
			if len(values) == 0 {
				continue
			}
			p50, p95, p99, peak := renderTimingPercentiles(values)
			t.Logf("phase=%s %s p50/95/99/max=%.2f/%.2f/%.2f/%.2fms", phase, name, p50, p95, p99, peak)
		}
		t.Logf("phase=%s moved=%d loading=%d allocations=%dMiB GC=%d readbacks=%d", phase, moved, loadingFrames, (b.TotalAlloc-a.TotalAlloc)>>20, b.NumGC-a.NumGC, g.gameLoop.renderer.loadDiagnostics.readbacks)
	}
}
