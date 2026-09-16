//go:build debug

package game

import (
	"fmt"
	"image"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// A static UI image must retain its texel selection across integer translations
// and live frames. Cover UI art families at native, reduced and enlarged sizes.
// Integer translations also expose precision-sensitive texture coordinates.
func TestDebugSim_UISamplingStable(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	sprites := graphics.NewSpriteManager()
	for _, name := range []string{"spellbook_open", "trap_recipe_book_open", "inventory_paperdoll_panel", "inventory_grid_panel", "party_member_panel", "icon_spell_firebolt", "theme_trim_gold", "theme_trim_silver", "theme_trim_bronze"} {
		if !sprites.HasSprite(name) {
			t.Fatalf("missing sampling fixture %s", name)
		}
		for _, scale := range []float64{0.5, 1, 1.1875, 1.25} {
			t.Run(fmt.Sprintf("%s/%g", name, scale), func(t *testing.T) {
				var source, dst *ebiten.Image
				var w, h int
				runOnDrawFrame(func(_ *ebiten.Image) {
					source = sprites.GetSprite(name)
					w, h = int(float64(source.Bounds().Dx())*scale), int(float64(source.Bounds().Dy())*scale)
					dst = ebiten.NewImage(w+100, h+100)
				})
				defer dst.Deallocate()
				var baseline *image.RGBA
				for frame := 0; frame < 12; frame++ {
					var got *image.RGBA
					x, y := (frame%4)*19, (frame%3)*23
					runOnDrawFrame(func(_ *ebiten.Image) {
						dst.Clear()
						drawImageScaled(dst, source, x, y, w, h)
						got = snapshotUIImage(dst)
					})
					if baseline == nil {
						baseline = got
						continue
					}
					changed, maxDelta := 0, 0
					for py := 0; py < h; py++ {
						for px := 0; px < w; px++ {
							a, b := baseline.RGBAAt(px, py), got.RGBAAt(px+x, py+y)
							d := max(abs(int(a.R)-int(b.R)), abs(int(a.G)-int(b.G)), abs(int(a.B)-int(b.B)), abs(int(a.A)-int(b.A)))
							maxDelta = max(maxDelta, d)
							if d > 2 {
								changed++
							}
						}
					}
					if changed > 0 {
						t.Fatalf("frame=%d offset=%d,%d: %d pixels moved to another texel; max channel delta=%d", frame, x, y, changed, maxDelta)
					}
				}
			})
		}
	}
}

func TestDebugSim_FrameSamplingStable(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	for style, spec := range interfaceFrames {
		for _, size := range [][2]int{{117, 32}, {1000, 24}, {37, 23}, {800, 600}} {
			t.Run(fmt.Sprintf("%s/%v", spec.name, size), func(t *testing.T) {
				var dst *ebiten.Image
				runOnDrawFrame(func(_ *ebiten.Image) { dst = ebiten.NewImage(size[0]+100, size[1]+100) })
				defer dst.Deallocate()
				var baseline *image.RGBA
				for frame := 0; frame < 6; frame++ {
					x, y := 19*(frame%4), 23*(frame%3)
					var got *image.RGBA
					runOnDrawFrame(func(_ *ebiten.Image) {
						dst.Clear()
						ui.drawThemeFrame(dst, interfaceFrame(style), x, y, size[0], size[1])
						got = snapshotUIImage(dst)
					})
					if baseline == nil {
						baseline = got
						continue
					}
					for py := 0; py < size[1]; py++ {
						for px := 0; px < size[0]; px++ {
							a, b := baseline.RGBAAt(px, py), got.RGBAAt(px+x, py+y)
							if max(abs(int(a.R)-int(b.R)), abs(int(a.G)-int(b.G)), abs(int(a.B)-int(b.B)), abs(int(a.A)-int(b.A))) > 2 {
								t.Fatalf("frame changed at (%d,%d), offset (%d,%d): %v vs %v", px, py, x, y, a, b)
							}
						}
					}
				}
			})
		}
	}
}
