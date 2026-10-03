package game

import (
	"slices"

	monsterPkg "ugataima/internal/monster"
)

// areaReach is the ground an area effect covers: a radius around its origin.
// Like every point blast it stops at walls, unless it fills the ground around
// the party (a nova) or falls from above (lobbed artillery). Every area hit
// picks its victims through it, so radius, walls and exclusions mean the same
// everywhere; who it may hurt stays with the caller.
type areaReach struct {
	x, y, radius float64
	throughWalls bool
}

// pointBlast is an explosion at a point, radius in tiles.
func (cs *CombatSystem) pointBlast(x, y, radiusTiles float64) areaReach {
	return areaReach{x: x, y: y, radius: radiusTiles * float64(cs.game.config.GetTileSize())}
}

// partyNova fills the ground around the party, walls or not.
func (cs *CombatSystem) partyNova(radius float64) areaReach {
	return areaReach{x: cs.game.camera.X, y: cs.game.camera.Y, radius: radius, throughWalls: true}
}

// reaches reports whether the area covers the point.
func (cs *CombatSystem) reaches(a areaReach, x, y float64) bool {
	dx, dy := x-a.x, y-a.y
	return dx*dx+dy*dy <= a.radius*a.radius && (a.throughWalls || cs.attackLineClear(a.x, a.y, x, y))
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
