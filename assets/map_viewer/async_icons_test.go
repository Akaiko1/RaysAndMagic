package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestEditorArtUsesSharedAsyncCache(t *testing.T) {
	t.Chdir("../..")
	for _, kind := range []string{"tile", "card", "portrait fallback"} {
		t.Run(kind, func(t *testing.T) {
			v := &viewer{iconImages: graphics.NewAsyncImageCache(64 << 20)}
			defer v.closeEditorArt()
			get := func() *ebiten.Image {
				switch kind {
				case "tile":
					return v.tileSpriteThumbnail("forest_oak")
				case "card":
					return v.iconForCard(&contentCard{kind: cardWeapon, icon: "forest_oak"})
				default:
					return v.charPortrait("absent_test_portrait", "forest_oak")
				}
			}
			if get() != nil {
				t.Fatal("editor loaded cold art synchronously")
			}
			deadline := time.Now().Add(5 * time.Second)
			var img *ebiten.Image
			for img == nil && time.Now().Before(deadline) {
				v.iconImages.Advance(256 << 10)
				img = get()
				time.Sleep(time.Millisecond)
			}
			if img == nil {
				t.Fatal("editor never displayed prepared art")
			}
			if get() != img {
				t.Fatal("editor cache identity changed")
			}
		})
	}
}

func TestMapPaletteThumbnailsSurviveSourceEviction(t *testing.T) {
	t.Chdir("../..")
	v := &viewer{iconImages: graphics.NewAsyncImageCache(64 << 20)}
	defer v.closeEditorArt()
	keys := []string{"forest_oak", "solstice_sera", "solstice_cinder_hound"}
	warm := map[string]*ebiten.Image{}
	pixels := map[string][]byte{}
	for _, key := range keys {
		deadline := time.Now().Add(5 * time.Second)
		for warm[key] == nil && time.Now().Before(deadline) {
			v.iconImages.Advance(256 << 10)
			warm[key] = v.tileSpriteThumbnail(key)
			time.Sleep(time.Millisecond)
		}
		img := warm[key]
		if img == nil {
			t.Fatalf("%s did not load", key)
		}
		if img.Bounds().Dx() > mapThumbnailSize || img.Bounds().Dy() > mapThumbnailSize {
			t.Fatal("retained full source")
		}
		pixels[key] = make([]byte, img.Bounds().Dx()*img.Bounds().Dy()*4)
		if os.Getenv("RAM_DEBUG_SIM") == "1" {
			img.ReadPixels(pixels[key])
		}
	}
	// Fill the shared source cache beyond its entry limit, as a large palette or
	// content tab does. Missing metadata entries also count toward that limit.
	for i := 0; i < 160; i++ {
		v.iconImages.Get(fmt.Sprintf("missing_palette_pressure_%d", i))
		v.iconImages.Advance(256 << 10)
	}
	for _, key := range keys {
		if img, ready := v.iconImages.Get(key); img != nil || ready {
			t.Fatalf("%s source was not evicted", key)
		}
		img := v.tileSpriteThumbnail(key + ".png")
		if img != warm[key] {
			t.Fatalf("%s flickered after source eviction", key)
		}
		got := make([]byte, len(pixels[key]))
		if os.Getenv("RAM_DEBUG_SIM") == "1" {
			img.ReadPixels(got)
		}
		if !bytes.Equal(got, pixels[key]) {
			t.Fatalf("%s thumbnail borrowed deallocated source pixels", key)
		}
	}
	v.iconImages.Advance(256 << 10)
	v.tileSpriteThumbnail("missing_palette_asset")
	if _, ok := v.mapThumbnails["missing_palette_asset"]; !ok {
		t.Fatal("missing art not memoized")
	}
	v.closeEditorArt()
	if len(v.mapThumbnails) != 0 {
		t.Fatal("thumbnail ownership survived close")
	}
}
