package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

func idleInventoryUI(t testing.TB, count int) *UISystem {
	t.Helper()
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	g.appScreen, g.menuOpen, g.currentTab = AppScreenInGame, true, TabInventory
	g.sprites = graphics.NewSpriteManager()
	g.party.Inventory = nil
	for i := 0; i < count; i++ {
		g.party.Inventory = append(g.party.Inventory, items.Item{Name: fmt.Sprintf("Found Sword %d", i), Type: items.ItemWeapon, InstanceID: uint64(i + 1)})
	}
	ui := NewUISystem(g)
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	t.Cleanup(screen.Deallocate)
	ui.Draw(screen)
	return ui
}

func TestDisplayedIdleInventoryDoesNotAllocatePerBinding(t *testing.T) {
	for _, count := range []int{64, 512, 2048} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ui := idleInventoryUI(t, count)
			if n := testing.AllocsPerRun(100, ui.dispatchDisplayedInput); n != 0 {
				t.Fatalf("idle displayed inventory allocates %g times per update", n)
			}
			// Skipping idle work must not bless changed content for a later click.
			ui.game.party.Inventory[0].InstanceID++
			if ui.displayedInputCurrent() {
				t.Fatal("idle path forgot the displayed item identity")
			}
		})
	}
}

func BenchmarkDisplayedInventoryIdle(b *testing.B) {
	for _, count := range []int{64, 512, 2048} {
		b.Run(fmt.Sprintf("items=%d", count), func(b *testing.B) {
			ui := idleInventoryUI(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ui.dispatchDisplayedInput()
			}
			b.StopTimer()
			b.ReportMetric(float64(len(ui.displayedInput.commands)), "bindings")
		})
	}
}

func idleSkyRenderer(t testing.TB, count int) *Renderer {
	t.Helper()
	r := &Renderer{game: &MMGame{}, standeeCoreCache: make(map[standeeCoreKey]*ebiten.Image)}
	resources := testMapRenderResources()
	for i := 0; i < count; i++ {
		key := standeeCoreKey{name: fmt.Sprintf("mob-frame-%d", i)}
		r.standeeCoreCache[key] = ebiten.NewImage(8, 8)
		resources.standees[key] = struct{}{}
	}
	r.commitMapRenderResidency("forest", resources)
	t.Cleanup(r.resetMapRenderResourceResidency)
	return r
}

func TestSkyCleanupIdleDoesNotRebuildOtherResources(t *testing.T) {
	for _, count := range []int{64, 512} {
		for _, sky := range []string{"empty", "current", "fading", "retained"} {
			t.Run(fmt.Sprintf("%d/%s", count, sky), func(t *testing.T) {
				r := idleSkyRenderer(t, count)
				var keep map[string]struct{}
				if sky != "empty" {
					img := ebiten.NewImage(4, 4)
					r.game.skyPanoramaCache = map[string]*ebiten.Image{sky: img}
					switch sky {
					case "current":
						r.game.skyPanorama = img
					case "fading":
						r.game.skyPanoramaPrev = img
					case "retained":
						keep = map[string]struct{}{sky: {}}
					}
					t.Cleanup(func() { r.game.skyPanorama = nil; r.game.skyPanoramaPrev = nil; r.deallocateUnusedSkyPanoramas(nil) })
				}
				if n := testing.AllocsPerRun(100, func() { r.deallocateUnusedSkyPanoramas(keep) }); n != 0 {
					t.Fatalf("unchanged sky ownership allocated %g times per tick", n)
				}
				if len(r.standeeCoreCache) != count {
					t.Fatal("sky cleanup released unrelated resident standees")
				}
			})
		}
	}
}

func TestSkyCleanupRetainsSharedAllocationUntilFinalOwner(t *testing.T) {
	for _, owner := range []string{"current", "fading", "region"} {
		t.Run(owner, func(t *testing.T) {
			r := idleSkyRenderer(t, 1)
			img := ebiten.NewImage(4, 4)
			r.game.skyPanoramaCache = map[string]*ebiten.Image{"keep": img, "alias": img}
			var keep map[string]struct{}
			switch owner {
			case "current":
				r.game.skyPanorama = img
			case "fading":
				r.game.skyPanoramaPrev = img
			case "region":
				keep = map[string]struct{}{"keep": {}}
			}
			released := 0
			r.mapRenderRegistry.release = func(releasedImage *ebiten.Image) {
				if releasedImage == img {
					released++
				}
				releasedImage.Deallocate()
			}
			r.deallocateUnusedSkyPanoramas(keep)
			if r.game.skyPanoramaCache["keep"] != img || released != 0 {
				t.Fatal("sky lost a live owner through its unowned alias")
			}
			r.game.skyPanorama = nil
			r.game.skyPanoramaPrev = nil
			r.deallocateUnusedSkyPanoramas(nil)
			r.deallocateUnusedSkyPanoramas(nil)
			if len(r.game.skyPanoramaCache) != 0 || released != 1 {
				t.Fatalf("final release: skies=%d releases=%d", len(r.game.skyPanoramaCache), released)
			}
		})
	}
}

func BenchmarkMapRenderSkyRetentionIdle(b *testing.B) {
	for _, count := range []int{64, 256, 512} {
		b.Run(fmt.Sprintf("standees=%d", count), func(b *testing.B) {
			r := idleSkyRenderer(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r.deallocateUnusedSkyPanoramas(nil)
			}
		})
	}
}
