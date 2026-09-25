//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/monster"
)

func TestDebugSim_MonsterPointerRenderedFrames(t *testing.T) {
	requireStandeeGPU(t)
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	fp := installFakePointer(t)
	g.world.Monsters = nil
	ts := float64(g.config.GetTileSize())
	for _, standee := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, key := range []string{"goblin", "dragon_green"} {
				t.Run(fmt.Sprintf("standee=%v/TB=%v/%s", standee, tb, key), func(t *testing.T) {
					g.config.Graphics.Standee.Enabled = standee
					g.turnBasedMode = tb
					m := monster.NewMonster3DFromConfig(g.camera.X+math.Cos(g.camera.Angle)*ts*2, g.camera.Y+math.Sin(g.camera.Angle)*ts*2, key, g.config)
					g.world.Monsters = []*monster.Monster3D{m}
					for _, direction := range []float64{0, math.Pi} {
						m.Direction = direction
						fp.moveTo(-1, -1)
						runOnDrawFrame(func(_ *ebiten.Image) {
							dst := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
							defer dst.Deallocate()
							r.RenderFirstPersonView(dst)
							g.gameLoop.ui.Draw(dst)
							if len(r.monsterPick.hits) != 1 {
								t.Errorf("draw did not publish one target: %d", len(r.monsterPick.hits))
								return
							}
							hit := r.monsterPick.hits[0]
							if hit.standee != standee {
								t.Errorf("unexpected render path: standee=%v", hit.standee)
							}
							if _, known := g.sprites.ImageOpaqueAt(hit.sprite, 0, 0); !known {
								t.Error("displayed animation has no CPU hit mask")
							}
							found := false
							for y := 0; y < dst.Bounds().Dy() && !found; y += 3 {
								for x := 0; x < dst.Bounds().Dx(); x += 3 {
									if g.monsterAtScreen(x, y) == m {
										found = true
										fp.moveTo(x, y)
										break
									}
								}
							}
							if !found {
								t.Error("rendered monster cannot be selected")
							}
							before := snapshotUIImage(dst)
							r.RenderFirstPersonView(dst)
							g.gameLoop.ui.Draw(dst)
							if r.hoveredMonster != m {
								t.Error("hover did not select the displayed monster")
							}
							after := snapshotUIImage(dst)
							brighter := 0
							for y := 0; y < after.Bounds().Dy(); y++ {
								for x := 0; x < after.Bounds().Dx(); x++ {
									if g.monsterAtScreen(x, y) != m {
										continue
									}
									a, b := after.RGBAAt(x, y), before.RGBAAt(x, y)
									if int(a.R)+int(a.G)+int(a.B) > int(b.R)+int(b.G)+int(b.B)+3 {
										brighter++
									}
								}
							}
							if brighter < 20 {
								t.Errorf("hover did not brighten monster pixels: %d", brighter)
							}
							if standee && !tb && key == "goblin" && direction == 0 {
								f, err := os.Create("/tmp/rays-monster-hover.png")
								if err != nil {
									t.Error(err)
								} else {
									if err := png.Encode(f, after); err != nil {
										t.Error(err)
									}
									f.Close()
								}
							}
							// Turning must update hover on this draw, not one draw later.
							angle := g.camera.Angle
							g.camera.Angle += math.Pi
							r.RenderFirstPersonView(dst)
							if r.hoveredMonster != nil {
								t.Error("turned frame retained old hover")
							}
							g.camera.Angle = angle
							r.RenderFirstPersonView(dst)
							if r.hoveredMonster != m {
								t.Error("returning frame delayed hover by one draw")
							}
							// A subsequent draw must discard actors from the preceding frame.
							g.world.Monsters = nil
							r.RenderFirstPersonView(dst)
							if len(r.monsterPick.hits) != 0 {
								t.Error("draw retained stale targets")
							}
							g.world.Monsters = []*monster.Monster3D{m}
						})
					}
				})
			}
		}
	}
}
