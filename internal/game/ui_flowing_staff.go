package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Like Overwatch's reticle, this is a native-size HUD glyph: a gold staff
// wrapped in a turquoise current, with a dark backing for daylight contrast.
func drawFlowingStaffCharge(screen *ebiten.Image, box layoutRect) {
	x, y := float32(box.x+6), float32(box.y+6)
	uiFillCircle(screen, x, y, 5.8, color.NRGBA{9, 23, 27, 242}, true)
	uiStrokeCircle(screen, x, y, 5.1, .8, color.NRGBA{73, 160, 155, 255}, true)
	uiStrokeLine(screen, x-2.2, y+3.6, x+1.7, y-3, 1.7, color.NRGBA{228, 190, 111, 255}, true)
	uiFillCircle(screen, x+1.7, y-3, 1.3, color.NRGBA{203, 255, 239, 255}, true)
	uiStrokeLine(screen, x-3.6, y-.6, x-1.2, y+.7, 1.1, color.NRGBA{86, 238, 209, 255}, true)
	uiStrokeLine(screen, x-1.2, y+.7, x+3.6, y-1.1, 1.1, color.NRGBA{86, 238, 209, 255}, true)
}

// The Sun Crucible's stored flare in the same native-size language: a captive
// sun held by three bronze ribs on a warm dark backing.
func drawSolarFlareCharge(screen *ebiten.Image, box layoutRect) {
	x, y := float32(box.x+6), float32(box.y+6)
	bronze := color.NRGBA{214, 150, 72, 255}
	uiFillCircle(screen, x, y, 5.8, color.NRGBA{27, 14, 8, 242}, true)
	uiStrokeCircle(screen, x, y, 5.1, .8, color.NRGBA{150, 98, 46, 255}, true)
	uiStrokeLine(screen, x, y+4.2, x, y+1.4, 1.2, bronze, true)
	uiStrokeLine(screen, x-.4, y+2.4, x-3, y-1.6, 1, bronze, true)
	uiStrokeLine(screen, x+.4, y+2.4, x+3, y-1.6, 1, bronze, true)
	uiFillCircle(screen, x, y-1.1, 2.4, color.NRGBA{255, 160, 52, 255}, true)
	uiFillCircle(screen, x-.3, y-1.5, 1.1, color.NRGBA{255, 242, 196, 255}, true)
}
