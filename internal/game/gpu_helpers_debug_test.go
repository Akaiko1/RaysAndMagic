//go:build debug

package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Read back once; encoding an Ebiten image directly reads every pixel through
// the GPU image API and makes large preview galleries unnecessarily slow.
func snapshotUIImage(src *ebiten.Image) *image.RGBA {
	pixels := image.NewRGBA(src.Bounds())
	src.ReadPixels(pixels.Pix)
	return pixels
}
