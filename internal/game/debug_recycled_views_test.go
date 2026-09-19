//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestDebugSim_RecycledImageViewsPreservePixels(t *testing.T) {
	requireStandeeGPU(t)
	for _, origin := range []image.Point{{}, {X: 7, Y: 9}} {
		for _, size := range []image.Point{{X: 16, Y: 12}, {X: 33, Y: 27}, {X: 69, Y: 55}} {
			t.Run(fmt.Sprintf("%v/%v", origin, size), func(t *testing.T) {
				runOnDrawFrame(func(*ebiten.Image) {
					src := ebiten.NewImage(64, 64)
					defer src.Deallocate()
					expectedSource := ebiten.NewImage(64, 64)
					defer expectedSource.Deallocate()
					for y := 0; y < 64; y += 3 {
						rows := min(3, 64-y)
						pixels := make([]byte, 64*rows*4)
						for i := 0; i < len(pixels); i += 4 {
							a := byte((i + y) % 256)
							pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = a/3, a/2, a, a
						}
						region := image.Rect(0, y, 64, y+rows)
						graphics.WritePixelsRegion(src, region, pixels)
						expectedSource.SubImage(region).(*ebiten.Image).WritePixels(pixels)
					}
					if !bytes.Equal(snapshotUIImage(src).Pix, snapshotUIImage(expectedSource).Pix) {
						t.Error("recycled strip upload changed pixels")
					}
					cut := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(32, 24))}
					view := src.SubImage(cut).(*ebiten.Image)
					reference := src.SubImage(cut).(*ebiten.Image)
					got, want := ebiten.NewImage(size.X, size.Y), ebiten.NewImage(size.X, size.Y)
					defer got.Deallocate()
					defer want.Deallocate()
					drawNineSliceScaled(got, view, 0, 0, size.X, size.Y, 4, 3)
					for _, op := range planNineSlice(32, 24, size.X, size.Y, 4, 3) {
						part := reference.SubImage(image.Rect(cut.Min.X+op.sx, cut.Min.Y+op.sy, cut.Min.X+op.sx+op.sw, cut.Min.Y+op.sy+op.sh)).(*ebiten.Image)
						drawImageScaled(want, part, op.dx, op.dy, op.dw, op.dh)
					}
					// Separate destination atlas positions may round the
					// final blended channel by one unit. Texel selection and
					// geometry must otherwise be identical.
					a, b := snapshotUIImage(got).Pix, snapshotUIImage(want).Pix
					delta := 0
					for i, v := range a {
						delta = max(delta, abs(int(v)-int(b[i])))
					}
					if delta > 1 {
						t.Errorf("recycled frame view changed pixels: maxDelta=%d", delta)
					}

				})
			})
		}
	}
}
