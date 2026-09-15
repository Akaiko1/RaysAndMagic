package graphics

import "github.com/hajimehoshi/ebiten/v2"

// DrawImageScaled is the common game/editor interface blit. Geometry and
// resampling belong here; optional color/blend settings are copied unchanged.
// Linear minification with mipmaps preserves thin baked-in frames. Native-size
// draws and enlargements stay nearest to keep production pixel art crisp.
func DrawImageScaled(dst, src *ebiten.Image, x, y, w, h float64, style *ebiten.DrawImageOptions) {
	if dst == nil || src == nil || w <= 0 || h <= 0 {
		return
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	var opts ebiten.DrawImageOptions
	if style != nil {
		opts = *style
	}
	opts.GeoM.Reset()
	opts.GeoM.Scale(w/float64(b.Dx()), h/float64(b.Dy()))
	opts.GeoM.Translate(x, y)
	opts.Filter = ebiten.FilterNearest
	opts.DisableMipmaps = false
	if w < float64(b.Dx()) || h < float64(b.Dy()) {
		opts.Filter = ebiten.FilterLinear
	}
	dst.DrawImage(src, &opts)
}
