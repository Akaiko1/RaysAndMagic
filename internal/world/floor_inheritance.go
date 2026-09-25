package world

import "ugataima/internal/config"

// FloorChoice distinguishes a resolved TileEmpty floor from unresolved ground.
type FloorChoice struct {
	Tile TileType3D
	OK   bool
}

type FloorResolution [][]FloorChoice

func (floors FloorResolution) At(x, y int) (TileType3D, bool) {
	if y < 0 || y >= len(floors) || x < 0 || x >= len(floors[y]) {
		return TileEmpty, false
	}
	f := floors[y][x]
	return f.Tile, f.OK
}

// CanSupplyUnderFloor applies to original floors and to the original floor
// carried through another object. Never classify the intervening prop as ground.
// Movement flags do not determine appearance: water and chasm bottoms can
// supply ground, while directional edges opt out explicitly in the tile data.
func (tm *TileManager) CanSupplyUnderFloor(tile TileType3D) bool {
	d := tm.GetTileData(tile)
	return d != nil && d.RenderType == config.TileRenderFloor &&
		!d.ExcludeAsUnderFloor && !d.InheritsNeighbourFloor()
}

// ResolveFloors propagates authored floors in simultaneous waves. Entities
// are pending cells even when the parser has placed a default floor beneath
// them. No fallback participates in the vote: callers apply it after resolution.
// Each newly resolved cell wakes its neighbours once, keeping long chains linear.
func (tm *TileManager) ResolveFloors(tiles [][]TileType3D, entities map[[2]int]bool) FloorResolution {
	floors := make(FloorResolution, len(tiles))
	pending := make([][]bool, len(tiles))
	queued := make([][]bool, len(tiles))
	frontier := make([][2]int, 0)
	for y, row := range tiles {
		floors[y] = make([]FloorChoice, len(row))
		pending[y] = make([]bool, len(row))
		queued[y] = make([]bool, len(row))
		for x, tile := range row {
			if entities[[2]int{x, y}] || tm.InheritsFloor(tile) {
				pending[y][x] = true
			} else if tm.CanSupplyUnderFloor(tile) {
				floors[y][x] = FloorChoice{Tile: tile, OK: true}
				frontier = append(frontier, [2]int{x, y})
			}
		}
	}
	for len(frontier) > 0 {
		candidates := make([][2]int, 0)
		for _, cell := range frontier {
			for _, n := range floorVoteNeighbours {
				x, y := cell[0]+n.dx, cell[1]+n.dy
				if y < 0 || y >= len(tiles) || x < 0 || x >= len(tiles[y]) ||
					!pending[y][x] || queued[y][x] {
					continue
				}
				queued[y][x] = true
				candidates = append(candidates, [2]int{x, y})
			}
		}
		choices := make([]FloorChoice, len(candidates))
		for i, cell := range candidates {
			x, y := cell[0], cell[1]
			owner := tiles[y][x]
			if entities[cell] {
				owner = TileEmpty
			}
			// An entity's inherited floor also becomes its movement tile. Visual
			// scenery may inherit water/chasm color, but automatic spawn ground
			// must remain walkable. Explicit ground_tile bypasses this vote.
			tile, ok := tm.voteUnderFloor(owner, x, y, floors.At, entities[cell])
			choices[i] = FloorChoice{Tile: tile, OK: ok}
		}
		frontier = frontier[:0]
		for i, cell := range candidates {
			x, y := cell[0], cell[1]
			queued[y][x] = false
			if choices[i].OK {
				floors[y][x] = choices[i]
				pending[y][x] = false
				frontier = append(frontier, cell)
			}
		}
	}
	return floors
}

func (tm *TileManager) voteUnderFloor(owner TileType3D, x, y int, at func(int, int) (TileType3D, bool), requireWalkable bool) (TileType3D, bool) {
	ownerData := tm.GetTileData(owner)
	// At most eight distinct donors: avoid a map allocation for every cell.
	var donors [8]TileType3D
	var counts [8]int
	count := 0
	best, bestScore := TileEmpty, 0
	for _, n := range floorVoteNeighbours {
		tile, ok := at(x+n.dx, y+n.dy)
		if !ok || !tm.CanSupplyUnderFloor(tile) || requireWalkable && !tm.IsWalkable(tile) {
			continue
		}
		excluded := false
		if ownerData != nil {
			for _, key := range ownerData.ExcludedUnderFloorTiles {
				if tm.GetTileKey(tile) == key {
					excluded = true
					break
				}
			}
		}
		if excluded {
			continue
		}
		i := 0
		for i < count && donors[i] != tile {
			i++
		}
		if i == count {
			donors[i] = tile
			count++
		}
		counts[i] += n.w
		if counts[i] > bestScore {
			best, bestScore = tile, counts[i]
		}
	}
	return best, bestScore > 0
}
