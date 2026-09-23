package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Keep the background below the live minimap and the outlines above it.
// Local coordinates also avoid coordinate-dependent vector tessellation cost.
// These derived images depend only on radius, never world or save identity.
const compassFramePadding = 12 // Outer ring and antialiasing coverage.

func (ui *UISystem) ensureCompassFrame(radius int) {
	center := radius + compassFramePadding
	if ui.compassFrameBackground == nil || ui.compassFrameRadius != radius {
		ui.releaseCompassFrame()
		background := ebiten.NewImage(2*center, 2*center)
		border := ebiten.NewImage(2*center, 2*center)
		c, r := float32(center), float32(radius)
		vector.FillCircle(background, c, c, r+5, color.RGBA{66, 48, 24, 245}, true)
		vector.FillCircle(background, c, c, r+3, color.RGBA{194, 153, 66, 255}, true)
		vector.FillCircle(background, c, c, r, color.RGBA{8, 14, 23, 235}, true)
		vector.StrokeCircle(border, c, c, r, 2, color.RGBA{98, 140, 181, 245}, true)
		vector.StrokeCircle(border, c, c, r+4, 1, color.RGBA{255, 218, 115, 245}, true)
		mask := ebiten.NewImage(2*radius, 2*radius)
		vector.FillCircle(mask, r, r, r, color.White, true)
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
	dst.DrawImage(img, &opts)
}

func (ui *UISystem) releaseCompassFrame() {
	if ui.compassFrameBackground != nil {
		ui.compassFrameBackground.Deallocate()
		ui.compassFrameOutline.Deallocate()
		ui.compassMapMask.Deallocate()
		ui.compassMapMask = nil
		ui.compassFrameBackground, ui.compassFrameOutline = nil, nil
	}
}
