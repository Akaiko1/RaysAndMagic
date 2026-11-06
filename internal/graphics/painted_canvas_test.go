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

// A default-mode sprite keeps a painted canvas bit-exact, key-close art inside
// it included, on every load path, while a full-despill family is keyed.
// Cases: default/full x color-key on/off x direct/indexed/worker/reload;
// opacity and bounds metadata must agree with decoding. Persistence is N/A:
// assets are reloaded.
func TestPaintedCanvasColorPreservation(t *testing.T) {
	art := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(art, art.Bounds(), image.NewUniform(color.RGBA{150, 40, 170, 255}), image.Point{}, draw.Src)
	draw.Draw(art, image.Rect(4, 4, 12, 12), image.NewUniform(color.RGBA{255, 0, 255, 255}), image.Point{}, draw.Src)
	const name = "story_plate"
	path := filepath.Join(t.TempDir(), name+".png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, art); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, tc := range []struct {
		full, key, preserve bool
	}{
		{false, true, true},
		{false, false, true},
		{true, true, false},
		{true, false, true},
	} {
		mode := map[bool]string{false: "default", true: "full"}[tc.full]
		state := map[bool]string{false: "key-off", true: "key-on"}[tc.key]
		t.Run(mode+"/"+state, func(t *testing.T) {
			manager := func() *SpriteManager {
				sm := NewSpriteManager()
				sm.SetColorKey(tc.key, 255, 0, 255, 60, true)
				if tc.full {
					sm.SetDespillFull([]string{name})
				}
				sm.spritePaths = map[string]string{name: path}
				sm.spriteDirType = map[string]string{name: "interface"}
				return sm
			}
			sm := manager()
			req := SpriteResourceRequest{Name: name}
			check := func(entry string, got PreparedSpriteResource) {
				t.Helper()
				if !got.Found || got.CPU == nil {
					t.Fatalf("%s did not decode", entry)
				}
				if tc.preserve {
					if got.CPU.Bounds() != art.Bounds() || !bytes.Equal(got.CPU.Pix, art.Pix) {
						t.Errorf("%s changed painting colors or opacity", entry)
					}
				} else if got.CPU.RGBAAt(8, 8).A != 0 || got.CPU.RGBAAt(0, 0) == art.RGBAAt(0, 0) {
					t.Errorf("%s stopped keying a full-despill sprite", entry)
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
			if opaque, known := manager().SpriteOpaqueAt(name, 8, 8); !known || opaque != tc.preserve {
				t.Errorf("opacity metadata differs: opaque=%t known=%t", opaque, known)
			}
			if bounds, _, _, known := manager().SpriteVisibleFrameBounds(name); !known || bounds != art.Bounds() {
				t.Errorf("visible bounds = %v known=%t, want %v", bounds, known, art.Bounds())
			}
		})
	}
}
