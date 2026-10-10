package game

import (
	"math"
)

// projectileView describes flight relative to the sightline at the projectile,
// not the camera's center axis. A shot released at the viewer exposes its rear
// face even when aimed toward the edge of the screen.
type projectileView struct {
	side, depth    float64
	right, forward float64
	distance       float64
}

func (r *Renderer) projectileView(x, y, vx, vy float64) projectileView {
	speed := math.Hypot(vx, vy)
	if speed == 0 {
		return projectileView{}
	}
	pose := r.game.logicalCameraPose()
	dx, dy := x-pose.x, y-pose.y
	distance := math.Hypot(dx, dy)
	if distance > 0 {
		dx, dy = dx/distance, dy/distance
	} else {
		dx, dy = math.Cos(pose.angle), math.Sin(pose.angle)
	}
	return projectileView{
		distance: distance,
		side:     (-vx*dy + vy*dx) / speed,
		depth:    (vx*dx + vy*dy) / speed,
		right:    -dx*math.Sin(r.game.camera.Angle) + dy*math.Cos(r.game.camera.Angle),
		forward:  dx*math.Cos(r.game.camera.Angle) + dy*math.Sin(r.game.camera.Angle),
	}
}

// Volume vertices are projected in camera space, so rotate their local
// sightline-relative axis back before perspective projection.
func (v projectileView) cameraAxis(axis [3]float64) [3]float64 {
	return [3]float64{v.forward*axis[0] + v.right*axis[2], axis[1], -v.right*axis[0] + v.forward*axis[2]}
}

func (v projectileView) screenDir() (float64, bool) {
	if v.sideWeight() == 0 {
		return 0, false
	}
	return math.Copysign(1, v.side), true
}

func (v projectileView) incoming() bool { return v.depth < 0 }

// Below one 8-bit alpha step a second silhouette has no useful contribution.
const projectileAlphaThreshold = 1.0 / 255

func (v projectileView) sideWeight() float64 {
	weight := v.side * v.side
	if weight < projectileAlphaThreshold {
		return 0
	}
	if weight > 1-projectileAlphaThreshold {
		return 1
	}
	return weight
}
func (v projectileView) faceWeight() float64 { return 1 - v.sideWeight() }

// Axial foreshortening must not shrink the perpendicular cross-section.
func projectilePerpendicular(dx, dy float64) (float64, float64) {
	if length := math.Hypot(dx, dy); length > 0 {
		return -dy / length, dx / length
	}
	return 0, 1
}
