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
		if !turnBased {
			for range 11 {
				m.AdvanceSpellControls(false)
			}
			if m.Fear.Frames != 1 {
				t.Fatal("lost fear remainder")
			}
		}
		m.AdvanceSpellControls(turnBased) // a fraction of a round still costs the whole turn
		if m.State == StateFleeing || m.Fear.Active() {
			t.Fatal("fear never expired")
		}
	}
}
