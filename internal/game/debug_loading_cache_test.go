//go:build debug

package game

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Production boot/config, real frame boundaries and a private persistent cache.
// Cache reuse must survive cancelling all region/floor GPU work, just as after
// leaving a world. This is loading evidence, not a GPU FPS benchmark.
func TestGameplayLoadingCache(t *testing.T) {
	requireStandeeGPU(t)
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	w, h := g.gameLoop.Layout(1280, 720)
	x, y := TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	if g.config.OpenWorldEnabled() {
		x, y = world.GlobalWorldManager.ProjectWorldPos("forest", x, y)
	}
	for _, phase := range []string{"cold", "disk_warm"} {
		runOnDrawFrame(func(*ebiten.Image) {
			if phase == "disk_warm" {
				g.gameLoop.closeResourceLoading()
				g.gameLoop.renderer.resetMapRenderResourceResidency()
				g.gameLoop.renderer.clearFloorAtlas()
			}
			g.setPartyPosition(x, y)
			g.snapFacing(0)
		})
		start := time.Now()
		captureGameplayPreviewFrame(t, g, w, h)
		elapsed := time.Since(start)
		files, err := filepath.Glob(filepath.Join(root, ".render-cache", "*.rgba"))
		if err != nil || len(files) == 0 {
			t.Fatal("production loading did not populate disk cache", err)
		}
		var bytes int64
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			bytes += info.Size()
		}
		if bytes > 256<<20 {
			t.Fatal("cache exceeds disk limit")
		}
		for range 3 {
			runtime.GC()
			runOnDrawFrame(func(*ebiten.Image) {})
		}
		var gpu ebiten.DebugInfo
		runOnDrawFrame(func(*ebiten.Image) { ebiten.ReadDebugInfo(&gpu) })
		stats := g.gameLoop.renderer.renderResourceStats()
		t.Logf("%s: ready=%s cache=%d files/%dMiB engineGPU=%dMiB ownedGPU=%dMiB", phase, elapsed, len(files), bytes>>20, gpu.TotalGPUImageMemoryUsageInBytes>>20, stats.ownedGPUBytes>>20)
		if g.gameLoop.loading.awaitingFrame {
			t.Fatalf("%s did not release loading", phase)
		}
	}
}
