package game

import (
	"context"
	"image"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// This opt-in probe uses shipped outdoor/dungeon sources and actual Draw frame
// boundaries. It measures submission latency and ownership, not driver VRAM.
func TestDebugSim_RedesignStreaming(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	setTestWorldManager(t, nil)
	g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	g.appScreen = AppScreenInGame
	g.sprites = graphics.NewSpriteManager()
	r := &Renderer{game: g, processedSpriteCache: make(map[processedSpriteKey]*ebiten.Image)}
	regions := []struct{ key, source, wall string }{
		{"forest", "forest_oak", "highlands_granite_wall"},
		{"clock_tower", "clock_tower", "clocktower_timber_wall"},
	}
	var updates, draws []time.Duration
	baseline := map[string]int64{}
	peakCPU := int64(0)
	for cycle := 0; cycle < 3; cycle++ {
		for _, region := range regions {
			ctx, cancel := context.WithCancel(context.Background())
			task := &mapRenderPrewarmTask{mapKey: region.key, world: g.world, generation: r.mapRenderGeneration, ctx: ctx, cancel: cancel, skiesDone: true, queueBudget: graphics.NewPreparationBudget(32 << 20)}
			task.plan = mapRenderPrewarmPlan{containerSprites: []string{region.source}, wallSprites: []string{region.wall}}
			task.prewarmer = newMapRenderPrewarmer(r, task)
			task.cpuImages = make(map[*ebiten.Image]*image.RGBA)
			task.preparedSprites = g.sprites.PrepareResources(ctx, []graphics.SpriteResourceRequest{{Name: region.source}, {Name: region.wall}}, task.queueBudget)
			r.mapRenderResourcePrewarmActive = task
			completed := false
			for frame := 0; frame < 20000; frame++ {
				runOnDrawFrame(func(screen *ebiten.Image) {
					start := time.Now()
					r.prewarmPendingMapRenderResources()
					updates = append(updates, time.Since(start))
					start = time.Now()
					r.drawMapRenderPrewarmUploads(screen)
					r.drawMapRenderShaderWarm(screen)
					draws = append(draws, time.Since(start))
				})
				_, peak := task.queueBudget.Usage()
				peakCPU = max(peakCPU, peak)
				if r.mapRenderResourcePrewarmActive == nil && len(r.mapRenderUploadQueue) == 0 && len(r.mapRenderShaderWarmTasks) == 0 {
					completed = true
					break
				}
			}
			cancel()
			if !completed {
				t.Fatalf("streaming stalled in %s", region.key)
			}
			runOnDrawFrame(func(_ *ebiten.Image) { r.evictMapRenderResidencyOutside(map[string]struct{}{region.key: {}}) })
			stats := r.renderResourceStats()
			if cycle == 0 {
				baseline[region.key] = stats.ownedGPUBytes
			} else if stats.ownedGPUBytes != baseline[region.key] {
				t.Fatalf("%s residency grew from %d to %d", region.key, baseline[region.key], stats.ownedGPUBytes)
			}
			if stats.retainedCPUBytes != 0 || stats.queuedCPUReservation != 0 {
				t.Fatal("completed streaming retained CPU work")
			}
			t.Logf("cycle=%d region=%s resources=%d allocations=%d estimated_GPU=%d", cycle, region.key, stats.resources, stats.allocations, stats.ownedGPUBytes)
		}
	}
	runOnDrawFrame(func(_ *ebiten.Image) { r.resetMapRenderResourceResidency() })
	if stats := r.renderResourceStats(); stats.ownedGPUBytes != 0 || stats.resources != 0 {
		t.Fatalf("final reset retained %+v", stats)
	}
	percentile := func(samples []time.Duration, p int) time.Duration {
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[(len(samples)-1)*p/100]
	}
	t.Logf("frames=%d update_p95=%s update_p99=%s draw_submission_p95=%s draw_submission_p99=%s peak_CPU_reservation=%d", len(updates), percentile(updates, 95), percentile(updates, 99), percentile(draws, 95), percentile(draws, 99), peakCPU)
}
