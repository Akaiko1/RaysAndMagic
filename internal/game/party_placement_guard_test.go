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

func TestSafePartyDestinationClampsWallIntoWalkable(t *testing.T) {
	g, ts := placementGuardGame(t)
	g.world.Tiles[5][5] = world.TileWall

	x, y := TileCenterFromTile(5, 5, ts)
	sx, sy := g.safePartyDestination(x, y)
	if int(sx/ts) == 5 && int(sy/ts) == 5 {
		t.Fatalf("destination stayed inside the wall tile")
	}
	if g.world.IsTileBlockingTerrainAt(int(sx/ts), int(sy/ts)) {
		t.Fatalf("clamped destination (%.1f,%.1f) is not walkable", sx, sy)
	}
}

func TestSafePartyDestinationKeepsOpenGroundExactly(t *testing.T) {
	g, ts := placementGuardGame(t)

	// Sub-tile offsets from a save must survive the guard untouched.
	x, y := TileCenterFromTile(8, 8, ts)
	x += 3.5
	y -= 2.25
	if sx, sy := g.safePartyDestination(x, y); sx != x || sy != y {
		t.Fatalf("open destination moved: (%.2f,%.2f) -> (%.2f,%.2f)", x, y, sx, sy)
	}
}

func TestSafePartyDestinationRespectsWaterAndFly(t *testing.T) {
	g, ts := placementGuardGame(t)
	g.world.Tiles[5][5] = world.TileDeepWater
	x, y := TileCenterFromTile(5, 5, ts)

	if sx, sy := g.safePartyDestination(x, y); int(sx/ts) == 5 && int(sy/ts) == 5 {
		t.Fatalf("unprotected party left on deep water")
	}

	g.walkOnWaterActive = true
	if sx, sy := g.safePartyDestination(x, y); sx != x || sy != y {
		t.Fatalf("walk-on-water destination moved: (%.2f,%.2f)", sx, sy)
	}
	g.walkOnWaterActive = false

	// Fly ignores terrain entirely; its expiry has its own eject.
	g.world.Tiles[5][5] = world.TileWall
	g.flyActive = true
	if sx, sy := g.safePartyDestination(x, y); sx != x || sy != y {
		t.Fatalf("fly destination moved: (%.2f,%.2f)", sx, sy)
	}
}

func TestSafePartyDestinationFlyStillClampsWhereFlyCannotGo(t *testing.T) {
	g, ts := placementGuardGame(t)
	g.flyActive = true

	// The border ring and out-of-bounds are solid even to Fly.
	for _, dest := range [][2]int{{0, 5}, {19, 5}, {5, 0}, {-3, 5}, {5, 25}} {
		x, y := TileCenterFromTile(dest[0], dest[1], ts)
		sx, sy := g.safePartyDestination(x, y)
		stx, sty := int(sx/ts), int(sy/ts)
		if stx == dest[0] && sty == dest[1] {
			t.Fatalf("fly destination (%d,%d) accepted outside flyable space", dest[0], dest[1])
		}
		if g.world.IsTileBlockingForFly(stx, sty) {
			t.Fatalf("fly destination (%d,%d) clamped to (%d,%d), still fly-blocked", dest[0], dest[1], stx, sty)
		}
	}
}

func TestSafePartyDestinationIgnoresStaleWorldWaterFlag(t *testing.T) {
	g, ts := placementGuardGame(t)
	g.world.Tiles[5][5] = world.TileDeepWater
	// A revisited map keeps flags from the previous visit; only the party's
	// live buffs may legalize water.
	g.world.SetWalkOnWaterActive(true)

	x, y := TileCenterFromTile(5, 5, ts)
	if sx, sy := g.safePartyDestination(x, y); int(sx/ts) == 5 && int(sy/ts) == 5 {
		t.Fatalf("stale world water flag legalized deep water without a live buff")
	}
}
