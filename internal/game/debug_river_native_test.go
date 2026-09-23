//go:build debug

package game

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/sound"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

type riverNativeFrame struct {
	gap, draw, update float64
	x, y, angle       float64
	loading           bool
	fps               float64
	vertices          int
}
type riverNativeDriver struct {
	g               *MMGame
	done            chan struct{}
	start, previous time.Time
	frames          []riverNativeFrame
	update          time.Duration
	minX, maxX      float64
	direction       float64
	ready           bool
	viewportLogged  bool
	startCapture    func()
	loadSave        func() error
	menuStarted     time.Time
	turnOnly        bool
	memory          runtime.MemStats
}

func (d *riverNativeDriver) Layout(w, h int) (int, int) { return d.g.Layout(w, h) }
func (d *riverNativeDriver) Update() error {
	defer trace.StartRegion(context.Background(), "river.Update").End()
	if d.loadSave != nil && !d.menuStarted.IsZero() && time.Since(d.menuStarted) >= 2*time.Second {
		if err := d.loadSave(); err != nil {
			return err
		}
		d.loadSave = nil
		d.g.appScreen = AppScreenInGame
	}
	started := time.Now()
	before, epoch := d.g.cameraPose(), d.g.cameraPresentation.epoch
	presentation := d.g.cameraPresentation
	if d.ready && !d.g.gameLoop.loading.awaitingFrame {
		if d.g.camera.X <= d.minX {
			d.direction = 1
		}
		if d.g.camera.X >= d.maxX {
			d.direction = -1
		}
		angle := 0.0
		if d.direction < 0 {
			angle = math.Pi
		}
		if d.turnOnly {
			d.g.camera.Angle += d.g.config.Movement.RotationSpeed * d.g.gameLoop.inputHandler.movementScale()
		} else {
			d.g.camera.Angle = angle
			speed := d.g.config.GetMoveSpeed() * d.g.gameLoop.inputHandler.movementScale() * d.g.config.Movement.RunMultiplier
			d.g.gameLoop.inputHandler.movePlayer(d.direction*speed, 0)
		}
	}
	err := d.g.Update()
	if d.g.cameraPresentation.epoch == epoch {
		d.g.cameraPresentation = presentation
	}
	d.g.finishCameraTick(before, epoch, started)
	d.update += time.Since(started)
	return err
}
func (d *riverNativeDriver) Draw(screen *ebiten.Image) {
	now := time.Now()
	if d.menuStarted.IsZero() {
		d.menuStarted = now
	}
	if !d.viewportLogged {
		fmt.Printf("native route: fullscreen=%v logical=%v display=%v scale=%g vsync=%v TPS=%d\n", ebiten.IsFullscreen(), screen.Bounds(), ebiten.Monitor().Name(), ebiten.Monitor().DeviceScaleFactor(), ebiten.IsVsyncEnabled(), ebiten.TPS())
		d.viewportLogged = true
	}
	region := trace.StartRegion(context.Background(), "river.Draw")
	d.g.Draw(screen)
	region.End()
	loading := d.g.gameLoop.loading == nil || d.g.gameLoop.loading.awaitingFrame
	if !d.ready && !loading && d.loadSave == nil {
		if d.startCapture != nil {
			d.startCapture()
		}
		d.ready = true
		d.start = now
		runtime.ReadMemStats(&d.memory)
		fmt.Printf("native route: ready at %v turn=%v\n", d.g.cameraPose(), d.turnOnly)
	}
	if d.ready && !d.previous.IsZero() {
		r := d.g.gameLoop.renderer
		d.frames = append(d.frames, riverNativeFrame{float64(now.Sub(d.previous)) / 1e6, float64(time.Since(now)) / 1e6, float64(d.update) / 1e6, d.g.camera.X, d.g.camera.Y, d.g.camera.Angle, loading, ebiten.ActualFPS(), r.statStandeeVertices})
	}
	d.previous = now
	d.update = 0
	if d.ready && now.Sub(d.start) > 25*time.Second {
		debugLiveGame = nil
		close(d.done)
	}
}

