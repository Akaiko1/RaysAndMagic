package keytracker

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestConsumerFirstCallerWins(t *testing.T) {
	pressed := map[ebiten.Key]bool{}
	c := Consumer{justPressed: func(k ebiten.Key) bool { return pressed[k] }}

	// Frame 1: Escape edge present - first caller consumes, second is refused,
	// an untouched key is unaffected.
	c.BeginFrame()
	pressed[ebiten.KeyEscape] = true
	pressed[ebiten.KeyEnter] = true
	if !c.Consume(ebiten.KeyEscape) {
		t.Fatal("first caller must get the edge")
	}
	if c.Consume(ebiten.KeyEscape) {
		t.Fatal("second caller must not get the same edge")
	}
	if !c.Consume(ebiten.KeyEnter) {
		t.Fatal("another key's edge must be independent")
	}

	// Frame 2: edge gone (key held past its press frame) - nobody fires, even a
	// caller that never ran during the press frame.
	c.BeginFrame()
	pressed[ebiten.KeyEscape] = false
	if c.Consume(ebiten.KeyEscape) {
		t.Fatal("a held key must not re-fire after its press frame")
	}

	// Frame 3: fresh press fires again.
	c.BeginFrame()
	pressed[ebiten.KeyEscape] = true
	if !c.Consume(ebiten.KeyEscape) {
		t.Fatal("a fresh press must fire")
	}
}
