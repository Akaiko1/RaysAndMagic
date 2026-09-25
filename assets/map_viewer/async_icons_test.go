package main

import (
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
			defer v.iconImages.Close()
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
