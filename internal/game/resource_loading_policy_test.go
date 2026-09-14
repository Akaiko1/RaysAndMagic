package game

import (
	"testing"
	"time"

	"ugataima/internal/character"
)

func TestSmallUIResourcesAvoidLoadingBarrier(t *testing.T) {
	for _, tc := range []struct {
		name, source                           string
		rendering, world, exhausted, wantPause bool
	}{
		{"small_ui", "party_member_panel", true, false, false, false},
		{"missing_ui", "missing_ui_policy_fixture", true, false, false, false},
		{"large_ui", "inventory_grid_panel", true, false, false, true},
		{"frame_budget", "party_member_panel", true, false, true, true},
		{"world_draw", "party_member_panel", true, true, false, true},
		{"update", "party_member_panel", false, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := loadingFixture(t)
			t.Chdir("../..")
			l := gl.loading
			l.rendering, l.worldPass = tc.rendering, tc.world
			if tc.exhausted {
				l.inlineUIBytes = smallUIFrameBytes
			}
			gl.game.sprites.SetDeferredResourceHandler(gl.deferGameplayResource)
			defer gl.game.sprites.SetDeferredResourceHandler(nil)
			func() {
				defer func() {
					if r := recover(); r != nil {
						if _, ok := r.(loadingRenderMiss); !ok {
							panic(r)
						}
					}
				}()
				sprite := gl.game.sprites.GetSprite(tc.source)
				if !tc.wantPause && sprite == nil {
					t.Fatal("small UI source was unavailable")
				}
			}()
			if l.awaitingFrame != tc.wantPause || l.stream.Pending() != tc.wantPause {
				t.Fatalf("pause=%v pending=%v, want %v", l.awaitingFrame, l.stream.Pending(), tc.wantPause)
			}
			if !tc.wantPause {
				if !l.started.IsZero() {
					t.Fatal("small UI started a loading banner")
				}
				before := l.inlineUIBytes
				gl.game.sprites.GetSprite(tc.source)
				if l.inlineUIBytes != before {
					t.Fatal("warm UI source spent another frame budget")
				}
			}
		})
	}
}

func TestDeferredHitTestDoesNotCacheUnknownScale(t *testing.T) {
	for _, name := range []string{"campfire", "grass", "missing_size_policy_fixture"} {
		for _, partial := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/cold", true: "/partial"}[partial], func(t *testing.T) {
				gl := loadingFixture(t)
				t.Chdir("../..")
				g := gl.game
				ApplySpriteColorKey(g.sprites, g.config)
				g.renderHelper = NewRenderingHelper(g)
				if partial {
					variants := g.sprites.GetSpriteVariants(name)
					if len(variants) == 0 {
						variants = []string{name}
					}
					g.sprites.SpriteVisibleFrameBounds(variants[0])
				}
				g.world.NPCs = []*character.NPC{{Name: "Cold scenery", Sprite: name, RenderCategory: "scenery", SizeClass: "full_tile", X: 10000, Y: 10000}}
				g.sprites.SetDeferredResourceHandler(gl.deferGameplayResource)
				g.findNPCAtScreen(0, 0)
				g.sprites.SetDeferredResourceHandler(nil)
				deadline := time.Now().Add(5 * time.Second)
				for gl.loading.stream.Pending() && time.Now().Before(deadline) {
					gl.loading.stream.Advance(256 << 10)
					time.Sleep(time.Millisecond)
				}
				if gl.loading.stream.Pending() {
					t.Fatal("stream did not finish")
				}
				got := g.renderHelper.visibleHeightFrameScale(name, false)
				want := NewRenderingHelper(g).visibleHeightFrameScale(name, false)
				if got != want {
					t.Fatalf("cached pending measurement: got=%v want=%v", got, want)
				}
				if again := g.renderHelper.visibleHeightFrameScale(name, false); again != want {
					t.Fatal("warm measurement drifted")
				}
			})
		}
	}
}
