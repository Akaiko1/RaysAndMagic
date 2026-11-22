package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// Every shipped melee item has its own procedural model. Synthetic effects
// without a source weapon retain a procedural category model.
func (r *Renderer) drawWeaponSilhouette(dst *ebiten.Image, kind swingKind, style string, x, y, angle, size, alpha float64) {
	model, ok := weaponBodyModels[r.weaponMaterialState.weaponKey]
	if !ok {
		switch kind {
		case swingStab:
			model = bodyParryDagger
		case swingLunge:
			model = bodyIronSpear
		case swingSmash:
			model = bodySteelMace
		case swingChop:
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
		case "clock_cogfang":
			model = bodyCogfangBlade
		case "clock_mainspring":
			model = bodyMainspringPike
		}
	}
	if kind == swingSmash {
		angle -= math.Pi / 2
	}
	r.drawWeaponBody(dst, x, y, angle, size, alpha, kind, model)
}
