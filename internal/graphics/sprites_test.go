package graphics

import (
	"image"
	"image/color"
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
		magenta = 0 // (255,0,255) exact key → transparent core
		fringe  = 1 // (170,40,170) magenta-tinted edge, OUTSIDE the core
		green   = 2 // unrelated colour, must survive
		skin    = 3 // (200,150,100) warm tone, must survive
	)
	mk := func() *image.NRGBA {
		im := image.NewNRGBA(image.Rect(0, 0, 4, 1))
		im.SetNRGBA(magenta, 0, color.NRGBA{255, 0, 255, 255})
		im.SetNRGBA(fringe, 0, color.NRGBA{170, 40, 170, 255})
		im.SetNRGBA(green, 0, color.NRGBA{10, 200, 30, 255})
		im.SetNRGBA(skin, 0, color.NRGBA{200, 150, 100, 255})
		return im
	}

	// Despill OFF: the core key erases; the fringe is left untouched (it's outside
	// the tolerance core, so the binary key never touches it).
	sm := NewSpriteManager()
	sm.SetColorKey(true, 255, 0, 255, 60, false)
	off := sm.applyColorKey(mk()).(*image.NRGBA)
	if off.NRGBAAt(magenta, 0).A != 0 {
		t.Errorf("exact magenta must be transparent")
	}
	if p := off.NRGBAAt(fringe, 0); p != (color.NRGBA{170, 40, 170, 255}) {
		t.Errorf("despill off: fringe must be untouched, got %v", p)
	}

	// Despill ON: core still erased; fringe keeps its base tone but loses the
	// magenta cast (R,B drop to the green level) and stays opaque — edge not eaten.
	sm.SetColorKey(true, 255, 0, 255, 60, true)
	on := sm.applyColorKey(mk()).(*image.NRGBA)
	if on.NRGBAAt(magenta, 0).A != 0 {
		t.Errorf("exact magenta must be transparent")
	}
	if p := on.NRGBAAt(fringe, 0); p != (color.NRGBA{40, 40, 40, 255}) {
		t.Errorf("despill: fringe should lose the magenta cast but stay opaque (want {40,40,40,255}), got %v", p)
	}
	for _, x := range []int{green, skin} {
		if got, want := on.NRGBAAt(x, 0), mk().NRGBAAt(x, 0); got != want {
			t.Errorf("non-magenta pixel at x=%d must be preserved, want %v got %v", x, want, got)
		}
	}

	// Disabled → unchanged.
	sm.SetColorKey(false, 255, 0, 255, 60, true)
	if got := sm.applyColorKey(mk()).At(magenta, 0); got != (color.NRGBA{255, 0, 255, 255}) {
		t.Errorf("disabled key must not alter pixels, got %v", got)
	}
}
