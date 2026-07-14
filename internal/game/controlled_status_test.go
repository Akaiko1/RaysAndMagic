package game

// Charm (Pacified) and Bind (Bound) are the two monster CONTROL statuses; their
// RT expiry lives in updateControlledMonsters. Completes the per-status
// behavior coverage alongside internal/monster (poison/root/stun) and
// internal/character (poison/burn/stun/flag conditions).

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestControlledMonsterStatusExpiry(t *testing.T) {
	bound := &monsterPkg.Monster3D{Name: "Skeleton", HitPoints: 10, Bound: true, BoundFramesRemaining: 2}
	charmed := &monsterPkg.Monster3D{Name: "Wolf", HitPoints: 10, Pacified: true, PacifiedFramesRemaining: 2}
	gl := &GameLoop{game: &MMGame{world: &world.World3D{Monsters: []*monsterPkg.Monster3D{bound, charmed}}}}

	gl.updateControlledMonsters()
	if !bound.Bound || !charmed.Pacified {
		t.Fatal("statuses must hold until their timers run out")
	}

	gl.updateControlledMonsters()
	if bound.Bound || bound.BoundFramesRemaining != 0 {
		t.Fatalf("bind must break on expiry: bound=%v left=%d", bound.Bound, bound.BoundFramesRemaining)
	}
	if charmed.Pacified {
		t.Fatal("charm must wear off on expiry")
	}
	if !charmed.WasAttacked {
		t.Fatal("a worn-off charm must re-aggro the monster (WasAttacked)")
	}
	if countCombatLog(gl.game, "breaks free") != 1 || countCombatLog(gl.game, "wears off") != 1 {
		t.Fatal("both expiries must announce themselves exactly once")
	}

	// TB semantics: a zero-frame control lasts the encounter - no decay ticks.
	forever := &monsterPkg.Monster3D{Name: "Lich", HitPoints: 10, Bound: true, BoundFramesRemaining: 0}
	gl.game.world.Monsters = []*monsterPkg.Monster3D{forever}
	gl.updateControlledMonsters()
	if !forever.Bound {
		t.Fatal("a control with no timer must persist (TB: lasts the encounter)")
	}
}
