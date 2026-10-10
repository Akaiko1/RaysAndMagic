package game

import (
	"math"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
)

// projectileLaunch freezes the physical ray and selected actor at release.
// Only presentation follows that actor's existing visual displacement.
type projectileLaunch struct {
	valid                   bool
	x, y, dx, dy            float64
	angle, distance, cone   float64
	target                  *monster.Monster3D
	aimX, aimY, aimDistance float64
	// passed freezes the target's drawn offset once a pierce leaves it.
	passed           bool
	passedX, passedY float64
	// startX/startY is a continuation leg's drawn offset at the actor it left.
	startX, startY            float64
	startAnchor, passedAnchor monsterVisualAnchor
}

// presentationReturnTiles is how far a drawn shot travels while easing from
// an actor's drawn position back onto its physical ray.
const presentationReturnTiles = 1.0

// continuationLaunch starts a ricochet or backwash leg at from's physical
// position. It is drawn leaving from's sprite and arriving at to's sprite; hit
// detection keeps the ungated world contact of every continuation.
func (cs *CombatSystem) continuationLaunch(from, to *monster.Monster3D) projectileLaunch {
	d := math.Hypot(to.X-from.X, to.Y-from.Y)
	if d <= 0 {
		return projectileLaunch{}
	}
	fx, fy := cs.monsterVisualPos(from)
	return projectileLaunch{x: from.X, y: from.Y, dx: (to.X - from.X) / d, dy: (to.Y - from.Y) / d,
		target: to, aimX: to.X, aimY: to.Y, aimDistance: d, startX: fx - from.X, startY: fy - from.Y,
		startAnchor: visualAnchorFor(from)}
}

// piercing hands the launch to a continuation that has just struck victim.
// Leaving the selected target freezes its drawn offset, so the ease back onto
// the physical ray survives a killing hit.
func (p projectileLaunch) piercing(cs *CombatSystem, victim *monster.Monster3D) projectileLaunch {
	if victim != nil && victim == p.target && !p.passed {
		if ox, oy, ok := p.targetOffset(cs); ok {
			p.passed, p.passedX, p.passedY = true, ox, oy
			p.passedAnchor = visualAnchorFor(victim)
		}
	}
	return p
}

// targetOffset is the selected target's drawn offset from its release position.
func (p projectileLaunch) targetOffset(cs *CombatSystem) (float64, float64, bool) {
	if p.passed {
		return p.passedX, p.passedY, true
	}
	if p.target == nil || !p.target.IsAlive() || p.aimDistance <= 0 || p.target.X != p.aimX || p.target.Y != p.aimY {
		return 0, 0, false
	}
	tx, ty := cs.monsterVisualPos(p.target)
	return tx - p.aimX, ty - p.aimY, true
}

// partyProjectileLaunch is shared by weapons, spells and flasks. Explicit aim
// keeps its target; keyboard aim chooses the nearest visible foe in the same
// forward cone and authored range, in either combat clock.
func (cs *CombatSystem) partyProjectileLaunch(angle, maxRange float64, explicit bool) projectileLaunch {
	g := cs.game
	x, y := cs.logicalCameraXY()
	cone := min(g.camera.FOV/2, projectileAssistMaxAngleRad)
	launch := projectileLaunch{valid: true, x: x, y: y, angle: angle, distance: maxRange, cone: cone}
	best := cs.partyAimTarget
	if best == nil && !explicit {
		bestDistance := maxRange
		for _, target := range g.world.Monsters {
			if target == nil || !target.IsAlive() || isExcludedFromPartyAutoTarget(target) {
				continue
			}
			d := math.Hypot(target.X-x, target.Y-y)
			vx, vy := cs.monsterVisualPos(target)
			if d <= 0 || d > bestDistance || !cs.attackLineClear(x, y, target.X, target.Y) ||
				!headingTowardWithin(x, y, math.Cos(angle), math.Sin(angle), vx, vy, cone) {
				continue
			}
			if best == nil || d < bestDistance || (d == bestDistance && target.ID < best.ID) {
				best, bestDistance = target, d
			}
		}
	}
	if best != nil {
		d := math.Hypot(best.X-x, best.Y-y)
		if d > 0 {
			launch.angle = math.Atan2(best.Y-y, best.X-x)
			launch.distance = min(maxRange, d)
			launch.target = best
			launch.aimX, launch.aimY, launch.aimDistance = best.X, best.Y, d
		}
	}
	launch.dx, launch.dy = math.Cos(launch.angle), math.Sin(launch.angle)
	return launch
}

// Authored width is lateral tolerance, not extra forward travel. A shot must
// leave the muzzle and reach the target's depth before its wide box can hit.
// Trailing volley darts cannot hit anything behind the shared release plane.
func (p projectileLaunch) reaches(x, y float64, target *collision.BoundingBox) bool {
	if !p.valid {
		return true
	}
	travel := (x-p.x)*p.dx + (y-p.y)*p.dy
	depth := (target.X-p.x)*p.dx + (target.Y-p.y)*p.dy
	if travel <= 0 || depth <= 0 || !headingTowardWithin(p.x, p.y, p.dx, p.dy, target.X, target.Y, p.cone) {
		return false
	}
	halfDepth := (math.Abs(p.dx)*target.Width + math.Abs(p.dy)*target.Height) / 2
	return travel+halfDepth >= depth
}

// Apply the actor's shared visual displacement to body, velocity and trail.
// Camera turns and TB/RT switches therefore use the same presentation as the
// monster. A physically moved target never steers either ray after launch.
// The target's offset grows to full at the target's depth and eases out over
// presentationReturnTiles for a shot that flies on past it; a continuation leg
// likewise eases out of the drawn offset of the actor it left.
func (p projectileLaunch) renderMotion(cs *CombatSystem, x, y, vx, vy float64) (float64, float64, float64, float64) {
	if cs == nil || cs.game == nil || cs.game.config == nil {
		return x, y, vx, vy
	}
	speed := vx*p.dx + vy*p.dy
	w, rate, start, startRate := p.presentationWeights(x, y, cs.game.config.GetTileSize())
	// Drawn offset and its rate of change per unit of travel along the ray.
	var ox, oy, rx, ry float64
	if tx, ty, ok := p.targetOffset(cs); ok {
		ox, oy, rx, ry = w*tx, w*ty, rate*tx, rate*ty
	}
	ox, oy = ox+start*p.startX, oy+start*p.startY
	rx, ry = rx+startRate*p.startX, ry+startRate*p.startY
	return x + ox, y + oy, vx + speed*rx, vy + speed*ry
}

// Both presentation axes use these weights, including continuation departures
// and the return to the physical ray after piercing a target.
func (p projectileLaunch) presentationWeights(x, y, tileSize float64) (target, targetRate, start, startRate float64) {
	travel := (x-p.x)*p.dx + (y-p.y)*p.dy
	ease := presentationReturnTiles * tileSize
	if d := p.aimDistance; d > 0 {
		switch {
		case travel <= d:
			target, targetRate = travel/d, 1/d
		case travel < d+ease:
			target, targetRate = 1-(travel-d)/ease, -1/ease
		}
	}
	departure := ease
	if p.aimDistance > 0 {
		departure = min(departure, p.aimDistance)
	}
	if travel < departure {
		start = 1 - max(0, travel)/departure
		if travel > 0 {
			startRate = -1 / departure
		}
	}
	return
}
