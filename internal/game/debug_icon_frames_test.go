//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// Native-size icon QA, not a gameplay-scene prerender. Exercise the actual
// game loader and editor cache, then their shared scaler at HUD/icon sizes.
func TestDebugSim_IconFrameGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live GPU")
	}
	loadTestConfig(t)
	t.Chdir("../..")
	old := config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalIconFrames = old })
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	frames := config.GlobalIconFrames
	var canvas *ebiten.Image
	runOnDrawFrame(func(*ebiten.Image) { canvas = ebiten.NewImage(960, 720); canvas.Fill(color.RGBA{22, 22, 25, 255}) })
	defer runOnDrawFrame(func(*ebiten.Image) { canvas.Deallocate() })
	name := "icon_item_carp_scale"
	definition := config.GlobalItems.Items["carp_scale"]
	oldRarity := definition.Rarity
	t.Cleanup(func() { definition.Rarity = oldRarity })
	for row, style := range []string{"basic", "asian", "boss"} {
		for col, rarity := range []string{"common", "rare", "legendary"} {
			definition.Rarity = rarity
			config.GlobalIconFrames = &config.IconFramesConfig{Frames: frames.Frames, Icons: map[string]string{name: style}}
			var gameSprite *ebiten.Image
			var manager *graphics.SpriteManager
			var editor *graphics.AsyncImageCache
			runOnDrawFrame(func(*ebiten.Image) {
				manager = graphics.NewSpriteManager()
				gameSprite = manager.GetSprite(name)
				editor = graphics.NewAsyncImageCache(1 << 20)
			})
			ready := false
			deadline := time.Now().Add(5 * time.Second)
			for !ready && time.Now().Before(deadline) {
				runOnDrawFrame(func(*ebiten.Image) {
					editor.Advance(256 << 10)
					var editorSprite *ebiten.Image
					editorSprite, ready = editor.Get(name)
					if ready {
						if editorSprite == nil || !bytes.Equal(snapshotUIImage(gameSprite).Pix, snapshotUIImage(editorSprite).Pix) {
							t.Error("game/editor icon mismatch")
							return
						}
						x, y := col*320+16, row*190+34
						drawDebugText(canvas, fmt.Sprintf("%s / %s", style, rarity), x, y-20)
						for i, size := range []int{128, 64, 32} {
							drawImageScaled(canvas, gameSprite, x+i*136, y, size, size)
							if i == 2 {
								drawImageScaled(canvas, gameSprite, x+206, y+74, 24, 24)
							}
						}
					}
				})
			}
			if !ready {
				t.Error("editor icon load timed out")
			}
			runOnDrawFrame(func(*ebiten.Image) { editor.Close(); manager.EvictResource(name, "") })
		}
	}
	config.GlobalIconFrames = frames
	definition.Rarity = oldRarity
	runOnDrawFrame(func(*ebiten.Image) {
		sm := graphics.NewSpriteManager()
		for i, key := range []string{"carp_scale", "koi_scale", "rainbow_salmon_scale"} {
			n := "icon_item_" + key
			x := 16 + i*190
			drawImageScaled(canvas, sm.GetSprite(n), x, 582, 96, 96)
			drawDebugText(canvas, key, x, 685)
			sm.EvictResource(n, "")
		}
		if folder := os.Getenv("RAM_ICON_GALLERY"); folder != "" {
			if err := os.MkdirAll(folder, 0755); err != nil {
				t.Error(err)
				return
			}
			p := filepath.Join(folder, "icon-frame-gallery-native-960x720.png")
			f, err := os.Create(p)
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if err := png.Encode(f, snapshotUIImage(canvas)); err != nil {
				t.Error(err)
			}
		}
	})
}

