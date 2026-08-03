package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestTurnBasedRangedRequiresLineOfSight guards the TB anti-kite fix: a ranged
// monster that is row/column-aligned and in range but separated from the party
// by a wall (no line of sight) must REPOSITION instead of plinking the wall -
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

func TestTurnBasedRangedUsesMeleeFromDiagonalContact(t *testing.T) {
	game, gl, tile := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, tile)

	bandit := spawnMonsterAtTile(game, "bandit", 11, 11, tile)
	bandit.DamageMin, bandit.DamageMax = 20, 20
	bandit.MeleeDamageType = monster.DamageDark.String()
	beforeHP := partyHPSum(game)

	runOneMonsterTurn(game, gl)

	if len(game.arrows) != 0 {
		t.Fatalf("diagonally adjacent ranged monster fired %d projectiles, want melee", len(game.arrows))
	}
	if got := partyHPSum(game); got >= beforeHP {
		t.Fatalf("diagonally adjacent ranged monster did not land melee: HP %d -> %d", beforeHP, got)
	}
}

// A ranged CHAMPION owns its main-hand delivery: adjacency must NOT let it
// enter the melee-adjacent branch and fire from a diagonal, bypassing the
// row/column lane rule that governs every other ranged shot in TB.
func TestTurnBasedRangedChampionKeepsLaneRuleOnDiagonal(t *testing.T) {
	game, gl, tile := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, tile)

	champ := spawnMonsterAtTile(game, "bandit", 11, 11, tile)
	champ.ChampionKey = "test_champion" // IsChampion: ranged flag owns delivery
	beforeHP := partyHPSum(game)

	runOneMonsterTurn(game, gl)

	if len(game.arrows) != 0 || len(game.magicProjectiles) != 0 {
		t.Fatalf("diagonal ranged champion fired (%d arrows, %d bolts) - lane rule bypassed",
			len(game.arrows), len(game.magicProjectiles))
	}
	if got := partyHPSum(game); got != beforeHP {
		t.Fatalf("diagonal ranged champion dealt damage: HP %d -> %d", beforeHP, got)
	}
}
