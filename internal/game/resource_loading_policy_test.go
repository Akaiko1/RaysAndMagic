package game

import (
	"testing"
	"time"
	"ugataima/internal/graphics"

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
				// In view: a hit test measures only what the renderer can draw.
				g.camera.Angle, g.camera.ViewDist = 0, g.config.GetViewDistance()
				g.world.NPCs = []*character.NPC{{Name: "Cold scenery", Sprite: name, RenderCategory: "scenery", SizeClass: "full_tile", X: g.camera.X + g.camera.ViewDist/2, Y: g.camera.Y}}
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

// A hit test measures art only for what the projection lets the renderer
// draw: an NPC beyond the view distance, even dead ahead, or behind the party,
// even close by, must not start a load for a region that is not resident.
func TestHitTestRequestsArtOnlyInsideTheViewDistance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance float64 // along the view axis, in view distances
		want     bool
	}{
		{"near", 0.25, true},
		{"just inside", 0.98, true},
		{"just outside", 1.02, false},
		{"far", 3, false},
		{"behind the party", -0.25, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := loadingFixture(t)
			t.Chdir("../..")
			g := gl.game
			ApplySpriteColorKey(g.sprites, g.config)
			g.renderHelper = NewRenderingHelper(g)
			g.camera.Angle, g.camera.ViewDist = 0, g.config.GetViewDistance()
			g.world.NPCs = []*character.NPC{{Name: "Cold scenery", Sprite: "campfire", RenderCategory: "scenery", SizeClass: "full_tile",
				X: g.camera.X + g.camera.ViewDist*tc.distance, Y: g.camera.Y}}
			var requested []string
			g.sprites.SetDeferredResourceHandler(func(request graphics.SpriteResourceRequest) bool {
				requested = append(requested, request.Name)
				return true
			})
			defer g.sprites.SetDeferredResourceHandler(nil)
			g.findNPCAtScreen(g.config.GetScreenWidth()/2, g.config.GetScreenHeight()/2)
			if got := len(requested) > 0; got != tc.want {
				t.Fatalf("art requested: %v (%v), want %v", got, requested, tc.want)
			}
		})
	}
}
