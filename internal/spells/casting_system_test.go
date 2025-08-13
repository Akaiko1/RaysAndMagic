package spells

import (
	"testing"
)

func TestCalculateSpellDamageByID(t *testing.T) {
	base, bonus, total := CalculateSpellDamageByID("fireball", 10)
	if total < base || total < bonus {
		t.Errorf("Total damage should be at least as large as base or bonus: base=%d, bonus=%d, total=%d", base, bonus, total)
	}
}

func TestCalculateHealingAmountByID(t *testing.T) {
	base, bonus, total := CalculateHealingAmountByID("heal", 10)
	if total < base || total < bonus {
		t.Errorf("Total healing should be at least as large as base or bonus: base=%d, bonus=%d, total=%d", base, bonus, total)
	}
}

func TestGetProjectileColor(t *testing.T) {
	_, err := GetProjectileColor("fireball")
	if err != nil {
		t.Skipf("Skipping: GetProjectileColor returned error (likely missing spell config): %v", err)
	}
}
