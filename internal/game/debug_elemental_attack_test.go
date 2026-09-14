//go:build debug

package game

import (
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"ugataima/internal/monster"
)

// Export the exact runtime glyph and its world/portrait wiring, with no asset
// generator or alternate animation implementation involved.
func TestDebugSim_ElementalAttackGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	out := os.Getenv("RAM_ELEMENTAL_QA_DIR")
	if out == "" {
		t.Skip("set RAM_ELEMENTAL_QA_DIR for image output")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.world.Monsters = nil
	ts := g.config.GetTileSize()
	m := monster.NewMonster3DFromConfig(g.camera.X+math.Cos(g.camera.Angle)*ts*1.5, g.camera.Y+math.Sin(g.camera.Angle)*ts*1.5, "goblin", g.config)
	m.HitPoints, m.MaxHitPoints = 1000, 1000
	g.world.Monsters = []*monster.Monster3D{m}
	school := g.monsterEffectContext(m).ElementalSchool
	g.addMonsterElementalAttackFX(m, school)
	g.addElementalAttackFX(0, 0, g.party.Members[0], school)
	frames := g.elementalAttackEffects[0].Frames
	save := func(name string, im *ebiten.Image) {
		f, err := os.Create(filepath.Join(out, name))
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		if err := png.Encode(f, im); err != nil {
			t.Error(err)
		}
	}
	for age := 0; age < frames; age++ {
		runOnDrawFrame(func(*ebiten.Image) {
			grid := ebiten.NewImage(600, 450)
			defer grid.Deallocate()
			grid.Fill(color.RGBA{20, 24, 29, 255})
			for i, school := range []string{"fire", "air", "water", "earth", "spirit", "mind", "body", "light", "dark"} {
				x, y := float64(i%3*200+100), float64(i/3*150+75)
				fx := elementalAttackEffect{School: school, Age: age, Frames: frames, Particles: g.config.Graphics.ElementalAttack.ParticleCount}
				drawElementalAttackGlyph(grid, fx, x, y, 42)
				ebitenutil.DebugPrintAt(grid, school, int(x)-18, int(y)+52)
			}
			save(fmt.Sprintf("elements_%02d.png", age), grid)
		})
	}
	for _, size := range [][2]int{{1280, 720}, {1600, 900}} {
		for _, age := range []int{frames / 5, frames / 3, frames * 2 / 3} {
			runOnDrawFrame(func(*ebiten.Image) {
				w, h := g.gameLoop.Layout(size[0], size[1])
				dst := ebiten.NewImage(w, h)
				defer dst.Deallocate()
				for i := range g.elementalAttackEffects {
					g.elementalAttackEffects[i].Age = age
				}
				r.RenderFirstPersonView(dst)
				g.gameLoop.ui.Draw(dst)
				visible := false
				for _, s := range r.unifiedSprites {
					if s.monster != m {
						continue
					}
					visible = true
					break
				}
				if !visible {
					t.Error("gallery monster was not projected")
				}
				save(fmt.Sprintf("game_%dx%d_%02d.png", w, h, age), dst)
			})
		}
	}
}

// World projection data must not create a second source of game tooltips.
func TestDebugSim_MonstersDoNotAddHoverTooltips(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live render harness")
	}
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	for _, state := range []string{"melee", "ranged", "champion", "dead", "occluded", "modal"} {
		t.Run(state, func(t *testing.T) {
			runOnDrawFrame(func(*ebiten.Image) {
				w, h := g.gameLoop.Layout(1280, 720)
				dst := ebiten.NewImage(w, h)
				defer dst.Deallocate()
				m := monster.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "goblin", g.config)
				g.mainMenuOpen = state == "modal"
				switch state {
				case "ranged":
					m.ProjectileSpell = "fireball"
				case "champion":
					m.ChampionKey = "weapon_master"
				case "dead":
					m.HitPoints = 0
				}
				g.depthBuffer = make([]float64, w)
				for i := range g.depthBuffer {
					g.depthBuffer[i] = math.Inf(1)
					if state == "occluded" {
						g.depthBuffer[i] = 1
					}
				}
				r.unifiedSprites = nil
				ui := g.gameLoop.ui
				ui.Draw(dst)
				before := append([]string(nil), ui.tooltipLines...)
				x, y := ebiten.CursorPosition()
				r.unifiedSprites = []UnifiedSpriteRenderData{{monster: m, screenXF: float64(x), sizeF: 100, bottomF: float64(y + 50), depthPerp: 10}}
				ui.Draw(dst)
				if !reflect.DeepEqual(before, ui.tooltipLines) {
					t.Errorf("projected monster changed game tooltip: before %q, after %q", before, ui.tooltipLines)
				}
			})
		})
	}
}
