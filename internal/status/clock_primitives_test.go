package status

import "testing"

// One accumulator: whole units out, the remainder kept, float noise short of
// a unit still counting; the nearest variant pays a half in advance.
func TestAccrue(t *testing.T) {
	for _, tc := range []struct {
		carry, amount, unit float64
		want                int
		left                float64
	}{
		{0.7, 0.5, 1, 1, 0.2},
		{30, 50, 64, 1, 16},
		{0, 130, 64, 2, 2},
		{0.3, 0.6, 1, 0, 0.9},
		{0.1 + 0.2, 0.7, 1, 1, 0},
	} {
		carry := tc.carry
		if got := Accrue(&carry, tc.amount, tc.unit); got != tc.want || carry < tc.left-1e-9 || carry > tc.left+1e-9 {
			t.Errorf("Accrue(%v, %v, %v) = %d left %v, want %d left %v", tc.carry, tc.amount, tc.unit, got, carry, tc.want, tc.left)
		}
	}
	carry, total := 0.0, 0
	var seq []int
	for range 5 {
		n := AccrueNearest(&carry, 0.6)
		seq = append(seq, n)
		total += n
	}
	if want := []int{1, 0, 1, 0, 1}; total != 3 || seq[0] != want[0] || seq[1] != want[1] || seq[2] != want[2] || seq[3] != want[3] || seq[4] != want[4] {
		t.Fatalf("AccrueNearest 0.6 x5 = %v, want %v", seq, want)
	}
}

// Every armed span spends a round's frames per TB turn, whole rounds or not;
// a save that kept only frames restores the same clock.
func TestArmAtTheRoundRate(t *testing.T) {
	const tps = 60
	var c DualClock
	c.Arm(10, tps)
	if c != (DualClock{Frames: 600, Turns: 4, Rate: 180}) {
		t.Fatalf("Arm(10 s) = %+v", c)
	}
	c = DualClock{}
	c.ArmFrames(75, tps)
	if c != (DualClock{Frames: 75, Turns: 1, Rate: 180}) {
		t.Fatalf("ArmFrames(75) = %+v", c)
	}
	c = FromFrames(721, tps)
	c.TickFrame()
	if c != (DualClock{Frames: 720, Turns: 4, Rate: 180}) || c != FromFrames(720, tps) {
		t.Fatalf("a frames-only restore drifted: %+v", c)
	}
	c.TickTurn()
	if c.Frames != 540 || c.Turns != 3 {
		t.Fatalf("a turn spent %+v, want a round of frames", c)
	}
}

// An authored pair keeps its own frames per round; part of it is the same
// part of both counts.
func TestPairClock(t *testing.T) {
	if c := PairClock(1.25, 1, 60, 1); c != (DualClock{Frames: 75, Turns: 1, Rate: 75}) {
		t.Fatalf("PairClock(1.25 s, 1) = %+v", c)
	}
	if c := PairClock(4.5, 2, 60, 0.5); c != (DualClock{Frames: 135, Turns: 1, Rate: 135}) {
		t.Fatalf("half of 4.5 s / 2 rounds = %+v", c)
	}
	if c := PairClock(5, 3, 120, 2.0/3); c.Frames != 400 || c.Turns != 2 {
		t.Fatalf("two thirds of 5 s / 3 rounds = %+v, want 400 frames 2 turns", c)
	}
	if c := PairClock(0, 0, 60, 1); c.Active() {
		t.Fatalf("an empty pair runs: %+v", c)
	}
}

// A repeating period lapses once per span and re-arms on the next tick.
func TestEveryClock(t *testing.T) {
	period := func() DualClock { return PairClock(5, 3, 60, 1) }
	var c DualClock
	lapses := 0
	for range 3 * period().Frames {
		if c.EveryClock(period, false) {
			lapses++
		}
	}
	if lapses != 3 {
		t.Fatalf("RT lapses = %d, want 3", lapses)
	}
	c, lapses = DualClock{}, 0
	for range 6 {
		if c.EveryClock(period, true) {
			lapses++
		}
	}
	if lapses != 2 {
		t.Fatalf("TB lapses over 6 turns = %d, want 2", lapses)
	}
}

// No time arms nothing: a zero span cannot leave a clock TB never spends.
func TestArmZeroArmsNothing(t *testing.T) {
	var c DualClock
	c.Arm(0, 60)
	if c.Active() {
		t.Fatalf("Arm(0) = %+v", c)
	}
	if c.Every(0, 60, true) || c.Active() {
		t.Fatalf("Every(0) ran a period: %+v", c)
	}
}
