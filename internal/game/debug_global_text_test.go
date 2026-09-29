//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

// Compare production entry points with the raw font mask and the existing metal
// ramp. A flat fill in the shared renderer or a heading bypass must fail here.
func TestDebugSim_AllTextMetallic(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, tint := range []color.Color{color.White, color.RGBA{63, 171, 99, 255}, color.NRGBA{221, 67, 141, 255}, color.NRGBA{221, 67, 141, 128}, color.RGBA{37, 80, 20, 128}, rarityGold, focusModeMetal, itemCountMetal} {
		for _, entry := range []string{"native", "reading", "center", "fade", "hidden"} {
			t.Run(fmt.Sprintf("%v/%s", tint, entry), func(t *testing.T) {
				runOnDrawFrame(func(_ *ebiten.Image) {
					const text = "HMWX"
					w, h := debugTextWidth(text)+2, debugTextCharHeight
					mask := ebiten.NewImage(w, h)
					defer mask.Deallocate()
					ebitenutil.DebugPrintAt(mask, text, -1, 0)
					font := snapshotUIImage(mask)
					dst := ebiten.NewImage(320, 120)
					defer dst.Deallocate()
					x, y, scale, alpha := 20, 20, 1, 1.0
					switch entry {
					case "native":
						if tint == color.White {
							drawDebugText(dst, text, x, y)
						} else {
							drawDebugTextColored(dst, text, x, y, tint)
						}
					case "reading":
						scale = readingTextScale
						drawReadingText(dst, text, x-scale, y-scale, tint)
					case "center", "fade", "hidden":
						scale = 4
						x, y = 160-(w+2)*scale/2+scale, 60-(h+2)*scale/2+scale
						if entry == "center" {
							drawScaledCenteredText(dst, text, 160, 60, float64(scale), tint)
						} else {
							alpha = 0.5
							if entry == "hidden" {
								alpha = 0
							}
							drawScaledMetalCenteredTextAlpha(dst, text, 160, 60, float64(scale), tint, alpha)
						}
					}
					got := snapshotUIImage(dst)
					straight := color.NRGBAModel.Convert(tint).(color.NRGBA)
					base := color.RGBA{straight.R, straight.G, straight.B, 255}
					bodyAlpha := float64(straight.A) / 255 * alpha
					checked := 0
					for sy := 0; sy < h; sy++ {
						shade := metalShade(base, float64(sy/2*2+1)/float64(h))
						want := color.RGBA{uint8(float64(shade.R) * bodyAlpha), uint8(float64(shade.G) * bodyAlpha), uint8(float64(shade.B) * bodyAlpha), uint8(255 * alpha)}
						for sx := 0; sx < w; sx++ {
							if font.RGBAAt(sx, sy).A != 255 {
								continue
							}
							pixel := got.RGBAAt(x+sx*scale, y+sy*scale)
							near := func(a, b byte) bool { return absInt(int(a)-int(b)) <= 1 }
							// The black outline can add coverage behind a translucent
							// glyph, but it cannot change its premultiplied RGB tint.
							alphaOK := near(pixel.A, want.A)
							if straight.A < 255 {
								alphaOK = int(pixel.A) >= int(255*bodyAlpha)-1 && int(pixel.A) <= int(255*alpha)+1
							}
							if !near(pixel.R, want.R) || !near(pixel.G, want.G) || !near(pixel.B, want.B) || !alphaOK {
								t.Errorf("glyph (%d,%d) = %v, want metal tint %v", sx, sy, pixel, want)
								return
							}
							checked++
						}
					}
					if checked < 20 {
						t.Error("font oracle did not cover enough glyph pixels")
					}
				})
			})
		}
	}
}

func TestDebugSim_FlaskCounterUsesDisplayedHero(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		for _, width := range []int{300, 500} {
			t.Run(fmt.Sprintf("%s/%d", key, width), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 800, 680)
				g := h.g
				g.selectedChar = 1
				for _, hero := range g.party.Members {
					hero.Inventory = nil
				}
				stock := items.CreateItemFromYAML(key)
				stock.Quantity = 14
				g.party.Members[0].Inventory = []items.Item{stock}
				stock.Quantity = 27
				g.party.Inventory = []items.Item{stock}
				stock.Quantity = 99
				g.party.Members[1].Inventory = []items.Item{stock}
				quick, _ := config.FlaskItem(key)
				bare := stock
				bare.Type, bare.Quantity = items.ItemQuest, 1
				_, slots := quickSlotRects(20, 20, width)
				runOnDrawFrame(func(_ *ebiten.Image) {
					g.party.Members[0].QuickSlots[0] = &bare
					h.screen.Clear()
					h.ui.drawQuickSlotBar(h.screen, 0, 20, 20, width, false)
					want := ebiten.NewImage(800, 680)
					defer want.Deallocate()
					want.DrawImage(h.screen, nil)
					glyph := outlinedLabelImage("41", itemCountMetal)
					b, r := glyph.Bounds(), slots[0]
					op := &ebiten.DrawImageOptions{}
					op.GeoM.Translate(float64(r.Max.X-b.Dx()), float64(r.Max.Y-b.Dy()))
					want.DrawImage(glyph, op)
					g.party.Members[0].QuickSlots[0] = &quick
					h.screen.Clear()
					h.ui.drawQuickSlotBar(h.screen, 0, 20, 20, width, false)
					got, expected := snapshotUIImage(h.screen), snapshotUIImage(want)
					for y := r.Min.Y; y < r.Max.Y; y++ {
						i := got.PixOffset(r.Min.X, y)
						if !bytes.Equal(got.Pix[i:i+r.Dx()*4], expected.Pix[i:i+r.Dx()*4]) {
							t.Errorf("quick-slot count must show displayed hero's 14 + shared 27, excluding selected hero's 99 (row %d)", y)
							break
						}
					}
				})
			})
		}
	}
}

func TestDebugSim_RosterInventoryWarning(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{800, 680}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			t.Chdir("../..")
			g.sprites = graphics.NewSpriteManager()
			g.rosterScreenOpen = true
			hero := g.party.Members[0]
			hero.Inventory = []items.Item{items.CreateItemFromYAML("black_dragon_statuette"), {Name: "Master key", Attributes: map[string]int{"master_key": 1}}}
			g.party.Reserve = append(g.party.Reserve, g.party.Members[1])
			g.requestRosterSwap(0, len(g.party.Reserve)-1)
			runOnDrawFrame(func(_ *ebiten.Image) {
				h.ui.Draw(h.screen)
				path := filepath.Join(os.TempDir(), fmt.Sprintf("roster_warning_%dx%d.png", size[0], size[1]))
				f, err := os.Create(path)
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				if err = png.Encode(f, snapshotUIImage(h.screen)); err != nil {
					t.Error(err)
				}
				t.Log(path)
			})
		})
	}
}
