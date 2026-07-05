package game

import (
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

// revivalTestGame builds a 4-member party game with a revival potion in the
// inventory at index 0. Loaded weapon+item configs are needed for both party
// creation and the revival_potion lookup.
func revivalTestGame(t *testing.T) *MMGame {
	g := selectionTestGame(t) // reuses bridge setup + party
	potion := items.CreateItemFromYAML("revival_potion")
	g.party.Inventory = append([]items.Item{potion}, g.party.Inventory...)
	return g
}

func TestRevivablePartyIndices_EmptyWhenAllAlive(t *testing.T) {
	g := selectionTestGame(t)
	if got := g.RevivablePartyIndices(); len(got) != 0 {
		t.Errorf("expected 0 revivable members, got %v", got)
	}
}

func TestRevivablePartyIndices_IncludesUnconscious(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[1].AddCondition(character.ConditionUnconscious)
	got := g.RevivablePartyIndices()
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("expected [1], got %v", got)
	}
}

func TestRevivablePartyIndices_IncludesDead(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[2].AddCondition(character.ConditionDead)
	got := g.RevivablePartyIndices()
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestRevivablePartyIndices_IncludesZeroHP(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[3].HitPoints = 0
	got := g.RevivablePartyIndices()
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("expected [3] (HP=0 alone qualifies), got %v", got)
	}
}

func TestRevivablePartyIndices_ExcludesEradicated(t *testing.T) {
	g := selectionTestGame(t)
	// Dead AND Eradicated -> not revivable (eradication is permanent).
	g.party.Members[0].HitPoints = 0
	g.party.Members[0].AddCondition(character.ConditionDead)
	g.party.Members[0].AddCondition(character.ConditionEradicated)
	if got := g.RevivablePartyIndices(); len(got) != 0 {
		t.Errorf("eradicated should be excluded, got %v", got)
	}
}

func TestRevivablePartyIndices_MultipleInOrder(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].AddCondition(character.ConditionUnconscious)
	g.party.Members[2].AddCondition(character.ConditionDead)
	g.party.Members[3].HitPoints = 0
	got := g.RevivablePartyIndices()
	want := []int{0, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestApplyReviveTo_SuccessClearsConditionsAndConsumes(t *testing.T) {
	g := revivalTestGame(t)
	target := g.party.Members[1]
	target.HitPoints = 0
	target.AddCondition(character.ConditionDead)
	target.AddCondition(character.ConditionUnconscious)
	startInvLen := len(g.party.Inventory)

	if !g.applyReviveTo(0, 1) {
		t.Fatalf("applyReviveTo returned false on valid revive")
	}
	if target.HasCondition(character.ConditionDead) {
		t.Errorf("Dead condition not removed")
	}
	if target.HasCondition(character.ConditionUnconscious) {
		t.Errorf("Unconscious condition not removed")
	}
	if target.HitPoints != target.MaxHitPoints {
		t.Errorf("revival_potion has full_heal=true; HP=%d, want max=%d", target.HitPoints, target.MaxHitPoints)
	}
	if len(g.party.Inventory) != startInvLen-1 {
		t.Errorf("inventory size %d, want %d (potion consumed)", len(g.party.Inventory), startInvLen-1)
	}
}

func TestApplyReviveTo_BoundsChecks(t *testing.T) {
	g := revivalTestGame(t)
	if g.applyReviveTo(-1, 0) {
		t.Errorf("itemIdx=-1 should fail")
	}
	if g.applyReviveTo(100, 0) {
		t.Errorf("itemIdx out of range should fail")
	}
	if g.applyReviveTo(0, -1) {
		t.Errorf("targetIdx=-1 should fail")
	}
	if g.applyReviveTo(0, 100) {
		t.Errorf("targetIdx out of range should fail")
	}
}

func TestApplyReviveTo_RejectsStaleNonReviveItem(t *testing.T) {
	g := revivalTestGame(t)
	// Replace the revive potion at idx 0 with a heal potion. Picker thinks
	// it's still pointing at a revive item - applyReviveTo must refuse.
	heal := items.CreateItemFromYAML("health_potion")
	g.party.Inventory[0] = heal
	g.party.Members[1].HitPoints = 0

	if g.applyReviveTo(0, 1) {
		t.Errorf("applyReviveTo should refuse when slot no longer holds a revive item")
	}
	// Inventory length unchanged - item not consumed on rejection.
	if g.party.Inventory[0].Name != heal.Name {
		t.Errorf("inventory mutated on rejected revive")
	}
}

func TestUseConsumable_RevivePath_ZeroTargets(t *testing.T) {
	g := revivalTestGame(t)
	// Everyone is fully alive; using the potion should NOT consume it.
	startLen := len(g.party.Inventory)
	if g.UseConsumableFromInventory(0, 0) {
		t.Errorf("UseConsumableFromInventory should return false when no targets")
	}
	if len(g.party.Inventory) != startLen {
		t.Errorf("potion consumed when no one was dead")
	}
	if g.revivalPickerOpen {
		t.Errorf("picker shouldn't open with 0 revivable")
	}
}

func TestUseConsumable_RevivePath_OneTarget_RevivesImmediately(t *testing.T) {
	g := revivalTestGame(t)
	g.party.Members[2].HitPoints = 0
	g.party.Members[2].AddCondition(character.ConditionDead)
	startLen := len(g.party.Inventory)

	if !g.UseConsumableFromInventory(0, 0) {
		t.Errorf("UseConsumableFromInventory should return true on 1-target revive")
	}
	if g.revivalPickerOpen {
		t.Errorf("picker shouldn't open for 1-target case")
	}
	if g.party.Members[2].HasCondition(character.ConditionDead) {
		t.Errorf("target 2 still has Dead condition after revive")
	}
	if len(g.party.Inventory) != startLen-1 {
		t.Errorf("potion not consumed on 1-target revive")
	}
}

func TestUseConsumable_RevivePath_MultipleTargets_OpensPicker(t *testing.T) {
	g := revivalTestGame(t)
	g.party.Members[1].AddCondition(character.ConditionUnconscious)
	g.party.Members[2].AddCondition(character.ConditionDead)
	startLen := len(g.party.Inventory)

	g.UseConsumableFromInventory(0, 0) // selectedChar irrelevant when picker opens
	if !g.revivalPickerOpen {
		t.Errorf("picker should open with 2+ revivable")
	}
	if g.revivalPickerItemIdx != 0 {
		t.Errorf("picker item index=%d, want 0", g.revivalPickerItemIdx)
	}
	if len(g.party.Inventory) != startLen {
		t.Errorf("potion consumed prematurely; should wait for picker confirm")
	}
	// Members still down - picker hasn't applied yet.
	if !g.party.Members[1].HasCondition(character.ConditionUnconscious) {
		t.Errorf("member 1 revived without picker confirm")
	}
}
