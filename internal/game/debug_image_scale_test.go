//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestDebugSim_SharedImageResampling(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{17, 17}, {17, 64}, {64, 17}, {64, 64}, {128, 128}} {
		for _, alpha := range []float32{0, 0.5, 1} {
			t.Run(fmt.Sprintf("%dx%d/alpha%.1f", size[0], size[1], alpha), func(t *testing.T) {
				var problem string
				runOnDrawFrame(func(_ *ebiten.Image) {
					pixels := image.NewRGBA(image.Rect(0, 0, 64, 64))
					for y := 0; y < 64; y++ {
						for x := 0; x < 64; x++ {
							c := color.RGBA{15, 10, 20, 255}
							if x == 0 || y == 0 || x == 63 || y == 63 || (x+y)%3 == 0 {
								c = color.RGBA{235, 180, 90, 255}
							}
							pixels.SetRGBA(x, y, c)
						}
					}
					src := ebiten.NewImageFromImage(pixels)
					defer src.Deallocate()
					got, want := ebiten.NewImage(150, 150), ebiten.NewImage(150, 150)
					defer got.Deallocate()
					defer want.Deallocate()
					style := &ebiten.DrawImageOptions{DisableMipmaps: true}
					style.GeoM.Translate(700, 900)
					style.ColorScale.Scale(0.75, 0.5, 1, alpha)
					saved := *style
					graphics.DrawImageScaled(got, src, 3, 5, float64(size[0]), float64(size[1]), style)
					if !reflect.DeepEqual(saved, *style) {
						problem = "shared resize mutated caller style"
						return
					}
					opts := *style
					opts.GeoM.Reset()
					opts.GeoM.Scale(float64(size[0])/64, float64(size[1])/64)
					opts.GeoM.Translate(3, 5)
					opts.DisableMipmaps = false
					if size[0] < 64 || size[1] < 64 {
						opts.Filter = ebiten.FilterLinear
					} else {
						opts.Filter = ebiten.FilterNearest
					}
					want.DrawImage(src, &opts)
					a, b := make([]byte, 150*150*4), make([]byte, 150*150*4)
					got.ReadPixels(a)
					want.ReadPixels(b)
					for i, v := range a {
						delta := int(v) - int(b[i])
						if delta < -2 || delta > 2 {
							problem = "wrong minification/enlargement filter, geometry or tint"
							return
						}
					}
					got.Clear()
					want.Clear()
					drawImageScaled(got, src, 3, 5, size[0], size[1])
					graphics.DrawImageScaled(want, src, 3, 5, float64(size[0]), float64(size[1]), nil)
					got.ReadPixels(a)
					want.ReadPixels(b)
					for i, v := range a {
						delta := int(v) - int(b[i])
						if delta < -2 || delta > 2 {
							problem = "game UI wrapper diverged from shared image resize"
							return
						}
					}
				})
				if problem != "" {
					t.Fatal(problem)
				}
			})
		}
	}
}
