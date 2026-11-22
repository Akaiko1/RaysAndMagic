package game

import (
	"ugataima/internal/collision"
	"ugataima/internal/world"
)

type rewardReachTerrain struct {
	*world.World3D
	passage bool
}

func (terrain rewardReachTerrain) IsTileBlocking(x, y int) bool {
	if x < 0 || y < 0 || y >= len(terrain.Tiles) || x >= len(terrain.Tiles[y]) {
		return true
	}
	// Terrain passage can cross an open pit, but must not let a hand reach through a
	// tree, wall or rock. Check the occupied cell too, unlike a sight ray.
	if terrain.IsTileOpaque(x, y) {
		return true
	}
	tile := terrain.Tiles[y][x]
	tm := world.GlobalTileManager
	if tm == nil {
		// Content-free worlds retain the legacy water exception.
		return tile != world.TileWater && tile != world.TileDeepWater && terrain.IsTileBlockingTerrainAt(x, y)
	}
	// Passage only adds reach: ground reachable on foot stays reachable on the
	// map border, which blocks the traversal but not a hand.
	if !tm.BlocksPickup(tile, false) {
		return false
	}
	return !terrain.passage || terrain.IsTileBlockingForTerrainPassage(x, y) || tm.BlocksPickup(tile, true)
}

// canReachWorldReward is the terrain gate for physical rewards: lecterns,
// crates and dropped containers. Callers keep their existing range/combat
// rules. Recheck on consumption so stale focus cannot outlive a travel buff.
func (g *MMGame) canReachWorldReward(x, y float64) bool {
	w := g.GetCurrentWorld()
	if w == nil || g.camera == nil || g.config == nil {
		return false
	}
	if g.collisionSystem != nil && !g.collisionSystem.CheckLineOfSight(g.camera.X, g.camera.Y, x, y) {
		return false
	}
	return collision.CheckMovementLine(rewardReachTerrain{World3D: w, passage: g.partyHasTerrainPassage()},
		g.config.GetTileSize(), g.camera.X, g.camera.Y, x, y)
}
