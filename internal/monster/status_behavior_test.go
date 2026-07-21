package monster

// Behavior suite for every monster status clock: poison (RT cadence + TB
// turn), stun (dual clock via the real Update path), root (TB held semantics +
// RT pin timer). Charm/Bind expiry lives in the game loop and is covered by
// internal/game tests.

import (
	"testing"

	"ugataima/internal/config"
)

func statusTestMonster() *Monster3D {
	return &Monster3D{Name: "T", HitPoints: 200, MaxHitPoints: 200}
}

func TestMonsterPoisonLifecycleRT(t *testing.T) {
	m := statusTestMonster()
	tps := config.GetTargetTPS() // config-less monsters fall back to this

	m.ApplyPoison(2 * tps)
	m.ApplyPoison(tps) // weaker re-apply
	if m.PoisonedFramesRemaining != 2*tps {
		t.Fatalf("refresh must never shorten: %d", m.PoisonedFramesRemaining)
	}

	for i := 0; i < 2*tps; i++ {
		m.TickPoison()
	}
	// 1% of 200 max HP = 2 per tick, 2 ticks.
	if m.HitPoints != 200-2*2 {
		t.Fatalf("2s poison at 1%% maxHP/s: HP=%d", m.HitPoints)
	}
	if m.PoisonedFramesRemaining != 0 || m.poisonTickTimer != 0 {
		t.Fatal("expiry must zero the clock and cadence timer")
	}
	hp := m.HitPoints
	m.TickPoison()
	if m.HitPoints != hp {
		t.Fatal("inactive poison must not tick")
	}
}

func TestMonsterPoisonTurnBased(t *testing.T) {
	m := statusTestMonster()
	m.ApplyPoison(100)
	m.TickPoisonTurn(60)
	if m.HitPoints != 198 {
		t.Fatalf("TB poison must tick once per turn: HP=%d", m.HitPoints)
	}
	m.TickPoisonTurn(60) // 40 left -> expires, still ticks
	if m.HitPoints != 196 || m.PoisonedFramesRemaining != 0 {
		t.Fatalf("final TB turn: HP=%d remaining=%d", m.HitPoints, m.PoisonedFramesRemaining)
	}
}

// RT stun ticks inside the real Update; its expiry must clear the TB clock too
// (a pure-RT stun that authored both would otherwise leave the star overlay
// and TB skip stuck on).
func TestMonsterStunRTExpiryClearsTurns(t *testing.T) {
	m := statusTestMonster()
	m.StunFramesRemaining = 2
	m.StunTurnsRemaining = 3

	m.Update(nil, 0, 0)
	if m.StateTimer != 0 {
		t.Fatal("stunned monster must not run its state machine")
	}
	m.Update(nil, 0, 0)
	if m.StunFramesRemaining != 0 || m.StunTurnsRemaining != 0 {
		t.Fatalf("RT stun expiry must clear both clocks: f=%d t=%d", m.StunFramesRemaining, m.StunTurnsRemaining)
	}
	// Post-stun the full update runs again - observable via the stun-DR memory
	// countdown, which is frozen during the stun's early return.
	m.StunDRMemoryFrames = 10
	m.Update(nil, 0, 0)
	if m.StunDRMemoryFrames != 9 {
		t.Fatal("monster must act again once the stun wears off")
	}
}

// TB root: the monster stays pinned for the WHOLE turn it started rooted
// (RootHeld covers the last rooted turn), then moves again.
func TestMonsterRootTurnHeldSemantics(t *testing.T) {
	m := statusTestMonster()
	m.RootTurnsRemaining = 2
	m.RootFramesRemaining = 120

	m.TickRootTurn()
	if !m.RootHeld() || m.RootTurnsRemaining != 1 {
		t.Fatalf("turn 1 must be held: held=%v left=%d", m.RootHeld(), m.RootTurnsRemaining)
	}
	m.TickRootTurn()
	if !m.RootHeld() || m.RootTurnsRemaining != 0 {
		t.Fatalf("LAST rooted turn must still be held: held=%v left=%d", m.RootHeld(), m.RootTurnsRemaining)
	}
	if m.RootFramesRemaining != 0 {
		t.Fatalf("TB root expiry must clear RT frames, got %d", m.RootFramesRemaining)
	}
	m.TickRootTurn()
	if m.RootHeld() {
		t.Fatal("root has expired; movement must be free again")
	}
}

// RT root: the full Update runs (state machine advances, so the monster still
// fights) but any displacement is undone - and the timer burns down.
func TestMonsterRootRTPinsPosition(t *testing.T) {
	m := statusTestMonster()
	m.RootFramesRemaining = 3
	m.RootTurnsRemaining = 2
	m.State = StatePatrolling
	x, y := m.X, m.Y

	for i := 0; i < 3; i++ {
		m.Update(nil, 500, 500)
	}
	if m.X != x || m.Y != y {
		t.Fatalf("rooted monster moved: (%.1f,%.1f)", m.X, m.Y)
	}
	if m.RootFramesRemaining != 0 || m.RootTurnsRemaining != 0 {
		t.Fatalf("RT root expiry must clear both clocks: frames=%d turns=%d", m.RootFramesRemaining, m.RootTurnsRemaining)
	}
	if m.StateTimer == 0 {
		t.Fatal("root pins position, not the state machine")
	}
}
