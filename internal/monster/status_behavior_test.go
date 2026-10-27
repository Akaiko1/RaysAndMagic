package monster

// Behavior suite for every monster status clock: poison (RT cadence + TB
// turn), stun (dual clock via the real Update path), root (TB held semantics +
// RT pin timer). Charm/Bind expiry lives in the game loop and is covered by
// internal/game tests.

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/status"
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
	if m.SoakDamage != 5 || m.Soak.Rate != 30 {
		t.Fatalf("armed soak = damage %d rate %d, want 5/30", m.SoakDamage, m.Soak.Rate)
	}

	for range 30 {
		m.TickDebuffs(false)
	}
	if m.Soak.Frames != 90 || m.Soak.Turns != 3 {
		t.Fatalf("soak after RT = %d frames/%d turns, want 90/3", m.Soak.Frames, m.Soak.Turns)
	}
	m.TickDebuffs(true)
	if m.Soak.Frames != 60 || m.Soak.Turns != 2 {
		t.Fatalf("soak after RT->TB = %d frames/%d turns, want 60/2", m.Soak.Frames, m.Soak.Turns)
	}
	for range 60 {
		m.TickDebuffs(false)
	}
	if m.SoakDamage != 0 || m.Soak.Frames != 0 || m.Soak.Turns != 0 || m.Soak.Rate != 0 {
		t.Fatalf("mixed-mode soak did not fully expire: damage=%d frames=%d turns=%d rate=%d",
			m.SoakDamage, m.Soak.Frames, m.Soak.Turns, m.Soak.Rate)
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

// A slow drags a pounce's and a phase shift's real-time wait - it moves and
// attacks slower - but never their turn-based rounds, where it skips turns.
func TestSlowDragsAbilityCooldownsInRealTimeOnly(t *testing.T) {
	const tps = 120
	pounce := func(m *Monster3D) *status.DualClock { m.ArmPounceCooldown(2); return &m.PounceCD }
	phase := func(m *Monster3D) *status.DualClock { m.ArmPhaseShiftCooldown(); return &m.PhaseShiftCD }
	for _, c := range []struct {
		name          string
		slow          int
		arm           func(*Monster3D) *status.DualClock
		frames, turns int
	}{
		{"pounce at 50% slow", 50, pounce, 1200, 2},     // 10 s
		{"phase shift", 0, phase, 1080, 3},              // 9 s
		{"phase shift at 30% slow", 30, phase, 1543, 3}, // 9 s / 0.7 = 1542.86 frames, the nearest frame
	} {
		m := &Monster3D{Abilities: Abilities{Pounce: PounceAbility{CooldownSeconds: 5}, PhaseShift: PhaseShiftAbility{CooldownSeconds: 9}}}
		if c.slow > 0 {
			m.ApplySlow(c.slow, 10*tps, 10)
		}
		if clock := c.arm(m); clock.Frames != c.frames || clock.Turns != c.turns {
			t.Errorf("%s: %d frames / %d turns, want %d / %d", c.name, clock.Frames, clock.Turns, c.frames, c.turns)
		}
	}
}

func TestPounceCooldownCarriesAcrossModes(t *testing.T) {
	m := &Monster3D{Abilities: Abilities{Pounce: PounceAbility{CooldownSeconds: 4}}}
	const tps = 120
	m.ArmPounceCooldown(2)
	if m.PounceCD.Frames != 4*tps || m.PounceCD.Turns != 2 {
		t.Fatalf("armed pounce cooldown = %d frames/%d turns, want %d/2",
			m.PounceCD.Frames, m.PounceCD.Turns, 4*tps)
	}

	m.TickAbilityClocks(true)
	if m.PounceCD.Turns != 1 || m.PounceCD.Frames != 2*tps || m.PounceCD.Rate != 2*tps {
		t.Fatalf("first TB tick = %d frames/%d turns at rate %d, want %d/1 at %d",
			m.PounceCD.Frames, m.PounceCD.Turns, m.PounceCD.Rate, 2*tps, 2*tps)
	}
	for range tps {
		m.TickAbilityClocks(false)
	}
	if m.PounceCD.Turns != 1 || m.PounceCD.Frames != tps {
		t.Fatalf("partial RT continuation = %d frames/%d turns, want %d/1",
			m.PounceCD.Frames, m.PounceCD.Turns, tps)
	}
	for range tps {
		m.TickAbilityClocks(false)
	}
	if m.PounceCD.Turns != 0 || m.PounceCD.Frames != 0 {
		t.Fatalf("mixed-mode expiry did not clear both clocks: %d frames/%d turns",
			m.PounceCD.Frames, m.PounceCD.Turns)
	}

	m.ArmPounceCooldown(2)
	for range tps {
		m.TickAbilityClocks(false)
	}
	if m.PounceCD.Frames != 3*tps || m.PounceCD.Turns != 2 {
		t.Fatalf("partial RT progress = %d frames/%d turns, want %d/2",
			m.PounceCD.Frames, m.PounceCD.Turns, 3*tps)
	}
	m.TickAbilityClocks(true)
	if m.PounceCD.Frames != tps || m.PounceCD.Turns != 1 {
		t.Fatalf("RT-to-TB continuation = %d frames/%d turns, want %d/1",
			m.PounceCD.Frames, m.PounceCD.Turns, tps)
	}
	m.TickAbilityClocks(true)
	if m.PounceCD.Frames != 0 || m.PounceCD.Turns != 0 {
		t.Fatalf("TB expiry did not clear RT clock: %d frames/%d turns", m.PounceCD.Frames, m.PounceCD.Turns)
	}
}

// RT stun ticks inside the real Update; its expiry must clear the TB clock too
// (a pure-RT stun that authored both would otherwise leave the star overlay
// and TB skip stuck on).
func TestMonsterStunRTExpiryClearsTurns(t *testing.T) {
	m := statusTestMonster()
	m.Stun.Frames = 2
	m.Stun.Turns = 3

	m.Update(nil, 0, 0)
	if m.StateTimer != 0 {
		t.Fatal("stunned monster must not run its state machine")
	}
	m.Update(nil, 0, 0)
	if m.Stun.Frames != 0 || m.Stun.Turns != 0 {
		t.Fatalf("RT stun expiry must clear both clocks: f=%d t=%d", m.Stun.Frames, m.Stun.Turns)
	}
	// Post-stun the full update runs again - observable via the stun-DR memory
	// countdown, which is frozen during the stun's early return.
	m.StunDRMemory = status.DualClock{Frames: 10, Turns: 1, Rate: 180}
	m.Update(nil, 0, 0)
	if m.StunDRMemory.Frames != 9 {
		t.Fatal("monster must act again once the stun wears off")
	}
}

// TB root: the monster stays pinned for the WHOLE turn it started rooted
// (RootHeld covers the last rooted turn), then moves again.
func TestMonsterRootTurnHeldSemantics(t *testing.T) {
	m := statusTestMonster()
	m.Root.Turns = 2
	m.Root.Frames = 120

	m.TickRootTurn()
	if !m.RootHeld() || m.Root.Turns != 1 {
		t.Fatalf("turn 1 must be held: held=%v left=%d", m.RootHeld(), m.Root.Turns)
	}
	m.TickRootTurn()
	if !m.RootHeld() || m.Root.Turns != 0 {
		t.Fatalf("LAST rooted turn must still be held: held=%v left=%d", m.RootHeld(), m.Root.Turns)
	}
	if m.Root.Frames != 0 {
		t.Fatalf("TB root expiry must clear RT frames, got %d", m.Root.Frames)
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
	m.Root.Frames = 3
	m.Root.Turns = 2
	m.State = StatePatrolling
	x, y := m.X, m.Y

	for i := 0; i < 3; i++ {
		m.Update(nil, 500, 500)
	}
	if m.X != x || m.Y != y {
		t.Fatalf("rooted monster moved: (%.1f,%.1f)", m.X, m.Y)
	}
	if m.Root.Frames != 0 || m.Root.Turns != 0 {
		t.Fatalf("RT root expiry must clear both clocks: frames=%d turns=%d", m.Root.Frames, m.Root.Turns)
	}
	if m.StateTimer == 0 {
		t.Fatal("root pins position, not the state machine")
	}
}

// A slowed monster skips every third TB turn; an old save's count of slowed
// turns resumes the cadence where it stood.
func TestSlowSkipCadenceAndLegacySave(t *testing.T) {
	for _, tc := range []struct {
		legacy int
		want   []bool
	}{
		{0, []bool{false, false, true, false, false, true}},
		{1, []bool{false, true, false, false, true}},
		{2, []bool{true, false, false, true}},
	} {
		m := &Monster3D{}
		m.ApplySlow(30, 3600, 20)
		m.RestoreLegacySlowSkip(tc.legacy, 3)
		for i, want := range tc.want {
			if got := m.ConsumeSlowedTurn(3); got != want {
				t.Fatalf("legacy %d turn %d: skipped=%v, want %v", tc.legacy, i+1, got, want)
			}
		}
	}
}
