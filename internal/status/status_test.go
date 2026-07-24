package status

import "testing"

func TestRefreshNeverShortens(t *testing.T) {
	r := 100
	if !Refresh(&r, 50) || r != 100 {
		t.Fatalf("weak refresh must not shorten: r=%d active", r)
	}
	if !Refresh(&r, 300) || r != 300 {
		t.Fatalf("strong refresh must extend: r=%d", r)
	}
	zero := 0
	if Refresh(&zero, 0) {
		t.Fatal("zero apply on zero clock must stay inactive")
	}
}

func TestRefreshDual(t *testing.T) {
	f, tn := 10, 0
	if !RefreshDual(&f, &tn, 5, 2) || f != 10 || tn != 2 {
		t.Fatalf("dual refresh: f=%d t=%d", f, tn)
	}
	f, tn = 0, 0
	if RefreshDual(&f, &tn, 0, 0) {
		t.Fatal("empty dual apply must stay inactive")
	}
}

func TestRefreshDualRatedPreservesActiveRateOnWeakRefresh(t *testing.T) {
	frames, turns, rate := 361, 4, 120
	if !RefreshDualRated(&frames, &turns, &rate, 120, 1) {
		t.Fatal("weak refresh deactivated the status")
	}
	if frames != 361 || turns != 4 || rate != 120 {
		t.Fatalf("weak refresh changed active contract: frames=%d turns=%d rate=%d", frames, turns, rate)
	}

	if TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("status expired with three turns remaining")
	}
	if frames != 360 || turns != 3 || rate != 120 {
		t.Fatalf("post-refresh TB tick: frames=%d turns=%d rate=%d, want 360/3/120", frames, turns, rate)
	}
}

func TestRefreshDualRatedRecalculatesRateWhenExtended(t *testing.T) {
	frames, turns, rate := 100, 1, 100
	if !RefreshDualRated(&frames, &turns, &rate, 480, 4) {
		t.Fatal("strong refresh deactivated the status")
	}
	if frames != 480 || turns != 4 || rate != 120 {
		t.Fatalf("strong refresh contract: frames=%d turns=%d rate=%d, want 480/4/120", frames, turns, rate)
	}
}

func TestTickFrameCrossClears(t *testing.T) {
	f, tn := 2, 3
	if TickFrame(&f, &tn) {
		t.Fatal("first tick must not expire")
	}
	if !TickFrame(&f, &tn) {
		t.Fatal("second tick must expire")
	}
	if f != 0 || tn != 0 {
		t.Fatalf("expiry must clear BOTH clocks: f=%d t=%d", f, tn)
	}
	if TickFrame(&f, &tn) {
		t.Fatal("ticking an inactive status must not expire again")
	}
}

func TestTickTurnCrossClears(t *testing.T) {
	f, tn := 300, 1
	if !TickTurn(&tn, &f) || tn != 0 || f != 0 {
		t.Fatalf("turn expiry must clear both clocks: f=%d t=%d", f, tn)
	}
}

func TestTickDoTFrameCadence(t *testing.T) {
	const tps = 4
	remaining, timer := 3*tps, 0
	ticks, expiries := 0, 0
	for i := 0; i < 3*tps; i++ {
		deal, exp := TickDoTFrame(&remaining, &timer, tps)
		if deal {
			ticks++
		}
		if exp {
			expiries++
		}
	}
	if ticks != 3 {
		t.Fatalf("3s DoT at 1 tick/s dealt %d ticks", ticks)
	}
	if expiries != 1 || remaining != 0 || timer != 0 {
		t.Fatalf("expiry: n=%d remaining=%d timer=%d", expiries, remaining, timer)
	}
	if deal, exp := TickDoTFrame(&remaining, &timer, tps); deal || exp {
		t.Fatal("inactive DoT must not tick")
	}
}

func TestTickDoTTurn(t *testing.T) {
	remaining, timer := 100, 30
	deal, exp := TickDoTTurn(&remaining, &timer, 60)
	if !deal || exp || remaining != 40 {
		t.Fatalf("first turn: deal=%v exp=%v remaining=%d", deal, exp, remaining)
	}
	deal, exp = TickDoTTurn(&remaining, &timer, 60)
	if !deal || !exp || remaining != 0 || timer != 0 {
		t.Fatalf("final turn: deal=%v exp=%v remaining=%d timer=%d", deal, exp, remaining, timer)
	}
	if deal, exp = TickDoTTurn(&remaining, &timer, 60); deal || exp {
		t.Fatal("inactive DoT must not tick per turn")
	}
}

