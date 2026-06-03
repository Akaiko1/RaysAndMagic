package monster

import "testing"

// TestTakeDamageResist_Pierce: resistance piercing reduces the effective
// resistance before damage reduction. TakeDamage (pierce 0) is the baseline.
func TestTakeDamageResist_Pierce(t *testing.T) {
	mk := func() *Monster3D {
		return &Monster3D{
			HitPoints: 1000, MaxHitPoints: 1000,
			Resistances: map[DamageType]int{DamageFire: 50},
		}
	}

	// No pierce: 100 fire vs 50% resist → 50 damage.
	base := mk().TakeDamageResist(100, DamageFire, 0, 0, 0)
	if base != 50 {
		t.Fatalf("no-pierce fire damage = %d, want 50", base)
	}
	// 50% pierce: resistance halved to 25% → 75 damage.
	pierced := mk().TakeDamageResist(100, DamageFire, 50, 0, 0)
	if pierced != 75 {
		t.Errorf("50%%-pierce fire damage = %d, want 75", pierced)
	}
	// Pierce on a type with no resistance is a no-op.
	if d := mk().TakeDamageResist(100, DamagePhysical, 50, 0, 0); d != 100 {
		t.Errorf("pierce vs no-resistance = %d, want 100", d)
	}
}
