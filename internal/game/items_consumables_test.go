package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// Regression: a heal potion must not be usable on an Eradicated member -
// applyHealTo only checked Unconscious/Dead, unlike its siblings
// RevivablePartyIndices/HealablePartyIndices which both exclude Eradicated.
// Only the Resurrect spell may clear that condition.
func TestApplyHealTo_RefusesEradicatedMember(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.HitPoints = 0
	member.Conditions = []character.Condition{character.ConditionEradicated}
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML("health_potion"))
	itemIdx := len(g.party.Inventory) - 1

	if g.applyHealTo(itemIdx, 0) {
		t.Fatal("a heal potion must not revive an Eradicated member")
	}
	if member.HitPoints != 0 {
		t.Errorf("Eradicated member HP = %d, want 0 (unhealed)", member.HitPoints)
	}
	if !member.HasCondition(character.ConditionEradicated) {
		t.Error("Eradicated condition should still be set")
	}
}
