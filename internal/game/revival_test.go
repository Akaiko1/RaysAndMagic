package game

import (
	"slices"
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

// Unconscious, Dead and HP 0 each qualify; Eradicated never does (it is
// permanent). Indices come back in party order.
func TestRevivablePartyIndices(t *testing.T) {
	tests := []struct {
		name string
		down func(m []*character.MMCharacter)
		want []int
	}{
		{"all alive", func([]*character.MMCharacter) {}, nil},
		{"unconscious", func(m []*character.MMCharacter) { m[1].AddCondition(character.ConditionUnconscious) }, []int{1}},
		{"dead", func(m []*character.MMCharacter) { m[2].AddCondition(character.ConditionDead) }, []int{2}},
		{"zero HP alone", func(m []*character.MMCharacter) { m[3].HitPoints = 0 }, []int{3}},
		{"dead and eradicated", func(m []*character.MMCharacter) {
			m[0].HitPoints = 0
			m[0].AddCondition(character.ConditionDead)
			m[0].AddCondition(character.ConditionEradicated)
		}, nil},
		{"several in party order", func(m []*character.MMCharacter) {
			m[0].AddCondition(character.ConditionUnconscious)
			m[2].AddCondition(character.ConditionDead)
			m[3].HitPoints = 0
		}, []int{0, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := selectionTestGame(t)
			tt.down(g.party.Members)
			if got := g.RevivablePartyIndices(); !slices.Equal(got, tt.want) {
				t.Errorf("revivable = %v, want %v", got, tt.want)
			}
		})
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

// Using a revive potion branches on the number of revivable members: none
// keeps the potion, one is revived at once, several open the picker and wait
// for its confirm before anything is spent.
func TestUseConsumableRevivePathByTargetCount(t *testing.T) {
	tests := []struct {
		name         string
		down         func(m []*character.MMCharacter)
		wantUsed     bool
		wantPicker   bool
		wantConsumed bool
		check        func(t *testing.T, g *MMGame)
	}{
		{"no target keeps the potion", func([]*character.MMCharacter) {}, false, false, false, nil},
		{"one target revives at once", func(m []*character.MMCharacter) {
			m[2].HitPoints = 0
			m[2].AddCondition(character.ConditionDead)
		}, true, false, true, func(t *testing.T, g *MMGame) {
			if g.party.Members[2].HasCondition(character.ConditionDead) {
				t.Error("target 2 still has Dead condition after revive")
			}
		}},
		{"several targets open the picker", func(m []*character.MMCharacter) {
			m[1].AddCondition(character.ConditionUnconscious)
			m[2].AddCondition(character.ConditionDead)
		}, false, true, false, func(t *testing.T, g *MMGame) {
			if g.revivalPickerItemIdx != 0 {
				t.Errorf("picker item index=%d, want 0", g.revivalPickerItemIdx)
			}
			if !g.party.Members[1].HasCondition(character.ConditionUnconscious) {
				t.Error("member 1 revived without picker confirm")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := revivalTestGame(t)
			tt.down(g.party.Members)
			startLen := len(g.party.Inventory)

			if used := g.UseConsumableFromInventory(0, 0); used != tt.wantUsed {
				t.Errorf("UseConsumableFromInventory = %v, want %v", used, tt.wantUsed)
			}
			if g.revivalPickerOpen != tt.wantPicker {
				t.Errorf("picker open = %v, want %v", g.revivalPickerOpen, tt.wantPicker)
			}
			wantLen := startLen
			if tt.wantConsumed {
				wantLen--
			}
			if len(g.party.Inventory) != wantLen {
				t.Errorf("inventory size %d, want %d (consumed=%v)", len(g.party.Inventory), wantLen, tt.wantConsumed)
			}
			if tt.check != nil {
				tt.check(t, g)
			}
		})
	}
}
