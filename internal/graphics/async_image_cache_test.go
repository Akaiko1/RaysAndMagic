package graphics

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAsyncImageCacheLifecycle(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "corrupt", "oversized", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image.png")
			pixels := image.NewRGBA(image.Rect(0, 0, 8, 8))
			pixels.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, pixels); err != nil {
				t.Fatal(err)
			}
			f.Close()
			if kind == "corrupt" {
				if err = os.WriteFile(path, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			limit := 256
			if kind == "oversized" {
				limit = 128
			}
			c := NewAsyncImageCache(limit)
			defer c.Close()
			c.manager.spritePaths = map[string]string{"a": path, "b": path}
			c.manager.spriteDirType = map[string]string{"a": "interface", "b": "interface"}
			name := "a"
			if kind == "missing" {
				name = "absent"
			}
			img, ready := c.Get(name)
			if img != nil {
				t.Fatal("cold source loaded synchronously")
			}
			if kind == "cancelled" {
				c.Advance(4)
				c.Close()
				c.Advance(4)
				if c.bytes != 0 || len(c.entries) != 0 || c.stream.Pending() {
					t.Fatal("close retained work")
				}
				if img, ready = c.Get(name); img != nil || !ready {
					t.Fatal("closed cache restarted")
				}
				return
			}
			deadline := time.Now().Add(3 * time.Second)
			for !ready && time.Now().Before(deadline) {
				c.Advance(32)
				img, ready = c.Get(name)
				time.Sleep(time.Millisecond)
			}
			if !ready {
				t.Fatal("request never settled")
			}
			if kind != "valid" {
				if img != nil || c.bytes != 0 || c.stream.Pending() {
					t.Fatal("failed image allocated/queued")
				}
				return
			}
			if img == nil || img.Bounds() != pixels.Bounds() || c.bytes != 256 {
				t.Fatal("valid source missing")
			}
			if again, done := c.Get(name); again != img || !done {
				t.Fatal("cache hit changed identity")
			}
			if n := testing.AllocsPerRun(100, func() { c.Get(name) }); n != 0 {
				t.Fatalf("hit allocates %g", n)
			}
			c.Get("b")
			for deadline = time.Now().Add(3 * time.Second); c.stream.Pending() && time.Now().Before(deadline); {
				c.Advance(32)
				time.Sleep(time.Millisecond)
			}
			if c.bytes > limit || c.entries["a"] != nil || c.entries["b"] == nil || c.entries["b"].image == nil {
				t.Fatal("LRU budget did not release old image")
			}
			if len(c.manager.ResourceImages(SpriteResourceRequest{Name: "a"})) != 0 {
				t.Fatal("evicted source still published")
			}
		})
	}
}
