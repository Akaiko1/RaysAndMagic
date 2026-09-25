package game

import (
	"fmt"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
)

func TestLoadingQueuesCombatBeforeFinishingRenderWork(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, first := range []string{"draw", "update"} {
			t.Run(fmt.Sprintf("tb=%v/first=%s", tb, first), func(t *testing.T) {
				gl := loadingFixture(t)
				t.Chdir("../..")
				g := gl.game
				g.turnBasedMode = tb
				mon := monster.NewMonster3DFromConfig(64, 64, "goblin", g.config)
				mon.IsEngagingPlayer = true
				g.world.Monsters = []*monster.Monster3D{mon}
				worker := make(chan struct{})
				gl.loading.pattern = worker
				gl.loading.begin(time.Now().Add(-time.Second))
				started := gl.loading.started
				screen := ebiten.NewImage(64, 64)
				defer screen.Deallocate()
				if first == "draw" {
					gl.Draw(screen)
				} else if err := gl.Update(); err != nil {
					t.Fatal(err)
				}
				if !gl.loading.stream.Pending() {
					t.Fatal("cold attack dependencies were left for a second loading episode")
				}
				if !gl.loading.awaitingFrame || !gl.loading.finished.IsZero() || gl.loading.started != started {
					t.Fatal("preflight split or completed the active loading episode")
				}
				close(worker)
				deadline := time.Now().Add(8 * time.Second)
				for gl.loading.stream.Pending() && time.Now().Before(deadline) {
					gl.advanceResourceLoading()
					time.Sleep(time.Millisecond)
				}
				if gl.loading.stream.Pending() {
					t.Fatal("combat preflight did not drain")
				}
				// Use a rejecting miss handler to verify both directions are already
				// resident; a synchronous fallback would conceal missing preflight work.
				misses := 0
				g.sprites.SetDeferredResourceHandler(func(_ graphics.SpriteResourceRequest) bool { misses++; return true })
				defer g.sprites.SetDeferredResourceHandler(nil)
				if g.authoredMonsterAttackFrameCount(mon) == 0 || misses != 0 {
					t.Fatal("next combat tick still required an animation load")
				}
			})
		}
	}
}

func TestLoadingBannerCombinesAdjacentEpisodes(t *testing.T) {
	start := time.Unix(100, 0)
	for _, gap := range []time.Duration{time.Millisecond, 200 * time.Millisecond, loadingBannerSettle, time.Second} {
		t.Run(gap.String(), func(t *testing.T) {
			l := &gameLoadingState{}
			l.begin(start)
			l.finished = start.Add(time.Second)
			next := l.finished.Add(gap)
			alpha := l.bannerAlpha(next)
			l.begin(next)
			if gap <= loadingBannerSettle {
				if alpha != 1 || l.bannerAlpha(next) != 1 || l.started != start {
					t.Fatal("adjacent demand made the banner fade or restart")
				}
			} else if l.started != next || l.bannerAlpha(next) != 0 {
				t.Fatal("separate loading episode skipped its initial delay")
			}
		})
	}
	// No final banner draw is required for a later episode to start fresh.
	l := &gameLoadingState{}
	l.begin(start)
	l.finished = start.Add(time.Second)
	next := start.Add(3 * time.Second)
	l.begin(next)
	if l.started != next || l.bannerAlpha(next) != 0 {
		t.Fatal("banner retained an old episode when Draw skipped its fade")
	}
}
