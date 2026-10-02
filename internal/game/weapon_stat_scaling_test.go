package game

import (
	"testing"

	"ugataima/internal/config"
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

	authored := func(key string) int {
		t.Helper()
		def, ok := config.GetWeaponDefinition(key)
		if !ok || def == nil {
			t.Fatalf("%s missing from weapons.yaml", key)
		}
		return def.Damage
	}

	// agility_katar: primary Speed, secondary Accuracy.
	katar := items.CreateWeaponFromYAML("agility_katar")
	want := authored("agility_katar")
	base, statBonus, total := cs.CalculateWeaponDamage(katar, char)
	wantBonus := char.Speed/WeaponPrimaryStatDivisor + char.Accuracy/WeaponSecondaryStatDivisor
	if base != want || statBonus != wantBonus || total != want+wantBonus {
		t.Errorf("katar: base=%d bonus=%d total=%d, want %d/%d/%d (Speed must drive the primary)",
			base, statBonus, total, want, wantBonus, want+wantBonus)
	}

	// chitin_spear: primary Might, secondary Speed.
	spear := items.CreateWeaponFromYAML("chitin_spear")
	want = authored("chitin_spear")
	base, statBonus, total = cs.CalculateWeaponDamage(spear, char)
	wantBonus = char.Might/WeaponPrimaryStatDivisor + char.Speed/WeaponSecondaryStatDivisor
	if base != want || statBonus != wantBonus || total != want+wantBonus {
		t.Errorf("spear: base=%d bonus=%d total=%d, want %d/%d/%d (secondary Speed must count)",
			base, statBonus, total, want, wantBonus, want+wantBonus)
	}
}
