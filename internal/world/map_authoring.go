package world

// AuthoredEntityGround distinguishes an explicit terrain choice from a derived
// spawn floor. Serializing a derived floor as authored would change how nearby
// entities inherit their ground on the next load.
func (md *MapData) AuthoredEntityGround(x, y int) (TileType3D, bool) {
	if md == nil || !md.hasCell(x, y) {
		return TileEmpty, false
	}
	tile := md.Tiles[y][x]
	if floor, derived := md.entityFloors[[2]int{x, y}]; derived && floor.Tile == tile {
		return tile, false
	}
	for _, spawn := range md.MonsterSpawns {
		if spawn.X == x && spawn.Y == y && spawn.GroundTile != "" {
			return tile, true
		}
	}
	return tile, tile != TileEmpty
}
