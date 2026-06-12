package monster

import "math"

// Tile size in pixels for grid-based movement. Defaults to the engine standard
// and is overridden from config at startup via SetTileSize so every radius /
// pathfinding conversion in this package follows world.tile_size.
var tileSize = 64.0

// SetTileSize wires the configured tile size into this package (bridge
// pattern, called once from startup before any monster is created).
func SetTileSize(ts float64) {
	if ts > 0 {
		tileSize = ts
	}
}

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
