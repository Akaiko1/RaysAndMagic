package graphics

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

type genericAlphaImage struct{ image.Image }

// The generic Image path is the semantic reference for all source formats,
// alpha thresholds, animation layouts and nonzero-origin, padded subimages.
func TestSpriteAlphaMetadata(t *testing.T) {
	for _, sheet := range []bool{false, true} {
		width := 8
		if sheet {
			width = 32
		}
		bounds := image.Rect(3, 5, 3+width, 13)
		rgba := image.NewRGBA(image.Rect(0, 0, width+9, 18))
		nrgba := image.NewNRGBA(rgba.Bounds())
		alpha := image.NewAlpha(rgba.Bounds())
		alpha16 := image.NewAlpha16(rgba.Bounds())
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				a := []uint8{0, 1, 23, 24, 25, 127, 254, 255}[(x+y)%8]
				rgba.SetRGBA(x, y, color.RGBA{A: a})
				nrgba.SetNRGBA(x, y, color.NRGBA{R: 17, G: 91, B: 233, A: a})
				alpha.SetAlpha(x, y, color.Alpha{A: a})
				alpha16.SetAlpha16(x, y, color.Alpha16{A: uint16(a) * 257})
			}
		}
		for name, src := range map[string]image.Image{"rgba": rgba.SubImage(bounds), "nrgba": nrgba.SubImage(bounds), "alpha": alpha.SubImage(bounds), "alpha16": alpha16.SubImage(bounds)} {
			t.Run(name, func(t *testing.T) {
				reference := genericAlphaImage{src}
				if got, want := spriteVisibleFrameBoundsFromImage(src), spriteVisibleFrameBoundsFromImage(reference); got != want {
					t.Fatalf("bounds %v want %v", got, want)
				}
				if got, want := spriteAlphaMaskFromImage(src), spriteAlphaMaskFromImage(reference); !reflect.DeepEqual(got, want) {
					t.Fatal("hit-test alpha changed")
				}
			})
		}
	}
	if spriteAlphaMaskFromImage(nil) != nil || spriteVisibleFrameBoundsFromImage(nil).known {
		t.Fatal("nil image gained visible pixels")
	}
	if spriteVisibleFrameBoundsFromImage(image.NewNRGBA(image.Rect(0, 0, 4, 4))).known {
		t.Fatal("transparent image gained visible pixels")
	}
	// Empty images (no rows, no columns, a subimage outside its source) have no
	// visible pixels and no mask in every format.
	for _, img := range []image.Image{
		image.NewNRGBA(image.Rect(0, 0, 0, 0)),
		image.NewRGBA(image.Rect(0, 0, 0, 5)),
		image.NewAlpha(image.Rect(0, 0, 5, 0)),
		genericAlphaImage{image.NewNRGBA(image.Rect(0, 0, 0, 0))},
		image.NewNRGBA(image.Rect(0, 0, 4, 4)).SubImage(image.Rect(9, 9, 10, 10)),
	} {
		if spriteVisibleFrameBoundsFromImage(img).known || spriteAlphaMaskFromImage(img) != nil {
			t.Fatalf("empty %T %v gained visible pixels", img, img.Bounds())
		}
	}
	// Each edge of the union comes from a different frame; a faint pixel
	// (alpha 23) at the far corner does not count.
	sheet := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for _, p := range [][3]int{{2, 1, 255}, {8 + 6, 6, 40}, {24, 3, 24}, {16 + 7, 7, 23}} {
		sheet.SetNRGBA(p[0], p[1], color.NRGBA{A: uint8(p[2])})
	}
	want := spriteVisibleFrameBounds{bounds: image.Rect(0, 1, 7, 7), frameWidth: 8, frameHeight: 8, known: true}
	if got := spriteVisibleFrameBoundsFromImage(sheet); got != want {
		t.Fatalf("sheet union bounds %v want %v", got, want)
	}
}

func BenchmarkSpriteVisibleBounds(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
	}
	for _, tc := range []struct {
		name string
		src  image.Image
	}{{"typed", src}, {"generic", genericAlphaImage{src}}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				spriteVisibleFrameBoundsFromImage(tc.src)
			}
		})
	}
}
