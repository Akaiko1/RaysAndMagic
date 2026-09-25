//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/quests"
)

// Contract: generated perimeter only, uniform fill, no extra highlight border.
// Cases cover all metals, buttons, locked/earned profile cards and odd geometry.
// Text uses the banner's cached glyphs at integer scale. Persistence is N/A.
func TestDebugSim_UIFrameAndTextCorrections(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	for _, size := range [][2]int{{280, 28}, {351, 93}, {800, 500}} {
		for _, style := range []interfaceFrame{frameGold, frameSilver, frameBronze} {
			t.Run(fmt.Sprintf("frame/%d/%v", style, size), func(t *testing.T) {
				var problem string
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(size[0], size[1])
					defer dst.Deallocate()
					ui.drawThemeFrame(dst, style, 0, 0, size[0], size[1])
					pixels := snapshotUIImage(dst)
					for y := 6; y < size[1]-6; y++ {
						for x := 6; x < size[0]-6; x++ {
							if pixels.RGBAAt(x, y) != interfacePanelFill {
								problem = "textured or doubled interior"
								return
							}
						}
					}
				})
				if problem != "" {
					t.Fatal(problem)
				}
			})
		}
	}
	for _, active := range []bool{false, true} {
		for _, kind := range []string{"profile", "button"} {
			t.Run(fmt.Sprintf("%s/active=%v", kind, active), func(t *testing.T) {
				var maxDelta int
				runOnDrawFrame(func(_ *ebiten.Image) {
					got, want := ebiten.NewImage(240, 100), ebiten.NewImage(240, 100)
					defer got.Deallocate()
					defer want.Deallocate()
					style := frameSilver
					if kind == "profile" {
						ui.drawProfileCard(got, layoutRect{0, 0, 240, 100}, active)
					} else {
						style = frameBronze
						ui.drawButtonFrame(got, 0, 0, 240, 100, active)
					}
					// Buttons retain the bronze rail on hover and brighten
					// it; profile cards switch metal when earned.
					var tint ebiten.ColorScale
					if active && kind == "profile" {
						style = frameGold
					}
					if active && kind == "button" {
						tint.Scale(1.35, 1.35, 1.35, 1)
					}
					ui.drawThemeFrameTint(want, style, 0, 0, 240, 100, tint)
					a, b := snapshotUIImage(got).Pix, snapshotUIImage(want).Pix
					for i, v := range a {
						d := int(v) - int(b[i])
						if d < 0 {
							d = -d
						}
						maxDelta = max(maxDelta, d)
					}
				})
				if maxDelta > 2 {
					t.Fatalf("control added another border over the shared frame (delta %d)", maxDelta)
				}
			})
		}
	}
	for _, withIcon := range []bool{false, true} {
		t.Run(fmt.Sprintf("tooltip/icon=%v", withIcon), func(t *testing.T) {
			var problem string
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(500, 180)
				defer dst.Deallocate()
				icon := ""
				if withIcon {
					icon = "icon_weapon_iron_sword"
				}
				drawTooltip(dst, []string{"Iron Sword", "A plain item description."}, nil, nil, nil, icon, 10, 10, 490, ui.game.sprites)
				p := snapshotUIImage(dst)
				for _, point := range []image.Point{{10, 10}, {11, 11}, {12, 14}} {
					if p.RGBAAt(point.X, point.Y) != (color.RGBA{30, 30, 60, 255}) {
						problem = "tooltip acquired a frame or textured backdrop"
					}
				}
			})
			if problem != "" {
				t.Fatal(problem)
			}
		})
	}
	for _, col := range []color.RGBA{rarityGold, {224, 214, 190, 255}} {
		t.Run(fmt.Sprintf("text/%v", col), func(t *testing.T) {
			var previous []byte
			for frame := 0; frame < 4; frame++ {
				var pixels []byte
				var problem string
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst, want := ebiten.NewImage(500, 80), ebiten.NewImage(500, 80)
					defer dst.Deallocate()
					const label = "Quest Journal"
					drawReadingText(dst, label, 10, 10, col)
					b := outlinedLabelImage(label, col).Bounds()
					drawScaledMetalCenteredText(want, label, 10+b.Dx(), 10+b.Dy(), 2, col)
					pixels = snapshotUIImage(dst).Pix
					if !bytes.Equal(pixels, snapshotUIImage(want).Pix) {
						problem = "reading text differs from the popup text path"
					}
					p := snapshotUIImage(dst)
					for y := 10; y < 10+b.Dy()*2; y += 2 {
						for x := 10; x < 10+b.Dx()*2; x += 2 {
							c := p.RGBAAt(x, y)
							if c != p.RGBAAt(x+1, y) || c != p.RGBAAt(x, y+1) || c != p.RGBAAt(x+1, y+1) {
								problem = "uneven scaled glyph pixels"
								return
							}
						}
					}
				})
				if problem != "" {
					t.Fatal(problem)
				}
				if previous != nil && !bytes.Equal(previous, pixels) {
					t.Fatal("static label changed between frames")
				}
				previous = pixels
			}
		})
	}
}

