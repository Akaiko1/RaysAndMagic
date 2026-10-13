package monster

import "testing"

func TestSpellFearOwnsRetreatUntilItsClockExpires(t *testing.T) {
	for _, turnBased := range []bool{false, true} {
		m := &Monster3D{HitPoints: 100, State: StateAttacking}
		m.ApplyFear(12)
		m.BeginCombatEngagement()
		m.StateTimer = 100000 // Ordinary flee timeout must not release spell fear.
		if turnBased {
			m.NextFleeTurnStep(nil, 100, 100, 1)
		} else {
			m.finishFleeIfExpired(100, 100)
		}
		if m.State != StateFleeing || m.CurrentAIBehavior().Caps().MayAttack {
			t.Fatalf("modeTB=%v interrupted forced retreat", turnBased)
		}
		m.AdvanceSpellControls(11)
		if m.FearFramesRemaining != 1 {
			t.Fatal("lost fear remainder")
		}
		m.AdvanceSpellControls(1)
		if m.State == StateFleeing || m.FearFramesRemaining != 0 {
			t.Fatal("fear never expired")
		}
	}
}
