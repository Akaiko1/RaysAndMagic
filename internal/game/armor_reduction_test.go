package game

// Armor damage-reduction tests — exercise the REAL CombatSystem path:
// CombatSystem.ApplyArmorDamageReduction → CalculateTotalArmorClass →
// CalculateArmorClassContribution (formula = base_armor + Endurance/divisor),
// then `final = damage - AC / ArmorPhysicalReductionDivisor`, floor 1.
//
// Replaces the placebo internal/character/armor_damage_test.go which used a
// hand-rolled `ApplyArmorDamageReduction` defined inside the test file and
// only ever verified its own implementation.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// equipArmorPieces equips the named items.yaml entries onto the first party
// member's armor slots. Caller is responsible for ensuring the character
// has any skill required to wear them.
func equipArmorPieces(t *testing.T, char *character.MMCharacter, keys ...string) {
	t.Helper()
	if char.Equipment == nil {
		char.Equipment = make(map[items.EquipSlot]items.Item)
	}
	for _, k := range keys {
		item, err := items.TryCreateItemFromYAML(k)
		if err != nil {
			t.Fatalf("item %q missing from items.yaml: %v", k, err)
		}
		if _, _, ok := char.EquipItem(item); !ok {
			t.Fatalf("EquipItem(%s) failed (missing skill?)", item.Name)
		}
	}
}

func TestApplyArmorDamageReduction_NoArmor_NoReduction(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	// Fresh character has no armor equipped.
	for _, damage := range []int{1, 10, 100} {
		got := cs.ApplyArmorDamageReduction(damage, char)
		if got != damage {
			t.Errorf("no-armor damage %d: got %d, want %d (untouched)", damage, got, damage)
		}
	}
}

func TestApplyArmorDamageReduction_LeatherArmorReducesBy_AC_over_Divisor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	// leather_armor (items.yaml): armor_class_base 2, endurance_scaling_divisor 5
	// Knight starter already has Leather skill via party setup; if not, the
	// helper will fail fast and tell us.
	equipArmorPieces(t, char, "leather_armor")

	expectedAC := cs.CalculateTotalArmorClass(char)
	if expectedAC <= 0 {
		t.Fatalf("expected positive AC after equipping leather armor, got %d", expectedAC)
	}
	wantReduction := expectedAC / ArmorPhysicalReductionDivisor

	for _, damage := range []int{20, 50, 100} {
		got := cs.ApplyArmorDamageReduction(damage, char)
		want := damage - wantReduction
		if want < 1 {
			want = 1
		}
		if got != want {
			t.Errorf("damage %d with AC %d: got %d, want %d (AC/%d = %d)",
				damage, expectedAC, got, want, ArmorPhysicalReductionDivisor, wantReduction)
		}
	}
}

func TestApplyArmorDamageReduction_FloorsAt1(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	// Stack the heaviest armor we have to push AC up; even if total AC
	// exceeds damage*2, final damage must clamp at 1.
	equipArmorPieces(t, char, "leather_armor", "leather_helmet", "leather_pants")

	got := cs.ApplyArmorDamageReduction(1, char)
	if got != 1 {
		t.Errorf("incoming 1 damage with heavy armor: got %d, want 1 (floor)", got)
	}
}

func TestApplyArmorDamageReduction_MultiSlotIsAdditive(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	// Measure AC after each piece is added — should grow strictly monotonically.
	zeroAC := cs.CalculateTotalArmorClass(char)
	if zeroAC != 0 {
		t.Fatalf("starter has unexpected baseline AC: %d", zeroAC)
	}
	equipArmorPieces(t, char, "leather_armor")
	oneAC := cs.CalculateTotalArmorClass(char)
	equipArmorPieces(t, char, "leather_helmet")
	twoAC := cs.CalculateTotalArmorClass(char)
	equipArmorPieces(t, char, "leather_pants")
	threeAC := cs.CalculateTotalArmorClass(char)

	if !(oneAC > zeroAC && twoAC > oneAC && threeAC > twoAC) {
		t.Errorf("AC should grow with each armor piece, got %d → %d → %d → %d",
			zeroAC, oneAC, twoAC, threeAC)
	}

	// Per-piece contributions should sum to total.
	pieces := []items.EquipSlot{items.SlotArmor, items.SlotHelmet, items.SlotBoots}
	sum := 0
	for _, slot := range pieces {
		if item, ok := char.Equipment[slot]; ok {
			sum += cs.CalculateArmorClassContribution(item, char)
		}
	}
	if sum != threeAC {
		t.Errorf("per-slot contributions sum %d ≠ total AC %d", sum, threeAC)
	}
}
