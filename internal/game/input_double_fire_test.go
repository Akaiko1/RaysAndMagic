package game

import (
	"testing"
	"time"
)

// One Space tap picks up exactly one ground container: a fresh press fires, a
// sustained hold only auto-repeats after rtHoldRepeatDelay.
func TestSpacePickupGate_TapFiresOnce(t *testing.T) {
	if spacePickupWanted(true, 1) != true {
		t.Fatal("fresh tap must allow a pickup")
	}
	// Held past the input cooldown but within a normal tap: must not re-fire.
	for _, frames := range []int{5, 12, 30, rtHoldRepeatDelay - 1} {
		if spacePickupWanted(false, frames) {
			t.Fatalf("held %d frames (a tap) must not re-fire a pickup", frames)
		}
	}
	if spacePickupWanted(false, rtHoldRepeatDelay) != true {
		t.Fatal("a deliberate hold must start vacuuming a pile")
	}
}

// A buffered click never outlives the UI layer it was aimed at: the queues
// flush on every modal<->world flip, and survive within one layer.
func TestClickQueueFlushedOnModalTransition(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	ui := &UISystem{game: g}

	// Baseline: world layer, queue empty.
	g.prevWorldClickAllowed = true

	// Dialog opens; a click arrives while it is open and sits in the buffer.
	g.dialogActive = true
	ui.updateMouseState() // world->modal flip
	g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: 100, y: 100, at: time.Now().UnixMilli()})

	// Dialog closes: the buffered click belongs to the closed layer.
	g.dialogActive = false
	ui.updateMouseState()
	if n := len(g.mouseLeftClicks); n != 0 {
		t.Fatalf("stale click survived the modal->world transition (%d left in queue)", n)
	}

	// Within one layer nothing flips, so buffered clicks (dialog double-click
	// convention) survive frame boundaries.
	g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: 50, y: 50, at: time.Now().UnixMilli()})
	ui.updateMouseState()
	if n := len(g.mouseLeftClicks); n != 1 {
		t.Fatalf("click within one UI layer must survive (%d left in queue)", n)
	}
}
