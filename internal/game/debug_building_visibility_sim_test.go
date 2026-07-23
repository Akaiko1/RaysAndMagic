//go:build debug

package game

// Headless regression for the vanishing-building report: standing on a
// grid-span facade's footprint (clock tower, pyramid) and turning so its
// anchor leaves the view cone must never drop the whole building. Renders a
// full turn of angles from every footprint tile of every grid-span NPC on
// the unified world and requires at least one collected facade segment at
// every angle (the camera plane always keeps half of the standing tile's
// slice in front).
//
// Run with:
//
//	RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_BuildingVisibility -v
import (
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_BuildingVisibility(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	ts := cfg.GetTileSize()

	buildings := 0
	for _, npc := range wm.OpenWorld.NPCs {
		if npc.GridSpanTiles < 2 {
			continue
		}
		buildings++
		// Region tracking must follow the camera or per-map render config
		// (sky, floor groups) would disagree with the building's map.
		tiles := g.buildingFootprintTiles(npc)
		if len(tiles) == 0 {
			t.Errorf("building %q has no footprint tiles", npc.Name)
			continue
		}
		for ti, c := range tiles {
			g.camera.X, g.camera.Y = c[0], c[1]
			g.syncOpenWorldRegion()
			// The +2 offset skips the exact facade-perpendicular angles: with
			// the camera ON the facade line the slab plane passes through the
			// eye there - a zero-width slice with genuinely nothing to draw.
			for deg := 2; deg < 360; deg += 5 {
				g.camera.Angle = float64(deg) * math.Pi / 180
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen.Clear()
					r.RenderFirstPersonView(screen)
				})
				segments := 0
				for _, s := range r.unifiedSprites {
					if s.npc == npc {
						segments++
					}
				}
				if segments == 0 {
					t.Errorf("building %q vanished: footprint tile %d (%.0f,%.0f) angle %d has 0 segments",
						npc.Name, ti, c[0]/ts, c[1]/ts, deg)
				}
			}
		}
	}
	if buildings == 0 {
		t.Fatal("no grid-span buildings on the unified world - the sweep tested nothing")
	}
	t.Logf("swept %d buildings, all footprint tiles, 72 angles each", buildings)
}
