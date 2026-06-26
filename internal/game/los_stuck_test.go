package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Reproduces the save-1 report: in REAL TIME, a ranged monster (pixie) plinks a
// tree it has no line of sight through, and a melee monster (goblin) freezes
// behind a tree instead of routing around it. TB already handles both
// (TestTurnBasedRangedRequiresLineOfSight); these guard the RT paths.

// A real-time ranged monster must NOT fire when a wall/tree blocks the shot.
func TestRealTime_RangedDoesNotFireWithoutLOS(t *testing.T) {
	run := func(t *testing.T, wall bool) int {
		t.Helper()
		cfg := loadTestConfig(t)
		w := newTestWorldSized(cfg, 20, 20)
		if wall {
			w.Tiles[5][6] = world.TileWall // opaque blocker between player and mob
		}
		game := newTestGame(cfg, w)
		game.turnBasedMode = false
		game.combat = NewCombatSystem(game)

		tile := float64(cfg.GetTileSize())
		game.camera.X, game.camera.Y = 3.5*tile, 5.5*tile
		game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

		m := monster.NewMonster3DFromConfig(8.5*tile, 5.5*tile, "bandit", cfg) // throwing_knife = ranged, arrow-based
		if m == nil {
			t.Skip("bandit config missing")
		}
		m.RangedAttackRange = 6 * tile // range > the 5-tile gap
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		m.State = monster.StateAttacking
		w.Monsters = []*monster.Monster3D{m}
		w.RegisterMonstersWithCollisionSystem(game.collisionSystem)

		for i := 0; i < 20; i++ {
			m.Update(game.collisionSystem, game.camera.X, game.camera.Y)
			game.combat.HandleMonsterInteractions()
		}
		return len(game.arrows)
	}

	t.Run("behind wall: no shot", func(t *testing.T) {
		if a := run(t, true); a != 0 {
			t.Errorf("ranged monster fired %d shots with no line of sight (plinking the tree)", a)
		}
	})
	t.Run("clear LOS: fires", func(t *testing.T) {
		if a := run(t, false); a == 0 {
			t.Errorf("ranged monster should fire with a clear line of sight, fired none")
		}
	})
}

// A real-time melee monster diagonally adjacent THROUGH a sealed tree corner
// (no LOS) must route around the obstacle, not freeze in its attack stance.
func TestRealTime_MeleeRoutesAroundTreeNotFreeze(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 20, 20)
	// Seal the diagonal between player (10,10) and goblin (11,11): the goblin is
	// tile-adjacent (Chebyshev 1) but has no line of sight through the corner.
	w.Tiles[10][11] = world.TileWall
	w.Tiles[11][10] = world.TileWall

	game := newTestGame(cfg, w)
	game.turnBasedMode = false
	game.combat = NewCombatSystem(game)

	tile := float64(cfg.GetTileSize())
	game.camera.X, game.camera.Y = 10.5*tile, 10.5*tile
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	gob := monster.NewMonster3DFromConfig(11.5*tile, 11.5*tile, "goblin", cfg)
	if gob == nil {
		t.Skip("goblin config missing")
	}
	gob.IsEngagingPlayer = true
	gob.WasAttacked = true
	gob.State = monster.StatePursuing
	w.Monsters = []*monster.Monster3D{gob}
	w.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	startTx, startTy := int(gob.X/tile), int(gob.Y/tile)
	moved := false
	for i := 0; i < 240; i++ {
		gob.Update(game.collisionSystem, game.camera.X, game.camera.Y)
		game.combat.HandleMonsterInteractions()
		if tx, ty := int(gob.X/tile), int(gob.Y/tile); tx != startTx || ty != startTy {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatalf("goblin froze behind the tree (no LOS) instead of routing around it")
	}
}
