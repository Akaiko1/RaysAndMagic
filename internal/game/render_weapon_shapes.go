package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// Every shipped melee item has its own procedural model. Synthetic effects
// without a source weapon retain a procedural category model.
func (r *Renderer) drawWeaponSilhouette(dst *ebiten.Image, kind, style string, x, y, angle, size, alpha float64) {
	model, ok := weaponBodyModels[r.weaponMaterialState.weaponKey]
	if !ok {
		switch kind {
		case "stab":
			model = 9
		case "lunge":
			model = 20
		case "smash":
			model = 41
		case "chop":
			model = 60
		}
		switch style {
		case "katana":
			model = 4
		case "muramasa":
			model = 6
		case "war_fan":
			model = 54
		case "clock_minute":
			model = 74
		case "clock_chime":
			model = 71
		case "clock_escapement":
			model = 72
		case "clock_cogfang":
			model = 70
		case "clock_mainspring":
			model = 73
		}
	}
	if kind == "smash" {
		angle -= math.Pi / 2
	}
	r.drawWeaponBody(dst, x, y, angle, size, alpha, kind, model)
}
