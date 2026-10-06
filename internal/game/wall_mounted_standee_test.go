package game

import (
	"math"
	"testing"

	"ugataima/internal/world"
)

// A wall-mounted NPC anchors flush on the wall face of its own tile (tile
// centre + half a tile toward the solid neighbour), with the slab running along
// the wall. Off-centre NPC positions snap to their tile first.
func TestWallStickPoseAnchorsOnWallFace(t *testing.T) {
	cfg := loadTestConfig(t)

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM })

	tm := world.NewTileManager(testTileSizeClasses())
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
	cx, cy := 1.5*ts, 1.5*ts
	for _, tc := range []struct {
		name           string
		wallX, wallY   int // neighbour of tile (1,1); -1 = no wall
		wantDX, wantDY float64
		wantYaw        float64
	}{
		{name: "north wall", wallX: 1, wallY: 0, wantDY: -0.5, wantYaw: 0},
		{name: "east wall", wallX: 2, wallY: 1, wantDX: 0.5, wantYaw: math.Pi / 2},
		{name: "south wall", wallX: 1, wallY: 2, wantDY: 0.5, wantYaw: 0},
		{name: "west wall", wallX: 0, wallY: 1, wantDX: -0.5, wantYaw: math.Pi / 2},
		{name: "no wall neighbour", wallX: -1, wallY: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := world.NewWorld3D(cfg)
			w.Width, w.Height = 3, 3
			w.Tiles = make([][]world.TileType3D, w.Height)
			for y := range w.Tiles {
				w.Tiles[y] = make([]world.TileType3D, w.Width)
				for x := range w.Tiles[y] {
					w.Tiles[y][x] = floorType
				}
			}
			if tc.wallX >= 0 {
				w.Tiles[tc.wallY][tc.wallX] = wallType
			}
			wm := world.NewWorldManager(cfg)
			wm.LoadedMaps = map[string]*world.World3D{"gate": w}
			wm.CurrentMapKey = "gate"
			world.GlobalWorldManager = wm
			g := newTestGame(cfg, w)

			wx, wy, yaw, found := g.wallStickPose(cx-0.3*ts, cy+0.2*ts)
			if found != (tc.wallX >= 0) {
				t.Fatalf("found = %v, want %v", found, tc.wallX >= 0)
			}
			if !found {
				return
			}
			if wantX, wantY := cx+tc.wantDX*ts, cy+tc.wantDY*ts; math.Abs(wx-wantX) > 1e-9 || math.Abs(wy-wantY) > 1e-9 {
				t.Fatalf("anchor = (%.1f,%.1f), want wall face (%.1f,%.1f)", wx, wy, wantX, wantY)
			}
			if math.Abs(yaw-tc.wantYaw) > 1e-9 {
				t.Fatalf("yaw = %.4f, want %.4f (slab along the wall)", yaw, tc.wantYaw)
			}
		})
	}
}
