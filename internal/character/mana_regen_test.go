package character

import (
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/stats"
)

// regenTestConfig builds a minimal config with one class so we can construct
// an MMCharacter via the standard setup* path. The test then overrides stats
// directly to whatever the scenario needs.
func regenTestConfig() *config.Config {
	return &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"cleric": {Base: stats.StatBonuses{Might: 10, Intellect: 10, Personality: 10, Endurance: 10, Accuracy: 10, Speed: 10, Luck: 10}},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 1, LevelMultiplier: 1},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 1},
		},
	}
}

func newRegenTestCharacter(personality int) *MMCharacter {
	c := CreateCharacter("RegenTest", ClassCleric, regenTestConfig())
	c.Personality = personality
	c.MaxSpellPoints = 100
	c.SpellPoints = 50
	return c
}

func TestCalculateManaRegenAmountScalesWithPersonality(t *testing.T) {
	cases := []struct {
		personality int
		want        int
	}{
		{-25, 1}, // 1 + (-25/10) = -1: the floor holds regen at 1
		{0, 1}, {9, 1}, {10, 2}, {25, 3}, {50, 6}, {100, 11},
	}
	for _, tc := range cases {
		c := newRegenTestCharacter(tc.personality)
		if got := c.CalculateManaRegenAmount(); got != tc.want {
			t.Errorf("Personality=%d: regen=%d, want %d", tc.personality, got, tc.want)
		}
	}
}

func TestCalculateManaRegenAmountUsesBuffBonuses(t *testing.T) {
	c := newRegenTestCharacter(10) // Personality 10 -> +1 -> base regen 2
	c.BuffBonuses = UniformStatBonuses(20)
	// +20 buff -> effective 30 -> 1 + 30/10 = 4
	if got := c.CalculateManaRegenAmount(); got != 4 {
		t.Errorf("buff +20 with Personality=10: regen=%d, want 4", got)
	}
}

func TestRegenerateSpellPointsAddsAndCaps(t *testing.T) {
	c := newRegenTestCharacter(10) // regen = 2
	c.MaxSpellPoints = 10
	c.SpellPoints = 7
	c.RegenerateSpellPoints()
	if c.SpellPoints != 9 { // 7 + 2
		t.Errorf("first regen: SP=%d, want 9", c.SpellPoints)
	}
	c.RegenerateSpellPoints()
	if c.SpellPoints != 10 { // capped at max
		t.Errorf("second regen (cap): SP=%d, want 10", c.SpellPoints)
	}
	c.RegenerateSpellPoints()
	if c.SpellPoints != 10 { // no-op when already at max
		t.Errorf("third regen (idempotent): SP=%d, want 10", c.SpellPoints)
	}
}

func TestRegenerateSpellPointsSkipsUnconscious(t *testing.T) {
	c := newRegenTestCharacter(10)
	c.SpellPoints = 50
	c.AddCondition(ConditionUnconscious)
	c.RegenerateSpellPoints()
	if c.SpellPoints != 50 {
		t.Errorf("unconscious char regenerated SP from 50 to %d", c.SpellPoints)
	}
}

func TestRegenerateSpellPointsSkipsDeadHP(t *testing.T) {
	c := newRegenTestCharacter(10)
	c.HitPoints = 0
	c.SpellPoints = 50
	c.RegenerateSpellPoints()
	if c.SpellPoints != 50 {
		t.Errorf("HP=0 char regenerated SP from 50 to %d", c.SpellPoints)
	}
}

// TestRealtimeRegenTimerCadence checks the real-time path the game drives
// (UpdateWithMode(PartyClock{})): SP regenerates once every regeneration
// period of frames, and the period restarts afterwards.
func TestRealtimeRegenTimerCadence(t *testing.T) {
	c := newRegenTestCharacter(10) // regen = 2
	c.MaxSpellPoints = 100
	c.SpellPoints = 0

	// One frame short of the interval - no regen yet.
	for i := 0; i < (config.RegenerationSeconds*config.GetTargetTPS())-1; i++ {
		c.UpdateWithMode(PartyClock{})
	}
	if c.SpellPoints != 0 {
		t.Fatalf("regen fired before timer reached threshold; SP=%d", c.SpellPoints)
	}

	// One more frame crosses the threshold.
	c.UpdateWithMode(PartyClock{})
	if c.SpellPoints != 2 {
		t.Errorf("after %d frames: SP=%d, want 2", (config.RegenerationSeconds * config.GetTargetTPS()), c.SpellPoints)
	}

	// Another full interval -> another +2.
	for i := 0; i < (config.RegenerationSeconds * config.GetTargetTPS()); i++ {
		c.UpdateWithMode(PartyClock{})
	}
	if c.SpellPoints != 4 {
		t.Errorf("after second interval: SP=%d, want 4", c.SpellPoints)
	}
}

// Turn-based frames never advance the real-time cadence: TB regen is paid per
// party round by the game (endPartyTurn), not per frame.
func TestTurnBasedFramesDoNotRegen(t *testing.T) {
	c := newRegenTestCharacter(10)
	c.SpellPoints = 30
	for i := 0; i < (config.RegenerationSeconds*config.GetTargetTPS())*3; i++ {
		c.UpdateWithMode(PartyClock{TurnBased: true})
	}
	if c.SpellPoints != 30 {
		t.Errorf("TB frames regenerated SP: %d, want 30", c.SpellPoints)
	}
	if c.SPRegen.Active() || c.HPRegen.Active() {
		t.Errorf("TB frames advanced the regeneration periods to %+v/%+v", c.SPRegen, c.HPRegen)
	}
}

// TestRealtimeRegenSkipsUnconscious: real-time frames should NOT advance SP
// on an unconscious character even when the timer would have fired.
func TestRealtimeRegenSkipsUnconscious(t *testing.T) {
	c := newRegenTestCharacter(10)
	c.MaxSpellPoints = 100
	c.SpellPoints = 30
	c.AddCondition(ConditionUnconscious)

	for i := 0; i < (config.RegenerationSeconds*config.GetTargetTPS())*3; i++ {
		c.UpdateWithMode(PartyClock{})
	}
	if c.SpellPoints != 30 {
		t.Errorf("unconscious real-time regen ran: SP=%d, want 30", c.SpellPoints)
	}
}
