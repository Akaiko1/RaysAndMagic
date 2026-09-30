package game

import (
	"image"
	"image/color"
	"math"

	"ugataima/internal/graphics"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The interface draws through these primitives only. Each takes UI units -
// the coordinates every layout computes - and converts them to the pixels of
// its destination, so the whole interface is drawn at the screen's native
// resolution by changing one number per surface.
//
// A surface's scale is looked up from the destination image: the frame's
// screen (beginUIFrame), offscreen layers (uiLayer) and clips cut from either
// (uiClip). Any other image - a label, a scratch buffer, the 3D view - is
// addressed in its own pixels, scale 1.

// uiSurface is a registered destination: its pixels per UI unit and its
// extent in UI units.
type uiSurface struct {
	scale  float64
	bounds image.Rectangle
}

var (
	uiFrameSurfaces = map[*ebiten.Image]uiSurface{} // the screen and clips cut this frame
	uiLayerSurfaces = map[*ebiten.Image]uiSurface{} // offscreen layers at the screen's resolution
	// uiScreenScale is the last presented screen's pixels per UI unit; input
	// reads the pointer against that frame, the one the player sees.
	uiScreenScale = 1.0
)

// uiScaleFuzz absorbs float error when a scaled extent lands on a whole pixel.
const uiScaleFuzz = 1e-6

// beginUIFrame registers the screen the interface draws on this frame.
func beginUIFrame(screen *ebiten.Image, scale float64) {
	clear(uiFrameSurfaces)
	uiClippedLabels = uiClippedLabels[:0]
	uiFrameSurfaces[screen] = uiSurface{scale, unscaleRect(screen.Bounds(), scale)}
	uiScreenScale = scale
}

func uiSurfaceOf(dst *ebiten.Image) (uiSurface, bool) {
	if s, ok := uiFrameSurfaces[dst]; ok {
		return s, true
	}
	s, ok := uiLayerSurfaces[dst]
	return s, ok
}

func uiScaleOf(dst *ebiten.Image) float64 {
	if s, ok := uiSurfaceOf(dst); ok {
		return s.scale
	}
	return 1
}

// uiBounds is dst's extent in UI units.
func uiBounds(dst *ebiten.Image) image.Rectangle {
	if s, ok := uiSurfaceOf(dst); ok {
		return s.bounds
	}
	return dst.Bounds()
}

// uiClip cuts r (UI units) out of dst; drawing on the clip keeps dst's scale.
func uiClip(dst *ebiten.Image, r image.Rectangle) *ebiten.Image {
	s := uiScaleOf(dst)
	r = r.Intersect(uiBounds(dst))
	sub := dst.SubImage(scaleRect(r, s)).(*ebiten.Image)
	if s != 1 {
		uiFrameSurfaces[sub] = uiSurface{s, r}
	}
	return sub
}

// uiSurfaceAt is dst addressed at another scale (pixels per unit) for the rest
// of the frame: a HUD piece that keeps one size at every interface size draws
// on it.
func uiSurfaceAt(dst *ebiten.Image, scale float64) *ebiten.Image {
	sub := dst.SubImage(dst.Bounds()).(*ebiten.Image)
	uiFrameSurfaces[sub] = uiSurface{scale, unscaleRect(dst.Bounds(), scale)}
	return sub
}

// uiPixelRect is r (UI units) in dst's own pixels, for the few passes that
// address pixels directly (shaders).
func uiPixelRect(dst *ebiten.Image, r image.Rectangle) image.Rectangle {
	return scaleRect(r, uiScaleOf(dst))
}

// uiPixels is a UI-unit extent in pixels at scale s, never short of the
// unit's last partial pixel.
func uiPixels(units int, s float64) int {
	return int(math.Ceil(float64(units)*s - uiScaleFuzz))
}

func scaleRect(r image.Rectangle, s float64) image.Rectangle {
	if s == 1 {
		return r
	}
	return image.Rect(int(math.Floor(float64(r.Min.X)*s+uiScaleFuzz)), int(math.Floor(float64(r.Min.Y)*s+uiScaleFuzz)), uiPixels(r.Max.X, s), uiPixels(r.Max.Y, s))
}

func unscaleRect(r image.Rectangle, s float64) image.Rectangle {
	if s == 1 {
		return r
	}
	unit := func(px int) int { return int(math.Floor(float64(px)/s + uiScaleFuzz)) }
	return image.Rect(unit(r.Min.X), unit(r.Min.Y), unit(r.Max.X), unit(r.Max.Y))
}

// uiLayer returns an offscreen layer of w x h UI units at the screen's
// resolution: layer itself while it still fits, else a fresh transparent
// image (layer is released). Primitives address a layer in UI units, and
// uiDrawImage composites it back pixel for pixel.
func uiLayer(layer *ebiten.Image, w, h int) *ebiten.Image {
	if uiLayerFits(layer, w, h) {
		return layer
	}
	uiReleaseLayer(layer)
	s := uiScreenScale
	img := ebiten.NewImage(max(1, uiPixels(w, s)), max(1, uiPixels(h, s)))
	uiLayerSurfaces[img] = uiSurface{s, image.Rect(0, 0, w, h)}
	return img
}

// uiLayerFits reports whether layer is a w x h layer at the screen's scale.
func uiLayerFits(layer *ebiten.Image, w, h int) bool {
	if layer == nil {
		return false
	}
	s, ok := uiLayerSurfaces[layer]
	return ok && s.scale == uiScreenScale && s.bounds.Size() == image.Pt(w, h)
}

func uiReleaseLayer(layer *ebiten.Image) {
	if layer == nil {
		return
	}
	delete(uiLayerSurfaces, layer)
	layer.Deallocate()
}

// uiResampleFilter replaces the nearest filter when art is scaled by k:
// whole steps stay exact, anything else takes the filter Ebitengine's own
// screen scaling uses, so pixels never double unevenly.
func uiResampleFilter(filter ebiten.Filter, k float64) ebiten.Filter {
	switch {
	case filter != ebiten.FilterNearest || isWholeScale(k):
		return filter
	case k > 1:
		return ebiten.FilterPixelated
	default:
		return ebiten.FilterLinear
	}
}

// uiLineWidth scales a stroke width; below scale 1 a line keeps at least one
// pixel, or thin rules would drop out of the frame.
func uiLineWidth(width, s float32) float32 {
	if s >= 1 || width <= 0 {
		return width * s
	}
	return max(width*s, min(width, 1))
}

func uiFillRect(dst *ebiten.Image, x, y, w, h float32, clr color.Color, aa bool) {
	s := float32(uiScaleOf(dst))
	vector.FillRect(dst, x*s, y*s, w*s, h*s, clr, aa)
}

func uiStrokeRect(dst *ebiten.Image, x, y, w, h, width float32, clr color.Color, aa bool) {
	s := float32(uiScaleOf(dst))
	vector.StrokeRect(dst, x*s, y*s, w*s, h*s, uiLineWidth(width, s), clr, aa)
}

func uiStrokeLine(dst *ebiten.Image, x0, y0, x1, y1, width float32, clr color.Color, aa bool) {
	s := float32(uiScaleOf(dst))
	vector.StrokeLine(dst, x0*s, y0*s, x1*s, y1*s, uiLineWidth(width, s), clr, aa)
}

func uiFillCircle(dst *ebiten.Image, cx, cy, r float32, clr color.Color, aa bool) {
	s := float32(uiScaleOf(dst))
	vector.FillCircle(dst, cx*s, cy*s, r*s, clr, aa)
}

func uiStrokeCircle(dst *ebiten.Image, cx, cy, r, width float32, clr color.Color, aa bool) {
	s := float32(uiScaleOf(dst))
	vector.StrokeCircle(dst, cx*s, cy*s, r*s, uiLineWidth(width, s), clr, aa)
}

// uiPath returns path in dst's pixels (the path itself stays in UI units).
func uiPath(dst *ebiten.Image, path *vector.Path) *vector.Path {
	s := uiScaleOf(dst)
	if s == 1 {
		return path
	}
	var scaled vector.Path
	op := &vector.AddPathOptions{}
	op.GeoM.Scale(s, s)
	scaled.AddPath(path, op)
	return &scaled
}

func uiFillPath(dst *ebiten.Image, path *vector.Path, fill *vector.FillOptions, draw *vector.DrawPathOptions) {
	vector.FillPath(dst, uiPath(dst, path), fill, draw)
}

func uiStrokePath(dst *ebiten.Image, path *vector.Path, stroke *vector.StrokeOptions, draw *vector.DrawPathOptions) {
	s := uiScaleOf(dst)
	if stroke != nil && s != 1 {
		scaled := *stroke
		scaled.Width = uiLineWidth(scaled.Width, float32(s))
		stroke = &scaled
	}
	vector.StrokePath(dst, uiPath(dst, path), stroke, draw)
}

// uiDrawImage draws src with op.GeoM in UI units. Art keeps its own pixels
// and is resampled once, straight to dst's resolution; a layer (uiLayer) is a
// UI-unit image already at that resolution and lands pixel for pixel.
func uiDrawImage(dst, src *ebiten.Image, op *ebiten.DrawImageOptions) {
	s, srcScale := uiScaleOf(dst), 1.0
	if layer, ok := uiLayerSurfaces[src]; ok {
		srcScale = layer.scale
	}
	if s == 1 && srcScale == 1 {
		dst.DrawImage(src, op)
		return
	}
	scaled := uiImageOptions(s, srcScale, op)
	dst.DrawImage(src, &scaled)
}

// uiImageOptions converts op (UI units) for a source holding srcScale pixels
// per unit onto a destination at dstScale.
func uiImageOptions(dstScale, srcScale float64, op *ebiten.DrawImageOptions) ebiten.DrawImageOptions {
	var scaled ebiten.DrawImageOptions
	if op != nil {
		scaled = *op
	}
	scaled.GeoM.Reset()
	scaled.GeoM.Scale(1/srcScale, 1/srcScale)
	if op != nil {
		scaled.GeoM.Concat(op.GeoM)
	}
	scaled.GeoM.Scale(dstScale, dstScale)
	scaled.Filter = uiResampleFilter(scaled.Filter, dstScale/srcScale)
	return scaled
}

// uiDrawImageScaled fits src into the UI-unit rect (x, y, w, h).
func uiDrawImageScaled(dst, src *ebiten.Image, x, y, w, h float64, op *ebiten.DrawImageOptions) {
	s := uiScaleOf(dst)
	graphics.DrawImageScaled(dst, src, x*s, y*s, w*s, h*s, op)
}

// uiDrawImageScaledEdgeGlow outlines src's silhouette around the UI-unit rect;
// offset is in UI units too.
func uiDrawImageScaledEdgeGlow(dst, src *ebiten.Image, x, y, w, h, offset float64, op *ebiten.DrawImageOptions) {
	s := uiScaleOf(dst)
	graphics.DrawImageScaledEdgeGlow(dst, src, x*s, y*s, w*s, h*s, offset*s, op)
}

func uiDrawTriangles(dst *ebiten.Image, vs []ebiten.Vertex, is []uint16, img *ebiten.Image, op *ebiten.DrawTrianglesOptions) {
	s := float32(uiScaleOf(dst))
	if s != 1 {
		scaled := make([]ebiten.Vertex, len(vs))
		for i, v := range vs {
			v.DstX, v.DstY = v.DstX*s, v.DstY*s
			scaled[i] = v
		}
		vs = scaled
	}
	dst.DrawTriangles(vs, is, img, op)
}

// rawCursorPosition is the pointer in the screen's pixels (a test seam).
var rawCursorPosition = ebiten.CursorPosition

// uiCursorPosition is the pointer in the screen's UI units.
func uiCursorPosition() (int, int) {
	x, y := rawCursorPosition()
	if uiScreenScale == 1 {
		return x, y
	}
	return int(float64(x) / uiScreenScale), int(float64(y) / uiScreenScale)
}
