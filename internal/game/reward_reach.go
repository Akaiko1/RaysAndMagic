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
	if terrain.passage && terrain.IsTileBlockingForTerrainPassage(x, y) {
		return true
	}
	if tm := world.GlobalTileManager; tm != nil {
		return tm.BlocksPickup(terrain.Tiles[y][x], terrain.passage)
	}
	// Content-free worlds retain the legacy water exception.
	tile := terrain.Tiles[y][x]
	return tile != world.TileWater && tile != world.TileDeepWater && terrain.IsTileBlockingTerrainAt(x, y)
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
