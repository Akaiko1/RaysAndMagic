package game

import (
	"math"
	"slices"

	monsterPkg "ugataima/internal/monster"
)

// areaReach is the ground an area effect covers: a radius around its origin,
// or a lane from it.
// Like every point blast it stops at walls, unless it fills the ground around
// the party (a nova) or falls from above (lobbed artillery). Every area hit
// picks its victims through it, so radius, walls and exclusions mean the same
// everywhere; who it may hurt stays with the caller.
type areaReach struct {
	x, y, radius float64
	throughWalls bool
	// A lane runs from (x, y) along the unit (dx, dy) for length, halfWidth
	// to each side; it replaces the radius.
	dx, dy, length, halfWidth float64
}

// pointBlast is an explosion at a point, radius in tiles.
func (cs *CombatSystem) pointBlast(x, y, radiusTiles float64) areaReach {
	return areaReach{x: x, y: y, radius: radiusTiles * float64(cs.game.config.GetTileSize())}
}

// laneBlast is a lane from (x, y) along a unit direction, sizes in tiles.
func (cs *CombatSystem) laneBlast(x, y, dx, dy, lengthTiles, widthTiles float64) areaReach {
	tile := float64(cs.game.config.GetTileSize())
	return areaReach{x: x, y: y, dx: dx, dy: dy, length: lengthTiles * tile, halfWidth: widthTiles * tile / 2}
}

// partyNova fills the ground around the party, walls or not.
func (cs *CombatSystem) partyNova(radius float64) areaReach {
	return areaReach{x: cs.game.camera.X, y: cs.game.camera.Y, radius: radius, throughWalls: true}
}

// reaches reports whether the area covers the point.
func (cs *CombatSystem) reaches(a areaReach, x, y float64) bool {
	dx, dy := x-a.x, y-a.y
	inside := dx*dx+dy*dy <= a.radius*a.radius
	if a.length > 0 {
		along, across := dx*a.dx+dy*a.dy, math.Abs(-dx*a.dy+dy*a.dx)
		inside = along >= 0 && along <= a.length && across <= a.halfWidth
	}
	return inside && (a.throughWalls || cs.attackLineClear(a.x, a.y, x, y))
}

// reachesParty reports whether the area covers the party.
func (cs *CombatSystem) reachesParty(a areaReach) bool {
	return cs.reaches(a, cs.game.camera.X, cs.game.camera.Y)
}

// forEachAreaVictim visits every live monster the area covers that hurts lets
// it touch, skipping the excluded (its source, the target it burst on).
func (cs *CombatSystem) forEachAreaVictim(a areaReach, hurts func(*monsterPkg.Monster3D) bool, visit func(*monsterPkg.Monster3D), exclude ...*monsterPkg.Monster3D) {
	if cs.game.world == nil {
		return
	}
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || slices.Contains(exclude, m) || !hurts(m) || !cs.reaches(a, m.X, m.Y) {
			continue
		}
		visit(m)
	}
}
