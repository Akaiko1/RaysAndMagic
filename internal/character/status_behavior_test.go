package character

// Behavior suite for every implemented party status: poison, burn, stun,
// unconscious, dead, eradicated. Exercises the real apply/tick/cure entry
// points end-to-end (condition flags AND clocks), one test per status family.

import (
	"testing"

	"ugataima/internal/config"
)

func statusTestChar() *MMCharacter {
	return &MMCharacter{Name: "T", HitPoints: 50, MaxHitPoints: 50}
}

func TestCharPoisonLifecycle(t *testing.T) {
	c := statusTestChar()
	const tps = 4

	c.ApplyPoison(2 * tps)
	if !c.HasCondition(ConditionPoisoned) {
		t.Fatal("apply must set the Poisoned condition")
	}
	c.ApplyPoison(tps) // weaker re-apply
	if c.PoisonFramesRemaining != 2*tps {
		t.Fatalf("refresh must never shorten: %d", c.PoisonFramesRemaining)
	}

	for i := 0; i < 2*tps; i++ {
		c.updatePoison(tps)
	}
	if c.HitPoints != 50-2*poisonPerTick() {
		t.Fatalf("2s poison at %d/s dealt wrong damage: HP=%d", poisonPerTick(), c.HitPoints)
	}
	if c.HasCondition(ConditionPoisoned) || c.PoisonFramesRemaining != 0 {
		t.Fatal("expiry must clear the condition and the clock")
	}

	c.ApplyPoison(1000)
	c.CurePoison()
	if c.HasCondition(ConditionPoisoned) || c.PoisonFramesRemaining != 0 || c.poisonTickTimer != 0 {
		t.Fatal("cure must zero the clock and the cadence timer, not just the icon")
	}
}

// TestCharPoisonTurnBased: a TB round consumes several seconds of duration, so
// it must deal that many ticks - the total over the whole DoT matches RT.
func TestCharPoisonTurnBased(t *testing.T) {
	c := statusTestChar()
	const tps = 60
	const round = 3 * tps // one TB round = three seconds of DoT time

	c.ApplyPoison(10 * tps)
	c.TickPoisonTurn(round, tps)
	if c.HitPoints != 50-3*poisonPerTick() {
		t.Fatalf("a 3s TB round must deal 3 poison ticks: HP=%d", c.HitPoints)
	}
	if c.PoisonFramesRemaining != 7*tps {
		t.Fatalf("round consumed %d frames, want %d", 10*tps-c.PoisonFramesRemaining, round)
	}
	for c.PoisonFramesRemaining > 0 {
		c.TickPoisonTurn(round, tps)
	}
	if c.HitPoints != 50-10*poisonPerTick() || c.HasCondition(ConditionPoisoned) {
		t.Fatalf("a 10s poison must deal 10 ticks in TB too and clear: HP=%d poisoned=%v",
			c.HitPoints, c.HasCondition(ConditionPoisoned))
	}
}

func TestCharBurnLifecycle(t *testing.T) {
	c := statusTestChar()
	// ApplyBurn banks half a second of cadence off GetTargetTPS; the ticking
	// clock must be that same one (as it is in the game) or the banked offset
	// would be worth a whole second and buy an extra tick.
	tps := config.GetTargetTPS()

	c.ApplyBurn(2 * tps)
	if !c.HasCondition(ConditionBurning) {
		t.Fatal("apply must set the Burning condition")
	}
	if c.burnTickTimer == 0 {
		t.Fatal("burn cadence must start desynced from poison")
	}
	c.ApplyPoison(2 * tps) // both DoTs run at once, independently
	for i := 0; i < 2*tps; i++ {
		c.updateBurn(tps)
		c.updatePoison(tps)
	}
	wantHP := 50 - 2*burnPerTick() - 2*poisonPerTick()
	if c.HitPoints != wantHP {
		t.Fatalf("burn (3x) + poison together: HP=%d want %d", c.HitPoints, wantHP)
	}
	if c.HasCondition(ConditionBurning) || c.HasCondition(ConditionPoisoned) {
		t.Fatal("both DoTs must clear their conditions on expiry")
	}
}

func TestCharBurnRefreshPreservesTickPhase(t *testing.T) {
	c := statusTestChar()
	tps := config.GetTargetTPS()

	c.ApplyBurn(10 * tps)
	advance := tps/2 - 10
	for range advance {
		c.updateBurn(tps)
	}
	if c.burnTickTimer != tps-10 {
		t.Fatalf("setup burn phase = %d, want %d", c.burnTickTimer, tps-10)
	}

	c.ApplyBurn(20 * tps) // stronger refresh extends duration, not the next tick
	if c.burnTickTimer != tps-10 {
		t.Fatalf("burn refresh reset tick phase to %d, want %d", c.burnTickTimer, tps-10)
	}
	for range 10 {
		c.updateBurn(tps)
	}
	if c.HitPoints != 50-burnPerTick() {
		t.Fatalf("refreshed burn delayed its due tick: HP=%d", c.HitPoints)
	}
}

func TestCharBurnTurnBased(t *testing.T) {
	c := statusTestChar()
	// ApplyBurn banks half a second of cadence (poison desync) measured in
	// GetTargetTPS frames, so the test must tick on that same clock.
	tps := config.GetTargetTPS()
	c.ApplyBurn(3 * tps)
	c.TickBurnTurn(3*tps, tps)
	if c.HitPoints != 50-3*burnPerTick() || c.HasCondition(ConditionBurning) {
		t.Fatalf("a 3s TB round must deal 3 burn ticks (%d each) and clear on expiry: HP=%d",
			burnPerTick(), c.HitPoints)
	}
}

