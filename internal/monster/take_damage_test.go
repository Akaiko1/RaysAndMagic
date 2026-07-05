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

	// No pierce: 100 fire vs 50% resist -> 50 damage.
	base := mk().TakeDamageResist(100, DamageFire, 0, 0, 0)
	if base != 50 {
		t.Fatalf("no-pierce fire damage = %d, want 50", base)
	}
	// 50% pierce: resistance halved to 25% -> 75 damage.
	pierced := mk().TakeDamageResist(100, DamageFire, 50, 0, 0)
	if pierced != 75 {
		t.Errorf("50%%-pierce fire damage = %d, want 75", pierced)
	}
	// Pierce on a type with no resistance is a no-op.
	if d := mk().TakeDamageResist(100, DamagePhysical, 50, 0, 0); d != 100 {
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

	base := mk().TakeDamageResist(100, DamageFire, 0, 0, 0)
	if base != 150 {
		t.Fatalf("no-pierce vulnerable fire damage = %d, want 150", base)
	}
	// Pierce must not shrink the vulnerability bonus.
	pierced := mk().TakeDamageResist(100, DamageFire, 50, 0, 0)
	if pierced != 150 {
		t.Errorf("50%%-pierce vulnerable fire damage = %d, want 150 (pierce should not touch vulnerability)", pierced)
	}
}

// A sealed (dormant) boss absorbs all damage and does not aggro; clearing the
// flag (its quest unseals it) restores normal damage + engagement.
func TestDormantBossInvulnerable(t *testing.T) {
	m := &Monster3D{HitPoints: 1600, MaxHitPoints: 1600, BossDormant: true}
	if got := m.TakeDamage(500, DamagePhysical, 99, 99); got != 0 {
		t.Errorf("sealed boss took %d damage, want 0", got)
	}
	if m.HitPoints != 1600 {
		t.Errorf("sealed boss HP = %d, want 1600 (untouched)", m.HitPoints)
	}
	if m.WasAttacked || m.IsEngagingPlayer {
		t.Error("sealed boss must not aggro when struck")
	}

	m.BossDormant = false
	if got := m.TakeDamage(500, DamagePhysical, 99, 99); got != 500 {
		t.Errorf("unsealed boss took %d damage, want 500", got)
	}
	if !m.IsEngagingPlayer {
		t.Error("unsealed boss must aggro when struck")
	}
}

// RT/TB parity: an enraged monster whose RT cooldown drops (enrage_cooldown_mult)
// gets proportionally more turn-based swings. The samurai's 0.6 mult is < 1 -> 2
// swings while enraged (HP <= enrage threshold), 1 otherwise.
func TestEnrageScalesTurnBasedAttacks(t *testing.T) {
	// Threshold buckets the multiplier maps to (mirrors the in-game rule).
	for _, c := range []struct {
		mult float64
		want int
	}{{0, 1}, {1.0, 1}, {0.9, 2}, {0.6, 2}, {0.5, 2}, {0.49, 4}, {0.25, 4}, {0.2, 8}} {
		if got := tbAttacksForCooldownMult(c.mult); got != c.want {
			t.Errorf("tbAttacksForCooldownMult(%.2f) = %d, want %d", c.mult, got, c.want)
		}
	}

	fast := &Monster3D{AttackCooldownMultiplier: 0.6}
	if got := fast.GetTurnBasedAttackCount(); got != 2 {
		t.Errorf("cooldown-only TB attacks = %d, want 2", got)
	}

	m := &Monster3D{
		HitPoints: 1600, MaxHitPoints: 1600,
		AttacksPerRound: 1, EnrageAtHP: 480, EnrageCooldownMult: 0.6,
	}
	if got := m.GetTurnBasedAttackCount(); got != 1 {
		t.Errorf("healthy boss TB attacks = %d, want 1", got)
	}
	m.HitPoints = 400 // <= 480 -> enraged
	if got := m.GetTurnBasedAttackCount(); got != 2 {
		t.Errorf("enraged boss TB attacks = %d, want 2 (cooldown 0.6 < 1)", got)
	}
}