func TestClear(t *testing.T) {
	remaining, timer := 500, 30
	Clear(&remaining, &timer)
	if remaining != 0 || timer != 0 {
		t.Fatalf("clear left remaining=%d timer=%d", remaining, timer)
	}
}

// TestRatedDualClockNoModeFarm: the user-reported exploit - a 5s/3turn stun,
// 2 turns spent in TB, then a switch to RT must NOT hand back the full 5
// seconds; the frame clock is clamped to the proportional remainder.
func TestRatedDualClockNoModeFarm(t *testing.T) {
	frames, turns, rate := 300, 3, 0

	// Two TB turns: each drains a proportional 100-frame share.
	if TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("stun expired after 1 of 3 turns")
	}
	if TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("stun expired after 2 of 3 turns")
	}
	if turns != 1 || frames != 100 {
		t.Fatalf("after 2 turns: turns=%d frames=%d, want 1/100", turns, frames)
	}

	// Switch to RT: expiry lands exactly 100 frames later, never 300.
	elapsed := 0
	for frames > 0 {
		elapsed++
		if elapsed > 300 {
			t.Fatal("stun never expired in RT")
		}
		if TickFrameRated(&frames, &turns, &rate) {
			break
		}
	}
	if elapsed != 100 {
		t.Fatalf("RT remainder = %d frames, want the proportional 100", elapsed)
	}
	if turns != 0 || frames != 0 || rate != 0 {
		t.Fatalf("expiry must clear everything: turns=%d frames=%d rate=%d", turns, frames, rate)
	}
}

// TestRatedDualClockReverseDirection: spending in RT first must drain the TB
// clock proportionally too (the farm works both ways).
func TestRatedDualClockReverseDirection(t *testing.T) {
	frames, turns, rate := 300, 3, 0
	for i := 0; i < 240; i++ { // ride out 4 of 5 seconds
		if TickFrameRated(&frames, &turns, &rate) {
			t.Fatalf("stun expired early at frame %d", i)
		}
	}
	if turns != 1 {
		t.Fatalf("after 240/300 frames turns=%d, want 1", turns)
	}
	// The last TB turn ends it - both clocks clear.
	if !TickTurnRated(&turns, &frames, &rate) {
		t.Fatalf("final turn must expire the stun (turns=%d frames=%d)", turns, frames)
	}
	if frames != 0 {
		t.Fatalf("expiry left frames=%d", frames)
	}
}

// TestRatedDualClockSingleTurnAuthoring: a 2s/1turn stun (lightning bolt
// authoring) - one TB turn IS the whole stun; RT spending keeps the single
// turn alive until the frames run out.
func TestRatedDualClockSingleTurnAuthoring(t *testing.T) {
	frames, turns, rate := 120, 1, 0
	if !TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("1-turn stun must expire on its only turn")
	}
	if frames != 0 {
		t.Fatalf("expiry left frames=%d", frames)
	}

	frames, turns, rate = 120, 1, 0
	for i := 0; i < 60; i++ {
		TickFrameRated(&frames, &turns, &rate)
	}
	if turns != 1 || frames != 60 {
		t.Fatalf("half-spent 2s/1t stun: turns=%d frames=%d, want 1/60", turns, frames)
	}
}

// New saves persist rate because remaining clocks alone cannot reconstruct the
// authored exchange ratio after arbitrary RT progress.
func TestRatedDualClockPersistedRateSurvivesLoad(t *testing.T) {
	frames, turns, rate := 361, 4, 120
	if TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("expired on first post-load turn")
	}
	if turns != 3 || frames != 360 || rate != 120 {
		t.Fatalf("post-load turn: turns=%d frames=%d rate=%d, want 3/360/120", turns, frames, rate)
	}
}

func TestRatedDualClockLegacyLoadFallback(t *testing.T) {
	frames, turns, rate := 200, 2, 0
	if TickTurnRated(&turns, &frames, &rate) {
		t.Fatal("legacy status expired on first post-load turn")
	}
	if turns != 1 || frames != 100 || rate != 100 {
		t.Fatalf("legacy fallback: turns=%d frames=%d rate=%d, want 1/100/100", turns, frames, rate)
	}
}
