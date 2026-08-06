package game

import (
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Kage sources compile at runtime, so a Go build can't catch shader syntax
// errors - compile them here so a broken shader fails the suite, not the game.
func TestKageShadersCompile(t *testing.T) {
	for name, src := range map[string]string{
		"floor":            floorShaderSrc,
		"sky":              skyShaderSrc,
		"standeeTrilinear": standeeTrilinearShaderSrc,
		"standeeVolume":    standeeVolumeShaderSrc,
		"turnBlur":         turnBlurShaderSrc,
	} {
		if _, err := ebiten.NewShader([]byte(src)); err != nil {
			t.Errorf("%s shader failed to compile: %v", name, err)
		}
	}
}

func TestWrapPanoramaOffsetBoundsLargeAngles(t *testing.T) {
	const width = 1774.0
	for _, offset := range []float64{-1e12 - 0.25, -width, -0.25, 0, width, 1e12 + 0.25} {
		got := wrapPanoramaOffset(offset, width)
		if got < 0 || got >= width {
			t.Fatalf("wrapPanoramaOffset(%g, %g) = %g, want [0,%g)", offset, width, got, width)
		}
	}
	if got := wrapPanoramaOffset(-0.25, width); math.Abs(got-(width-0.25)) > 1e-9 {
		t.Fatalf("negative wrap = %g, want %g", got, width-0.25)
	}
}

func TestSkyShaderWrapKeepsEveryColumnCovered(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	t.Chdir("../..")
	source, err := decodePNG("assets/sprites/sky/highlands_panorama_night.png")
	if err != nil {
		t.Fatal(err)
	}
	panorama := ebiten.NewImageFromImage(source)
	shader, err := ebiten.NewShader([]byte(skyShaderSrc))
	if err != nil {
		t.Fatal(err)
	}

	const dstW, dstH = 1024, 384
	srcW, srcH := float32(panorama.Bounds().Dx()), float32(panorama.Bounds().Dy())
	srcSpan := float32(dstW) * srcH / dstH
	dst := ebiten.NewImage(dstW, dstH)
	pixels := make([]byte, 4*dstW*dstH)
	indices := []uint16{0, 1, 2, 1, 3, 2}
	opts := &ebiten.DrawTrianglesShaderOptions{}
	opts.Images[0] = panorama

	// Exercise positive, negative and exact-boundary phases. The large values
	// also guard against a long unnormalised RT camera angle losing enough
	// float precision to expose the texture atlas between panorama repetitions.
	for _, center := range []float32{0, 0.25, srcW - 0.25, srcW, -srcW, 1e6 + 0.25, -1e6 - 0.25} {
		wrappedCenter := float32(wrapPanoramaOffset(float64(center), float64(srcW)))
		sx0, sx1 := wrappedCenter-srcSpan/2, wrappedCenter+srcSpan/2
		vertices := []ebiten.Vertex{
			{DstX: 0, DstY: 0, SrcX: sx0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: dstW, DstY: 0, SrcX: sx1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: 0, DstY: dstH, SrcX: sx0, SrcY: srcH, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			{DstX: dstW, DstY: dstH, SrcX: sx1, SrcY: srcH, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		}
		dst.Clear()
		dst.DrawTrianglesShader(vertices, indices, shader, opts)
		dst.ReadPixels(pixels)
		for x := 0; x < dstW; x++ {
			var alphaSum, lumaSum int
			for y := 0; y < dstH; y++ {
				i := 4 * (y*dstW + x)
				alphaSum += int(pixels[i+3])
				lumaSum += 3*int(pixels[i]) + 6*int(pixels[i+1]) + int(pixels[i+2])
			}
			if alphaSum != 255*dstH {
				t.Fatalf("phase %.2f column %d alpha average %.2f: panorama wrap sampled outside its atlas region", center, x, float64(alphaSum)/dstH)
			}
			if avg := float64(lumaSum) / (10 * dstH); math.IsNaN(avg) || avg < 5 {
				t.Fatalf("phase %.2f column %d luma average %.2f: black panorama gap", center, x, avg)
			}
		}
	}
}

// Kage's pixel unit allows differently-sized images, but imageSrcNAt keeps
// image-0 pixel coordinates rather than scaling them to image N. Exercise the
// actual GPU path so a bad mapping cannot shrink a distant standee into the
// upper-left part of its quad (which also makes a floor-anchored token float).
func TestStandeeTrilinearShaderFillsOriginalQuad(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	shader, err := ebiten.NewShader([]byte(standeeTrilinearShaderSrc))
	if err != nil {
		t.Fatal(err)
	}
	level0 := ebiten.NewImage(8, 8)
	level1 := ebiten.NewImage(4, 4)
	level2 := ebiten.NewImage(2, 2)
	coreLevel := ebiten.NewImage(4, 4)
	level1.Fill(color.RGBA{R: 0xff, A: 0xff})
	level2.Fill(color.RGBA{B: 0xff, A: 0xff})
	coreLevel.Fill(color.RGBA{G: 0xff, A: 0xff})

	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0.5, SrcY: 0.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 8, DstY: 0, SrcX: 7.5, SrcY: 0.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 0, DstY: 8, SrcX: 0.5, SrcY: 7.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
		{DstX: 8, DstY: 8, SrcX: 7.5, SrcY: 7.5, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: 0.5, Custom2: 1},
	}
	dst := ebiten.NewImage(8, 8)
	opts := &ebiten.DrawTrianglesShaderOptions{}
	opts.Images[0] = level0
	opts.Images[1] = level1
	opts.Images[2] = level2
	opts.Images[3] = coreLevel
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got.A < 250 || got.R < 125 || got.R > 130 || got.B < 125 || got.B > 130 {
				t.Fatalf("pixel (%d,%d) = %#v; mip blend must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// Core shells use one tap from their prefiltered mip. Exercise its
	// differently-sized coordinate mapping across the complete destination too.
	for i := range vertices {
		vertices[i].Custom1 = 1
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got != (color.RGBA{G: 0xff, A: 0xff}) {
				t.Fatalf("core pixel (%d,%d) = %#v; selected mip must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// The far sticker uses the same selected sticker mip with a single tap.
	// Its level-space mapping must also cover the complete destination.
	for i := range vertices {
		vertices[i].Custom1 = 2
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got != (color.RGBA{R: 0xff, A: 0xff}) {
				t.Fatalf("far-sticker pixel (%d,%d) = %#v; selected mip must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// At 1:1 or magnification the visible sticker bypasses mip sampling and
	// retains the authored base pixels.
	level0.Fill(color.RGBA{R: 0xff, G: 0xff, A: 0xff})
	for i := range vertices {
		vertices[i].Custom1 = 0
		vertices[i].Custom2 = 0
	}
	dst.Clear()
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA)
			if got != (color.RGBA{R: 0xff, G: 0xff, A: 0xff}) {
				t.Fatalf("base pixel (%d,%d) = %#v; level 0 must fill the opaque 8x8 quad", x, y, got)
			}
		}
	}

	// Animation frames are non-zero-origin SubImages. Their level 0 must still
	// occupy the complete normalized mip image instead of retaining sheet-space
	// coordinates and being drawn outside it.
	sheet := ebiten.NewImage(12, 10)
	frame := sheet.SubImage(image.Rect(4, 2, 12, 10)).(*ebiten.Image)
	frame.Fill(color.RGBA{G: 0xff, A: 0xff})
	r := &Renderer{}
	chain := r.standeeMipChainFor(standeeMipKey{}, frame)
	for _, point := range []image.Point{image.Pt(0, 0), image.Pt(7, 7)} {
		got := color.RGBAModel.Convert(chain.levels[0].At(point.X, point.Y)).(color.RGBA)
		if got != (color.RGBA{G: 0xff, A: 0xff}) {
			t.Fatalf("normalized SubImage mip at %v = %#v; want opaque green", point, got)
		}
	}
}

func TestStandeeVolumeShaderPreservesLayersAndWallClip(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("GPU readback requires the live Ebitengine debug-sim loop")
	}
	shader, err := ebiten.NewShader([]byte(standeeVolumeShaderSrc))
	if err != nil {
		t.Fatal(err)
	}

	sticker := ebiten.NewImage(8, 8)
	stickerMip := ebiten.NewImage(8, 8)
	stickerNextMip := ebiten.NewImage(8, 8)
	core := ebiten.NewImage(8, 8)
	sticker.Fill(color.RGBA{R: 0xff, A: 0xff})
	stickerMip.Fill(color.RGBA{R: 0xff, A: 0xff})
	stickerNextMip.Fill(color.RGBA{R: 0xff, A: 0xff})
	core.Fill(color.RGBA{G: 0xff, A: 0xff})

	// At near depth 8 these invariants produce a face from y=0 through y=8.
	// Far depth 10 is slightly smaller, just like a real thick slab.
	const heightScale = 64
	const bottomScale = 32
	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 100, ColorB: 0, ColorA: 6, Custom0: 10, Custom1: 8, Custom2: 0, Custom3: 0},
		{DstX: 8, DstY: 0, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 100, ColorB: 0, ColorA: 6, Custom0: 10, Custom1: 8, Custom2: 1, Custom3: 1},
		{DstX: 0, DstY: 8, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 100, ColorB: 0, ColorA: 6, Custom0: 10, Custom1: 8, Custom2: 0, Custom3: 0},
		{DstX: 8, DstY: 8, SrcX: heightScale, SrcY: bottomScale, ColorR: 1, ColorG: 100, ColorB: 0, ColorA: 6, Custom0: 10, Custom1: 8, Custom2: 1, Custom3: 1},
	}
	indices := []uint16{0, 1, 2, 1, 3, 2}
	opts := &ebiten.DrawTrianglesShaderOptions{}
	opts.Images[0] = sticker
	opts.Images[1] = stickerMip
	opts.Images[2] = stickerNextMip
	opts.Images[3] = core
	dst := ebiten.NewImage(8, 8)

	draw := func() []byte {
		dst.Clear()
		dst.DrawTrianglesShader(vertices, indices, shader, opts)
		pixels := make([]byte, 8*8*4)
		dst.ReadPixels(pixels)
		return pixels
	}
	pixel := func(pixels []byte, x, y int) color.RGBA {
		i := (y*8 + x) * 4
		return color.RGBA{R: pixels[i], G: pixels[i+1], B: pixels[i+2], A: pixels[i+3]}
	}

	pixels := draw()
	if got := pixel(pixels, 4, 4); got.R < 250 || got.G != 0 || got.A < 250 {
		t.Fatalf("near sticker = %#v, want opaque red front face", got)
	}

	// A transparent sticker must reveal the virtual wooden shell stack rather
	// than flattening the standee to a single face.
	sticker.Clear()
	stickerMip.Clear()
	stickerNextMip.Clear()
	pixels = draw()
	if got := pixel(pixels, 4, 4); got.G < 150 || got.R != 0 || got.A < 250 {
		t.Fatalf("core shell = %#v, want visible opaque green volume", got)
	}

	// A wall at depth 7 is in front of every slab layer. Its top at y=4 keeps
	// the canopy visible above it and clips the lower half, matching the general
	// per-surface standee path.
	sticker.Fill(color.RGBA{R: 0xff, A: 0xff})
	for i := range vertices {
		vertices[i].ColorG = 7
		vertices[i].ColorB = 4
	}
	pixels = draw()
	if got := pixel(pixels, 4, 2); got.R < 250 || got.A < 250 {
		t.Fatalf("canopy above wall = %#v, want visible sticker", got)
	}
	if got := pixel(pixels, 4, 6); got.A != 0 {
		t.Fatalf("slab below wall top = %#v, want transparent clipped pixel", got)
	}
}
