//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
)

// Exercise the production inventory draw, not only its text helper. Labels
// must retain native glyph pixels at the layout's chosen integer scale.
func TestDebugSim_InventoryTypographyUsesWholePixels(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{800, 600}, {800, 680}, {1024, 768}, {1280, 720}, {1920, 1080}, {3840, 2160}} {
		for _, balances := range [][2]int{{0, 5}, {12345678, 1234}} {
			t.Run(fmt.Sprintf("%v/%v", size, balances), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				g.showPartyStats = true
				g.menuOpen, g.currentTab = true, TabInventory
				g.party.Members[0].Name = "Gareth"
				g.party.Gold, g.party.Food = balances[0], balances[1]
				fp := installFakePointer(t)
				fp.moveTo(0, 0)
				l := computeInventoryContentLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content)
				if size[1] <= 768 && l.textScale != 1 {
					t.Fatal("compact inventory headings must use native text size")
				}
				labels := []struct {
					text string
					r    layoutRect
					col  color.RGBA
				}{
					{"Gareth", l.headings[0], rarityGold},
					{"Personal bag", l.headings[1], rarityGold},
					{"Shared bag", l.headings[2], rarityGold},
					{fmt.Sprintf("Gold %d", balances[0]), l.resources[0], color.RGBA{255, 255, 255, 255}},
					{fmt.Sprintf("Food %d", balances[1]), l.resources[1], color.RGBA{255, 255, 255, 255}},
				}
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.ui.Draw(h.screen)
					got := snapshotUIImage(h.screen)
					for _, label := range labels {
						r := label.r
						want := ebiten.NewImage(r.w, r.h)
						want.Fill(got.RGBAAt(r.x, r.y))
						b := outlinedLabelImage(label.text, label.col).Bounds()
						if b.Dx()*l.textScale > r.w || b.Dy()*l.textScale > r.h {
							t.Errorf("%q does not fit its reserved row", label.text)
						}
						drawScaledMetalCenteredText(want, label.text, r.w/2, r.h/2, float64(l.textScale), label.col)
						pixels := snapshotUIImage(want)
						for y := 0; y < r.h; y++ {
							i, j := got.PixOffset(r.x, r.y+y), pixels.PixOffset(0, y)
							if !bytes.Equal(got.Pix[i:i+r.w*4], pixels.Pix[j:j+r.w*4]) {
								t.Errorf("%q differs from crisp integer-scale text at row %d", label.text, y)
								break
							}
						}
						want.Deallocate()
					}
				})
			})
		}
	}
}

func TestDebugSim_PersonalInventory(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	mw, mh := MinimumWindowSize()
	for _, size := range [][2]int{{mw, mh}, {800, 600}, {1024, 768}, {1280, 720}, {1920, 1080}, {3840, 2160}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			previous := config.GlobalIconFrames
			config.GlobalIconFrames = nil
			t.Cleanup(func() { config.GlobalIconFrames = previous })
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			t.Chdir("../..")
			if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
				t.Fatal(err)
			}
			g.sprites = graphics.NewSpriteManager()
			g.showPartyStats = true
			g.menuOpen, g.currentTab, g.selectedChar = true, TabInventory, 0
			hero := g.party.Members[0]
			hero.Inventory = nil
			g.party.Inventory = nil
			for i, key := range []string{"health_potion", "mana_potion", "antivenom", "wolf_pelt", "spider_silk", "dawnleaf", "mooncap", "harm_flask", "fire_flask", "iron_sword", "carp_scale", "koi_scale", "ruby", "revival_potion", "leather_armor", "magic_dagger"} {
				it, err := items.TryCreateItemFromYAML(key)
				if err != nil {
					continue
				}
				it.Quantity = 1
				if it.Stackable() {
					it.Quantity = 12 + i
				}
				g.party.AddItem(it)
				if i%2 == 0 {
					g.party.Bag(hero).Add(it)
				}
			}
			// Also exercise a full bag and its second page without duplicate stacking.
			extra := items.CreateWeaponFromYAML("iron_sword")
			if extra.Name != "" {
				for i := 0; i < 17; i++ {
					g.party.Inventory = append(g.party.Inventory, extra)
				}
			}
			for i, count := range []int{1, 12, 123, 1001} {
				it := items.CreateItemFromYAML("health_potion")
				it.Quantity = count
				hero.QuickSlots[i] = &it
			}
			shortcut, _ := config.FlaskItem("harm_flask")
			hero.QuickSlots[4] = &shortcut
			fp := installFakePointer(t)
			fp.moveTo(0, 0)
			capture := func(suffix string) {
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.ui.Draw(h.screen)
					path := filepath.Join(os.TempDir(), fmt.Sprintf("personal_inventory_%dx%d%s.png", size[0], size[1], suffix))
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
			}
			capture("")
			if size[0] == 800 || size[0] == 1024 || size[0] == 1920 {
				h.ui.setInventoryTab(inventoryTabOwning(items.Item{Type: items.ItemConsumable}), hero)
				h.ui.setInventoryTab(inventoryTabOwning(items.Item{Type: items.ItemWeapon}))
				h.ui.inventoryPage = 1
				capture("_filtered")
			}

		})
	}
}