func TestCharStunDualClock(t *testing.T) {
	c := statusTestChar()
	c.ApplyCharStun(2, 3)
	if !c.IsStunned() || !c.HasCondition(ConditionStunned) {
		t.Fatal("apply must stun")
	}
	c.ApplyCharStun(1, 1) // weaker re-apply
	if c.Stun.Frames != 2 || c.Stun.Turns != 3 {
		t.Fatal("stun refresh must never shorten either clock")
	}

	// RT expiry clears the TB clock and the condition.
	c.tickStunFrames()
	c.tickStunFrames()
	if c.IsStunned() || c.Stun.Turns != 0 || c.HasCondition(ConditionStunned) {
		t.Fatalf("RT expiry must clear everything: f=%d t=%d", c.Stun.Frames, c.Stun.Turns)
	}

	// TB expiry clears the RT clock symmetrically.
	c.ApplyCharStun(500, 1)
	c.TickStunTurn()
	if c.IsStunned() || c.Stun.Frames != 0 || c.HasCondition(ConditionStunned) {
		t.Fatalf("TB expiry must clear everything: f=%d t=%d", c.Stun.Frames, c.Stun.Turns)
	}

	c.ApplyCharStun(0, 0)
	if c.IsStunned() || c.HasCondition(ConditionStunned) {
		t.Fatal("empty apply must not stun")
	}
}

func TestCharStunWeakRefreshPreservesModeExchangeRate(t *testing.T) {
	c := statusTestChar()
	c.ApplyCharStun(480, 4)
	for range 119 {
		c.tickStunFrames()
	}
	if c.Stun.Frames != 361 || c.Stun.Turns != 4 || c.Stun.Rate != 120 {
		t.Fatalf("RT progress: frames=%d turns=%d rate=%d, want 361/4/120",
			c.Stun.Frames, c.Stun.Turns, c.Stun.Rate)
	}

	c.ApplyCharStun(120, 1)
	c.TickStunTurn()
	if c.Stun.Frames != 241 || c.Stun.Turns != 3 || c.Stun.Rate != 120 {
		t.Fatalf("weak refresh changed TB remainder: frames=%d turns=%d rate=%d, want 241/3/120",
			c.Stun.Frames, c.Stun.Turns, c.Stun.Rate)
	}
}

// The flag-only statuses share the Conditions list plumbing: idempotent add,
// exact remove, no cross-talk.
func TestCharFlagConditions(t *testing.T) {
	c := statusTestChar()
	for _, cond := range []Condition{ConditionUnconscious, ConditionDead, ConditionEradicated} {
		c.AddCondition(cond)
		c.AddCondition(cond) // must not duplicate
		if !c.HasCondition(cond) {
			t.Fatalf("%v must be present after add", cond)
		}
	}
	if n := len(c.Conditions); n != 3 {
		t.Fatalf("conditions must not duplicate: len=%d", n)
	}
	c.RemoveCondition(ConditionDead)
	if c.HasCondition(ConditionDead) || !c.HasCondition(ConditionUnconscious) || !c.HasCondition(ConditionEradicated) {
		t.Fatal("remove must drop exactly the given condition")
	}
}

// The DoTs' per-second hero loss, from the status table.
// dotPerTick is the expected loss of one DoT tick on a hero of maxHP, worked
// out here from the status row: its percent of max HP, at least its minimum.
func dotPerTick(key string, maxHP int) int {
	s := config.Status(key)
	return max(s.PercentPerSecond*maxHP/100, s.MinimumDamage)
}

func poisonPerTick() int { return dotPerTick("poison", 50) }
func burnPerTick() int   { return dotPerTick("burn", 50) }

// Hero poison and burn take a percent of max HP per tick: a small pool loses
// the floor, a large one its percent, and the two DoTs stack, in both modes.
func TestCharDoTScalesWithMaxHP(t *testing.T) {
	// The real tick rate: burn banks half a game second against poison.
	tps := config.GetTargetTPS()
	for _, maxHP := range []int{50, 1000} {
		for _, key := range []string{"poison", "burn", "both"} {
			for _, tb := range []bool{false, true} {
				c := &MMCharacter{Name: "T", HitPoints: maxHP, MaxHitPoints: maxHP}
				want := 0
				if key != "burn" {
					c.ApplyPoison(3 * tps)
					want += 3 * dotPerTick("poison", maxHP)
				}
				if key != "poison" {
					c.ApplyBurn(3 * tps)
					want += 3 * dotPerTick("burn", maxHP)
				}
				if tb {
					c.TickPoisonTurn(3*tps, tps)
					c.TickBurnTurn(3*tps, tps)
				} else {
					for range 3 * tps {
						c.updatePoison(tps)
						c.updateBurn(tps)
					}
				}
				if lost := maxHP - c.HitPoints; lost != want {
					t.Errorf("%s on %d max HP (TB=%v) took %d, want %d", key, maxHP, tb, lost, want)
				}
			}
		}
	}
	// The large pool must be past both floors, or the percent goes untested.
	if dotPerTick("poison", 1000) <= config.Status("poison").MinimumDamage || dotPerTick("burn", 1000) <= config.Status("burn").MinimumDamage {
		t.Fatal("fixture: 1000 max HP does not lift the DoTs above their floors")
	}
}
