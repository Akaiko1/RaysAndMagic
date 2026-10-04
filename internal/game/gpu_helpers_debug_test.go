//go:build debug

package game

import (
	"image"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// requireStandeeGPU gates a check that needs the live GPU harness.
func requireStandeeGPU(t *testing.T) {
	t.Helper()
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires the live GPU harness")
	}
	// Desktop focus changes must not suspend a diagnostic waiting for Draw.
	ebiten.SetRunnableOnUnfocused(true)
}

// Read back once; encoding an Ebiten image directly reads every pixel through
// the GPU image API and makes large preview galleries unnecessarily slow.
func snapshotUIImage(src *ebiten.Image) *image.RGBA {
	pixels := image.NewRGBA(src.Bounds())
	src.ReadPixels(pixels.Pix)
	return pixels
}
