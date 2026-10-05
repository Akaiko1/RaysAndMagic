package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// Hammer choreography supplies the cross-head angle; the shared weapon model
// expects the haft axis. The striking blocks lead and the grip follows them.
func (r *Renderer) drawHammerHead(dst *ebiten.Image, x, y, size, angle float64, stone [3]int, alpha float64) {
	r.drawWeaponSilhouette(dst, "smash", "", x, y, angle-math.Pi/2, size, stone, alpha)
}
