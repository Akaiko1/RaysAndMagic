package game

import (
	"math"
	"testing"

	damagecalc "ugataima/internal/damage"
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
			t.Errorf("member %d never targeted by melee - not random", j)
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
		if cs.rangedTarget() != m[0] {
			nonTank++
		}
	}
	frac := float64(nonTank) / trials
	t.Logf("TB ranged non-tank fraction: %.3f (target %.2f)", frac, RangedOffTankChance)
	if math.Abs(frac-RangedOffTankChance) > 0.05 {
		t.Errorf("TB ranged off-tank fraction %.3f not within 0.05 of %.2f", frac, RangedOffTankChance)
	}
}

// This integration table mutation-checks the production projectile wiring in
// both clocks. With an all-human party, each clock must retain the authored
// 70/30 tank/off-tank split.
func TestRangedProjectile_UsesSharedTankBiasedTargeting(t *testing.T) {
	for _, turnBased := range []bool{false, true} {
		name := "real_time"
		if turnBased {
			name = "turn_based"
		}
		t.Run(name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			cs.game.turnBasedMode = turnBased
			members := cs.game.party.Members
			if len(members) < 2 {
				t.Skip("need >=2 members")
			}
			const trials = 4000
			offTank := 0
			for i := 0; i < trials; i++ {
				for _, member := range members {
					member.Race = "human"
					member.HitPoints = member.MaxHitPoints
				}
				cs.applyMonsterProjectileDamage(nil, "Test", monsterCharacterHit{
					Parts: damagecalc.Parts{True: 1}, DamageType: "true", IgnoresDodge: true,
				})
				if members[0].HitPoints == members[0].MaxHitPoints {
					offTank++
				}
			}
			fraction := float64(offTank) / trials
			if math.Abs(fraction-RangedOffTankChance) > 0.04 {
				t.Fatalf("off-tank fraction = %.3f, want %.2f", fraction, RangedOffTankChance)
			}
		})
	}
}
