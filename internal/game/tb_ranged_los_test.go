package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestTurnBasedRangedRequiresLineOfSight guards the TB anti-kite fix: a ranged
// monster that is row/column-aligned and in range but separated from the party
// by a wall (no line of sight) must REPOSITION instead of plinking the wall —
// otherwise the party hides round a corner and regens mana for free. With a
// clear shot it still fires.
func TestTurnBasedRangedRequiresLineOfSight(t *testing.T) {
	run := func(t *testing.T, wall bool) (moved bool, arrows int) {
		t.Helper()
		cfg := loadTestConfig(t)
		w := newTestWorldSized(cfg, 20, 20)
		if wall {
			// Opaque+blocking wall on row 5 between player (tile 3) and mob (tile 8).
			w.Tiles[5][6] = world.TileWall
		}
		game := newTestGame(cfg, w)
		game.turnBasedMode = true
		game.combat = NewCombatSystem(game)

		tile := float64(cfg.GetTileSize())
		game.camera.X = 3.5 * tile // tile (3,5)
		game.camera.Y = 5.5 * tile
		game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

		m := monster.NewMonster3DFromConfig(8.5*tile, 5.5*tile, "bandit", cfg) // throwing_knife ranged
		if m == nil {
			t.Fatal("bandit config missing")
		}
		m.RangedAttackRange = 6 * tile // 6-tile range > the 5-tile gap
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		w.Monsters = []*monster.Monster3D{m}
		w.RegisterMonstersWithCollisionSystem(game.collisionSystem)

		game.currentTurn = 1
		game.monsterTurnResolved = false
		game.frameCount = 1

		gl := &GameLoop{game: game}
		ox, oy := m.X, m.Y
		gl.updateMonstersTurnBased()
		return m.X != ox || m.Y != oy, len(game.arrows)
	}

	t.Run("behind wall: repositions, no shot", func(t *testing.T) {
		moved, arrows := run(t, true)
		if arrows != 0 {
			t.Errorf("expected no shot through a wall, got %d arrows", arrows)
		}
		if !moved {
			t.Errorf("expected the ranged monster to move (reposition) when LOS is blocked")
		}
	})

	t.Run("clear LOS: fires", func(t *testing.T) {
		_, arrows := run(t, false)
		if arrows != 1 {
			t.Errorf("expected 1 shot with a clear line of sight, got %d arrows", arrows)
		}
	})
}
