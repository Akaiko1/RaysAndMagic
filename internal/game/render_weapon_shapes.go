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
			model = bodyParryDagger
		case "lunge":
			model = bodyIronSpear
		case "smash":
			model = bodySteelMace
		case "chop":
			model = bodySteelAxe
		default:
			model = bodyIronSword
		}
		switch style {
		case "katana":
			model = bodyKatana
		case "muramasa":
			model = bodyMuramasa
		case "war_fan":
			model = bodyGaleWarFan
		case "clock_minute":
			model = bodyMinuteHand
		case "clock_chime":
			model = bodyChimeMaul
		case "clock_escapement":
			model = bodyEscapementMace
		case "clock_cogfang":
			model = bodyCogfangBlade
		case "clock_mainspring":
			model = bodyMainspringPike
		}
	}
	if kind == "smash" {
		angle -= math.Pi / 2
	}
	r.drawWeaponBody(dst, x, y, angle, size, alpha, kind, model)
}
