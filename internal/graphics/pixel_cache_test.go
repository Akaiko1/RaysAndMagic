package graphics

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestPixelCacheCompressionDoesNotHoldPublicationLock(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelled), func(t *testing.T) {
			c := PixelCache{Dir: t.TempDir()}
			img := image.NewRGBA(image.Rect(0, 0, 512, 512))
			key := PixelCacheKey("parallel", img)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			pixelCacheWriteMu.Lock()
			locked := true
			defer func() {
				if locked {
					pixelCacheWriteMu.Unlock()
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("store did not finish")
				}
			}()
			go func() { defer close(done); c.Store(ctx, key, []*image.RGBA{img}) }()
			compressed := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				files, _ := filepath.Glob(filepath.Join(c.Dir, ".pixels-*"))
				for _, file := range files {
					if info, err := os.Stat(file); err == nil && info.Size() > 0 {
						compressed = true
					}
				}
				if compressed {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !compressed {
				t.Fatal("compression blocked behind another worker's publication lock")
			}
			if _, err := os.Stat(c.path(key)); !os.IsNotExist(err) {
				t.Fatal("entry published before atomic rename")
			}
			if cancelled {
				cancel()
			}
			pixelCacheWriteMu.Unlock()
			locked = false
			<-done
			_, hit := c.Load(context.Background(), key, []image.Point{{X: 512, Y: 512}})
			if hit == cancelled {
				t.Fatalf("cache hit=%v after cancel=%v", hit, cancelled)
			}
			if files, _ := filepath.Glob(filepath.Join(c.Dir, ".pixels-*")); len(files) != 0 {
				t.Fatal("store left temporary files")
			}
		})
	}
}

func TestPixelCachePrunesAtTaskBoundaryAndCleansOrphans(t *testing.T) {
	for _, pressure := range []bool{false, true} {
		t.Run(fmt.Sprintf("pressure=%v", pressure), func(t *testing.T) {
			c := PixelCache{Dir: t.TempDir()}
			old := time.Now().Add(-2 * pixelCacheTempMaxAge)
			for _, name := range []string{".pixels-abandoned", ".pixels-active", "unrelated"} {
				file := filepath.Join(c.Dir, name)
				if err := os.WriteFile(file, []byte("keep-or-clean"), 0600); err != nil {
					t.Fatal(err)
				}
				if name != ".pixels-active" {
					if err := os.Chtimes(file, old, old); err != nil {
						t.Fatal(err)
					}
				}
			}
			oversized := filepath.Join(c.Dir, "old.rgba")
			if pressure {
				f, err := os.Create(oversized)
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(pixelCacheDiskLimit + 1)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
				if err := os.Chtimes(oversized, old, old); err != nil {
					t.Fatal(err)
				}
			}
			img := image.NewRGBA(image.Rect(0, 0, 16, 16))
			key := PixelCacheKey("batch", img)
			for i := 0; i < 4; i++ {
				c.Store(context.Background(), key, []*image.RGBA{img})
			}
			if _, err := os.Stat(filepath.Join(c.Dir, ".pixels-abandoned")); err != nil {
				t.Fatal("Store pruned during a batch")
			}
			if pressure {
				if _, err := os.Stat(oversized); err != nil {
					t.Fatal("Store scanned/evicted entries during a batch")
				}
			}
			c.Prune()
			if _, err := os.Stat(filepath.Join(c.Dir, ".pixels-abandoned")); !os.IsNotExist(err) {
				t.Fatal("stale temporary file retained")
			}
			if pressure {
				if _, err := os.Stat(oversized); !os.IsNotExist(err) {
					t.Fatal("oversized cache not pruned")
				}
			}
			for _, name := range []string{".pixels-active", "unrelated"} {
				if _, err := os.Stat(filepath.Join(c.Dir, name)); err != nil {
					t.Fatalf("prune removed %s", name)
				}
			}
			if _, ok := c.Load(context.Background(), key, []image.Point{{X: 16, Y: 16}}); !ok {
				t.Fatal("recent cache entry lost")
			}
		})
	}
}

func TestPixelCacheCountsRecentTemporaryBytes(t *testing.T) {
	c := PixelCache{Dir: t.TempDir()}
	temp := filepath.Join(c.Dir, ".pixels-active")
	if err := os.WriteFile(temp, make([]byte, 1024), 0600); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	key := PixelCacheKey("test", img)
	c.Store(context.Background(), key, []*image.RGBA{img})
	c.prune(1024)
	if _, err := os.Stat(c.path(key)); !os.IsNotExist(err) {
		t.Fatal("temporary bytes did not count towards the budget")
	}
	if _, err := os.Stat(temp); err != nil {
		t.Fatal("active writer was removed")
	}
}
