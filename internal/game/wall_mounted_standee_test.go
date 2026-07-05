package game

import (
	"math"
	"testing"

	"ugataima/internal/world"
)

// Regression: a wall-mounted gate token vanished when the party stood in the
// MIDDLE of its tile. The NPC render loop uses a one-tile near-cull for normal
// floor-anchored sprites, while the env-sprite metrics also drop anything closer
// than ~5px. When the anchor was the tile CENTRE, a camera at the centre sat at
// distance ~0 and the gate was culled upstream - before any wall-mount draw code
// ran. After moving the anchor to the wall, the distance is still only half a
// tile, so wall-mounted NPCs must bypass the one-tile near-cull specifically.
//
// The fix anchors wall-mounted NPCs at wallStickPose (offset ~half a tile toward
// the wall). This test proves that, with the camera at the tile centre, that
// anchor is comfortably beyond the near-cull, so the gate is no longer deleted.
func TestWallStickPose_AnchorClearsNearCullAtTileCentre(t *testing.T) {
	cfg := loadTestConfig(t)

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM })

	tm := world.NewTileManager()
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	world.GlobalTileManager = tm

	wallType, ok := tm.GetTileTypeFromKey("wall")
	if !ok {
		t.Fatal(`"wall" tile key missing from tiles.yaml`)
	}
	if !tm.IsSolid(wallType) {
		t.Fatal(`"wall" tile is not solid - pick another for the fixture`)
	}
	floorType, ok := tm.GetTileTypeFromKey("grass")
	if !ok {
		floorType = world.TileEmpty
	}

	ts := float64(cfg.GetTileSize())

	// 3x3 world: centre (1,1) is floor, its EAST neighbour (2,1) is a wall.
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 3, 3
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = floorType
		}
	}
	w.Tiles[1][2] = wallType

	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"gate": w}
	wm.CurrentMapKey = "gate"
	world.GlobalWorldManager = wm

	g := newTestGame(cfg, w)

	// Gate sits at the centre of tile (1,1); the party stands on that same tile.
	npcX := (1.0 + 0.5) * ts
	npcY := (1.0 + 0.5) * ts
	camX, camY := npcX, npcY

	wx, wy, yaw, found := g.wallStickPose(npcX, npcY)
	if !found {
		t.Fatal("wallStickPose found no wall neighbour for an east-walled tile")
	}

	// Anchoring at the tile centre would be culled (camera coincides with it).
	if d := math.Hypot(npcX-camX, npcY-camY); d >= 5 {
		t.Fatalf("fixture wrong: camera not at tile centre (d=%.1f)", d)
	}

	// Anchoring at the wall pose clears the tiny env-sprite near-cull, but it is
	// still inside the generic one-tile cull. That generic cull must not apply
	// to wall-mounted NPCs.
	wallDist := math.Hypot(wx-camX, wy-camY)
	if wallDist < 5 {
		t.Fatalf("wall anchor only %.1fpx from a centred camera - still inside the near-cull", wallDist)
	}
	if wallDist >= ts {
		t.Fatalf("fixture wrong: wall anchor %.1fpx should still be inside the generic one-tile near-cull %.1f", wallDist, ts)
	}
	if wallDist < ts*0.4 {
		t.Errorf("wall anchor %.1fpx from centre, expected ~half a tile (%.1f)", wallDist, ts*0.5)
	}

	// East wall => anchor slides east, slab axis runs along the wall (yaw = pi/2).
	if wx <= npcX {
		t.Errorf("east wall: anchor should be east of centre (wx=%.1f npcX=%.1f)", wx, npcX)
	}
	if math.Abs(yaw-math.Pi/2) > 1e-6 {
		t.Errorf("east/west wall yaw should be pi/2, got %.4f", yaw)
	}
}
