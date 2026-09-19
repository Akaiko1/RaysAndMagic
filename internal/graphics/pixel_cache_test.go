package graphics

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestPixelCacheRoundTripAndInvalidation(t *testing.T) {
	source := image.NewRGBA(image.Rect(3, 5, 20, 14))
	for i := range source.Pix {
		source.Pix[i] = byte(i * 17)
	}
	crop := source.SubImage(image.Rect(5, 6, 12, 10)).(*image.RGBA)
	// Incompressible data spans multiple I/O buffers and exercises the final
	// partial-buffer flush before an entry becomes visible to a new owner.
	large := image.NewRGBA(image.Rect(0, 0, 128, 512))
	state := uint32(7)
	for i := range large.Pix {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		large.Pix[i] = byte(state)
	}
	for _, tc := range []struct {
		name   string
		images []*image.RGBA
	}{
		{"whole", []*image.RGBA{source}}, {"subimage_stride", []*image.RGBA{crop}}, {"mips", []*image.RGBA{source, crop, image.NewRGBA(image.Rect(0, 0, 1, 1))}},
		{"multiple_io_buffers", []*image.RGBA{large}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := PixelCache{Dir: t.TempDir()}
			ctx := context.Background()
			key := PixelCacheKey("algorithm-v1", tc.images...)
			sizes := make([]image.Point, len(tc.images))
			for i, img := range tc.images {
				sizes[i] = img.Bounds().Size()
			}
			if _, ok := cache.Load(ctx, key, sizes); ok {
				t.Fatal("cold hit")
			}
			cache.Store(ctx, key, tc.images)
			// A new owner represents reload/restart; no in-memory state is required.
			restored, ok := (PixelCache{Dir: cache.Dir}).Load(ctx, key, sizes)
			if !ok {
				t.Fatal("warm miss")
			}
			for i, img := range tc.images {
				for y := 0; y < img.Bounds().Dy(); y++ {
					start := img.PixOffset(img.Bounds().Min.X, img.Bounds().Min.Y+y)
					if !bytes.Equal(img.Pix[start:start+img.Bounds().Dx()*4], restored[i].Pix[y*restored[i].Stride:(y+1)*restored[i].Stride]) {
						t.Fatal("pixels changed")
					}
				}
			}
			if _, ok := cache.Load(ctx, PixelCacheKey("algorithm-v2", tc.images...), sizes); ok {
				t.Fatal("version reused")
			}
			tc.images[0].Pix[0] ^= 1
			if PixelCacheKey("algorithm-v1", tc.images...) == key {
				t.Fatal("pixel edit ignored")
			}
			tc.images[0].Pix[0] ^= 1
			if _, ok := cache.Load(ctx, key, []image.Point{{X: 999999, Y: 999999}}); ok {
				t.Fatal("invalid layout accepted")
			}
			data, err := os.ReadFile(cache.path(key))
			if err != nil {
				t.Fatal(err)
			}
			data[len(data)-1] ^= 1
			if err := os.WriteFile(cache.path(key), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, ok := cache.Load(ctx, key, sizes); ok {
				t.Fatal("corrupt checksum accepted")
			}
			cache.Store(ctx, key, tc.images)
			if _, ok := cache.Load(ctx, key, sizes); !ok {
				t.Fatal("corrupt entry not replaceable")
			}
		})
	}
}

func TestPixelCacheUnavailableCancelledAndEviction(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	key := PixelCacheKey("test", img)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name, dir string
		ctx       context.Context
	}{
		{"disabled", "", context.Background()}, {"cancelled", t.TempDir(), ctx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := PixelCache{Dir: tc.dir}
			c.Store(tc.ctx, key, []*image.RGBA{img})
			if _, ok := c.Load(tc.ctx, key, []image.Point{{X: 16, Y: 16}}); ok {
				t.Fatal("unexpected hit")
			}
		})
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, []byte("keep"), 0600)
	c := PixelCache{Dir: filepath.Join(blocker, "unwritable")}
	c.Store(context.Background(), key, []*image.RGBA{img})
	c = PixelCache{Dir: dir}
	c.Store(context.Background(), key, []*image.RGBA{img})
	c.prune(0)
	if _, err := os.Stat(c.path(key)); !os.IsNotExist(err) {
		t.Fatal("entry not evicted")
	}
	if data, err := os.ReadFile(blocker); err != nil || string(data) != "keep" {
		t.Fatal("pruning touched unrelated data")
	}
}
