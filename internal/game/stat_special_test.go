package game

import (
	"testing"
	"ugataima/internal/character"
)

func TestRollPerfectDodge_BoundariesAndLuck(t *testing.T) {
	cs := newTestCombatSystem()

	t.Run("No dodge at 0%", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 0}
		for i := 0; i < 20; i++ {
			dodged, chance := cs.RollPerfectDodge(chr)
			if chance != 0 {
				t.Fatalf("expected chance 0, got %d", chance)
			}
			if dodged {
				t.Fatalf("expected no dodge at 0%%, got dodge on roll %d", i)
			}
		}
	})

	t.Run("Always dodge at 100%", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 500} // 500/5 = 100
		for i := 0; i < 10; i++ {
			dodged, chance := cs.RollPerfectDodge(chr)
			if chance != 100 {
				t.Fatalf("expected chance 100, got %d", chance)
			}
			if !dodged {
				t.Fatalf("expected dodge at 100%%, got false")
			}
		}
	})

	t.Run("Luck contributes as Luck/5", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 25}
		dodged, chance := cs.RollPerfectDodge(chr)
		_ = dodged       // probabilistic
		if chance != 5 { // 25/5 = 5
			t.Fatalf("expected chance 5 from luck, got %d", chance)
		}
	})
}
