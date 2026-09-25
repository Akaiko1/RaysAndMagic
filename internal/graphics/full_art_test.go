package graphics

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Full paintings retain every authored color and their opaque canvas.
// Cases: painting/sprite x color-key on/off x direct/indexed/worker/reload;
// metadata must agree with decoding. Persistence is N/A: assets are reloaded.
func TestFullArtColorPreservation(t *testing.T) {
	art := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(art, art.Bounds(), image.NewUniform(color.RGBA{255, 0, 255, 255}), image.Point{}, draw.Src)
	draw.Draw(art, image.Rect(4, 4, 12, 12), image.NewUniform(color.RGBA{150, 40, 170, 255}), image.Point{}, draw.Src)
	path := filepath.Join(t.TempDir(), "painting.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, art); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, tc := range []struct {
		name          string
		key, preserve bool
	}{
		{"full_art_lich_card", true, true},
		{"full_art_lich_card", false, true},
		{"lich", true, false},
		{"lich", false, true},
	} {
		state := "key-off"
		if tc.key {
			state = "key-on"
		}
		t.Run(tc.name+"/"+state, func(t *testing.T) {
			manager := func() *SpriteManager {
				sm := NewSpriteManager()
				sm.SetColorKey(tc.key, 255, 0, 255, 60, true)
				sm.spritePaths = map[string]string{tc.name: path}
				sm.spriteDirType = map[string]string{tc.name: "interface"}
				return sm
			}
			sm := manager()
			req := SpriteResourceRequest{Name: tc.name}
			check := func(entry string, got PreparedSpriteResource) {
				t.Helper()
				if !got.Found || got.CPU == nil {
					t.Fatalf("%s did not decode", entry)
				}
				if tc.preserve {
					if got.CPU.Bounds() != art.Bounds() || !bytes.Equal(got.CPU.Pix, art.Pix) {
						t.Errorf("%s changed painting colors or opacity", entry)
					}
				} else if got.CPU.RGBAAt(0, 0).A != 0 || got.CPU.RGBAAt(8, 8) == art.RGBAAt(8, 8) {
					t.Errorf("%s stopped cleaning ordinary keyed sprites", entry)
				}
			}
			check("direct", sm.decodePreparedResourceAtPath(req, path))
			check("indexed", sm.decodePreparedResource(req))
			count := 0
			for result := range sm.PrepareResources(context.Background(), []SpriteResourceRequest{req}) {
				check("worker", result)
				result.QueueLease.Release()
				count++
			}
			if count != 1 {
				t.Fatalf("worker returned %d results", count)
			}
			check("reload", manager().decodePreparedResource(req))
			if opaque, known := manager().SpriteOpaqueAt(tc.name, 0, 0); !known || opaque != tc.preserve {
				t.Errorf("opacity metadata differs: opaque=%t known=%t", opaque, known)
			}
			wantBounds := image.Rect(4, 4, 12, 12)
			if tc.preserve {
				wantBounds = art.Bounds()
			}
			if bounds, _, _, known := manager().SpriteVisibleFrameBounds(tc.name); !known || bounds != wantBounds {
				t.Errorf("visible bounds = %v known=%t, want %v", bounds, known, wantBounds)
			}
		})
	}
}
