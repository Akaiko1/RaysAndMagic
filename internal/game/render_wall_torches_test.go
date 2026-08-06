package game

import (
	"math"
	"reflect"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Wall torches are static map dressing. Entering a torch-lit map while Fly is
// active must produce the same corner cache as entering it on foot.
func TestJapaneseCastleWallTorchesIgnoreFly(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "japanese_castle")
	castle := wm.GetCurrentWorld()
	if castle == nil {
		t.Fatal("japanese castle world missing")
	}

	g := newTestGame(cfg, castle)
	r := NewRenderer(g)
	walking := append([]wallTorchPoint(nil), r.wallTorches...)
	if len(walking) == 0 {
		t.Fatal("japanese castle should have authored wall torches")
	}

	castle.SetFlyActive(true)
	t.Cleanup(func() { castle.SetFlyActive(false) })
	r.buildWallTorches()
	if !reflect.DeepEqual(r.wallTorches, walking) {
		t.Fatalf("torch cache changed under Fly: got %d points, want %d", len(r.wallTorches), len(walking))
	}
}

// Water is impassable without a traversal buff, but it is not a wall face.
// Culverts has canal/floor corners that used to create floating torches because
// the corner detector treated every movement blocker as a backing wall.
func TestCulvertsWallTorchesRequireTwoWallFaces(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "culverts")
	culverts := wm.GetCurrentWorld()
	if culverts == nil {
		t.Fatal("culverts world missing")
	}

	g := newTestGame(cfg, culverts)
	r := NewRenderer(g)
	if len(r.wallTorches) == 0 {
		t.Fatal("culverts should have authored wall torches")
	}
	walking := append([]wallTorchPoint(nil), r.wallTorches...)
	tileSize := float64(cfg.GetTileSize())
	for i, torch := range walking {
		tx := int(math.Floor(torch.X / tileSize))
		ty := int(math.Floor(torch.Y / tileSize))
		if !wallTorchHostTile(culverts, tx, ty) {
			t.Fatalf("torch %d host (%d,%d) is not authored walkable floor", i, tx, ty)
		}
		dx := -1
		if torch.X-float64(tx)*tileSize > tileSize/2 {
			dx = 1
		}
		dy := -1
		if torch.Y-float64(ty)*tileSize > tileSize/2 {
			dy = 1
		}
		for _, neighbour := range [][2]int{{tx + dx, ty}, {tx, ty + dy}} {
			data := world.GlobalTileManager.GetTileData(culverts.GetTileAtGrid(neighbour[0], neighbour[1]))
			if data == nil || data.RenderType != config.TileRenderWall {
				t.Fatalf("torch %d at host (%d,%d) uses non-wall support (%d,%d)",
					i, tx, ty, neighbour[0], neighbour[1])
			}
		}
	}

	culverts.SetWalkOnWaterActive(true)
	t.Cleanup(func() { culverts.SetWalkOnWaterActive(false) })
	r.buildWallTorches()
	if !reflect.DeepEqual(r.wallTorches, walking) {
		t.Fatalf("torch cache changed under Walk on Water: got %d points, want %d", len(r.wallTorches), len(walking))
	}
}
