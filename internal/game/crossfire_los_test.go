package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Nothing aggros through a wall - summons included. Party sight has always been
// gated on line of sight (CanStartPlayerEngagement); crossfire target selection
// was pure distance, so a summon pulled mobs through walls that the party itself
// could never have pulled. Both directions are gated now, and both stay sticky:
// sight is required to START a fight, never to continue one.
func TestCrossfireFoeNeedsLineOfSight(t *testing.T) {
	// mob at (5,5), summon at (8,5), optional wall between them at (6,5). The
	// party stands far away so the summon is the closer candidate either way.
	setup := func(t *testing.T, blocked bool) (*MMGame, *monster.Monster3D, *monster.Monster3D) {
		t.Helper()
		cfg := loadTestConfig(t)
		game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
		game.combat = NewCombatSystem(game)
		ts := float64(cfg.GetTileSize())

		mx, my := TileCenterFromTile(5, 5, ts)
		sx, sy := TileCenterFromTile(8, 5, ts)
		mob := monster.NewMonster3DFromConfig(mx, my, "goblin", game.config)
		summon := monster.NewMonster3DFromConfig(sx, sy, "revenant", game.config)
		if mob == nil || summon == nil {
			t.Fatal("goblin and revenant must load from monsters.yaml")
		}
		markCardAlly(summon)
		if blocked {
			game.world.Tiles[5][6] = world.TileWall
		}
		game.world.Monsters = []*monster.Monster3D{mob, summon}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		placePlayerAtTile(game, 1, 18, ts)

		if got := game.collisionSystem.CheckLineOfSight(mob.X, mob.Y, summon.X, summon.Y); got == blocked {
			t.Fatalf("setup: line of sight = %v with blocked = %v", got, blocked)
		}
		return game, mob, summon
	}

	t.Run("wall blocks acquisition both ways", func(t *testing.T) {
		game, mob, summon := setup(t, true)
		game.refreshMonsterAIState()
		if mob.AIFoe != nil {
			t.Errorf("mob acquired a summon through a wall: %v", mob.AIFoe.Name)
		}
		if summon.AIFoe != nil {
			t.Errorf("summon acquired a mob through a wall: %v", summon.AIFoe.Name)
		}
	})

	t.Run("clear sight acquires", func(t *testing.T) {
		game, mob, summon := setup(t, false)
		game.refreshMonsterAIState()
		if mob.AIFoe != summon {
			t.Errorf("mob foe = %v, want the summon in plain sight", mob.AIFoe)
		}
		if summon.AIFoe != mob {
			t.Errorf("summon foe = %v, want the mob in plain sight", summon.AIFoe)
		}
	})

	t.Run("a started fight survives losing sight", func(t *testing.T) {
		game, mob, summon := setup(t, false)
		game.refreshMonsterAIState()
		if mob.AIFoe != summon {
			t.Fatalf("setup: mob did not engage the summon")
		}
		game.world.Tiles[5][6] = world.TileWall // quarry rounds a corner
		game.refreshMonsterAIState()
		if mob.AIFoe != summon {
			t.Errorf("mob dropped its fight at the first wall: foe = %v", mob.AIFoe)
		}
		if summon.AIFoe != mob {
			t.Errorf("summon dropped its fight at the first wall: foe = %v", summon.AIFoe)
		}
	})
}