// Use actual composed assets and the HUD entry point. Plain and timed HUD
// cells must retain the same single outer frame as inventory/book scaling.
func TestDebugSim_MigratedIconCatalog(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live GPU")
	}
	loadTestConfig(t)
	t.Chdir("../..")
	old := config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalIconFrames = old })
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(config.GlobalIconFrames.Icons))
	for name := range config.GlobalIconFrames.Icons {
		names = append(names, name)
	}
	sort.Strings(names)
	runOnDrawFrame(func(*ebiten.Image) {
		sm := graphics.NewSpriteManager()
		ui := &UISystem{game: &MMGame{sprites: sm}}
		canvas := ebiten.NewImage(960, 720)
		defer canvas.Deallocate()
		for start := 0; start < len(names); start += 24 {
			canvas.Fill(color.RGBA{22, 22, 25, 255})
			for i, name := range names[start:min(start+24, len(names))] {
				sprite := sm.GetSprite(name)
				if sprite == nil {
					t.Errorf("missing migrated icon %s", name)
					continue
				}
				x, y := (i%6)*160+8, (i/6)*180+4
				drawImageScaled(canvas, sprite, x, y, 128, 128)
				label := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(name, "icon_item_"), "icon_weapon_"), "icon_spell_"), "icon_trap_")
				drawDebugText(canvas, label, x, y+132)
				ui.drawSpellIcon(canvas, x, y+146, 24, name, "", 0, 0)
				ui.drawSpellIcon(canvas, x+34, y+146, 24, name, "", 50, 100)
				for _, size := range []int{24, 32, 64, 128} {
					plain, hud, timed := ebiten.NewImage(size, size), ebiten.NewImage(size, size), ebiten.NewImage(size, size)
					drawImageScaled(plain, sprite, 0, 0, size, size)
					ui.drawSpellIcon(hud, 0, 0, size, name, "", 0, 0)
					ui.drawSpellIcon(timed, 0, 0, size, name, "", 50, 100)
					want, got, withBar := snapshotUIImage(plain), snapshotUIImage(hud), snapshotUIImage(timed)
					if !bytes.Equal(want.Pix, got.Pix) {
						maxDelta := 0
						for j, v := range want.Pix {
							d := int(v) - int(got.Pix[j])
							maxDelta = max(maxDelta, d, -d)
						}
						// Fractional minification at different GPU atlas offsets
						// can round a filtered channel by one byte.
						if maxDelta > 1 {
							t.Errorf("%s at %d: HUD pixel difference, maximum channel delta %d", name, size, maxDelta)
						}
					}
					inset := sm.ContentIconFrameInset(name, size)
					for py := 0; py < size; py++ {
						for px := 0; px < size; px++ {
							if px >= inset && px < size-inset && py >= inset && py < size-inset {
								continue
							}
							if !iconPixelNear(want.RGBAAt(px, py), withBar.RGBAAt(px, py)) {
								t.Errorf("%s at %d: duration bar covers frame at %d,%d", name, size, px, py)
								break
							}
						}
					}
					plain.Deallocate()
					hud.Deallocate()
					timed.Deallocate()
				}
				sm.EvictResource(name, "")
			}
			if folder := os.Getenv("RAM_ICON_GALLERY"); folder != "" {
				p := filepath.Join(folder, fmt.Sprintf("icon-catalog-native-960x720-%02d.png", start/24+1))
				f, err := os.Create(p)
				if err != nil {
					t.Error(err)
					continue
				}
				if err := png.Encode(f, snapshotUIImage(canvas)); err != nil {
					t.Error(err)
				}
				f.Close()
			}
		}
	})
}

func iconPixelNear(a, b color.RGBA) bool {
	for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
		if d < -1 || d > 1 {
			return false
		}
	}
	return true
}

// Show each inventory card beside its independent full illustration through
// the production loader/scaler. These are asset galleries, not world renders.
func TestDebugSim_CardArtworkGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live GPU")
	}
	loadTestConfig(t)
	t.Chdir("../..")
	old := config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalIconFrames = old })
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for key, def := range config.GlobalItems.Items {
		if def.Type == "card" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	runOnDrawFrame(func(*ebiten.Image) {
		sm := graphics.NewSpriteManager()
		canvas := ebiten.NewImage(960, 880)
		defer canvas.Deallocate()
		for start := 0; start < len(keys); start += 12 {
			canvas.Fill(color.RGBA{22, 22, 25, 255})
			for i, key := range keys[start:min(start+12, len(keys))] {
				iconName, fullName := "icon_item_"+key, "full_art_"+key
				icon, full := sm.GetSprite(iconName), sm.GetSprite(fullName)
				if icon == nil || full == nil {
					t.Errorf("missing icon/full artwork: %s", key)
					continue
				}
				if icon.Bounds().Dx() != 128 || icon.Bounds().Dy() != 128 || full.Bounds().Dx() < 512 || full.Bounds().Dy() < 512 {
					t.Errorf("wrong art resolution: %s", key)
				}
				x, y := i%3*320, i/3*220
				drawImageScaled(canvas, icon, x+4, y+32, 96, 96)
				drawImageScaled(canvas, icon, x+32, y+144, 32, 32)
				drawImageScaled(canvas, full, x+112, y+4, 192, 192)
				drawDebugText(canvas, key, x+4, y+201)
				sm.EvictResource(iconName, "")
				sm.EvictResource(fullName, "")
			}
			if folder := os.Getenv("RAM_ICON_GALLERY"); folder != "" {
				if err := os.MkdirAll(folder, 0755); err != nil {
					t.Error(err)
					return
				}
				f, err := os.Create(filepath.Join(folder, fmt.Sprintf("cards-native-960x880-%02d.png", start/12+1)))
				if err != nil {
					t.Error(err)
					continue
				}
				if err := png.Encode(f, snapshotUIImage(canvas)); err != nil {
					t.Error(err)
				}
				f.Close()
			}
		}
	})
}
