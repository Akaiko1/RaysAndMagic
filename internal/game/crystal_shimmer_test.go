package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"math"
	"os"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestCrystalShimmerCadence(t *testing.T) {
	s := &character.CrystalShimmer{IntervalSeconds: 5, DurationSeconds: 1.4, Crystals: make([]character.FacadeCrystal, 4)}
	for _, tps := range []int{30, 60, 120} {
		for _, seconds := range []float64{0, 4.9, 6.5, 9.9, 11.5} {
			if _, strength := crystalShimmerAt(s, int64(seconds*float64(tps)), tps, 123); strength != 0 {
				t.Fatalf("unexpected pulse at %v", seconds)
			}
		}
		seen := map[int]bool{}
		for interval := 1; interval <= 100; interval++ {
			frame := int64(interval*5*tps + tps/2)
			index, strength := crystalShimmerAt(s, frame, tps, 123)
			next, _ := crystalShimmerAt(s, frame+int64(tps/4), tps, 123)
			if strength <= 0 || index != next {
				t.Fatal("missing or unstable pulse")
			}
			seen[index] = true
		}
		if len(seen) != 4 {
			t.Fatal("random choice does not cover all gems")
		}
	}
}

func TestSolsticeEntranceWallPlacement(t *testing.T) {
	t.Chdir("../..")
	for _, stitched := range []bool{false, true} {
		g, wm, _ := bootOpenWorldGame(t, stitched)
		for _, key := range []string{"deep_jungle", "solstice_approach"} {
			if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			target := "solstice_jungle_entrance"
			if key == "solstice_approach" {
				target = "solstice_vestibule_entrance"
			}
			found := false
			for _, npc := range g.world.NPCs {
				if npc.Key != target {
					continue
				}
				found = true
				dx, dy := g.buildingWallOffset(npc)
				ts := float64(g.config.GetTileSize())
				if math.Abs(math.Hypot(dx, dy)-ts/2) > 1e-6 {
					t.Fatalf("%s stitched=%v: no flush wall offset", key, stitched)
				}
				for _, tile := range g.buildingFootprintTiles(npc) {
					if !world.GlobalTileManager.IsSolid(g.world.GetTileAt(tile[0]+2*dx, tile[1]+2*dy)) {
						t.Fatal("gap behind facade")
					}
					if world.GlobalTileManager.IsSolid(g.world.GetTileAt(tile[0]-2*dx, tile[1]-2*dy)) {
						t.Fatal("blocked approach")
					}
				}
				if npc.CrystalShimmer == nil || len(npc.CrystalShimmer.Crystals) != 4 {
					t.Fatal("missing crystal data")
				}
			}
			if stitched && key == "deep_jungle" {
				if found {
					t.Fatal("stitched jungle passage retained its split-map facade")
				}
			} else if !found {
				t.Fatalf("missing facade in %s (%v)", key, wm.FailedMaps)
			}
		}
	}
}

func TestCrystalShimmerLightsOnlySelectedSocket(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live GPU draw harness")
	}
	g := &MMGame{config: &config.Config{}, frameCount: 330}
	// The game config supplies its normal 60 TPS default.
	g.frameCount = int64(g.config.GetTPS()*5 + g.config.GetTPS()/2)
	npc := &character.NPC{CrystalShimmer: &character.CrystalShimmer{IntervalSeconds: 5, DurationSeconds: 1.4, Crystals: []character.FacadeCrystal{
		{Center: [2]float64{.25, .5}, Radius: [2]float64{.1, .2}, Color: [3]int{255, 70, 30}},
		{Center: [2]float64{.75, .5}, Radius: [2]float64{.1, .2}, Color: [3]int{30, 150, 255}},
	}}}
	r := &Renderer{game: g}
	selected, _ := crystalShimmerAt(npc.CrystalShimmer, g.frameCount, g.config.GetTPS(), 0)
	runOnDrawFrame(func(_ *ebiten.Image) {
		src, dst := ebiten.NewImage(200, 100), ebiten.NewImage(200, 200)
		defer src.Deallocate()
		defer dst.Deallocate()
		src.Fill(color.RGBA{30, 30, 30, 255})
		dst.Fill(color.RGBA{30, 30, 30, 255})
		r.drawBillboardCrystalShimmer(dst, npc, src, 0, 0, 200)
		pixels := image.NewRGBA(dst.Bounds())
		dst.ReadPixels(pixels.Pix)
		for i, c := range npc.CrystalShimmer.Crystals {
			p := pixels.RGBAAt(int(c.Center[0]*200), int(c.Center[1]*200))
			lit := p.R > 30 || p.G > 30 || p.B > 30
			if lit != (i == selected) {
				t.Errorf("socket %d selected=%d color=%v", i, selected, p)
			}
		}
	})
}
