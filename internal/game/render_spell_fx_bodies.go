package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// fxSegment draws a straight solid bar between two points - what turns a row of
// dots into a filament, a thong or a shaft.
func (r *Renderer) fxSegment(screen *ebiten.Image, x1, y1, x2, y2, thick float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return
	}
	r.drawGlowRectRotated(screen, (x1+x2)/2, (y1+y2)/2, length, thick, math.Atan2(dy, dx), rgb, alpha, blend)
}
