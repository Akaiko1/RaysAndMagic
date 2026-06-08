package game

import (
	"math"
	"testing"
)

// TestTankTarget_FrontSlotThenFallback: the tank is party slot 0 while alive,
// else the first living member.
func TestTankTarget_FrontSlotThenFallback(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members
	if len(m) < 2 {
		t.Skip("need >=2 members")
	}
	for _, x := range m {
		x.HitPoints = x.MaxHitPoints
	}
	if cs.tankTarget() != m[0] {
		t.Fatalf("tank should be slot 0 when alive")
	}
	m[0].HitPoints = 0 // KO the front slot
	if cs.tankTarget() != m[1] {
		t.Fatalf("tank should fall back to first living member (slot 1) when slot 0 is down")
	}
}

// TestMeleeTargetsRandomLiving: melee picks varied living members, not always
// the same slot (both modes use randomLivingMember).
func TestMeleeTargetsRandomLiving(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	m := cs.game.party.Members
	n := len(m)
	if n < 3 {
		t.Skip("need >=3 members")
	}
	for _, x := range m {
		x.HitPoints = x.MaxHitPoints
	}
	hits := make([]int, n)
	for i := 0; i < 3000; i++ {
		tgt := cs.randomLivingMember()
		for j := range m {
			if m[j] == tgt {
				hits[j]++
			}
		}
	}
	t.Logf("melee target distribution: %v", hits)
	for j := 0; j < n; j++ {
		if hits[j] == 0 {
			t.Errorf("member %d never targeted by melee — not random", j)
		}
	}
}

// TestRangedTB_SeventyThirtyTankSplit: turn-based ranged hits the tank ~70% and
// a non-tank ~30% (RangedOffTankChance).
func TestRangedTB_SeventyThirtyTankSplit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.turnBasedMode = true
	m := cs.game.party.Members
	if len(m) < 2 {
		t.Skip("need >=2 members")
	}
	for _, x := range m {
		x.HitPoints = x.MaxHitPoints
	}
	const trials = 6000
	nonTank := 0
	for i := 0; i < trials; i++ {
		if cs.rangedTBTarget() != m[0] {
			nonTank++
		}
	}
	frac := float64(nonTank) / trials
	t.Logf("TB ranged non-tank fraction: %.3f (target %.2f)", frac, RangedOffTankChance)
	if math.Abs(frac-RangedOffTankChance) > 0.05 {
		t.Errorf("TB ranged off-tank fraction %.3f not within 0.05 of %.2f", frac, RangedOffTankChance)
	}
}

// TestRangedRT_AlwaysTank: real-time ranged single-target always lands on the
// tank (slot 0).
func TestRangedRT_AlwaysTank(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.turnBasedMode = false
	m := cs.game.party.Members
	if len(m) < 2 {
		t.Skip("need >=2 members")
	}
	for _, x := range m {
		x.Luck = 0 // no dodge
	}
	for i := 0; i < 40; i++ {
		for _, x := range m { // reset so the tank never dies → never falls back
			x.HitPoints = x.MaxHitPoints
		}
		cs.applyMonsterProjectileDamage("Test", 999, "true", 0) // true dmg bypasses armor
		if m[0].HitPoints >= m[0].MaxHitPoints {
			t.Fatalf("RT ranged did not hit the tank (slot 0) on iter %d", i)
		}
		for j := 1; j < len(m); j++ {
			if m[j].HitPoints < m[j].MaxHitPoints {
				t.Fatalf("RT ranged hit non-tank slot %d (should only ever hit the tank)", j)
			}
		}
	}
}
