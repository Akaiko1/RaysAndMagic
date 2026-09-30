package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Keep the background below the live minimap and the outlines above it.
// Local coordinates also avoid coordinate-dependent vector tessellation cost.
// These derived images depend only on radius, never world or save identity.
const compassFramePadding = 12 // Outer ring and antialiasing coverage.

func (ui *UISystem) ensureCompassFrame(radius int) {
	center := radius + compassFramePadding
	if ui.compassFrameRadius != radius || !uiLayerFits(ui.compassFrameBackground, 2*center, 2*center) {
		ui.releaseCompassFrame()
		background := uiLayer(nil, 2*center, 2*center)
		border := uiLayer(nil, 2*center, 2*center)
		c, r := float32(center), float32(radius)
		uiFillCircle(background, c, c, r+5, color.RGBA{66, 48, 24, 245}, true)
		uiFillCircle(background, c, c, r+3, color.RGBA{194, 153, 66, 255}, true)
		uiFillCircle(background, c, c, r, color.RGBA{8, 14, 23, 235}, true)
		uiStrokeCircle(border, c, c, r, 2, color.RGBA{98, 140, 181, 245}, true)
		uiStrokeCircle(border, c, c, r+4, 1, color.RGBA{255, 218, 115, 245}, true)
		mask := uiLayer(nil, 2*radius, 2*radius)
		uiFillCircle(mask, r, r, r, color.White, true)
		ui.compassMapMask = mask
		ui.compassFrameBackground, ui.compassFrameOutline = background, border
		ui.compassFrameRadius = radius
	}
}

func (ui *UISystem) drawCompassFrame(dst *ebiten.Image, x, y, radius int, outline bool) {
	ui.ensureCompassFrame(radius)
	center := radius + compassFramePadding
	img := ui.compassFrameBackground
	if outline {
		img = ui.compassFrameOutline
	}
	var opts ebiten.DrawImageOptions
	opts.GeoM.Translate(float64(x-center), float64(y-center))
	uiDrawImage(dst, img, &opts)
}

func (ui *UISystem) releaseCompassFrame() {
	uiReleaseLayer(ui.compassFrameBackground)
	uiReleaseLayer(ui.compassFrameOutline)
	uiReleaseLayer(ui.compassMapMask)
	ui.compassFrameBackground, ui.compassFrameOutline, ui.compassMapMask = nil, nil, nil
}
