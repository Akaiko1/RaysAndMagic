//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// This is the gameplay measurement/capture entry point. Unlike isolated shader
// fixtures, it uses authored size classes, configured open-world stitching,
// production Layout/Update/Draw and the complete resource-loading barrier.
// RAM_RIVER_PREVIEW optionally saves native, unscaled game frames.
func TestRiverGameplay(t *testing.T) {
	requireStandeeGPU(t)
	root := t.TempDir()
	storage.SetDataRootForTesting(root)
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	w, h := g.gameLoop.Layout(1920, 1080)
	t.Logf("production viewport=%dx%d tree size=%g open world=%v", w, h, g.config.Graphics.SizeClasses["tree"], g.config.OpenWorldEnabled())
	if out := os.Getenv("RAM_RIVER_PREVIEW"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
	}
	target := ebiten.NewImage(w, h)
	defer target.Deallocate()
	for _, pose := range []struct {
		name        string
		x, y, angle float64
		tb          bool
	}{
		{"river_along_rt", 13, 36, 0, false},
		{"river_across_rt", 13, 36, math.Pi / 2, false},
		{"clearing_rt", 26, 44, 0, false},
		{"river_along_tb", 13, 36, 0, true},
	} {
		if only := os.Getenv("RAM_RIVER_POSE"); only != "" && only != pose.name {
			continue
		}
		x, y := TileCenterFromTile(int(pose.x), int(pose.y), float64(g.config.GetTileSize()))
		angle := pose.angle
		if g.config.OpenWorldEnabled() {
			x, y = world.GlobalWorldManager.ProjectWorldPos("forest", x, y)
			angle = world.GlobalWorldManager.ProjectAngle("forest", angle)
		}
		g.setPartyPosition(x, y)
		g.snapFacing(angle)
		g.turnBasedMode = pose.tb
		shot := captureGameplayPreviewFrame(t, g, w, h)
		if out := os.Getenv("RAM_RIVER_PREVIEW"); out != "" {
			file, err := os.Create(filepath.Join(out, fmt.Sprintf("%s_%dx%d.png", pose.name, w, h)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, shot)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		var draws, intervals, vertices []float64
		var previous time.Time
		for i := 0; i < 150; i++ {
			runOnDrawFrame(func(_ *ebiten.Image) {
				started := time.Now()
				if err := g.Update(); err != nil {
					t.Error(err)
				}
				drawStart := time.Now()
				g.Draw(target)
				if i >= 25 {
					draws = append(draws, float64(time.Since(drawStart))/float64(time.Millisecond))
					intervals = append(intervals, float64(started.Sub(previous))/float64(time.Millisecond))
					vertices = append(vertices, float64(g.gameLoop.renderer.statStandeeVertices))
				}
				previous = started
			})
		}
		d50, d95, _, _ := renderTimingPercentiles(draws)
		f50, f95, _, _ := renderTimingPercentiles(intervals)
		v50, _, _, _ := renderTimingPercentiles(vertices)
		t.Logf("%s: CPU Draw p50/p95=%.2f/%.2fms; frame interval p50/p95=%.2f/%.2fms; vertices=%.0f", pose.name, d50, d95, f50, f95, v50)
	}
	var gpu ebiten.DebugInfo
	ebiten.ReadDebugInfo(&gpu)
	t.Logf("engine GPU image estimate=%dMiB", gpu.TotalGPUImageMemoryUsageInBytes/(1<<20))
}
