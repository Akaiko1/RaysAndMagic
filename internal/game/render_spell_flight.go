package game

import (
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
// the target sightline and expose their end face.
func (r *Renderer) spellHandOffset(shot MagicProjectile, p projectileFxProfile, view projectileView, width, height int) (float64, float64, float64) {
	switch p.style {
	case "ice_shard", "void_needle", "fire_dart", "shadow_bolt", "harm", "ray_of_light":
	default:
		return 0, 0, 0
	}
	if shot.Owner != ProjectileOwnerPlayer || view.incoming() {
		return 0, 0, 0
	}
	convergence := bowHandConvergence(view.distance, float64(r.game.config.GetTileSize())) * view.faceWeight()
	return float64(width) * .055 * convergence, float64(height) * .025 * convergence, convergence
}

func (v projectileView) spellProjection(convergence float64) spellProjection {
	p := spellProjection{dx: 1, head: v.faceWeight(), axial: math.Max(.08, math.Abs(v.side)), axis: v.cameraAxis([3]float64{v.side, 0, v.depth})}
	p.axial += (1 - p.axial) * p.head
	if dir, ok := v.screenDir(); ok {
		p.dx = dir
	}
	if convergence > 0 {
		angle := -math.Pi + .10 + .08*convergence
		p.dx, p.dy, p.head, p.axial = math.Cos(angle), math.Sin(angle), 0, math.Max(.07, convergence)
		p.axis = v.cameraAxis([3]float64{v.side - .45*convergence, -.12 * convergence, v.depth})
	}
	return p
}
