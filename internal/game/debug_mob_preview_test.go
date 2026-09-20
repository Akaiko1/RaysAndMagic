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
)

// Exercise the editor's actual configured stage on the live graphics driver.
func TestDebugSim_MobPreviewTiming(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live graphics driver")
	}
	cfg := setupPreviewSandboxTest(t)
	t.Chdir("../..")
	p, err := NewMobPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	// Ground solo/flock, left-facing art, flying, inert fish and arboreal:
	// warm, walk, synchronized strike, replay, and selection replacement.
	// Persistence is N/A for an editor specimen.
	for _, key := range []string{"goblin", "wolf", "bear", "bandit", "pixie", "common_carp", "ring_tailed_lemur", "elf_archer", "lich", "lich_king"} {
		p.Select(key)
		var firstDraw time.Duration
		runOnDrawFrame(func(*ebiten.Image) {
			start := time.Now()
			p.Scene() // Show the specimen immediately, before background preparation.
			firstDraw = time.Since(start)
		})
		deadline := time.Now().Add(15 * time.Second)
		for p.loading() && time.Now().Before(deadline) {
			p.Step()
			runOnDrawFrame(func(*ebiten.Image) { p.Scene() })
		}
		if p.loading() {
			t.Fatalf("%s preparation did not finish", key)
		}
		var maxStep, maxDraw, totalDraw time.Duration
		minDistance := 1e9
		attacks, previousAttack := 0, 0
		minY, maxY := p.Monsters()[0].Y, p.Monsters()[0].Y
		minX, maxX := p.Monsters()[0].X, p.Monsters()[0].X
		resources := p.g.gameLoop.renderer.renderResourceStats().ownedGPUBytes
		for i := 0; i < cfg.GetTPS()*20; i++ {
			start := time.Now()
			p.Step()
			maxStep = max(maxStep, time.Since(start))
			for _, m := range p.Monsters() {
				minDistance = min(minDistance, (m.X-p.g.camera.X)/float64(cfg.GetTileSize()))
				if m.AttackAnimFrames != p.Monsters()[0].AttackAnimFrames {
					t.Fatalf("%s: unsynchronized attack", key)
				}
				if m.IsEngagingPlayer {
					t.Fatalf("%s attacked the camera", key)
				}
				minY, maxY = min(minY, m.Y), max(maxY, m.Y)
				minX, maxX = min(minX, m.X), max(maxX, m.X)
				if math.Abs(m.Y-p.g.camera.Y) > (m.X-p.g.camera.X)*.6 {
					t.Errorf("%s left the visible clearing", key)
				}
			}
			currentAttack := p.Monsters()[0].AttackAnimFrames
			if currentAttack > previousAttack {
				attacks++
			}
			previousAttack = currentAttack
			if i%10 == 0 {
				runOnDrawFrame(func(*ebiten.Image) {
					start := time.Now()
					p.Scene()
					d := time.Since(start)
					maxDraw = max(maxDraw, d)
					totalDraw += d
					if dir := os.Getenv("RAM_MOB_PREVIEW_DIR"); dir != "" && (i == 120 || i == 500) {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%04d.png", key, i)))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(f, snapshotUIImage(p.scene))
						f.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
		if minDistance < 1 {
			t.Errorf("%s entered camera foreground: %v", key, minDistance)
		}
		span := math.Hypot(maxX-minX, maxY-minY)
		if p.Monsters()[0].FishLeap == nil && span < float64(cfg.GetTileSize())*.25 {
			t.Errorf("%s did not actually patrol: displacement %v", key, span)
		}
		if p.Monsters()[0].FishLeap == nil && maxY-minY < float64(cfg.GetTileSize())*.25 {
			t.Errorf("%s only paced along a line: sideways displacement %v", key, maxY-minY)
		}
		if p.attackFrames > 0 && attacks < 4 {
			t.Errorf("%s did not repeat attacks: %d", key, attacks)
		}
		if p.attackFrames == 0 && attacks != 0 {
			t.Errorf("%s invented an unauthored attack", key)
		}
		if after := p.g.gameLoop.renderer.renderResourceStats().ownedGPUBytes; after != resources {
			t.Errorf("%s: Draw created resources after preparation: %d -> %d", key, resources, after)
		}
		t.Logf("%s: first Draw %s; min depth %.2f tiles; patrol span %.2f tiles; max Step %s; max Draw %s; mean Draw %s", key, firstDraw, minDistance, span/float64(cfg.GetTileSize()), maxStep, maxDraw, totalDraw/240)
	}
}
