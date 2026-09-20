package graphics

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/config"
)

// Rule: one black-backed icon gets exactly one semantic frame, regardless of
// load entry point. Cells: three styles x default/rare/legendary x synchronous
// and worker decoding; reload, alpha art and legacy passthrough. Save data is
// unchanged: presentation is re-derived from current content on every load.
func TestContentIconPreparation(t *testing.T) {
	t.Chdir("../..")
	oldItems, oldFrames := config.GlobalItems, config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalItems, config.GlobalIconFrames = oldItems, oldFrames })
	config.GlobalItems = &config.ItemSystemConfig{Items: map[string]*config.ItemDefinitionConfig{"test": {Rarity: "common"}}}
	frames := map[string]string{}
	for _, s := range []string{"basic", "asian", "boss"} {
		frames[s] = "assets/sprites/interface/icon_frames/icon_frame_" + s + ".png"
	}
	art := image.NewRGBA(image.Rect(0, 0, 128, 128))
	draw.Draw(art, image.Rect(32, 32, 96, 96), image.NewUniform(color.RGBA{180, 60, 30, 255}), image.Point{}, draw.Src)
	path := filepath.Join(t.TempDir(), "art.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, art); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, style := range []string{"basic", "asian", "boss"} {
		for _, rarity := range []string{"common", "uncommon", "rare", "legendary", "unique"} {
			t.Run(style+"/"+rarity, func(t *testing.T) {
				config.GlobalItems.Items["test"].Rarity = rarity
				config.GlobalIconFrames = &config.IconFramesConfig{Frames: frames, Icons: map[string]string{"icon_item_test": style}}
				sm := NewSpriteManager()
				request := SpriteResourceRequest{Name: "icon_item_test"}
				sm.spritePaths = map[string]string{request.Name: path}
				sm.spriteDirType = map[string]string{request.Name: "interface"}
				want := sm.decodePreparedResource(request)
				if !want.Found {
					t.Fatal("icon not prepared")
				}
				if got := want.CPU.RGBAAt(64, 24); got != (color.RGBA{0, 0, 0, 255}) {
					t.Fatalf("background: %v", got)
				}
				if got := want.CPU.RGBAAt(64, 64); got != art.RGBAAt(64, 64) {
					t.Fatalf("art changed: %v", got)
				}
				tint, _ := config.IconFrameColor(request.Name)
				mask := sm.iconFrames[request.Name].mask
				colored := 0
				for y := 0; y < 128; y++ {
					for x := 0; x < 128; x++ {
						alpha := mask.RGBAAt(x, y).A
						if alpha > 32 && (x < 8 || x >= 120 || y < 8 || y >= 120) {
							got := want.CPU.RGBAAt(x, y)
							expected := color.RGBA{uint8(uint32(tint.R) * uint32(alpha) / 255), uint8(uint32(tint.G) * uint32(alpha) / 255), uint8(uint32(tint.B) * uint32(alpha) / 255), 255}
							if math.Abs(float64(got.R)-float64(expected.R)) > 1 || math.Abs(float64(got.G)-float64(expected.G)) > 1 || math.Abs(float64(got.B)-float64(expected.B)) > 1 || got.A != expected.A {
								t.Fatalf("frame tint at %d,%d: %v, want %v", x, y, got, expected)
							}
							colored++
						}
					}
				}
				if colored < 128 {
					t.Fatal("frame disappeared")
				}
				for result := range sm.PrepareResources(context.Background(), []SpriteResourceRequest{request}) {
					if !result.Found || !bytes.Equal(want.CPU.Pix, result.CPU.Pix) {
						t.Fatal("background path differs")
					}
					result.QueueLease.Release()
				}
				fresh := NewSpriteManager().decodePreparedResourceAtPath(request, path)
				if !bytes.Equal(want.CPU.Pix, fresh.CPU.Pix) {
					t.Fatal("reload differs")
				}
				if opaque, known := sm.SpriteOpaqueAt(request.Name, 16, 16); !known || !opaque {
					t.Fatal("hit metadata ignored the opaque black backing")
				}
				if b, w, h, known := sm.SpriteVisibleFrameBounds(request.Name); !known || b != image.Rect(0, 0, 128, 128) || w != 128 || h != 128 {
					t.Fatal("visible metadata differs from composed pixels")
				}
				legacy := sm.decodePreparedResourceAtPath(SpriteResourceRequest{Name: "legacy"}, path)
				if !bytes.Equal(legacy.CPU.Pix, art.Pix) {
					t.Fatal("legacy art changed")
				}
			})
		}
	}
}

