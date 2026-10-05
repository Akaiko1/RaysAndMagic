package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

func directionalSpellKind(kind int) bool {
	switch kind {
	case spellIce, spellVoidNeedle, spellFireDart, spellShadow, spellHarm, spellLight:
		return true
	}
	return false
}

// Like outgoing arrows, pointed spells leave the hand in profile, converge on
// the view axis and expose their end face. Incoming shots never show a broadside.
func (r *Renderer) spellHandOffset(shot MagicProjectile, p projectileFxProfile, width, height int) (float64, float64, float64) {
	switch p.style {
	case "ice_shard", "void_needle", "fire_dart", "shadow_bolt", "harm", "ray_of_light":
	default:
		return 0, 0, 0
	}
	if shot.Owner != ProjectileOwnerPlayer || r.projectileMovesTowardCamera(shot.VelX, shot.VelY) {
		return 0, 0, 0
	}
	if _, side := r.projectileScreenDir(shot.VelX, shot.VelY); side {
		return 0, 0, 0
	}
	distance := math.Hypot(shot.X-r.game.camera.X, shot.Y-r.game.camera.Y)
	convergence := bowHandConvergence(distance, float64(r.game.config.GetTileSize()))
	return float64(width) * .055 * convergence, float64(height) * .025 * convergence, convergence
}

func (r *Renderer) drawOutgoingSpellFromHand(dst *ebiten.Image, x, y, size float64, shot MagicProjectile, rgb [3]int, p projectileFxProfile, crit float64, id int) bool {
	_, _, convergence := r.spellHandOffset(shot, p, dst.Bounds().Dx(), dst.Bounds().Dy())
	if convergence <= 0 {
		return false
	}
	// The volume turns toward the vanishing point while leaving the hand.
	previousAxis := r.spellFlightAxis
	r.spellFlightAxis = [3]float64{-.45 * convergence, -.12 * convergence, 1}
	angle := -math.Pi + .10 + .08*convergence
	previous := r.spellAxialScale
	r.spellAxialScale = math.Max(.07, convergence)
	r.drawSpellProjectileFx(dst, x, y, size, math.Cos(angle), math.Sin(angle), rgb, p, crit, id)
	r.spellAxialScale = previous
	r.spellFlightAxis = previousAxis
	return true
}
