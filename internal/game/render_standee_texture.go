package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Large prewarmed textures can remain unused for many frames. Managed images
// initially need padded backing allocations before the engine migrates them
// into source atlases; that padding can double both power-of-two dimensions.
// Explicit backing avoids this transient overhead for the large derived
// images. Small mip levels keep atlas sharing and its batching benefit.
// This policy changes storage only, never source resolution or filtering.
func standeeTextureUnmanaged(bounds image.Rectangle) bool {
	return int64(bounds.Dx())*int64(bounds.Dy()) >= 256*256
}

func newStandeeTexture(bounds image.Rectangle) *ebiten.Image {
	return ebiten.NewImageWithOptions(image.Rect(0, 0, bounds.Dx(), bounds.Dy()),
		&ebiten.NewImageOptions{Unmanaged: standeeTextureUnmanaged(bounds)})
}

func newStandeeTextureFromPixels(cpu *image.RGBA) *ebiten.Image {
	return ebiten.NewImageFromImageWithOptions(cpu,
		&ebiten.NewImageFromImageOptions{Unmanaged: standeeTextureUnmanaged(cpu.Bounds())})
}