// Presentation state is derived, never persisted. Cover every journal rank,
// every roster tier, and both tab states through their production draw calls.
func TestDebugSim_JournalRarityAndTabPresentation(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{config: cfg, sprites: graphics.NewSpriteManager()}}
	var rims []color.RGBA
	for _, tc := range []struct {
		name              string
		complete, claimed bool
		buttons           int
	}{
		{"active", false, false, 0}, {"ready", true, false, 1}, {"concluded", true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &quests.Quest{ID: tc.name, Definition: &quests.QuestDefinition{Name: "A quest", Description: "Find the relic.", Type: "kill", TargetCount: 3}, Completed: tc.complete, RewardsClaimed: tc.claimed}
			var rim color.RGBA
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(620, 200)
				defer dst.Deallocate()
				ui.displayedInput.commands = nil
				ui.displayedInput.building = true
				copy := questCardCopyFor(q.Description(), 600, 2)
				ui.drawJournalEntry(dst, q, layoutRect{10, 10, 600, copy.height}, copy)
				p := snapshotUIImage(dst)
				rim = p.RGBAAt(11, 40)
				if p.RGBAAt(609, 40) != rim {
					t.Error("right quest rail is missing")
				}
			})
			if len(ui.displayedInput.commands) != tc.buttons {
				t.Fatalf("claim bindings=%d want %d", len(ui.displayedInput.commands), tc.buttons)
			}
			for _, prior := range rims {
				if prior == rim {
					t.Fatal("quest states have identical rails")
				}
			}
			rims = append(rims, rim)
		})
	}
	pc := newPartyCreateState(cfg)
	heroes := append(pc.pool, pc.slots[:]...)
	seen := map[string]bool{}
	for _, hero := range heroes {
		rarity := hero.cardRarity(cfg)
		if seen[rarity] {
			continue
		}
		seen[rarity] = true
		for _, size := range []int{80, 144} {
			t.Run(fmt.Sprintf("%s/%d", rarity, size), func(t *testing.T) {
				var same bool
				runOnDrawFrame(func(_ *ebiten.Image) {
					r := rect{10, 10, size, size * 496 / 320}
					got := ebiten.NewImage(size+20, r.h+20)
					defer got.Deallocate()
					ui.drawHeroCard(got, hero, r, false)
					c := rarityRGBA(rarity)
					if rarity == "common" {
						c = color.RGBA{190, 130, 72, 255}
					}
					p := snapshotUIImage(got)
					same = true
					for x := 10 + size/2 - 8; x < 10+size/2+8; x++ {
						actual := p.RGBAAt(x, 11)
						if abs(int(actual.R)-int(c.R)) > 2 || abs(int(actual.G)-int(c.G)) > 2 || abs(int(actual.B)-int(c.B)) > 2 {
							same = false
						}
					}
				})
				if !same {
					t.Fatal("hero border does not use the item rarity palette")
				}
			})
		}
	}
	if len(seen) != 4 {
		t.Fatalf("covered %d roster tiers", len(seen))
	}
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprintf("tab/active=%v", active), func(t *testing.T) {
			var clear bool
			runOnDrawFrame(func(_ *ebiten.Image) {
				dst := ebiten.NewImage(140, 60)
				defer dst.Deallocate()
				bg := color.RGBA{90, 110, 150, 255}
				dst.Fill(bg)
				ui.drawDialogTab(dst, layoutRect{10, 10, 110, 32}, active)
				p := snapshotUIImage(dst)
				clear = p.RGBAAt(10, 10) == bg && p.RGBAAt(119, 10) == bg && p.RGBAAt(11, 11) == bg && p.RGBAAt(118, 11) == bg
			})
			if !clear {
				t.Fatal("tab fill leaks into clipped corners")
			}
		})
	}
}
