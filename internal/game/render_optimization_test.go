package game

import (
	"fmt"
	"image"
	"math"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// All caches here are transient: save/load must rebuild them, never persist
// camera projections, visibility candidates, source views or texture ownership.
func TestPatternPlanCachePreservesGeometryAndBounds(t *testing.T) {
	pf := testPeriodicFrame()
	src := ebiten.NewImage(512, 512)
	defer src.Deallocate()
	var cache patternPlanCache
	for _, size := range [][2]int{{20, 20}, {800, 600}, {1900, 952}, {3820, 2032}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			want, ok := planPatternFrame(pf, size[0], size[1], 6)
			got := cache.get("frame", src, pf, size[0], size[1], 6)
			if got.ok != ok || len(got.ops) != len(want) {
				t.Fatal("cached geometry changed")
			}
			for i, op := range want {
				v := got.ops[i]
				if v.part.Bounds() != image.Rect(op.sx, op.sy, op.sx+op.sw, op.sy+op.sh) || v.dx != op.dx || v.dy != op.dy || v.dw != op.dw || v.dh != op.dh {
					t.Fatalf("blit %d changed", i)
				}
			}
			if n := testing.AllocsPerRun(100, func() { cache.get("frame", src, pf, size[0], size[1], 6) }); n != 0 {
				t.Fatalf("unchanged panel allocates %g", n)
			}
		})
	}
	for i := 0; i < 80; i++ {
		cache.get("frame", src, pf, 1900+i, 952, 6)
	}
	if cache.bytes > patternPlanCacheBytes || len(cache.entries) > patternPlanCacheEntries {
		t.Fatal("resize cache is unbounded")
	}
	replacement := ebiten.NewImage(512, 512)
	defer replacement.Deallocate()
	cache.get("frame", replacement, pf, 1900, 952, 6)
	for _, p := range cache.entries {
		if p.source == src {
			t.Fatal("replacement retains obsolete source")
		}
	}
}

func TestPatternDrawReusesPlan(t *testing.T) {
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	screen := ebiten.NewImage(800, 600)
	defer screen.Deallocate()
	ui.drawPatternFrame(screen, "menu_panel_frame", 0, 0, 800, 600, generatedPatternFrameSlice)
	if len(ui.patternPlans.entries) != 1 {
		t.Fatal("draw bypasses plan cache")
	}
	plan := ui.patternPlans.entries[0]
	ui.drawPatternFrame(screen, "menu_panel_frame", 10, 10, 800, 600, generatedPatternFrameSlice)
	if len(ui.patternPlans.entries) != 1 || ui.patternPlans.entries[0] != plan {
		t.Fatal("screen position rebuilt immutable frame plan")
	}
}

func TestResidentNoOpDoesNotRebuildManifest(t *testing.T) {
	for _, count := range []int{0, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			resources := testMapRenderResources()
			for i := 0; i < count; i++ {
				name := fmt.Sprint(i)
				resources.standees[standeeCoreKey{name: name}] = struct{}{}
				resources.sources[mapRenderSourceKey{name: name}] = struct{}{}
			}
			r := &Renderer{game: &MMGame{}, mapRenderResidentMapKeys: []string{"keep"}, mapRenderResourcesByMap: map[string]*mapRenderRegionResources{"keep": resources}}
			keep := map[string]struct{}{"keep": {}}
			if n := testing.AllocsPerRun(100, func() { r.evictMapRenderResidencyOutside(keep); r.deallocateUnusedSkyPanoramas(nil) }); n != 0 {
				t.Fatalf("unchanged residency allocated %g", n)
			}
			if r.mapRenderResourcesByMap["keep"] != resources {
				t.Fatal("unchanged ownership replaced")
			}
		})
	}
}

func TestSkyRetentionWithoutFullManifest(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		resident, active, stale, cancelled, want bool
	}{
		{name: "resident", resident: true, want: true},
		{name: "active", active: true, want: true},
		{name: "stale", active: true, stale: true},
		{name: "cancelled", active: true, cancelled: true},
		{name: "unowned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sky := ebiten.NewImage(8, 8)
			r := &Renderer{game: &MMGame{skyPanoramaCache: map[string]*ebiten.Image{"sky": sky}}, mapRenderGeneration: 2}
			resources := testMapRenderResources()
			resources.skies["sky"] = struct{}{}
			if tc.resident {
				r.mapRenderResidentMapKeys = []string{"keep"}
				r.mapRenderResourcesByMap = map[string]*mapRenderRegionResources{"keep": resources}
			}
			if tc.active {
				task := &mapRenderPrewarmTask{generation: 2, prewarmer: &mapRenderPrewarmer{resources: resources}}
				if tc.stale {
					task.generation = 1
				}
				if tc.cancelled {
					task.state = mapRenderTaskCancelled
				}
				r.mapRenderResourcePrewarmActive = task
			}
			r.deallocateUnusedSkyPanoramas(nil)
			if (r.game.skyPanoramaCache["sky"] != nil) != tc.want {
				t.Fatal("sky ownership changed")
			}
			if tc.want {
				sky.Deallocate()
			}
		})
	}
}

func TestRenderSpatialIndexConservativeAndStable(t *testing.T) {
	source := []TransparentSpriteData{}
	for y := -32; y <= 32; y++ {
		for x := -32; x <= 32; x++ {
			source = append(source, TransparentSpriteData{worldX: float64(x*64 + 32), worldY: float64(y*64 + 32)})
		}
	}
	var index renderSpatialIndex
	for _, tc := range []struct{ x, y, radius float64 }{{0, 0, 1}, {1023, 1025, 64}, {-1024, -1024, 320}, {0, 0, 3200}, {8192, 8192, 64}} {
		got := index.query(source, tc.x, tc.y, tc.radius, 64)
		seen := make(map[int]bool)
		for pos, i := range got {
			if pos > 0 && got[pos-1] >= i {
				t.Fatal("candidate order changed")
			}
			seen[i] = true
		}
		for i, v := range source {
			// Include the most extreme adjacent-wall position as well as centre.
			for _, off := range []float64{-64, 0, 64} {
				if math.Hypot(v.worldX+off-tc.x, v.worldY-tc.y) <= tc.radius && !seen[i] {
					t.Fatalf("missed visible candidate %d", i)
				}
			}
		}
		if n := testing.AllocsPerRun(50, func() { index.query(source, tc.x, tc.y, tc.radius, 64) }); n != 0 {
			t.Fatalf("query allocated %g", n)
		}
	}
	replaced := []TransparentSpriteData{{worldX: 32, worldY: 32}}
	if got := index.query(replaced, 0, 0, 64, 64); !reflect.DeepEqual(got, []int{0}) {
		t.Fatal("source replacement not indexed")
	}
	if got := index.query(nil, 0, 0, 64, 64); len(got) != 0 {
		t.Fatal("old world retained")
	}
}

func TestRenderCameraBasisInvalidatesOnInputs(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 2, 2))
	r := &Renderer{game: g}
	for _, tc := range []struct{ angle, fov float64 }{{0, 1}, {1, 1}, {1, 2}, {-1, 2}, {0, 1}} {
		g.camera.Angle, g.camera.FOV = tc.angle, tc.fov
		b := r.cameraBasis()
		if b.dirX != math.Cos(tc.angle) || b.dirY != math.Sin(tc.angle) || b.planeX != math.Cos(tc.angle+math.Pi/2)*math.Tan(tc.fov/2) {
			t.Fatal("stale camera basis")
		}
	}
}
