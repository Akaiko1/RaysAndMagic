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
	if bound.Bound || bound.BoundFramesRemaining != 0 || !bound.WasAttacked || !bound.IsEngagingPlayer {
		t.Fatalf("bind expiry must restore immediate hostile behavior: bound=%v left=%d hit=%v engaging=%v",
			bound.Bound, bound.BoundFramesRemaining, bound.WasAttacked, bound.IsEngagingPlayer)
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

func TestControlEffectsRemainMutuallyExclusive(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)

	cardAlly := monsterPkg.NewMonster3DFromConfig(0, 0, "masked_huntress", game.config)
	markCardAlly(cardAlly)
	game.combat.applyPacify(cardAlly, 60, "Charm")
	if !cardAlly.Bound || cardAlly.Pacified || cardAlly.PacifiedFramesRemaining != 0 {
		t.Fatalf("Charm must not replace card-ally Bind: bound=%v pacified=%v frames=%d",
			cardAlly.Bound, cardAlly.Pacified, cardAlly.PacifiedFramesRemaining)
	}

	undead := monsterPkg.NewMonster3DFromConfig(0, 0, "skeleton", game.config)
	undead.Pacified = true // malformed old/runtime state: Bind is authoritative
	undead.PacifiedFramesRemaining = 42
	game.combat.applyBindUndead(undead, 60, "Bind Undead")
	if !undead.Bound || undead.Pacified || undead.PacifiedFramesRemaining != 0 {
		t.Fatalf("Bind must normalize conflicting Charm: bound=%v pacified=%v frames=%d",
			undead.Bound, undead.Pacified, undead.PacifiedFramesRemaining)
	}

	// The behavior policy is defensive too: a malformed old/runtime state must
	// follow Bind's party-side target rule rather than silently becoming a
	// pacified follower because one consumer checked Pacified first.
	tile := float64(game.config.GetTileSize())
	enemy := monsterPkg.NewMonster3DFromConfig(4*tile, 0, "goblin", game.config)
	undead.X, undead.Y = 0, 0
	undead.Pacified = true
	undead.PacifiedFramesRemaining = 42
	game.world.Monsters = []*monsterPkg.Monster3D{undead, enemy}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshMonsterAIState()
	if undead.AIFoe != enemy || undead.AITargetX != enemy.X || undead.AITargetY != enemy.Y {
		t.Fatalf("malformed bound+charm must still seek an enemy: foe=%v target=(%.0f,%.0f), want %s at (%.0f,%.0f)",
			undead.AIFoe, undead.AITargetX, undead.AITargetY, enemy.Name, enemy.X, enemy.Y)
	}
}
