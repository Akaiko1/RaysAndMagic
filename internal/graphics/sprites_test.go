package graphics

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetSpriteVariants(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "base only",
			files:    []string{"grass.png"},
			expected: []string{"grass"},
		},
		{
			name:     "stops at first missing numbered variant",
			files:    []string{"grass.png", "grass3.png", "grass128.png"},
			expected: []string{"grass"},
		},
		{
			name:     "uses continuous numbered variants",
			files:    []string{"grass.png", "grass0.png", "grass1.png", "grass2.png"},
			expected: []string{"grass", "grass0", "grass1", "grass2"},
		},
		{
			name:     "ignores variants after a gap",
			files:    []string{"grass.png", "grass0.png", "grass2.png"},
			expected: []string{"grass", "grass0"},
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			spriteDir := filepath.Join(tempDir, "assets", "sprites", "environment")
			if err := os.MkdirAll(spriteDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, file := range tt.files {
				if err := os.WriteFile(filepath.Join(spriteDir, file), []byte{}, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chdir(tempDir); err != nil {
				t.Fatal(err)
			}

			sm := NewSpriteManager()
			got := sm.GetSpriteVariants("grass")
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("variants = %v, want %v", got, tt.expected)
			}
		})
	}
}

// applyColorKey must zero the alpha of pixels within tolerance of the key color
// (default magenta), leave others untouched, and no-op when disabled.
func TestApplyColorKey(t *testing.T) {
	const ( // pixel x-positions
		magenta        = 0 // (255,0,255) exact key -> transparent core
		darkEdge       = 1 // dark magenta fringe from generated PNG edges
		fringe         = 2 // (170,40,170) magenta-tinted edge, OUTSIDE the core
		green          = 3 // unrelated colour, must survive
		skin           = 4 // (200,150,100) warm tone, must survive
		interiorPurple = 5 // purple-ish interior colour; magenta is not allowed in-game
		blue           = 6 // non-magenta blue, must survive
	)
	mk := func() *image.NRGBA {
		im := image.NewNRGBA(image.Rect(0, 0, 8, 1))
		im.SetNRGBA(magenta, 0, color.NRGBA{255, 0, 255, 255})
		im.SetNRGBA(darkEdge, 0, color.NRGBA{30, 2, 29, 255})
		im.SetNRGBA(fringe, 0, color.NRGBA{170, 40, 170, 255})
		im.SetNRGBA(green, 0, color.NRGBA{10, 200, 30, 255})
		im.SetNRGBA(skin, 0, color.NRGBA{200, 150, 100, 255})
		im.SetNRGBA(interiorPurple, 0, color.NRGBA{35, 20, 90, 255})
		im.SetNRGBA(blue, 0, color.NRGBA{20, 35, 90, 255})
		im.SetNRGBA(7, 0, color.NRGBA{20, 80, 20, 255})
		return im
	}

	// Despill OFF: the core key erases; the fringe is left untouched (it's outside
	// the tolerance core, so the binary key never touches it).
	sm := NewSpriteManager()
	sm.SetColorKey(true, 255, 0, 255, 60, false)
	off := sm.applyColorKey("", mk()).(*image.NRGBA)
	if off.NRGBAAt(magenta, 0).A != 0 {
		t.Errorf("exact magenta must be transparent")
	}
	if p := off.NRGBAAt(fringe, 0); p != (color.NRGBA{170, 40, 170, 255}) {
		t.Errorf("despill off: fringe must be untouched, got %v", p)
	}

	// Despill ON: core still erased; fringe keeps its base tone but loses the
	// magenta cast (R,B drop to the green level) and stays opaque - edge not eaten.
	sm.SetColorKey(true, 255, 0, 255, 60, true)
	on := sm.applyColorKey("", mk()).(*image.NRGBA)
	if on.NRGBAAt(magenta, 0).A != 0 {
		t.Errorf("exact magenta must be transparent")
	}
	if p := on.NRGBAAt(fringe, 0); p != (color.NRGBA{40, 40, 40, 255}) {
		t.Errorf("despill: fringe should lose the magenta cast but stay opaque (want {40,40,40,255}), got %v", p)
	}
	if p := on.NRGBAAt(darkEdge, 0); p != (color.NRGBA{3, 2, 2, 255}) {
		t.Errorf("despill: dark edge fringe should lose the magenta cast (want {3,2,2,255}), got %v", p)
	}
	if p := on.NRGBAAt(interiorPurple, 0); p != (color.NRGBA{20, 20, 75, 255}) {
		t.Errorf("despill: interior magenta hue should be stripped too (want {20,20,75,255}), got %v", p)
	}
	for _, x := range []int{green, skin, blue} {
		if got, want := on.NRGBAAt(x, 0), mk().NRGBAAt(x, 0); got != want {
			t.Errorf("non-magenta pixel at x=%d must be preserved, want %v got %v", x, want, got)
		}
	}

	// Disabled -> unchanged.
	sm.SetColorKey(false, 255, 0, 255, 60, true)
	if got := sm.applyColorKey("", mk()).At(magenta, 0); got != (color.NRGBA{255, 0, 255, 255}) {
		t.Errorf("disabled key must not alter pixels, got %v", got)
	}
}