func TestDebugSim_ItemCountsUseNativeMetalText(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{800, 600}, {800, 680}, {1024, 768}, {1280, 720}, {1920, 1080}, {3840, 2160}} {
		for _, count := range []int{0, 1, 12, 123, 1001} {
			for _, flask := range []bool{false, true} {
				if count == 0 && !flask {
					continue
				}
				t.Run(fmt.Sprintf("%v/count=%d/flask=%v", size, count, flask), func(t *testing.T) {
					h := newDisplayedModalHarness(t, size[0], size[1])
					g := h.g
					g.showPartyStats = true
					g.menuOpen, g.currentTab = true, TabInventory
					hero := g.party.Members[0]
					fp := installFakePointer(t)
					fp.moveTo(0, 0)
					key := "health_potion"
					if flask {
						key = "harm_flask"
					}
					stock := items.CreateItemFromYAML(key)
					stock.Quantity = count
					quick := stock
					if flask {
						quick, _ = config.FlaskItem(key)
					}
					// Render an otherwise identical icon without quantity text as the
					// background oracle. The quest type has no stack count or equip tint.
					bare := stock
					bare.Type, bare.Quantity = items.ItemQuest, 1
					if itemTooltipIconName(bare) != itemTooltipIconName(stock) || itemTooltipIconName(bare) != itemTooltipIconName(quick) {
						t.Fatal("count fixture must retain the same icon")
					}
					setStock := func(item items.Item) {
						hero.Inventory, g.party.Inventory = nil, nil
						if count > 0 {
							hero.Inventory = []items.Item{item}
							g.party.Inventory = []items.Item{item}
						}
					}
					content := computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content
					l := computeInventoryContentLayout(content)
					_, slots := quickSlotRects(l.quickSlots.x, l.quickSlots.y, l.quickSlots.w)
					quickCount := count
					if flask {
						quickCount *= 2 // One physical stack in each carried bag.
					}
					checks := []struct {
						r    image.Rectangle
						n    int
						show bool
					}{{slots[0], quickCount, true}}
					if count > 0 {
						for _, grid := range []layoutRect{l.personalGrid, l.grid} {
							x, y, w, h := scaleInventorySourceRect(grid.x, grid.y, grid.w, grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
							pad := max(1, w/16)
							checks = append(checks, struct {
								r    image.Rectangle
								n    int
								show bool
							}{image.Rect(x+pad, y+pad, x+w-pad, y+h-pad), count, count > 1})
						}
					}
					runOnDrawFrame(func(_ *ebiten.Image) {
						hero.QuickSlots[0] = &bare
						setStock(bare)
						h.screen.Fill(color.RGBA{50, 60, 70, 255})
						h.ui.drawInventoryContent(h.screen, content)
						want := ebiten.NewImage(size[0], size[1])
						defer want.Deallocate()
						want.DrawImage(h.screen, nil)
						for _, check := range checks {
							if !check.show {
								continue
							}
							glyph := outlinedLabelImage(fmt.Sprint(check.n), itemCountMetal)
							b := glyph.Bounds()
							if b.Dx() > check.r.Dx() || b.Dy() > check.r.Dy() {
								t.Errorf("native count %v does not fit icon %v", b, check.r)
							}
							op := &ebiten.DrawImageOptions{}
							op.GeoM.Translate(float64(check.r.Max.X-b.Dx()), float64(check.r.Max.Y-b.Dy()))
							want.DrawImage(glyph, op)
						}
						hero.QuickSlots[0] = &quick
						setStock(stock)
						h.screen.Fill(color.RGBA{50, 60, 70, 255})
						h.ui.drawInventoryContent(h.screen, content)
						got, expected := snapshotUIImage(h.screen), snapshotUIImage(want)
						for i, check := range checks {
							for y := check.r.Min.Y; y < check.r.Max.Y; y++ {
								x := got.PixOffset(check.r.Min.X, y)
								end := x + check.r.Dx()*4
								if !bytes.Equal(got.Pix[x:end], expected.Pix[x:end]) {
									t.Errorf("surface %d count %d differs from native metallic digits over the unchanged icon (row %d)", i, check.n, y)
									break
								}
							}
						}
					})
				})
			}
		}
	}
}
