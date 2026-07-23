//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Pure-math standee cost proxy for TREES only - a DEBUG MODULE, not a
// regression test. It touches no textures and needs no render context: it just
// evaluates the SAME geometry the renderer pays per tree (shell count plus
// crossed-pair vs distant single-plane LOD) across a distance sweep, so the
// tree-standee draw budget can be reasoned about without profiling a live
// frame. The heavier full-frame timing lives in debug_render_walk_sim_test.go.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_StandeeCost -v

import (
	"math"
	"os"
	"testing"

	"ugataima/internal/config"
)

func TestDebugSim_StandeeCost(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	tileSize := float64(cfg.GetTileSize())
	screenW := cfg.GetScreenWidth()
	halfFovTan := math.Tan(cfg.GetCameraFOV() / 2)
	lodTiles := cfg.Graphics.TreeStandeeLODTiles
	// Both crossed slabs use the shared standee thickness from config.
	h := cfg.Graphics.Standee.ThicknessTiles * tileSize / 2

	t.Logf("tile=%.0f screenW=%d fov/2 tan=%.3f LOD=%.1ft thickness=%.3ftiles(h=%.1fpx)",
		tileSize, screenW, halfFovTan, lodTiles, cfg.Graphics.Standee.ThicknessTiles, h)

	// Cost is rendered slab surfaces: each slab has far sticker, core shells,
	// and near sticker; the distant LOD halves the crossed pair to one slab.
	treeCost := func(distance float64) (shells, planes, cost int, lod bool) {
		shells = standeeShellCount(h, screenW, halfFovTan, distance)
		planes = 2
		lod = treeIsBillboardLOD(distance, tileSize, lodTiles)
		if lod {
			planes = 1
		}
		return shells, planes, (shells + 2) * planes, lod
	}

	lodCutReported := false
	for tiles := 1; tiles <= int(cfg.GetViewDistance()/tileSize); tiles++ {
		dist := float64(tiles) * tileSize
		shells, planes, cost, lod := treeCost(dist)
		if lod && !lodCutReported {
			t.Logf("--- billboard-LOD kicks in at %d tiles ---", tiles)
			lodCutReported = true
		}
		if tiles <= 12 || tiles%8 == 0 {
			t.Logf("dist=%2dt: shells=%2d planes=%d cost=%2d lod=%v", tiles, shells, planes, cost, lod)
		}
	}

	// Shell cost is monotonic non-increasing with distance; the distant LOD
	// halves the number of slabs.
	prev := math.MaxInt32
	for tiles := 1; tiles <= 48; tiles++ {
		_, _, cost, _ := treeCost(float64(tiles) * tileSize)
		if cost > prev {
			t.Errorf("tree cost rose with distance at %d tiles: %d > %d", tiles, cost, prev)
		}
		prev = cost
	}
}
