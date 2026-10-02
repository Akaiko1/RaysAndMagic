package game

import (
	"testing"

	"ugataima/internal/world"
)

// placementGuardGame primes the tile manager and an open 20x20 world - the
// harness for safePartyDestination tests (stale save/pose coordinates).
func placementGuardGame(t *testing.T) (*MMGame, float64) {
	t.Helper()
	setTestWorldManager(t, nil)
	prev := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = prev })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	cfg := loadTestConfig(t)
	game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
	ts := float64(cfg.GetTileSize())
	placePlayerAtTile(game, 10, 10, ts)
	return game, ts
}

// safePartyDestination keeps a legal destination exactly (sub-tile offsets of
// a save included) and otherwise moves the party onto a tile its live state
// may stand on: walkable ground, water only under a live Walk on Water, and
// anything inside the flyable space under Fly. A stale world water flag from
// an earlier visit legalizes nothing.
func TestSafePartyDestinationTable(t *testing.T) {
	wall := func(g *MMGame) { g.world.Tiles[5][5] = world.TileWall }
	water := func(g *MMGame) { g.world.Tiles[5][5] = world.TileDeepWater }
	fly := func(g *MMGame) { g.flyActive = true }
	tests := []struct {
		name   string
		setup  []func(g *MMGame)
		tile   [2]int
		offset [2]float64
		stays  bool
	}{
		{"wall clamps into walkable", []func(*MMGame){wall}, [2]int{5, 5}, [2]float64{}, false},
		{"open ground keeps sub-tile offset", nil, [2]int{8, 8}, [2]float64{3.5, -2.25}, true},
		{"deep water ejects an unprotected party", []func(*MMGame){water}, [2]int{5, 5}, [2]float64{}, false},
		{"walk on water keeps deep water", []func(*MMGame){water, func(g *MMGame) { g.walkOnWaterActive = true }}, [2]int{5, 5}, [2]float64{}, true},
		{"stale world water flag keeps nothing", []func(*MMGame){water, func(g *MMGame) { g.world.SetWalkOnWaterActive(true) }}, [2]int{5, 5}, [2]float64{}, false},
		// Fly ignores terrain entirely; its expiry has its own eject.
		{"fly ignores a wall", []func(*MMGame){wall, fly}, [2]int{5, 5}, [2]float64{}, true},
		// The border ring and out-of-bounds are solid even to Fly.
		{"fly clamps off the west border", []func(*MMGame){fly}, [2]int{0, 5}, [2]float64{}, false},
		{"fly clamps off the east border", []func(*MMGame){fly}, [2]int{19, 5}, [2]float64{}, false},
		{"fly clamps off the north border", []func(*MMGame){fly}, [2]int{5, 0}, [2]float64{}, false},
		{"fly clamps in from out of bounds west", []func(*MMGame){fly}, [2]int{-3, 5}, [2]float64{}, false},
		{"fly clamps in from out of bounds south", []func(*MMGame){fly}, [2]int{5, 25}, [2]float64{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, ts := placementGuardGame(t)
			for _, setup := range tt.setup {
				setup(g)
			}
			x, y := TileCenterFromTile(tt.tile[0], tt.tile[1], ts)
			x, y = x+tt.offset[0], y+tt.offset[1]
			sx, sy := g.safePartyDestination(x, y)
			if tt.stays {
				if sx != x || sy != y {
					t.Fatalf("legal destination moved: (%.2f,%.2f) -> (%.2f,%.2f)", x, y, sx, sy)
				}
				return
			}
			stx, sty := int(sx/ts), int(sy/ts)
			if stx == tt.tile[0] && sty == tt.tile[1] {
				t.Fatalf("destination stayed on illegal tile (%d,%d)", stx, sty)
			}
			blocked := g.world.IsTileBlockingTerrainAt(stx, sty)
			if g.flyActive {
				blocked = g.world.IsTileBlockingForTerrainPassage(stx, sty)
			}
			if blocked {
				t.Fatalf("destination (%d,%d) clamped to blocked tile (%d,%d)", tt.tile[0], tt.tile[1], stx, sty)
			}
		})
	}
}
