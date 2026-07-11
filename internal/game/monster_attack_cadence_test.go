package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// Regression: a kiting player (stepping in and out of melee range) used to reset
// a monster's attack cadence, because the only cooldown was time spent in the
// attacking state - and leaving that state on a range-exit discarded it. Now a
// persistent AttackCDFrames ticks in real frames regardless of AI state, so the
// monster can't attack faster than its configured cooldown no matter how the
// state churns. This drives the worst case: the monster looks freshly re-entered
// into the attacking state (StateTimer==1) in range EVERY frame.
func TestRTMonsterAttackCadence_KitingCannotBypassCooldown(t *testing.T) {
	g, thief := newThiefTestGame(t)
	thief.MaxHitPoints, thief.HitPoints = 1_000_000, 1_000_000
	thief.Luck = -thief.BuffBonuses.Luck // zero effective Luck -> no party-side dodge skews the count

	mob := monsterPkg.NewMonster3DFromConfig(g.camera.X+1, g.camera.Y, "minotaur", g.config)
	if mob == nil {
		t.Fatal("failed to load minotaur from monsters.yaml")
	}
	g.world.Monsters = []*monsterPkg.Monster3D{mob}

	cd := mob.AttackCooldownFrames()
	if cd < 2 {
		t.Fatalf("attack cooldown too small to exercise: %d frames", cd)
	}

	frames := cd * 5
	attacks := 0
	for i := 0; i < frames; i++ {
		mob.State = monsterPkg.StateAttacking
		mob.StateTimer = 1 // the exploit: every frame looks like a fresh attack entry
		before := mob.AttackCDFrames
		g.combat.HandleMonsterInteractions()
		// An attack RESOLVED iff the cooldown was re-armed (jumped up). Counting
		// by the victim's HP was flaky: the minotaur's random damage roll can be
		// mitigated to zero, making a real attack invisible to an HP check.
		if mob.AttackCDFrames > before {
			attacks++
		}
	}

	if attacks == 0 {
		t.Fatal("monster never attacked")
	}
	// Capped at ~one attack per cooldown, not one per frame (the old bug -> ~frames attacks).
	if maxAttacks := frames/cd + 2; attacks > maxAttacks {
		t.Fatalf("monster attacked %d times in %d frames (cd=%d) - kiting bypassed the cooldown; want <= %d",
			attacks, frames, cd, maxAttacks)
	}
}
