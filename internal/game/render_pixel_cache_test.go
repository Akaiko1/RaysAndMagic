package game

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestStandeePreparationWorkerDiskCache(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
		tint float64
	}{
		{"small", 17, 9, 0}, {"transparent", 64, 64, 0.65}, {"bounded_source", 1100, 600, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := image.NewRGBA(image.Rect(3, 5, 3+tc.w, 5+tc.h))
			for i := 0; i < len(cpu.Pix); i += 4 {
				a := byte((i / 4) % 256)
				cpu.Pix[i], cpu.Pix[i+1], cpu.Pix[i+2], cpu.Pix[i+3] = a/3, a/2, a, a
			}
			want := prepareStandeePixels(cpu, tc.tint, true)
			cache := graphics.PixelCache{Dir: t.TempDir()}
			for _, phase := range []string{"cold", "restart", "corrupt"} {
				if phase == "corrupt" {
					files, _ := filepath.Glob(filepath.Join(cache.Dir, "*.rgba"))
					for _, f := range files {
						os.WriteFile(f, []byte("bad"), 0600)
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				results := prepareMapRenderStandees(ctx, []mapRenderStandeeJob{{cpu: cpu}}, tc.tint, cache, graphics.NewPreparationBudget(32<<20))
				select {
				case result := <-results:
					if !reflect.DeepEqual(result.prepared, want) {
						t.Fatalf("%s pixels differ", phase)
					}
					result.lease.Release()
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				cancel()
			}
			if same := prepareCachedStandeePixels(context.Background(), cache, cpu, tc.tint+0.1); !reflect.DeepEqual(same, prepareStandeePixels(cpu, tc.tint+0.1, true)) {
				t.Fatal("tint reused")
			}
		})
	}
}

func TestFloorAtlasDiskCache(t *testing.T) {
	for _, dims := range []image.Point{{X: 8, Y: 8}, {X: 7, Y: 5}, {X: 16, Y: 8}} {
		textures := []floorTexture{{width: dims.X, height: dims.Y, pixels: make([]byte, dims.X*dims.Y*4)}}
		for i := range textures[0].pixels {
			textures[0].pixels[i] = byte(i * 7)
		}
		cache := graphics.PixelCache{Dir: t.TempDir()}
		for _, phase := range []string{"cold", "restart", "source_edit"} {
			if phase == "source_edit" {
				textures[0].pixels[0] ^= 127
			}
			got, w, h, m := prepareCachedFloorAtlas(context.Background(), cache, textures)
			want, ww, wh, wm := prepareFloorAtlas(textures)
			if w != ww || h != wh || m != wm || got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("%s %v changed", phase, dims)
			}
		}
	}
}

func TestDemandUploadsUseSharedBoundedBatch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sizes []image.Point
		want  int
	}{
		{"empty", nil, 0}, {"small_batch", []image.Point{{X: 16, Y: 16}, {X: 32, Y: 32}, {X: 1, Y: 1}}, 3},
		{"bytes", []image.Point{{X: 1024, Y: 1024}, {X: 1024, Y: 1024}, {X: 1, Y: 1}}, 2},
		{"oversized_makes_progress", []image.Point{{X: 2048, Y: 2048}, {X: 1, Y: 1}}, 1},
		{"count", make([]image.Point, 40), 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := &gameLoadingState{}
			for _, size := range tc.sizes {
				img := ebiten.NewImage(max(1, size.X), max(1, size.Y))
				defer img.Deallocate()
				l.uploads = append(l.uploads, img)
			}
			original := append([]*ebiten.Image(nil), l.uploads...)
			dst := &recordingMapRenderUploadDestination{}
			l.submitDemandUploads(dst)
			if len(dst.images) != tc.want || len(l.uploads) != len(original)-tc.want {
				t.Fatalf("submitted %d remaining %d", len(dst.images), len(l.uploads))
			}
			for i, img := range dst.images {
				if img != original[i] || dst.options[i].ColorScale.A() != 0 {
					t.Fatal("order or transparent draw changed")
				}
			}
			for len(l.uploads) > 0 {
				l.submitDemandUploads(dst)
			}
			if len(dst.images) != len(original) {
				t.Fatal("queue did not drain exactly once")
			}
		})
	}
}
