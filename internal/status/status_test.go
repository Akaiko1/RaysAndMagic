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
