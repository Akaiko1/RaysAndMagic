package main

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Editor map and palette thumbnails share firstFrame: a horizontal sheet of N
// square poses shows its first square, anything else is shown whole.
func TestFirstFrameSelectsLeadingSquare(t *testing.T) {
	atlas := ebiten.NewImage(64, 64)
	offsetStrip := atlas.SubImage(image.Rect(4, 2, 36, 10)).(*ebiten.Image)
	for _, tc := range []struct {
		name string
		img  *ebiten.Image
		want image.Rectangle
	}{
		{"square sprite", ebiten.NewImage(8, 8), image.Rect(0, 0, 8, 8)},
		{"four-pose idle sheet", ebiten.NewImage(32, 8), image.Rect(0, 0, 8, 8)},
		{"sheet inside an atlas", offsetStrip, image.Rect(4, 2, 12, 10)},
		{"wide non-sheet", ebiten.NewImage(30, 8), image.Rect(0, 0, 30, 8)},
		{"tall sprite", ebiten.NewImage(8, 32), image.Rect(0, 0, 8, 32)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstFrame(tc.img).Bounds(); got != tc.want {
				t.Fatalf("firstFrame bounds = %v, want %v", got, tc.want)
			}
		})
	}
	if firstFrame(nil) != nil {
		t.Fatal("missing sprite must stay nil so the caller draws its fallback marker")
	}
}
