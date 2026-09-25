//go:build debug

package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
)

// Every in-world pixel of the circular viewport must contain map content.
// Radius changes reveal more tiles at a fixed scale; live markers use the same
// viewport. Derived caches are rebuilt on first draw after load, not serialized.
func TestCompassMapFillsViewport(t *testing.T) {
	requireStandeeGPU(t)
	g, _, tile := tbBehaviorGame(t, 40, 40)
	g.sprites = nil
	ui := &UISystem{game: g}
	for _, radius := range []int{36, 43, 64} {
		for _, edge := range []bool{false, true} {
			t.Run(fmt.Sprintf("radius%d/edge%v", radius, edge), func(t *testing.T) {
				player := 20
				if edge {
					player = 0
				}
				g.camera.X, g.camera.Y = (float64(player)+0.5)*tile, (float64(player)+0.5)*tile
				g.world.NPCs = nil
				runOnDrawFrame(func(*ebiten.Image) {
					ui.rebuildCompassTileLayer(player, player, radius)
					side := radius * 2
					pixels := make([]byte, side*side*4)
					ui.compassTileLayer.ReadPixels(pixels)
					for y := 0; y < side; y++ {
						for x := 0; x < side; x++ {
							dx, dy := float64(x)+0.5-float64(radius), float64(y)+0.5-float64(radius)
							distance2 := dx*dx + dy*dy
							alpha := pixels[(y*side+x)*4+3]
							inWorld := !edge || (dx >= -3 && dy >= -3)
							if distance2 < float64((radius-1)*(radius-1)) && inWorld && alpha < 230 {
								t.Errorf("unfilled map pixel (%d,%d): alpha=%d", x, y, alpha)
								return
							}
							if (distance2 > float64((radius+1)*(radius+1)) || !inWorld) && alpha != 0 {
								t.Errorf("map escaped viewport/world at (%d,%d): alpha=%d", x, y, alpha)
								return
							}
						}
					}
				})
				for _, npc := range []struct {
					name   string
					dx, dy int
				}{
					{"near", 3, 0}, {"far", 10, 0}, {"diagonal", 7, 7}, {"outside", 12, 0},
				} {
					t.Run(npc.name, func(t *testing.T) {
						runOnDrawFrame(func(*ebiten.Image) {
							side := radius*2 + 16
							target := ebiten.NewImage(side, side)
							defer target.Deallocate()
							g.world.NPCs = nil
							ui.drawCompassMinimap(target, radius+8, radius+8, radius)
							before := make([]byte, target.Bounds().Dx()*target.Bounds().Dy()*4)
							target.ReadPixels(before)
							cached := ui.compassTileLayer
							g.world.NPCs = []*character.NPC{{X: (float64(player+npc.dx) + 0.5) * tile, Y: (float64(player+npc.dy) + 0.5) * tile}}
							target.Clear()
							ui.drawCompassMinimap(target, radius+8, radius+8, radius)
							after := make([]byte, len(before))
							target.ReadPixels(after)
							changed := false
							for i := range before {
								changed = changed || before[i] != after[i]
							}
							want := npc.name == "near" || (radius == 64 && npc.name != "outside")
							if changed != want {
								t.Errorf("NPC visible=%v want %v", changed, want)
							}
							if cached != ui.compassTileLayer {
								t.Error("live marker rebuilt static map")
							}
						})
					})
				}
			})
		}
	}
	ui.releaseCompassFrame()
}
