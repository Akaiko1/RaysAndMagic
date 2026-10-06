package main

import (
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

const mapThumbnailSize = 128

// Retain compact, owned first frames for the editor session. The finite content
// catalog bounds the key set; each image uses at most 64 KiB instead of keeping
// a full source sheet alive. Map and palette share these images across scrolling
// and map changes. Full-size Shift previews still use gameSprites.
func (v *viewer) tileSpriteThumbnail(sprite string) *ebiten.Image {
	sprite = strings.TrimSuffix(sprite, ".png")
	if sprite == "" {
		return nil
	}
	if img, ok := v.mapThumbnails[sprite]; ok {
		return img
	}
	source, ready := v.iconImages.Get(sprite)
	if !ready {
		return nil
	}
	if v.mapThumbnails == nil {
		v.mapThumbnails = make(map[string]*ebiten.Image)
	}
	var thumbnail *ebiten.Image
	if source = firstFrame(source); source != nil {
		b := source.Bounds()
		scale := math.Min(1, float64(mapThumbnailSize)/float64(max(b.Dx(), b.Dy())))
		w, h := max(1, int(math.Round(float64(b.Dx())*scale))), max(1, int(math.Round(float64(b.Dy())*scale)))
		thumbnail = ebiten.NewImage(w, h)
		graphics.DrawImageScaled(thumbnail, source, 0, 0, float64(w), float64(h), nil)
	}
	// A settled missing image is cached too, so scrolling cannot requeue it.
	v.mapThumbnails[sprite] = thumbnail
	return thumbnail
}

func (v *viewer) closeEditorArt() {
	for _, img := range v.mapThumbnails {
		if img != nil {
			img.Deallocate()
		}
	}
	clear(v.mapThumbnails)
	v.iconImages.Close()
}
