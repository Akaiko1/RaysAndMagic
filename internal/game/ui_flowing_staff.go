package game

import (
	"image/color"
	"math"

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

// The Skirmisher's loaded shot: a fletched arrow whose head carries the gold
// spark the combat log gives a critical hit.
func drawSkirmishCharge(screen *ebiten.Image, box layoutRect) {
	x, y := float32(box.x+6), float32(box.y+6)
	fletch := color.NRGBA{176, 98, 66, 255}
	uiFillCircle(screen, x, y, 5.8, color.NRGBA{16, 15, 11, 242}, true)
	uiStrokeCircle(screen, x, y, 5.1, .8, color.NRGBA{132, 104, 64, 255}, true)
	uiStrokeLine(screen, x-3.3, y+3.3, x+2.2, y-2.2, 1.1, color.NRGBA{226, 210, 176, 255}, true)
	uiStrokeLine(screen, x-3.3, y+3.3, x-3.4, y+1.4, .9, fletch, true)
	uiStrokeLine(screen, x-3.3, y+3.3, x-1.4, y+3.4, .9, fletch, true)
	uiStrokeLine(screen, x+2.6, y-4.4, x+2.6, y-.8, .9, combatMessageGold, true)
	uiStrokeLine(screen, x+.8, y-2.6, x+4.4, y-2.6, .9, combatMessageGold, true)
	uiFillCircle(screen, x+2.6, y-2.6, 1.1, color.NRGBA{255, 246, 200, 255}, true)
}

// The Clockwork Pistol's wound springs: a brass mainspring coiled out from a
// steel arbor, in the tower's own brass.
func drawWindUpCharge(screen *ebiten.Image, box layoutRect) {
	x, y := float32(box.x+6), float32(box.y+6)
	brass := compassRGB(clockBrass, 255)
	uiFillCircle(screen, x, y, 5.8, color.NRGBA{18, 14, 9, 242}, true)
	uiStrokeCircle(screen, x, y, 5.1, .8, color.NRGBA{150, 112, 60, 255}, true)
	const segments, turns = 20, 1.75
	px, py := x, y
	for i := 1; i <= segments; i++ {
		t := float64(i) / segments
		a, r := t*turns*2*math.Pi, .6+t*3.4
		nx, ny := x+float32(r*math.Cos(a)), y+float32(r*math.Sin(a))
		uiStrokeLine(screen, px, py, nx, ny, .9, brass, true)
		px, py = nx, ny
	}
	uiFillCircle(screen, x, y, 1.1, compassRGB(clockSteel, 255), true)
}
