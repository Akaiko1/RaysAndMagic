package game

import "testing"

// TileIndex must FLOOR, not truncate: the summon free-tile search and the tiles
// a spawn is later tested against have to agree on which tile a negative world
// coordinate belongs to.
func TestTileIndexFloorsNegativeWorldCoordinates(t *testing.T) {
	const tileSize = 64.0
	tests := []struct {
		pos  float64
		want int
	}{
		{pos: -64.1, want: -2},
		{pos: -64, want: -1},
		{pos: -30, want: -1},
		{pos: -0.1, want: -1},
		{pos: 0, want: 0},
		{pos: 63.9, want: 0},
		{pos: 64, want: 1},
	}
	for _, tt := range tests {
		if got := TileIndex(tt.pos, tileSize); got != tt.want {
			t.Errorf("TileIndex(%v) = %d, want %d", tt.pos, got, tt.want)
		}
	}
}

// The center of the tile TileIndex reports must map back to the same index.
func TestTileIndexAgreesWithTileCenter(t *testing.T) {
	const tileSize = 64.0
	for _, pos := range []float64{-129, -64, -1, 0, 1, 63, 64, 4096} {
		index := TileIndex(pos, tileSize)
		if got := TileIndex(TileCenter(pos, tileSize), tileSize); got != index {
			t.Errorf("TileIndex(TileCenter(%v)) = %d, want %d", pos, got, index)
		}
		x, y := TileCenterFromTile(index, index, tileSize)
		if TileIndex(x, tileSize) != index || TileIndex(y, tileSize) != index {
			t.Errorf("TileCenterFromTile(%d) = (%v, %v), which reads back as a different tile", index, x, y)
		}
	}
}
