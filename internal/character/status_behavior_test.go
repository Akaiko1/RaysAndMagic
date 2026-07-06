package character

// Behavior suite for every implemented party status: poison, burn, stun,
// unconscious, dead, eradicated. Exercises the real apply/tick/cure entry
// points end-to-end (condition flags AND clocks), one test per status family.

import "testing"

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
	if c.HitPoints != 50-2*PoisonDamagePerTick {
		t.Fatalf("2s poison at %d/s dealt wrong damage: HP=%d", PoisonDamagePerTick, c.HitPoints)
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

func TestCharPoisonTurnBased(t *testing.T) {
	c := statusTestChar()
	c.ApplyPoison(100)
	c.TickPoisonTurn(60)
	if c.HitPoints != 50-PoisonDamagePerTick {
		t.Fatalf("TB poison must tick once per turn: HP=%d", c.HitPoints)
	}
	c.TickPoisonTurn(60) // 40 remaining -> expires this turn, still ticks
	if c.HitPoints != 50-2*PoisonDamagePerTick || c.HasCondition(ConditionPoisoned) {
		t.Fatalf("final TB turn must tick and clear: HP=%d poisoned=%v", c.HitPoints, c.HasCondition(ConditionPoisoned))
	}
}

func TestCharBurnLifecycle(t *testing.T) {
	c := statusTestChar()
	const tps = 60 // ApplyBurn desyncs by GetTargetTPS()/2; keep clocks consistent

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
	wantHP := 50 - 2*BurnDamagePerTick - 2*PoisonDamagePerTick
	if c.HitPoints != wantHP {
		t.Fatalf("burn (3x) + poison together: HP=%d want %d", c.HitPoints, wantHP)
	}
	if c.HasCondition(ConditionBurning) || c.HasCondition(ConditionPoisoned) {
		t.Fatal("both DoTs must clear their conditions on expiry")
	}
}

func TestCharBurnTurnBased(t *testing.T) {
	c := statusTestChar()
	c.ApplyBurn(60)
	c.TickBurnTurn(60)
	if c.HitPoints != 50-BurnDamagePerTick || c.HasCondition(ConditionBurning) {
		t.Fatalf("TB burn must tick %d and clear on expiry: HP=%d", BurnDamagePerTick, c.HitPoints)
	}
}

func TestCharStunDualClock(t *testing.T) {
	c := statusTestChar()
	c.ApplyCharStun(2, 3)
	if !c.IsStunned() || !c.HasCondition(ConditionStunned) {
		t.Fatal("apply must stun")
	}
	c.ApplyCharStun(1, 1) // weaker re-apply
	if c.StunFramesRemaining != 2 || c.StunTurnsRemaining != 3 {
		t.Fatal("stun refresh must never shorten either clock")
	}

	// RT expiry clears the TB clock and the condition.
	c.tickStunFrames()
	c.tickStunFrames()
	if c.IsStunned() || c.StunTurnsRemaining != 0 || c.HasCondition(ConditionStunned) {
		t.Fatalf("RT expiry must clear everything: f=%d t=%d", c.StunFramesRemaining, c.StunTurnsRemaining)
	}

	// TB expiry clears the RT clock symmetrically.
	c.ApplyCharStun(500, 1)
	c.TickStunTurn()
	if c.IsStunned() || c.StunFramesRemaining != 0 || c.HasCondition(ConditionStunned) {
		t.Fatalf("TB expiry must clear everything: f=%d t=%d", c.StunFramesRemaining, c.StunTurnsRemaining)
	}

	c.ApplyCharStun(0, 0)
	if c.IsStunned() || c.HasCondition(ConditionStunned) {
		t.Fatal("empty apply must not stun")
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
