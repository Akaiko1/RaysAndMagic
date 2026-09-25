package collision

// SightChecker is shared by live collision, frozen AI snapshots and test worlds.
type SightChecker interface {
	CheckLineOfSight(x1, y1, x2, y2 float64) bool
}

// CanAttackFrom distinguishes an airborne attack position from a walkable
// ground tile. Test checkers without terrain metadata keep their sight contract.
func CanAttackFrom(checker SightChecker, x, y float64) bool {
	if origin, ok := checker.(interface{ CanAttackFrom(float64, float64) bool }); ok {
		return origin.CanAttackFrom(x, y)
	}
	return true
}

// AttackLineClear requires valid endpoints and reciprocal visibility. Movement
// permissions (flight or terrain overrides) never grant attacks inside objects.
func AttackLineClear(checker SightChecker, x1, y1, x2, y2 float64) bool {
	if attack, ok := checker.(interface {
		AttackLineClear(float64, float64, float64, float64) bool
	}); ok {
		return attack.AttackLineClear(x1, y1, x2, y2)
	}
	return checker == nil || CanAttackFrom(checker, x1, y1) && CanAttackFrom(checker, x2, y2) &&
		checker.CheckLineOfSight(x1, y1, x2, y2) && checker.CheckLineOfSight(x2, y2, x1, y1)
}

func canAttackFromTile(tiles TileChecker, tileSize float64, blockers map[sightTileKey]int, x, y float64) bool {
	tx, ty := tileCoord(x, 1/tileSize), tileCoord(y, 1/tileSize)
	width, height := tiles.GetWorldBounds()
	if tx < 0 || ty < 0 || tx >= width || ty >= height || blockers[sightTileKey{x: tx, y: ty}] > 0 {
		return false
	}
	if terrain, ok := tiles.(interface{ CanProjectileMoveTo(float64, float64) bool }); ok {
		return terrain.CanProjectileMoveTo(x, y)
	}
	return !tiles.IsTileOpaque(tx, ty)
}

// CanAttackFrom uses the world's projectile-height terrain rule; water and gaps
// are open airspace, while walls and blocking scenery are not firing positions.
func (cs *CollisionSystem) CanAttackFrom(x, y float64) bool {
	cs.sightMu.RLock()
	defer cs.sightMu.RUnlock()
	return canAttackFromTile(cs.tileChecker, cs.tileSize, cs.sightBlockerTiles, x, y)
}

func (cs *CollisionSnapshot) CanAttackFrom(x, y float64) bool {
	return canAttackFromTile(cs.tileChecker, cs.tileSize, cs.sightBlockerTiles, x, y)
}

// AttackLineClear observes both endpoints and rays under one lock, so a moving
// door cannot change the blocker map halfway through a live attack query.
func (cs *CollisionSystem) AttackLineClear(x1, y1, x2, y2 float64) bool {
	cs.sightMu.RLock()
	defer cs.sightMu.RUnlock()
	return attackLineClearTiles(cs.tileChecker, cs.tileSize, cs.sightBlockerTiles, x1, y1, x2, y2)
}

func (cs *CollisionSnapshot) AttackLineClear(x1, y1, x2, y2 float64) bool {
	return attackLineClearTiles(cs.tileChecker, cs.tileSize, cs.sightBlockerTiles, x1, y1, x2, y2)
}

func attackLineClearTiles(tiles TileChecker, size float64, blockers map[sightTileKey]int, x1, y1, x2, y2 float64) bool {
	if !canAttackFromTile(tiles, size, blockers, x1, y1) || !canAttackFromTile(tiles, size, blockers, x2, y2) {
		return false
	}
	if hit, _ := castRayTiles(tiles, size, blockers, x1, y1, x2, y2, true); hit.Hit {
		return false
	}
	hit, _ := castRayTiles(tiles, size, blockers, x2, y2, x1, y1, true)
	return !hit.Hit
}
