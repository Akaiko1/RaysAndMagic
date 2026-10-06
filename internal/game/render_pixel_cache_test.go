package game

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/storage"
)

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

func TestFloorPreparationPrunesPixelCache(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelled), func(t *testing.T) {
			t.Chdir(t.TempDir())
			cache := graphics.PixelCache{Dir: storage.RenderCacheDir()}
			pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
			floorKey := graphics.PixelCacheKey("floor", pixels)
			standeeKey := graphics.PixelCacheKey("standee", pixels)
			cache.Store(context.Background(), floorKey, []*image.RGBA{pixels})
			cache.Store(context.Background(), standeeKey, []*image.RGBA{pixels, pixels})
			unrelated := filepath.Join(cache.Dir, "keep.rgba")
			if err := os.WriteFile(unrelated, []byte("unrelated"), 0600); err != nil {
				t.Fatal(err)
			}
			orphan := filepath.Join(cache.Dir, ".pixels-abandoned")
			if err := os.WriteFile(orphan, []byte("abandoned"), 0600); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-48 * time.Hour)
			if err := os.Chtimes(orphan, old, old); err != nil {
				t.Fatal(err)
			}
			r := &Renderer{}
			r.startFloorPreparation("prune", nil)
			result := r.floorPreparation.result
			if cancelled {
				r.cancelFloorPreparation()
			}
			for range result {
			}
			if _, err := os.Stat(orphan); !os.IsNotExist(err) {
				t.Fatal("completed preparation task did not prune orphan cache data")
			}
			// Cancellation may precede migration. A later load must retry it.
			if cancelled {
				r.startFloorPreparation("retry", nil)
				for range r.floorPreparation.result {
				}
			}
			if _, ok := cache.Load(context.Background(), floorKey, []image.Point{{X: 4, Y: 4}}); !ok {
				t.Fatal("migration deleted the current floor atlas")
			}
			if _, ok := cache.Load(context.Background(), standeeKey, []image.Point{{X: 4, Y: 4}, {X: 4, Y: 4}}); ok {
				t.Fatal("migration retained the retired standee cache")
			}
			if _, err := os.Stat(unrelated); err != nil {
				t.Fatal("migration deleted an unrelated file")
			}
			marker := filepath.Join(cache.Dir, ".single-image-v1")
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("migration did not record completion")
			}
		})
	}
}

func TestFloorAtlasCacheUsesSettings(t *testing.T) {
	cache := graphics.PixelCache{Dir: t.TempDir()}
	cpu := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range cpu.Pix {
		cpu.Pix[i] = 255
	}
	ctx := context.Background()
	textures := []floorTexture{{width: 16, height: 16, pixels: cpu.Pix}}
	expected, _, _, _ := prepareFloorAtlas(textures)
	settings := fmt.Sprintf("%s:mips=%d", floorPixelCacheVersion, maxFloorMipLevels)
	cache.Store(ctx, graphics.PixelCacheKey(settings, cpu), []*image.RGBA{expected})
	got, _, _, _ := prepareCachedFloorAtlas(ctx, cache, textures)
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("settings cache hit changed floor pixels")
	}
	files, err := filepath.Glob(filepath.Join(cache.Dir, "*.rgba"))
	if err != nil || len(files) != 1 {
		t.Fatalf("preparation missed the full settings key: %d entries, %v", len(files), err)
	}
}
