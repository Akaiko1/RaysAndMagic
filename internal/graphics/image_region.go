package graphics

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
)

// WritePixelsRegion borrows a view only for this upload. It never retains a
// wrapper per strip on a long-lived texture or recycles an owned image.
func WritePixelsRegion(dst *ebiten.Image, region image.Rectangle, pixels []byte) {
	part := dst.RecyclableSubImage(region)
	defer part.Recycle()
	part.WritePixels(pixels)
}
