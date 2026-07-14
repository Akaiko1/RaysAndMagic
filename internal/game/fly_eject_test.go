package game

import (
	"testing"

	"ugataima/internal/world"
)

// TestFlyEjectFromWall: when Fly lapses while the party hovers inside solid
// terrain (Fly let them pass through it), they must be surfaced to the nearest
// walkable tile - otherwise every move is wall-locked with no way out.
func TestFlyEjectFromWall(t *testing.T) {
	prevTM := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = prevTM })
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	game, _, ts := tbBehaviorGame(t, 10, 10)
	// Wall the party's tile; leave the rest empty (walkable).
	game.world.Tiles[5][5] = world.TileWall
	placePlayerAtTile(game, 5, 5, ts)

	// Fly ends this tick: tick the registry the way the game loop does.
	game.flyActive, game.flyDuration = true, 1
	for _, b := range game.timedBuffs() {
		tickBuff(b.active, b.duration, b.onExpire)
	}

	if game.flyActive {
		t.Fatal("Fly should have expired")
	}
	tx, ty := int(game.camera.X/ts), int(game.camera.Y/ts)
	if game.world.IsTileBlockingTerrainAt(tx, ty) {
		t.Fatalf("party still inside solid terrain at (%d,%d) after Fly lapsed", tx, ty)
	}
}
