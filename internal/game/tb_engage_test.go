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

	placePlayerAtTile(game, 10, 13, tileSize)

	hit := monster.NewMonster3DFromConfig(20.5*tileSize, 13.5*tileSize, "goblin", cfg)
	nearSame := monster.NewMonster3DFromConfig(26.5*tileSize, 13.5*tileSize, "goblin", cfg)    // 6 from hit; 16 from party
	farSame := monster.NewMonster3DFromConfig(30.5*tileSize, 13.5*tileSize, "goblin", cfg)     // 10 from hit: outside pack
	nearOther := monster.NewMonster3DFromConfig(20.5*tileSize, 19.5*tileSize, "orc", cfg)      // 6 from hit; different type
	beyondLeash := monster.NewMonster3DFromConfig(27.5*tileSize, 13.5*tileSize, "goblin", cfg) // 7 from hit; 17 from party

	worldTest.Monsters = []*monster.Monster3D{hit, nearSame, farSame, nearOther, beyondLeash}
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
	if beyondLeash.IsEngagingPlayer || beyondLeash.WasAttacked {
		t.Fatal("pack response bypassed the maximum party pursuit range")
	}

	// Setup: both goblins sit past their own sight, so only the TB pack rule
	// (and the hit) can engage them.
	for _, m := range []*monster.Monster3D{hit, nearSame, farSame} {
		if Distance(game.camera.X, game.camera.Y, m.X, m.Y) <= m.AlertRadius {
			t.Fatalf("setup: %s must be outside its own alert radius", m.ID)
		}
	}
	pack := tileSize * TurnBasedPackAggroRadiusTiles
	if Distance(hit.X, hit.Y, nearSame.X, nearSame.Y) > pack || Distance(hit.X, hit.Y, farSame.X, farSame.Y) <= pack {
		t.Fatal("setup: near goblin must be inside and far goblin outside the TB pack radius")
	}

	game.currentTurn = 1
	game.monsterTurnResolved = false
	game.frameCount = 42

	gl := &GameLoop{game: game}

	oldHitX, oldHitY := hit.X, hit.Y
	oldNearX, oldNearY := nearSame.X, nearSame.Y
	oldFarX, oldFarY := farSame.X, farSame.Y

	gl.updateMonstersTurnBased()

	if hit.X == oldHitX && hit.Y == oldHitY {
		t.Fatalf("expected engaged monster outside its alert radius to act in turn-based mode")
	}
	if nearSame.X == oldNearX && nearSame.Y == oldNearY {
		t.Fatal("pack member at the pursuit boundary did not act")
	}
	if farSame.X != oldFarX || farSame.Y != oldFarY {
		t.Fatalf("expected disengaged monster outside its alert radius to remain idle")
	}
}
