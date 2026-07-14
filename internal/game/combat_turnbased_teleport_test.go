package game

import (
	"testing"

	"ugataima/internal/monster"
)

// P1a regression: the turn-based "stuck -> teleport toward player" fallback
// must never land a party-targeting monster on the player's tile. The diagonal
// offset set includes the player tile when the mob is diagonal-adjacent.
func TestTBTeleportFallbackSkipsPlayerTile(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 12, 12)
	game := newTestGame(cfg, w)
	game.combat = NewCombatSystem(game)
	tile := float64(cfg.GetTileSize())

	game.camera.X, game.camera.Y = TileCenterFromTile(6, 6, tile)
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)

	mx, my := TileCenterFromTile(5, 5, tile) // diagonal-adjacent: (+1,+1) == player tile (6,6)
	m := monster.NewMonster3DFromConfig(mx, my, "goblin", cfg)
	m.IsEngagingPlayer, m.WasAttacked = true, true
	game.registerSpawnedMonster(m)

	gl := &GameLoop{game: game}
	diag := [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} // (1,1) -> the player tile, and the closest
	const sentinel = 1e18
	bx, by, bd := gl.pickBestTeleportOffset(m, tile, game.camera.X, game.camera.Y, diag, sentinel)

	if bd >= sentinel {
		t.Fatal("expected a valid teleport tile in an open world")
	}
	ptx, pty := game.GetPlayerTilePosition()
	if int(bx/tile) == ptx && int(by/tile) == pty {
		t.Fatalf("teleport picked the player tile (%d,%d) - it must be skipped", ptx, pty)
	}
}
