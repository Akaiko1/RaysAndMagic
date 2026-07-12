package game

import (
	"testing"

	"ugataima/internal/monster"
)

func TestTurnBasedHitEngagesPack(t *testing.T) {
	cfg := loadTestConfig(t)

	worldTest := newTestWorldSized(cfg, 40, 40)
	game := newTestGame(cfg, worldTest)
	game.turnBasedMode = true
	game.combat = NewCombatSystem(game)

	tileSize := float64(cfg.GetTileSize())

	game.camera.X = tileSize
	game.camera.Y = tileSize
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	hit := monster.NewMonster3DFromConfig(12*tileSize, 13*tileSize, "goblin", cfg)
	nearSame := monster.NewMonster3DFromConfig(18*tileSize, 13*tileSize, "goblin", cfg) // 6 tiles away
	farSame := monster.NewMonster3DFromConfig(22*tileSize, 13*tileSize, "goblin", cfg)  // 10 tiles away
	nearOther := monster.NewMonster3DFromConfig(12*tileSize, 19*tileSize, "orc", cfg)   // 6 tiles away, different type

	worldTest.Monsters = []*monster.Monster3D{hit, nearSame, farSame, nearOther}
	worldTest.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.combat.ApplyDamageToMonster(hit, 1, "Test", false)

	if !hit.IsEngagingPlayer {
		t.Fatalf("expected hit monster to engage in turn-based mode")
	}
	if !nearSame.IsEngagingPlayer {
		t.Fatalf("expected nearby same-type monster to engage in turn-based mode")
	}
	if farSame.IsEngagingPlayer {
		t.Fatalf("expected distant same-type monster to remain disengaged")
	}
	if nearOther.IsEngagingPlayer {
		t.Fatalf("expected nearby different-type monster to remain disengaged")
	}

	visionRange := tileSize * 6.0
	if Distance(game.camera.X, game.camera.Y, hit.X, hit.Y) <= visionRange {
		t.Fatalf("expected hit monster to be outside vision range for test setup")
	}

	game.currentTurn = 1
	game.monsterTurnResolved = false
	game.frameCount = 42

	gl := &GameLoop{game: game}

	oldHitX, oldHitY := hit.X, hit.Y
	oldFarX, oldFarY := farSame.X, farSame.Y

	gl.updateMonstersTurnBased()

	if hit.X == oldHitX && hit.Y == oldHitY {
		t.Fatalf("expected engaged monster outside vision range to act in turn-based mode")
	}
	if farSame.X != oldFarX || farSame.Y != oldFarY {
		t.Fatalf("expected disengaged monster outside vision range to remain idle")
	}
}
