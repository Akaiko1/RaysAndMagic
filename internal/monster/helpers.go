package monster

import "math"

// Tile size constant for grid-based movement.
const tileSize = 64.0

// Helper: tile coordinate conversions (search: tile-center).
func worldToTile(pos float64) int {
	return int(math.Floor(pos / tileSize))
}

func tileToWorldCenter(tileX, tileY int) (float64, float64) {
	return float64(tileX)*tileSize + tileSize/2, float64(tileY)*tileSize + tileSize/2
}

func worldToTileCenter(x, y float64) (float64, float64) {
	return tileToWorldCenter(worldToTile(x), worldToTile(y))
}
