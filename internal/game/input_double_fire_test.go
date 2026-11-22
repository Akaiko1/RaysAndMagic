package game

import "testing"

// One Space tap picks up exactly one ground container: a fresh press fires, a
// sustained hold only auto-repeats after rtHoldRepeatSeconds.
func TestSpacePickupGate_TapFiresOnce(t *testing.T) {
	var g *MMGame // the loaded config's rate
	repeat := g.framesForSeconds(rtHoldRepeatSeconds)
	if g.spacePickupWanted(true, 1) != true {
		t.Fatal("fresh tap must allow a pickup")
	}
	// Held past the input cooldown but within a normal tap: must not re-fire.
	for _, frames := range []int{5, 12, 30, repeat - 1} {
		if g.spacePickupWanted(false, frames) {
			t.Fatalf("held %d frames (a tap) must not re-fire a pickup", frames)
		}
	}
	if g.spacePickupWanted(false, repeat) != true {
		t.Fatal("a deliberate hold must start vacuuming a pile")
	}
}

func TestDoubleClickWindowRequiresAQuickSecondClick(t *testing.T) {
	const now int64 = 10_000
	if !withinDoubleClickWindow(now, now-doubleClickWindowMs) {
		t.Fatal("click at the window boundary must count as a double-click")
	}
	if withinDoubleClickWindow(now, now-doubleClickWindowMs-1) {
		t.Fatal("a click after the short double-click window must be a new selection")
	}
	if withinDoubleClickWindow(now, 0) {
		t.Fatal("the initial click tracker must never count as a double-click")
	}
}