// Every migrated source must fit its assigned frame before runtime tinting.
// Include one neighboring pixel: filtering must not hide a weapon tip or a
// wide silhouette beneath the rail/corner when the icon is reduced for the HUD.
func TestAuthoredIconsFitAssignedFrames(t *testing.T) {
	t.Chdir("../..")
	data, err := os.ReadFile("assets/icon_frames.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.IconFramesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	read := func(path string) image.Image {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		im, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return im
	}
	masks := map[string]image.Image{}
	for style, path := range cfg.Frames {
		masks[style] = read(path)
	}
	paths := map[string]string{}
	if err := filepath.WalkDir("assets/sprites/interface", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".png") {
			paths[strings.TrimSuffix(d.Name(), ".png")] = path
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for name, style := range cfg.Icons {
		t.Run(name, func(t *testing.T) {
			art, mask := read(paths[name]), masks[style]
			if art.Bounds() != image.Rect(0, 0, 128, 128) {
				t.Fatal("icon must retain its square logical canvas")
			}
			for y := range 128 {
				for x := range 128 {
					r, g, b, a := art.At(x, y).RGBA()
					if a != 65535 {
						t.Fatalf("background is not opaque at %d,%d", x, y)
					}
					if max(r, g, b) <= 32*257 {
						continue
					}
					for my := max(0, y-1); my <= min(127, y+1); my++ {
						for mx := max(0, x-1); mx <= min(127, x+1); mx++ {
							_, _, _, alpha := mask.At(mx, my).RGBA()
							if alpha > 32*257 {
								t.Fatalf("%s frame overlaps art at %d,%d", style, x, y)
							}
						}
					}
				}
			}
		})
	}
}

// Import rule: keep metal rails and square corner ornaments readable at icon
// scale. Whole-canvas reduction previously erased the rail bevels. All styles
// must retain a 3..6 pixel metal band on every side and leave the art center clear.
func TestAuthoredIconFrameMasks(t *testing.T) {
	t.Chdir("../..")
	for _, style := range []string{"basic", "asian", "boss"} {
		t.Run(style, func(t *testing.T) {
			f, err := os.Open(fmt.Sprintf("assets/sprites/interface/icon_frames/icon_frame_%s.png", style))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			im, _, err := image.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Dx() != 128 || im.Bounds().Dy() != 128 {
				t.Fatal("wrong mask size")
			}
			for y := 32; y < 96; y++ {
				for x := 32; x < 96; x++ {
					_, _, _, a := im.At(x, y).RGBA()
					if a != 0 {
						t.Fatalf("frame paints interior at %d,%d", x, y)
					}
				}
			}
			for side := 0; side < 4; side++ {
				visible := 0
				for d := 0; d < 10; d++ {
					x, y := 64, d
					switch side {
					case 1:
						x, y = 127-d, 64
					case 2:
						x, y = 64, 127-d
					case 3:
						x, y = d, 64
					}
					_, _, _, a := im.At(x, y).RGBA()
					if a >= 64*257 {
						visible++
					}
				}
				if visible < 3 || visible > 6 {
					t.Fatalf("rail %d lost authored metal width: %d visible pixels, want 3..6", side, visible)
				}
			}
		})
	}
}

// Every collectible has separate authored resources for its compact physical
// card icon and the full-scene viewer. Keep coverage tied to content, not a
// fixed roster count, so newly authored cards cannot silently lose full art.
func TestCardFullArtAssets(t *testing.T) {
	t.Chdir("../..")
	raw, err := os.ReadFile("assets/items.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var content config.ItemSystemConfig
	if err := yaml.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	for key, def := range content.Items {
		if def.Type != "card" {
			continue
		}
		t.Run(key, func(t *testing.T) {
			for _, tc := range []struct {
				prefix, folder string
				minSize        int
			}{
				{"icon_item_", "items", 128}, {"full_art_", "cards", 512},
			} {
				f, err := os.Open(filepath.Join("assets/sprites/interface", tc.folder, tc.prefix+key+".png"))
				if err != nil {
					t.Fatal(err)
				}
				dims, err := png.DecodeConfig(f)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if dims.Width != dims.Height || dims.Width < tc.minSize {
					t.Fatalf("%s: invalid artwork size %dx%d", tc.prefix, dims.Width, dims.Height)
				}
			}
		})
	}
}
