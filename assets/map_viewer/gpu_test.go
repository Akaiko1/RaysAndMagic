package main

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Opt into the same live GPU test mode as the game package. Ordinary editor
// tests still run without a window; pixel ownership checks require this loop.
func TestMain(m *testing.M) {
	if os.Getenv("RAM_DEBUG_SIM") != "1" {
		os.Exit(m.Run())
	}
	g := &editorTestLoop{m: m, done: make(chan struct{})}
	ebiten.SetWindowSize(320, 240)
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}

type editorTestLoop struct {
	m       *testing.M
	done    chan struct{}
	started bool
	code    int
}

func (g *editorTestLoop) Update() error {
	if !g.started {
		g.started = true
		go func() { g.code = g.m.Run(); close(g.done) }()
	}
	select {
	case <-g.done:
		return ebiten.Termination
	default:
		return nil
	}
}
func (*editorTestLoop) Draw(*ebiten.Image)         {}
func (*editorTestLoop) Layout(int, int) (int, int) { return 320, 240 }
