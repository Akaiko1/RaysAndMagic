package game

import (
	"os"
	"testing"

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

// runOnDrawFrame executes fn inside a live Draw frame and blocks until done.
// Only call from debug sims (RAM_DEBUG_SIM=1) - without the game loop running
// there is nothing to drain the channel.
func runOnDrawFrame(fn func(screen *ebiten.Image)) {
	done := make(chan struct{})
	debugSimJobs <- func(s *ebiten.Image) {
		fn(s)
		close(done)
	}
	<-done
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
	}
}

func (*testMainGame) Layout(int, int) (int, int) { return 320, 240 }

func TestMain(m *testing.M) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		os.Exit(m.Run())
	}
	g := &testMainGame{m: m, code: 1, done: make(chan struct{})}
	ebiten.SetWindowSize(320, 240)
	ebiten.SetWindowTitle("RaysAndMagic debug sims")
	ebiten.SetVsyncEnabled(false) // measurement frames, not display frames
	ebiten.SetTPS(ebiten.SyncWithFPS)
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}
