package game

import (
	"image"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// displayDeviceScale is the monitor's device pixels per window pixel (2 on
// Retina and HiDPI-scaled monitors). A seam so tests can pose any display.
var displayDeviceScale = func() float64 {
	if m := ebiten.Monitor(); m != nil {
		if s := m.DeviceScaleFactor(); s > 0 {
			return s
		}
	}
	return 1
}

// wholeScaleTolerance absorbs the rounding of a frame sized in whole steps of
// the screen (3000/999 = 3.003); anything closer than this is a whole scale.
const wholeScaleTolerance = 0.01

func isWholeScale(scale float64) bool {
	return scale >= 1-wholeScaleTolerance && math.Abs(scale-math.Round(scale)) <= wholeScaleTolerance
}

// interfaceSizeFrame is one interface size preset resolved for the current window.
type interfaceSizeFrame struct {
	w, h  int
	scale float64 // screen pixels per UI unit
	sharp bool    // scale is a whole number, so every text pixel stays square
	same  bool    // this screen offers nothing larger than the previous preset
}

// pixelScale is the screen's pixels per UI unit: a whole scale is snapped to
// its step, so every unit covers the same square of pixels.
func (f interfaceSizeFrame) pixelScale() float64 {
	if f.sharp {
		return math.Round(f.scale)
	}
	return f.scale
}

// resolveInterfaceFrames maps every preset onto the window. Art is drawn from
// its source at any scale, but the pixel font only scales crisply by whole
// numbers, so the presets above Normal take the whole-number steps above it,
// in order, while the UI minimum still fits. A screen with no such step (1080p
// and smaller) falls back to the preset's fractional scale, where text is
// larger but slightly soft. Normal is unchanged.
func resolveInterfaceFrames(sizes []config.InterfaceSize, outW, outH int, deviceScale float64) []interfaceSizeFrame {
	frames := make([]interfaceSizeFrame, len(sizes))
	if len(sizes) == 0 || outW <= 0 || outH <= 0 {
		return frames
	}
	pw := int(math.Round(float64(outW) * max(deviceScale, 1)))
	ph := int(math.Round(float64(outH) * max(deviceScale, 1)))
	frame := func(w, h int) interfaceSizeFrame {
		scale := min(float64(pw)/float64(w), float64(ph)/float64(h))
		return interfaceSizeFrame{w: w, h: h, scale: scale, sharp: isWholeScale(scale)}
	}
	frames[0] = frame(logicalScreenSize(outW, outH, 1))
	minW, minH := MinimumWindowSize()
	var steps []interfaceSizeFrame
	for k := int(math.Floor(frames[0].scale+wholeScaleTolerance)) + 1; pw/k >= minW && ph/k >= minH; k++ {
		steps = append(steps, frame(pw/k, ph/k))
	}
	for i := 1; i < len(sizes); i++ {
		switch {
		case i-1 < len(steps):
			frames[i] = steps[i-1]
		case len(steps) == 0:
			frames[i] = frame(logicalScreenSize(outW, outH, sizes[i].Scale))
		default:
			frames[i] = frames[i-1]
		}
		frames[i].same = frames[i].w == frames[i-1].w && frames[i].h == frames[i-1].h
	}
	return frames
}

// DrawFinalScreen keeps whole-number frame scales exact. Layout hands back the
// screen's own pixels, which can fall a pixel short of the window (x1.0003),
// and Ebitengine would then pick its soft pixelated filter; snapping to the
// step and to whole-pixel offsets keeps the crisp nearest-neighbour copy.
func (g *MMGame) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, geoM ebiten.GeoM) {
	ebiten.DefaultDrawFinalScreen(screen, offscreen, snapFinalScreenGeoM(screen.Bounds(), offscreen.Bounds(), geoM))
}

func snapFinalScreenGeoM(screen, frame image.Rectangle, geoM ebiten.GeoM) ebiten.GeoM {
	scale := geoM.Element(0, 0)
	if !isWholeScale(scale) {
		return geoM
	}
	k := math.Round(scale)
	var snapped ebiten.GeoM
	snapped.Scale(k, k)
	snapped.Translate(
		math.Floor((float64(screen.Dx())-float64(frame.Dx())*k)/2),
		math.Floor((float64(screen.Dy())-float64(frame.Dy())*k)/2),
	)
	return snapped
}
