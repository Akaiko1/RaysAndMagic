package monster

import (
	"testing"

	damagecalc "ugataima/internal/damage"
)

// TestTakeDamageResist_Pierce: resistance piercing reduces the effective
// resistance before damage reduction. Pierce 0 is the baseline.
func TestTakeDamageResist_Pierce(t *testing.T) {
	mk := func() *Monster3D {
		return &Monster3D{
			HitPoints: 1000, MaxHitPoints: 1000,
			Resistances: map[DamageType]int{DamageFire: 50},
		}
	}

	// No pierce: 100 fire vs 50% resist -> 50 damage.
	base := takeDamageParts(mk(), damagecalc.Parts{Normal: 100}, DamageFire, 0)
	if base != 50 {
		t.Fatalf("no-pierce fire damage = %d, want 50", base)
	}
	// 50% pierce: resistance halved to 25% -> 75 damage.
	pierced := takeDamageParts(mk(), damagecalc.Parts{Normal: 100}, DamageFire, 50)
	if pierced != 75 {
		t.Errorf("50%%-pierce fire damage = %d, want 75", pierced)
	}
	// Pierce on a type with no resistance is a no-op.
	if d := takeDamageParts(mk(), damagecalc.Parts{Normal: 100}, DamagePhysical, 50); d != 100 {
		t.Errorf("pierce vs no-resistance = %d, want 100", d)
	}
}

// GM resist-pierce must only eat positive resistance. A negative resistance is
// a vulnerability (bonus damage); piercing it would blunt that bonus, which is
// backwards for a mechanic meant to help the attacker.
func TestTakeDamageResist_PierceIgnoresVulnerability(t *testing.T) {
	mk := func() *Monster3D {
		return &Monster3D{
			HitPoints: 1000, MaxHitPoints: 1000,
			Resistances: map[DamageType]int{DamageFire: -50}, // 50% vulnerable
		}
	}

	base := takeDamageParts(mk(), damagecalc.Parts{Normal: 100}, DamageFire, 0)
	if base != 150 {
		t.Fatalf("no-pierce vulnerable fire damage = %d, want 150", base)
	}
	// Pierce must not shrink the vulnerability bonus.
	pierced := takeDamageParts(mk(), damagecalc.Parts{Normal: 100}, DamageFire, 50)
	if pierced != 150 {
		t.Errorf("50%%-pierce vulnerable fire damage = %d, want 150 (pierce should not touch vulnerability)", pierced)
	}
}

// A sealed (dormant) boss absorbs all damage and does not aggro; clearing the
// flag (its quest unseals it) restores normal damage + engagement.
func TestDormantBossInvulnerable(t *testing.T) {
	m := &Monster3D{HitPoints: 1600, MaxHitPoints: 1600, BossDormant: true}
	if got := takeDamageParts(m, damagecalc.Parts{Normal: 500}, DamagePhysical, 0); got != 0 {
		t.Errorf("sealed boss took %d damage, want 0", got)
	}
	if m.HitPoints != 1600 {
		t.Errorf("sealed boss HP = %d, want 1600 (untouched)", m.HitPoints)
	}
	if m.WasAttacked || m.IsEngagingPlayer {
		t.Error("sealed boss must not aggro when struck")
	}

	m.BossDormant = false
	if got := takeDamageParts(m, damagecalc.Parts{Normal: 500}, DamagePhysical, 0); got != 500 {
		t.Errorf("unsealed boss took %d damage, want 500", got)
	}
	if !m.IsEngagingPlayer {
		t.Error("unsealed boss must aggro when struck")
	}
}
