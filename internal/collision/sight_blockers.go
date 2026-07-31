package collision

import "math"

type sightTileKey struct {
	x int
	y int
}

// tileCoord is the shared world-to-grid conversion for DDA rays and dynamic
// sight blockers. Floor, rather than integer truncation, keeps negative
// stitched-world coordinates on the same tile in both systems. invTileSize is
// passed by callers that already compute it for a ray.
func tileCoord(worldCoord, invTileSize float64) int {
	return int(math.Floor(worldCoord * invTileSize))
}

// indexEntitySightTiles updates the dynamic opacity overlay for every tile the
// entity's box occupies. Counts make replacement and overlapping blockers safe:
// removing one entity cannot reveal a tile still covered by another.
func (cs *CollisionSystem) indexEntitySightTiles(entity *Entity, delta int) {
	if cs == nil || entity == nil || entity.BoundingBox == nil || !entity.blocksSight || cs.tileSize <= 0 || delta == 0 {
		return
	}
	if cs.sightBlockerTiles == nil {
		cs.sightBlockerTiles = make(map[sightTileKey]int)
	}
	minX, minY, maxX, maxY := entity.BoundingBox.GetBounds()
	// Bounding boxes are half-open for tile occupancy. Without Nextafter, a box
	// ending exactly on a grid line would incorrectly occlude the next tile too.
	maxX = math.Nextafter(maxX, math.Inf(-1))
	maxY = math.Nextafter(maxY, math.Inf(-1))
	invTileSize := 1 / cs.tileSize
	startX := tileCoord(minX, invTileSize)
	startY := tileCoord(minY, invTileSize)
	endX := tileCoord(maxX, invTileSize)
	endY := tileCoord(maxY, invTileSize)
	for y := startY; y <= endY; y++ {
		for x := startX; x <= endX; x++ {
			key := sightTileKey{x: x, y: y}
			next := cs.sightBlockerTiles[key] + delta
			if next <= 0 {
				delete(cs.sightBlockerTiles, key)
			} else {
				cs.sightBlockerTiles[key] = next
			}
		}
	}
}