// Opt-in actual engine clock, production save/config/world/Update/Draw. Unlike
// submission diagnostics this presents the world to the native window, with
// the shipped 120 TPS and vsync setting and no forced GPU readbacks.
func TestRiverNativeRoute(t *testing.T) {
	if os.Getenv("RAM_NATIVE_RIVER") == "" {
		t.Skip("native route is opt-in")
	}
	requireStandeeGPU(t)
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	audioManager, err := sound.LoadGlobal("assets/audio.yaml", filepath.Join(root, "audio_settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer audioManager.Close()
	g.soundManager = audioManager
	source := os.Getenv("RAM_RIVER_SAVE")
	startAtRiver := os.Getenv("RAM_RIVER_START") != ""
	if source == "" && !startAtRiver {
		t.Fatal("RAM_RIVER_SAVE or RAM_RIVER_START required")
	}
	saved := filepath.Join(root, "river.json")
	if source != "" {
		bytes, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(saved, bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	g.showFPS = os.Getenv("RAM_RIVER_OVERLAY") != ""
	tile := float64(g.config.GetTileSize())
	minX, _ := world.GlobalWorldManager.ProjectWorldPos("forest", 4.5*tile, 36.5*tile)
	maxX, _ := world.GlobalWorldManager.ProjectWorldPos("forest", 43.5*tile, 36.5*tile)
	d := &riverNativeDriver{g: g, done: make(chan struct{}), direction: -1, minX: minX, maxX: maxX}
	d.turnOnly = os.Getenv("RAM_RIVER_TURN") != ""
	d.loadSave = func() error {
		if source != "" {
			if err := g.LoadGameFromFile(saved); err != nil {
				return err
			}
		}
		if startAtRiver {
			x, y := world.GlobalWorldManager.ProjectWorldPos("forest", 30.5*tile, 36.5*tile)
			g.setPartyPosition(x, y)
			g.snapFacing(world.GlobalWorldManager.ProjectAngle("forest", math.Pi))
		}
		if os.Getenv("RAM_RIVER_LAND") != "" {
			x, y := world.GlobalWorldManager.ProjectWorldPos("forest", 30.5*tile, 44.5*tile)
			g.setPartyPosition(x, y)
			d.minX, _ = world.GlobalWorldManager.ProjectWorldPos("forest", 26.5*tile, 44.5*tile)
			d.maxX, _ = world.GlobalWorldManager.ProjectWorldPos("forest", 41.5*tile, 44.5*tile)
		}
		return nil
	}
	g.appScreen = AppScreenMainMenu
	if path := os.Getenv("RAM_RIVER_CPU"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		d.startCapture = func() {
			if err := pprof.StartCPUProfile(f); err != nil {
				t.Error(err)
			}
		}
		defer pprof.StopCPUProfile()
	}
	if path := os.Getenv("RAM_RIVER_TRACE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		prev := d.startCapture
		d.startCapture = func() {
			if prev != nil {
				prev()
			}
			if err := trace.Start(f); err != nil {
				t.Error(err)
			}
		}
		defer trace.Stop()
	}
	runOnDrawFrame(func(*ebiten.Image) {
		ebiten.SetWindowSize(g.config.GetScreenWidth(), g.config.GetScreenHeight())
		ebiten.SetFullscreen(g.config.Display.Fullscreen && os.Getenv("RAM_RIVER_WINDOWED") == "")
		ebiten.SetTPS(g.config.GetTPS())
		ebiten.SetVsyncEnabled(!g.config.Display.DisableVsyncOnMac)
		if os.Getenv("RAM_RIVER_VSYNC") != "" {
			ebiten.SetVsyncEnabled(true)
		}
		debugLiveGame = d
	})
	defer runOnDrawFrame(func(*ebiten.Image) {
		debugLiveGame = nil
		ebiten.SetTPS(ebiten.SyncWithFPS)
		ebiten.SetVsyncEnabled(false)
		ebiten.SetFullscreen(false)
		ebiten.SetWindowSize(320, 240)
	})
	select {
	case <-d.done:
	case <-time.After(150 * time.Second):
		t.Fatal("native route did not finish")
	}
	var gaps, draws, updates []float64
	loading := 0
	for _, f := range d.frames {
		gaps = append(gaps, f.gap)
		draws = append(draws, f.draw)
		updates = append(updates, f.update)
		if f.loading {
			loading++
		}
		if f.gap > 40 || f.draw > 40 || f.update > 40 {
			t.Logf("stall gap/draw/update=%.1f/%.1f/%.1fms pos=%.1f,%.1f angle=%.2f loading=%v reportedFPS=%.1f vertices=%d", f.gap, f.draw, f.update, f.x, f.y, f.angle, f.loading, f.fps, f.vertices)
		}
	}
	for name, values := range map[string][]float64{"interval": gaps, "Draw": draws, "Update": updates} {
		p50, p95, p99, peak := renderTimingPercentiles(values)
		t.Logf("%s p50/95/99/max=%.2f/%.2f/%.2f/%.2fms", name, p50, p95, p99, peak)
	}
	t.Logf("frames=%d loading=%d range=%.1f..%.1f final=%s", len(d.frames), loading, d.minX, d.maxX, fmt.Sprint(g.cameraPose()))
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	t.Logf("memory: allocated=%dMiB live=%dMiB GC=%d pause=%.2fms", (memory.TotalAlloc-d.memory.TotalAlloc)/(1<<20), memory.HeapAlloc/(1<<20), memory.NumGC-d.memory.NumGC, float64(memory.PauseTotalNs-d.memory.PauseTotalNs)/1e6)
}
