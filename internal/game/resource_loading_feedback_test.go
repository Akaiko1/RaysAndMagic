package game

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestAreaLoadingFeedbackStartsWithPause(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, entry := range []string{"update", "draw"} {
			for _, work := range []string{"manifest", "upload", "shader", "floor"} {
				t.Run(fmt.Sprintf("tb=%v/%s/%s", tb, entry, work), func(t *testing.T) {
					gl := neighbourhoodLoadingFixture(t)
					g, r := gl.game, gl.renderer
					g.turnBasedMode = tb
					for _, key := range []string{"east", "west", "north", "south"} {
						if key != "east" || work != "manifest" {
							r.commitMapRenderResidency(key, testMapRenderResources())
						}
					}
					task := &mapRenderPrewarmTask{mapKey: "east"}
					switch work {
					case "upload":
						r.mapRenderUploadQueue = []mapRenderUpload{{task: task}}
					case "shader":
						r.mapRenderShaderWarmTasks = []*mapRenderPrewarmTask{task}
					case "floor":
						_, cancel := context.WithCancel(context.Background())
						r.floorPreparation = &floorPreparation{cancel: cancel, result: make(chan preparedFloor)}
					}
					before := g.frameCount
					if entry == "update" {
						if err := gl.Update(); err != nil {
							t.Fatal(err)
						}
					} else {
						frame := ebiten.NewImage(64, 64)
						defer frame.Deallocate()
						gl.Draw(frame)
					}
					l := gl.loading
					if !l.awaitingFrame || g.frameCount != before {
						t.Fatal("world preparation did not keep the simulation paused")
					}
					if !l.area || l.bannerAlpha(l.started) != 1 || gl.ui.loadingBannerLabel() != "Loading area..." {
						t.Fatal("first paused frame must show area feedback without a delay")
					}
				})
			}
		}
	}
}

func TestDeferredLoadingFeedbackClassifiesActualWork(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		rendering, worldPass, wantArea bool
	}{
		{"update_combat", false, false, true},
		{"world_draw", true, true, true},
		{"interface_draw", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := loadingFixture(t)
			l := gl.loading
			l.rendering, l.worldPass = tc.rendering, tc.worldPass
			// An unavailable source takes the deferred path regardless of the
			// inline UI byte allowance. It never synchronously decodes a file.
			func() {
				defer func() {
					if p := recover(); p != nil {
						if _, ok := p.(loadingRenderMiss); !ok {
							panic(p)
						}
					}
				}()
				gl.deferGameplayResource(graphics.SpriteResourceRequest{Name: "feedback_test_missing_source"})
			}()
			if l.area != tc.wantArea {
				t.Fatal("feedback used the visible UI instead of the resource's owner")
			}
			if got := l.bannerAlpha(l.started); got != float64(boolInt(tc.wantArea)) {
				t.Fatalf("first-frame alpha = %v", got)
			}
		})
	}
}

func TestAreaFeedbackLifetimeKeepsInterfacePolicy(t *testing.T) {
	start := time.Unix(100, 0)
	for _, area := range []bool{false, true} {
		for _, duration := range []time.Duration{time.Millisecond, 2 * time.Second} {
			t.Run(fmt.Sprintf("area=%v/duration=%s", area, duration), func(t *testing.T) {
				l := &gameLoadingState{}
				if area {
					l.beginArea(start)
				} else {
					l.begin(start)
				}
				l.finished, l.awaitingFrame = start.Add(duration), false
				wantVisible := area || duration >= loadingBannerDelay
				if (l.bannerAlpha(l.finished) > 0) != wantVisible || l.awaitingFrame {
					t.Fatal("completion hid area feedback or extended gameplay pause")
				}
			})
		}
	}
	for _, drawFade := range []bool{false, true} {
		t.Run(fmt.Sprintf("separate_ui/draw_fade=%v", drawFade), func(t *testing.T) {
			l := &gameLoadingState{}
			l.beginArea(start)
			l.finished = start.Add(time.Second)
			next := start.Add(2 * time.Second)
			if drawFade {
				l.bannerAlpha(next)
			}
			l.begin(next)
			if l.area || l.bannerAlpha(next) != 0 {
				t.Fatal("later interface load inherited immediate area feedback")
			}
		})
	}
	for _, areaFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("adjacent/area_first=%v", areaFirst), func(t *testing.T) {
			l := &gameLoadingState{}
			if areaFirst {
				l.beginArea(start)
			} else {
				l.begin(start)
			}
			l.finished = start.Add(time.Second)
			next := l.finished.Add(time.Millisecond)
			if areaFirst {
				l.begin(next)
			} else {
				l.beginArea(next)
			}
			if !l.area || l.bannerAlpha(next) != 1 {
				t.Fatal("mixed episode lost immediate area feedback")
			}
		})
	}
}
