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

func TestRuntimeResourceMissStreamsWithoutSynchronousPublication(t *testing.T) {
	for _, kind := range []string{"sprite", "animation", "alpha", "bounds", "corrupt", "cancel", "cancel_partial"} {
		t.Run(kind, func(t *testing.T) {
			sm := NewSpriteManager()
			path := filepath.Join(t.TempDir(), "resource.png")
			cpu := image.NewRGBA(image.Rect(0, 0, 64, 16))
			cpu.SetRGBA(3, 4, color.RGBA{R: 255, A: 255})
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, cpu); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if kind == "corrupt" {
				if err := os.WriteFile(path, []byte("invalid PNG"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			name := "source"
			request := SpriteResourceRequest{Name: name}
			if kind == "animation" {
				request.AnimationType = "walking_r"
				name += "_walking_r"
			}
			sm.spritePaths = map[string]string{name: path}
			sm.spriteDirType = map[string]string{name: "environment"}
			stream := NewResourceStream(sm)
			t.Cleanup(stream.Close)
			sm.SetDeferredResourceHandler(func(request SpriteResourceRequest) bool { stream.Request(request); return true })
			switch kind {
			case "animation":
				if sm.GetAnimation("source", "walking_r") != nil {
					t.Fatal("cold animation loaded synchronously")
				}
			case "alpha":
				if _, known := sm.SpriteOpaqueAt(name, 3, 4); known {
					t.Fatal("alpha decoded synchronously")
				}
			case "bounds":
				if _, _, _, known := sm.SpriteVisibleFrameBounds(name); known {
					t.Fatal("bounds decoded synchronously")
				}
			default:
				if sm.GetSprite(name) != nil {
					t.Fatal("cold sprite loaded synchronously")
				}
			}
			if !stream.Pending() || len(sm.ResourceImages(request)) != 0 {
				t.Fatal("miss did not remain queued and unpublished")
			}
			if kind == "cancel" {
				stream.Advance(64)
				stream.Close()
				stream.Advance(64)
				if stream.Pending() || len(sm.ResourceImages(request)) != 0 {
					t.Fatal("cancelled demand was published")
				}
				return
			}
			stream.Request(request)
			if len(stream.queue) != 1 {
				t.Fatal("duplicate demand was queued twice")
			}
			deadline := time.Now().Add(5 * time.Second)
			for stream.Pending() && time.Now().Before(deadline) {
				stream.Advance(64)
				if kind == "cancel_partial" && stream.commit != nil {
					stream.Advance(64)
					if len(sm.ResourceImages(request)) != 0 {
						t.Fatal("partial upload was published")
					}
					stream.Close()
					if stream.Pending() || len(sm.ResourceImages(request)) != 0 {
						t.Fatal("partial upload survived cancellation")
					}
					return
				}
				time.Sleep(time.Millisecond)
			}
			if stream.Pending() {
				t.Fatal("resource request never settled")
			}
			switch kind {
			case "animation":
				if anim := sm.GetAnimation("source", "walking_r"); anim == nil || len(anim.Frames) != 4 {
					t.Fatal("animation was not published")
				}
			case "cancel_partial":
				t.Fatal("partial commit cancellation was never exercised")
			case "corrupt":
				if sm.GetSprite(name) == nil || stream.Pending() {
					t.Fatal("corrupt source did not settle to a stable fallback")
				}
			default:
				if sm.GetSprite(name) == nil {
					t.Fatal("sprite was not published")
				}
				if opaque, known := sm.SpriteOpaqueAt(name, 3, 4); !known || !opaque {
					t.Fatal("CPU alpha metadata was not published")
				}
				if _, _, _, known := sm.SpriteVisibleFrameBounds(name); !known {
					t.Fatal("visible bounds were not published")
				}
			}
			if stream.Pending() {
				t.Fatal("published resource triggered another load")
			}
			if kind == "corrupt" {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, cpu); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
				sm.EvictResource(name, "")
				if sm.GetSprite(name) != nil || !stream.Pending() {
					t.Fatal("explicit invalidation did not retry a repaired source asynchronously")
				}
				deadline := time.Now().Add(5 * time.Second)
				for stream.Pending() && time.Now().Before(deadline) {
					stream.Advance(64)
					time.Sleep(time.Millisecond)
				}
				if stream.Pending() {
					t.Fatal("repaired source never settled")
				}
				if opaque, known := sm.SpriteOpaqueAt(name, 3, 4); !known || !opaque {
					t.Fatal("repaired source retained negative metadata")
				}
			}
		})
	}
}
