package game

import (
	"testing"

	"ugataima/internal/items"
)

// Speed-scaling weapons must actually scale with Speed: the old hand-rolled
// stat switch silently mapped a Speed primary to Might and dropped a Speed
// secondary entirely, while the tooltip honestly said "Scales with Speed".
func TestWeaponDamage_SpeedScaling(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	char.Equipment = map[items.EquipSlot]items.Item{} // no gear bonuses
	char.Might = 3
	char.Accuracy = 20
	char.Speed = 30

	// agility_katar: damage 7, primary Speed, secondary Accuracy.
	katar := items.CreateWeaponFromYAML("agility_katar")
	base, statBonus, total := cs.CalculateWeaponDamage(katar, char)
	wantBonus := char.Speed/WeaponPrimaryStatDivisor + char.Accuracy/WeaponSecondaryStatDivisor
	if base != 7 || statBonus != wantBonus || total != 7+wantBonus {
		t.Errorf("katar: base=%d bonus=%d total=%d, want 7/%d/%d (Speed must drive the primary)",
			base, statBonus, total, wantBonus, 7+wantBonus)
	}

	// chitin_spear: damage 15, primary Might, secondary Speed.
	spear := items.CreateWeaponFromYAML("chitin_spear")
	base, statBonus, total = cs.CalculateWeaponDamage(spear, char)
	wantBonus = char.Might/WeaponPrimaryStatDivisor + char.Speed/WeaponSecondaryStatDivisor
	if base != 15 || statBonus != wantBonus || total != 15+wantBonus {
		t.Errorf("spear: base=%d bonus=%d total=%d, want 15/%d/%d (secondary Speed must count)",
			base, statBonus, total, wantBonus, 15+wantBonus)
	}
}
