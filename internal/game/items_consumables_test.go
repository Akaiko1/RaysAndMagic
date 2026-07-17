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

// A manually parked Eradicated member can be selected for inventory inspection.
// With no eligible ally, a health potion must remain unspent rather than trying
// to self-heal or opening an empty picker.
func TestUseConsumable_ParkedEradicatedSelectionCannotSelfHeal(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.HitPoints = 0
	member.Conditions = []character.Condition{character.ConditionEradicated}
	if !g.selectPartyMemberManually(0) {
		t.Fatal("manual park on the eradicated member failed")
	}
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML("health_potion"))
	itemIdx := len(g.party.Inventory) - 1
	unitsBefore := g.party.GetTotalItems()

	if g.UseConsumableFromInventory(itemIdx, g.selectedChar) {
		t.Fatal("heal potion must not apply with an Eradicated member selected")
	}
	if member.HitPoints != 0 || !member.HasCondition(character.ConditionEradicated) {
		t.Fatalf("Eradicated member changed: HP=%d conditions=%v", member.HitPoints, member.Conditions)
	}
	if got := g.party.GetTotalItems(); got != unitsBefore {
		t.Fatalf("potion units = %d, want %d (refused use must not consume)", got, unitsBefore)
	}
}

func TestHealthPotion_IncapacitatedOwnerHealsEligibleAlly(t *testing.T) {
	conditions := []struct {
		name      string
		condition character.Condition
	}{
		{name: "unconscious", condition: character.ConditionUnconscious},
		{name: "dead", condition: character.ConditionDead},
		{name: "eradicated", condition: character.ConditionEradicated},
	}

	for _, tc := range conditions {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			owner := g.party.Members[0]
			owner.HitPoints = 0
			owner.AddCondition(tc.condition)
			ally := g.party.Members[1]
			ally.HitPoints = 1
			g.party.Inventory = []items.Item{items.CreateItemFromYAML("health_potion")}

			if !g.UseConsumableFromInventory(0, 0) {
				t.Fatal("health potion was not redirected to the only eligible ally")
			}
			if ally.HitPoints <= 1 {
				t.Fatalf("eligible ally was not healed: HP=%d", ally.HitPoints)
			}
			if len(g.party.Inventory) != 0 {
				t.Fatal("redirected health potion was not consumed")
			}
			if !owner.HasCondition(tc.condition) {
				t.Fatalf("owner lost %s while redirecting a health potion", tc.name)
			}
		})
	}
}
