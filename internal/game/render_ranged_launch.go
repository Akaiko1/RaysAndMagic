package game

import "math"

// rangedRelease is where a bow shot left the hand on screen, before Panini:
// the bow stays there for its animation, aimed along the shot as it was seen
// (aim relative to the camera), however the party steps or turns.
type rangedRelease struct {
	x, y, aim, pitch float64
	valid            bool
}

// rangedReleaseRay projects the shot's displayed ray where it leaves the hand,
// once, at release: the target's visual displacement and body height count,
// later target moves never re-aim the bow.
func (r *Renderer) rangedReleaseRay(launch projectileLaunch) rangedRelease {
	g := r.game
	if !launch.valid {
		return rangedRelease{}
	}
	distance := g.config.GetTileSize()
	vx, vy := math.Cos(launch.angle), math.Sin(launch.angle)
	px, py := g.camera.X+vx*distance, g.camera.Y+vy*distance
	rx, ry, rvx, rvy := launch.renderMotion(g.combat, px, py, vx, vy)
	drop := launch.eyeDrop(g, px, py)
	p, ok := r.projectMovingEntity(rx, ry, drop, 1, 1, 1)
	if !ok {
		return rangedRelease{}
	}
	return rangedRelease{x: float64(p.screenX), y: p.centerY, aim: math.Atan2(rvy, rvx) - g.camera.Angle, pitch: math.Atan2(drop, distance), valid: true}
}

// rangedBowRelease is the bow's attachment point and aim this frame. Camera-
// local models are not warped by Panini; only this point crosses from the
// world projection to display pixels.
func (r *Renderer) rangedBowRelease(fx rangedHeldAnimation) (x, y, aim, pitch float64) {
	g := r.game
	w, h := float64(g.worldWidth()), float64(g.worldHeight())
	x, y, aim = w*.5, g.viewHorizon(), fx.aim()
	if rel := fx.release; rel.valid {
		x, y, aim, pitch = rel.x, rel.y, rel.aim, rel.pitch
	}
	hx, hy := bowLaunchHandOffset(w, h, 1)
	x, y = g.widePaniniProjection().presentedPoint(x+hx, y+hy)
	return
}

// The arrow and its held bow use the same right-hand release offset. The
// arrow eases onto the physical ray during its first three tiles of travel.
func bowLaunchHandOffset(width, height, convergence float64) (float64, float64) {
	return width * .055 * convergence, height * .025 * convergence
}
