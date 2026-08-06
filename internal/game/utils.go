package game

import "math"

// AngleNorth is the camera angle that faces north (up). Cardinal convention:
// East=0, South=pi/2, West=pi, North=3pi/2 (see snapToCardinalDirection).
const AngleNorth = 3 * math.Pi / 2

// Distance calculates the Euclidean distance between two 2D points.
// This is a utility to avoid repeating math.Sqrt(dx*dx + dy*dy) throughout the codebase.
func Distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// DistanceSquared calculates the squared distance between two 2D points.
// Use this when comparing distances to avoid the sqrt overhead.
func DistanceSquared(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return dx*dx + dy*dy
}

// TileCenter returns the center coordinate of a tile given a position and tile size.
func TileCenter(pos, tileSize float64) float64 {
	return math.Floor(pos/tileSize)*tileSize + tileSize/2
}

// TileCenterFromTile returns the world center for a tile coordinate pair (search: tile-center).
func TileCenterFromTile(tileX, tileY int, tileSize float64) (float64, float64) {
	return float64(tileX)*tileSize + tileSize/2, float64(tileY)*tileSize + tileSize/2
}

// TileIndex converts a world coordinate to its tile index. Floor, not integer
// truncation: truncation rounds toward zero, so a negative coordinate lands one
// tile off (-30 with a 64 tile reads as 0 instead of -1). Matches TileCenter
// above and collision's tileCoord, so a spawn search and the ray that later
// checks it agree on which tile a point belongs to.
func TileIndex(pos, tileSize float64) int {
	return int(math.Floor(pos / tileSize))
}