func TestApplyColorKey_GameSpriteColorSafety(t *testing.T) {
	spritePaths := []string{
		"../../assets/sprites/environment/nature/deep_jungle_fern.png",
		"../../assets/sprites/environment/nature/deep_jungle_log.png",
		"../../assets/sprites/environment/nature/forest_oak.png",
		"../../assets/sprites/environment/nature/old_platan.png",
		"../../assets/sprites/environment/nature/palm.png",
		"../../assets/sprites/environment/nature/firefly_swarm.png",
		"../../assets/sprites/environment/props/stone_lantern.png",
		"../../assets/sprites/environment/walls/deep_jungle_wall_0.png",
		"../../assets/sprites/interface/spells/icon_spell_firebolt.png",
		"../../assets/sprites/interface/spells/icon_spell_fireball.png",
		"../../assets/sprites/interface/spells/icon_spell_inferno.png",
		"../../assets/sprites/interface/spells/icon_spell_ice_bolt.png",
		"../../assets/sprites/interface/spells/icon_spell_ray_of_light.png",
		"../../assets/sprites/interface/spells/icon_spell_bless.png",
		"../../assets/sprites/interface/weapons/icon_weapon_gold_sword.png",
		"../../assets/sprites/interface/weapons/icon_weapon_bow_of_hellfire.png",
		"../../assets/sprites/interface/items/icon_item_golden_idol.png",
		"../../assets/sprites/interface/items/icon_item_red_dragon_statuette.png",
		"../../assets/sprites/interface/items/icon_item_gold_dragon_statuette.png",
		"../../assets/sprites/interface/ui/spellbook_tab_fire.png",
		"../../assets/sprites/interface/ui/spellbook_tab_water.png",
		"../../assets/sprites/mobs/ashigaru_firelock.png",
		"../../assets/sprites/mobs/dragon_red.png",
		"../../assets/sprites/mobs/dragon_gold.png",
		"../../assets/sprites/mobs/dragon_green.png",
		"../../assets/sprites/mobs/goblin.png",
		"../../assets/sprites/mobs/jungle_goblin.png",
		"../../assets/sprites/mobs/old_samurai.png",
		"../../assets/sprites/mobs/puma.png",
	}

	sm := NewSpriteManager()
	sm.SetColorKey(true, 255, 0, 255, 60, true)

	for _, path := range spritePaths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src := loadPNGForColorKeyTest(t, path)
			filtered := sm.applyColorKey("", src).(*image.NRGBA)
			b := src.Bounds()
			changedNonMagenta := 0
			magentaAfter := 0
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					before := src.NRGBAAt(x, y)
					after := filtered.NRGBAAt(x, y)
					if isColorKeyCandidate(before) {
						if isVisibleMagentaHue(after) {
							magentaAfter++
						}
						continue
					}
					if before != after {
						changedNonMagenta++
						if changedNonMagenta <= 5 {
							t.Logf("non-magenta pixel changed at (%d,%d): %v -> %v", x, y, before, after)
						}
					}
				}
			}
			if changedNonMagenta != 0 {
				t.Fatalf("changed %d non-magenta pixels", changedNonMagenta)
			}
			if magentaAfter != 0 {
				t.Fatalf("left %d visible magenta-hue pixels after filtering", magentaAfter)
			}
		})
	}
}

func loadPNGForColorKeyTest(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

func isColorKeyCandidate(p color.NRGBA) bool {
	if p.A == 0 {
		return true
	}
	if absByteDiff(p.R, 255) <= 60 && p.G <= 60 && absByteDiff(p.B, 255) <= 60 {
		return true
	}
	return isVisibleMagentaHue(p)
}

func isVisibleMagentaHue(p color.NRGBA) bool {
	if p.A == 0 {
		return false
	}
	return int(minByte(p.R, p.B))-int(p.G) > despillHueFloor
}

func absByteDiff(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

func minByte(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}
