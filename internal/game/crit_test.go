package game

import (
	"testing"
	"ugataima/internal/character"
)

// newTestCombatSystem creates a minimal combat system suitable for unit tests
func newTestCombatSystem() *CombatSystem {
	g := &MMGame{}
	return &CombatSystem{game: g}
}

func TestRollCriticalChance_BoundariesAndLuck(t *testing.T) {
	cs := newTestCombatSystem()

	t.Run("No crit at 0%", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 0}
		for i := 0; i < 20; i++ { // multiple rolls to ensure determinism at 0%
			crit, total := cs.RollCriticalChance(0, chr)
			if total != 0 {
				t.Fatalf("expected total crit 0, got %d", total)
			}
			if crit {
				t.Fatalf("expected no crit at 0%%, got crit on roll %d", i)
			}
		}
	})

	t.Run("Always crit at 100%", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 0}
		for i := 0; i < 20; i++ { // multiple rolls to ensure determinism at 100%
			crit, total := cs.RollCriticalChance(100, chr)
			if total != 100 {
				t.Fatalf("expected total crit 100, got %d", total)
			}
			if !crit {
				t.Fatalf("expected crit at 100%%, got non-crit on roll %d", i)
			}
		}
	})

	t.Run("Luck contributes to crit chance", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 20} // luck/4 = 5
		crit, total := cs.RollCriticalChance(0, chr)
		// crit outcome is probabilistic; verify computed total chance only
		_ = crit
		if total != 5 {
			t.Fatalf("expected total crit 5 from luck, got %d", total)
		}
	})

	t.Run("Crit chance clamps above 100", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 20}       // +5 from luck
		crit, total := cs.RollCriticalChance(95, chr) // 95 + 5 => 100
		if total != 100 {
			t.Fatalf("expected total crit clamped to 100, got %d", total)
		}
		if !crit {
			t.Fatalf("expected crit at 100%% total chance")
		}
	})

	t.Run("Crit chance clamps below 0", func(t *testing.T) {
		chr := &character.MMCharacter{Luck: 0}
		crit, total := cs.RollCriticalChance(-10, chr)
		if total != 0 {
			t.Fatalf("expected total crit clamped to 0, got %d", total)
		}
		if crit {
			t.Fatalf("expected no crit at 0%% total chance")
		}
	})
}
