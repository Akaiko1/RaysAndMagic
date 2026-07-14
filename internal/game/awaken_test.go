package game

import (
	"testing"

	"ugataima/internal/character"
)

// TestAwaken_RevivesAllUnconsciousToOneHP: Awaken rouses EVERY unconscious party
// member to 1 HP (and only the unconscious - not the alive, not the truly dead).
func TestAwaken_RevivesAllUnconsciousToOneHP(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)

	// Two members knocked out, one healthy (the caster = Members[0]).
	ko1 := game.party.Members[1]
	ko2 := game.party.Members[2]
	for _, m := range []*character.MMCharacter{ko1, ko2} {
		m.HitPoints = 0
		m.AddCondition(character.ConditionUnconscious)
	}
	healthy := game.party.Members[3]
	healthy.HitPoints = 20
	hpBefore := healthy.HitPoints

	equipSpellAndPrepareCaster(t, game.combat, "awaken", 100, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("awaken cast failed")
	}

	for i, m := range []*character.MMCharacter{ko1, ko2} {
		if m.HasCondition(character.ConditionUnconscious) {
			t.Errorf("KO member %d still unconscious after Awaken", i+1)
		}
		if m.HitPoints != 1 {
			t.Errorf("KO member %d should be at 1 HP, got %d", i+1, m.HitPoints)
		}
	}
	// A healthy member is untouched (Awaken only revives, doesn't heal/cap).
	if healthy.HitPoints != hpBefore {
		t.Errorf("healthy member HP changed: %d -> %d", hpBefore, healthy.HitPoints)
	}
}
