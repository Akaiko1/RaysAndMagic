package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// One row per wheel gesture, including fractional trackpad deltas.
func rosterScrollAfterWheel(current, maximum int, wheel float64) int {
	current = min(maximum, max(0, current))
	if wheel < 0 {
		return min(maximum, current+1)
	}
	if wheel > 0 {
		return max(0, current-1)
	}
	return current
}

// Roster controls share the game's button frame, but the arrow is geometry,
// never a font glyph. Integer anchors keep it crisp at every logical size.
func (ui *UISystem) drawScrollArrowButton(screen *ebiten.Image, x, y, w, h int, up, hover, enabled bool) {
	ui.drawButtonFrame(screen, x, y, w, h, hover && enabled)
	cx, cy := float32(x+w/2), float32(y+h/2)
	size := float32(math.Floor(float64(min(w, h)) * 0.22))
	direction := float32(1)
	if up {
		direction = -1
	}
	tint := color.RGBA{224, 183, 100, 255}
	if !enabled {
		tint = color.RGBA{104, 94, 75, 255}
	} else if hover {
		tint = color.RGBA{255, 225, 157, 255}
	}
	for _, pass := range []struct {
		offset, width float32
		color         color.RGBA
	}{{1, 3, color.RGBA{16, 14, 12, 255}}, {0, 2, tint}} {
		base := cy + pass.offset
		tip := base + direction*size
		shoulder := base - direction
		vector.StrokeLine(screen, cx-size, shoulder, cx, tip, pass.width, pass.color, true)
		vector.StrokeLine(screen, cx, tip, cx+size, shoulder, pass.width, pass.color, true)
		vector.StrokeLine(screen, cx, tip-direction, cx, base-direction*size, pass.width, pass.color, true)
	}
}
