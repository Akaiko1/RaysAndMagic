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

// TestMonsterPoisonTurnBased: a TB round consumes several seconds of duration
// and must deal that many ticks (1% max HP each), so the DoT's total matches RT.
func TestMonsterPoisonTurnBased(t *testing.T) {
	m := statusTestMonster()
	tps := config.GetTargetTPS() // config-less monsters fall back to this
	round := 3 * tps             // one TB round = three seconds of DoT time
	const perTick = 2            // 1% of 200 max HP

	m.ApplyPoison(10 * tps)
	m.TickPoisonTurn(round)
	if m.HitPoints != 200-3*perTick {
		t.Fatalf("a 3s TB round must deal 3 poison ticks: HP=%d", m.HitPoints)
	}
	if m.PoisonedFramesRemaining != 7*tps {
		t.Fatalf("round consumed %d frames, want %d", 10*tps-m.PoisonedFramesRemaining, round)
	}
	for m.PoisonedFramesRemaining > 0 {
		m.TickPoisonTurn(round)
	}
	if m.HitPoints != 200-10*perTick {
		t.Fatalf("a 10s poison must deal 10 ticks in TB too: HP=%d", m.HitPoints)
	}
}

func TestChampionSoakCarriesAcrossModes(t *testing.T) {
	m := statusTestMonster()
	m.ApplySoak(5, 120, 4)
	if m.SoakDamage != 5 || m.SoakRate != 30 {
		t.Fatalf("armed soak = damage %d rate %d, want 5/30", m.SoakDamage, m.SoakRate)
	}

	for range 30 {
		m.TickSoakFrame()
	}
	if m.SoakFrames != 90 || m.SoakTurns != 3 {
		t.Fatalf("soak after RT = %d frames/%d turns, want 90/3", m.SoakFrames, m.SoakTurns)
	}
	m.TickSoakTurn()
	if m.SoakFrames != 60 || m.SoakTurns != 2 {
		t.Fatalf("soak after RT->TB = %d frames/%d turns, want 60/2", m.SoakFrames, m.SoakTurns)
	}
	for range 60 {
		m.TickSoakFrame()
	}
	if m.SoakDamage != 0 || m.SoakFrames != 0 || m.SoakTurns != 0 || m.SoakRate != 0 {
		t.Fatalf("mixed-mode soak did not fully expire: damage=%d frames=%d turns=%d rate=%d",
			m.SoakDamage, m.SoakFrames, m.SoakTurns, m.SoakRate)
	}
}

func TestInvulnerableBossAbsorbsPoisonInBothModes(t *testing.T) {
	for _, flag := range []struct {
		name  string
		apply func(*Monster3D)
	}{
		{name: "dormant", apply: func(m *Monster3D) { m.BossDormant = true }},
		{name: "warded", apply: func(m *Monster3D) { m.BossWarded = true }},
	} {
		for _, turnBased := range []bool{false, true} {
			mode := "RT"
			if turnBased {
				mode = "TB"
			}
			t.Run(flag.name+"/"+mode, func(t *testing.T) {
				m := statusTestMonster()
				flag.apply(m)
				tps := config.GetTargetTPS()
				m.ApplyPoison(tps)
				if turnBased {
					m.TickPoisonTurn(tps)
				} else {
					for range tps {
						m.TickPoison()
					}
				}
				if m.HitPoints != m.MaxHitPoints {
					t.Fatalf("invulnerable boss took poison damage: HP=%d", m.HitPoints)
				}
				if m.PoisonedFramesRemaining != 0 {
					t.Fatalf("poison clock did not advance while damage was absorbed: %d", m.PoisonedFramesRemaining)
				}
			})
		}
	}
}

func TestPounceCooldownCarriesAcrossModes(t *testing.T) {
	m := &Monster3D{PounceCooldownSeconds: 4}
	const tps = 120
	m.ArmPounceCooldown(tps, 2)
	if m.PounceCDFrames != 4*tps || m.PounceCDTurns != 2 {
		t.Fatalf("armed pounce cooldown = %d frames/%d turns, want %d/2",
			m.PounceCDFrames, m.PounceCDTurns, 4*tps)
	}

	m.TickPounceCooldownTurn()
	if m.PounceCDTurns != 1 || m.PounceCDFrames != 2*tps || m.PounceCDRate != 2*tps {
		t.Fatalf("first TB tick = %d frames/%d turns at rate %d, want %d/1 at %d",
			m.PounceCDFrames, m.PounceCDTurns, m.PounceCDRate, 2*tps, 2*tps)
	}
	for range tps {
		m.TickPounceCooldownFrame()
	}
	if m.PounceCDTurns != 1 || m.PounceCDFrames != tps {
		t.Fatalf("partial RT continuation = %d frames/%d turns, want %d/1",
			m.PounceCDFrames, m.PounceCDTurns, tps)
	}
	for range tps {
		m.TickPounceCooldownFrame()
	}
	if m.PounceCDTurns != 0 || m.PounceCDFrames != 0 {
		t.Fatalf("mixed-mode expiry did not clear both clocks: %d frames/%d turns",
			m.PounceCDFrames, m.PounceCDTurns)
	}

	m.ArmPounceCooldown(tps, 2)
	for range tps {
		m.TickPounceCooldownFrame()
	}
	if m.PounceCDFrames != 3*tps || m.PounceCDTurns != 2 {
		t.Fatalf("partial RT progress = %d frames/%d turns, want %d/2",
			m.PounceCDFrames, m.PounceCDTurns, 3*tps)
	}
	m.TickPounceCooldownTurn()
	if m.PounceCDFrames != 2*tps || m.PounceCDTurns != 1 {
		t.Fatalf("RT-to-TB continuation = %d frames/%d turns, want %d/1",
			m.PounceCDFrames, m.PounceCDTurns, 2*tps)
	}
	m.TickPounceCooldownTurn()
	if m.PounceCDFrames != 0 || m.PounceCDTurns != 0 {
		t.Fatalf("TB expiry did not clear RT clock: %d frames/%d turns", m.PounceCDFrames, m.PounceCDTurns)
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
