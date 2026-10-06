package game

import "github.com/hajimehoshi/ebiten/v2"

// End-on spells use the same material with depth-facing geometry: narrow
// lances expose their facets and lightning branches around its charged core.
func (r *Renderer) drawSpellProjectileFxHeadOn(screen *ebiten.Image, cx, cy, size float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	r.drawSpellProjectileFx(screen, cx, cy, size, 0, 0, core, p, critBoost, id)
}
