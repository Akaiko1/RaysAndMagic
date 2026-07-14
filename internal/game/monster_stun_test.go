package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// TestStunnedMonster_DoesNotAttackRealTime: a stunned monster must take no action
// in the real-time interaction pass. Regression for the bug where Update() froze
// the stun (and StateTimer) but HandleMonsterInteractions ignored the stun, so a
// monster frozen at StateTimer==1 in StateAttacking struck every single frame.
func TestStunnedMonster_DoesNotAttackRealTime(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.camera.X, cs.game.camera.Y = 0, 0
	for _, mm := range cs.game.party.Members {
		mm.Luck = 0 // deterministic: no Perfect Dodge so an attack always lands
	}

	mk := func(stun int) *monsterPkg.Monster3D {
		return &monsterPkg.Monster3D{
			ID: "m1", Name: "Test", X: 10, Y: 0,
			State: monsterPkg.StateAttacking, StateTimer: 1,
			StunFramesRemaining: stun,
			AttackRadius:        64,
			DamageMin:           20, DamageMax: 20,
			HitPoints: 30, MaxHitPoints: 30,
		}
	}
	partyHP := func() int {
		s := 0
		for _, mm := range cs.game.party.Members {
			s += mm.HitPoints
		}
		return s
	}

	// Stunned -> no attack, party HP unchanged.
	cs.game.world.Monsters = []*monsterPkg.Monster3D{mk(10)}
	before := partyHP()
	cs.HandleMonsterInteractions()
	if got := partyHP(); got != before {
		t.Errorf("stunned monster dealt damage: party HP %d -> %d", before, got)
	}

	// Same monster, not stunned -> it attacks, proving the assertion is meaningful.
	cs.game.world.Monsters = []*monsterPkg.Monster3D{mk(0)}
	before = partyHP()
	cs.HandleMonsterInteractions()
	if got := partyHP(); got >= before {
		t.Errorf("un-stunned monster should deal damage: party HP %d -> %d", before, got)
	}
}
