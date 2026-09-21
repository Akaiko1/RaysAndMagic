package game

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/storage"
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

func TestPreparationTasksPrunePixelCache(t *testing.T) {
	for _, kind := range []string{"standee", "floor"} {
		for _, cancelled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancel=%v", kind, cancelled), func(t *testing.T) {
				t.Chdir(t.TempDir())
				cache := graphics.PixelCache{Dir: storage.RenderCacheDir()}
				orphan := filepath.Join(cache.Dir, ".pixels-abandoned")
				if err := os.WriteFile(orphan, []byte("abandoned"), 0600); err != nil {
					t.Fatal(err)
				}
				old := time.Now().Add(-48 * time.Hour)
				if err := os.Chtimes(orphan, old, old); err != nil {
					t.Fatal(err)
				}
				if kind == "standee" {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if cancelled {
						cancel()
					}
					jobs := []mapRenderStandeeJob{{cpu: image.NewRGBA(image.Rect(0, 0, 8, 8))}, {cpu: image.NewRGBA(image.Rect(0, 0, 16, 16))}}
					for result := range prepareMapRenderStandees(ctx, jobs, .5, cache) {
						result.lease.Release()
					}
				} else {
					r := &Renderer{}
					r.startFloorPreparation("prune", nil)
					result := r.floorPreparation.result
					if cancelled {
						r.cancelFloorPreparation()
					}
					for range result {
					}
				}
				if _, err := os.Stat(orphan); !os.IsNotExist(err) {
					t.Fatal("completed preparation task did not prune orphan cache data")
				}
			})
		}
	}
}

func TestStandeeCacheInvalidatesWoodTone(t *testing.T) {
	original := standeeWoodTone
	t.Cleanup(func() { standeeWoodTone = original })
	cpu := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range cpu.Pix {
		cpu.Pix[i] = 255
	}
	cache := graphics.PixelCache{Dir: t.TempDir()}
	before := prepareCachedStandeePixels(context.Background(), cache, cpu, 0)
	standeeWoodTone = [3]float64{.1, .2, .3}
	after := prepareCachedStandeePixels(context.Background(), cache, cpu, 0)
	want := prepareStandeePixels(cpu, 0, true)
	if bytes.Equal(before.core.Pix, after.core.Pix) || !reflect.DeepEqual(after, want) {
		t.Fatal("wood tone change reused stale derived pixels")
	}
}

// Seed a valid entry using the settings contract, then drive the production
// preparation path. Recomputing under another key would create a second file.
func TestPreparedPixelCacheUsesAllSettings(t *testing.T) {
	for _, surface := range []string{"standee", "floor"} {
		t.Run(surface, func(t *testing.T) {
			cache := graphics.PixelCache{Dir: t.TempDir()}
			cpu := image.NewRGBA(image.Rect(0, 0, 16, 16))
			for i := range cpu.Pix {
				cpu.Pix[i] = 255
			}
			ctx := context.Background()
			if surface == "standee" {
				expected := prepareStandeePixels(cpu, .5, true)
				settings := fmt.Sprintf("%s:tint=%016x:max_pixels=%d:mips=%d:wood=%016x,%016x,%016x", standeePixelCacheVersion, math.Float64bits(.5), standeeRenderSourceMaxPixels, maxMipLevel, math.Float64bits(standeeWoodTone[0]), math.Float64bits(standeeWoodTone[1]), math.Float64bits(standeeWoodTone[2]))
				cache.Store(ctx, graphics.PixelCacheKey(settings, cpu), append(append([]*image.RGBA(nil), expected.stickerMips...), expected.coreMips...))
				if got := prepareCachedStandeePixels(ctx, cache, cpu, .5); !reflect.DeepEqual(got, expected) {
					t.Fatal("settings cache hit changed standee pixels")
				}
			} else {
				textures := []floorTexture{{width: 16, height: 16, pixels: cpu.Pix}}
				expected, _, _, _ := prepareFloorAtlas(textures)
				settings := fmt.Sprintf("%s:mips=%d", floorPixelCacheVersion, maxFloorMipLevels)
				cache.Store(ctx, graphics.PixelCacheKey(settings, cpu), []*image.RGBA{expected})
				got, _, _, _ := prepareCachedFloorAtlas(ctx, cache, textures)
				if !reflect.DeepEqual(got, expected) {
					t.Fatal("settings cache hit changed floor pixels")
				}
			}
			files, err := filepath.Glob(filepath.Join(cache.Dir, "*.rgba"))
			if err != nil || len(files) != 1 {
				t.Fatalf("preparation missed the full settings key: %d entries, %v", len(files), err)
			}
		})
	}
}
