package graphics

import "github.com/hajimehoshi/ebiten/v2"

// DrawImageScaled is the common game/editor interface blit. Geometry and
// resampling belong here; optional color/blend settings are copied unchanged.
// Linear minification with mipmaps preserves thin baked-in frames. Native-size
// draws and enlargements use pixelated filtering: pixel clusters stay crisp
// without nearest-neighbor texel jumps at fractional enlargement ratios.
func DrawImageScaled(dst, src *ebiten.Image, x, y, w, h float64, style *ebiten.DrawImageOptions) {
	if dst == nil || src == nil || w <= 0 || h <= 0 {
		return
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	opts := scaledImageOptions(src, x, y, w, h, style)
	dst.DrawImage(src, &opts)
}

func scaledImageOptions(src *ebiten.Image, x, y, w, h float64, style *ebiten.DrawImageOptions) ebiten.DrawImageOptions {
	b := src.Bounds()
	var opts ebiten.DrawImageOptions
	if style != nil {
		opts = *style
	}
	opts.GeoM.Reset()
	opts.GeoM.Scale(w/float64(b.Dx()), h/float64(b.Dy()))
	opts.GeoM.Translate(x, y)
	opts.Filter = ebiten.FilterPixelated
	opts.DisableMipmaps = false
	if w < float64(b.Dx()) || h < float64(b.Dy()) {
		opts.Filter = ebiten.FilterLinear
	}
	return opts
}

// DrawImageScaledEdgeGlow uses the same geometry and filtering as the UI image
// it outlines. Call before drawing the original image over its interior.
func DrawImageScaledEdgeGlow(dst, src *ebiten.Image, x, y, w, h, offset float64, style *ebiten.DrawImageOptions) {
	if dst == nil || src == nil || w <= 0 || h <= 0 || src.Bounds().Empty() {
		return
	}
	opts := scaledImageOptions(src, x, y, w, h, style)
	DrawImageEdgeGlow(dst, src, &opts, offset)
}

// DrawImageEdgeGlow follows the source alpha silhouette, never its rectangular
// bounds. World and UI callers retain their own transform, tint and blend.
func DrawImageEdgeGlow(dst, src *ebiten.Image, style *ebiten.DrawImageOptions, offset float64) {
	if dst == nil || src == nil || offset <= 0 {
		return
	}
	var base ebiten.DrawImageOptions
	if style != nil {
		base = *style
	}
	for _, d := range [8][2]float64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		opts := base
		opts.GeoM.Translate(d[0]*offset, d[1]*offset)
		dst.DrawImage(src, &opts)
	}
}
