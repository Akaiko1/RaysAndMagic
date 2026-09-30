package game

import (
	"fmt"
	"os"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Debug-sim render harness. The normal suite runs exactly as before (plain
// m.Run, no window). Under RAM_DEBUG_SIM=1 the tests run on a goroutine
// alongside a REAL ebiten game loop, and render-driving sims submit work into
// live Draw frames via runOnDrawFrame - each measured frame gets a real frame
// boundary, so ebiten's per-frame internals are reclaimed (running thousands
// of renders inside one Update tick grew the heap without bound).

// debugSimJobs carries one closure per Draw frame from a sim to the loop.
var debugSimJobs = make(chan func(*ebiten.Image))

// Optional production game driven by the engine's real Update/Draw cadence.
// Install and remove only in runOnDrawFrame jobs; ordinary GPU tests keep
// their existing one-job-per-frame behavior.
var debugLiveGame ebiten.Game

// runOnDrawFrame executes fn inside a live Draw frame and blocks until done.
// Only call from debug sims (RAM_DEBUG_SIM=1) - without the game loop running
// there is nothing to drain the channel.
// Used by the -tags debug simulations (staticcheck reports it unused without
// that tag).
func runOnDrawFrame(fn func(screen *ebiten.Image)) {
	done := make(chan struct{})
	debugSimJobs <- func(s *ebiten.Image) {
		fn(s)
		close(done)
	}
	<-done
}

// drawLaidOutFrame draws the exploration frame as Draw presents it after
// Layout: the world at its own resolution, the interface at the screen's.
// Like drawLaidOutUI, used by the -tags debug galleries.
func drawLaidOutFrame(g *MMGame, screen *ebiten.Image) {
	beginUIFrame(screen, g.uiPixelScale())
	g.gameLoop.drawExplorationFrame(screen)
}

// drawLaidOutUI draws only the interface onto a screen Layout sized.
func drawLaidOutUI(g *MMGame, screen *ebiten.Image) {
	beginUIFrame(screen, g.uiPixelScale())
	g.gameLoop.ui.Draw(screen)
}

type testMainGame struct {
	m       *testing.M
	code    int
	started bool
	done    chan struct{}
}

func (g *testMainGame) Update() error {
	if !g.started {
		g.started = true
		go func() {
			g.code = g.m.Run()
			close(g.done)
		}()
	}
	select {
	case <-g.done:
		return ebiten.Termination
	default:
		if debugLiveGame != nil {
			return debugLiveGame.Update()
		}
		return nil
	}
}

// Draw runs at most ONE queued sim job per frame - the frame boundary between
// jobs is the whole point of this harness.
func (g *testMainGame) Draw(screen *ebiten.Image) {
	select {
	case job := <-debugSimJobs:
		job(screen)
	default:
		if debugLiveGame != nil {
			debugLiveGame.Draw(screen)
		}
	}
}

func (*testMainGame) Layout(w, h int) (int, int) {
	if debugLiveGame != nil {
		return debugLiveGame.Layout(w, h)
	}
	return 320, 240
}

// monitorDeviceScale is the real monitor query; tests run with the seam pinned
// to one device pixel per window pixel so posed windows lay out the same on
// every machine. Tests that pose a HiDPI display set the seam themselves; the
// native-window debug route restores this one.
var monitorDeviceScale = displayDeviceScale

func TestMain(m *testing.M) {
	displayDeviceScale = func() float64 { return 1 }
	// RAM_TEST_UI_FONT=<key> runs the whole suite in one of the shipped
	// fonts, so every text-fit check is exercised with its glyph widths.
	if key := os.Getenv("RAM_TEST_UI_FONT"); key != "" {
		cfg, err := config.LoadConfig("../../config.yaml")
		if err == nil {
			err = loadUIFonts(cfg.Display.Fonts)
		}
		if err != nil || uiFontAtlases[key] == nil {
			panic(fmt.Sprintf("RAM_TEST_UI_FONT=%s: %v", key, err))
		}
		setActiveUIFont(uiFontAtlases[key])
	}
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		os.Exit(m.Run())
	}
	g := &testMainGame{m: m, code: 1, done: make(chan struct{})}
	ebiten.SetWindowSize(320, 240)
	ebiten.SetWindowTitle("RaysAndMagic debug sims")
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(false) // measurement frames, not display frames
	ebiten.SetTPS(ebiten.SyncWithFPS)
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}
