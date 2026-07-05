package collision

// EntitySnapshot is a value copy of one entity's mutable state, frozen at
// Snapshot() time - safe to read concurrently no matter what happens to the
// live Entity afterward.
type EntitySnapshot struct {
	Box           BoundingBox
	CollisionType CollisionType
	Solid         bool
}

// CollisionSnapshot is an immutable, concurrency-safe point-in-time view of a
// CollisionSystem's entities (plus its tile checker, which is already
// immutable for the lifetime of a map). Build ONE per tick with Snapshot()
// BEFORE dispatching parallel workers; every worker then queries this frozen
// copy instead of the live CollisionSystem, so any number of goroutines can
// read it concurrently with zero locking - including while OTHER goroutines
// are writing their own entities on the LIVE system, since a snapshot and the
// system it was taken from share no mutable memory.
//
// This is what makes the real-time monster update race-free: see
// game.MonsterWrapper / entities.EntityUpdater.UpdateMonstersParallel for the
// two-phase flow (workers compute against a snapshot in Update(); the caller
// applies each monster's resulting position + collision type to the LIVE
// system afterward, serially, once all workers have finished).
type CollisionSnapshot struct {
	tileChecker TileChecker
	tileSize    float64
	entities    map[string]EntitySnapshot
}

// Snapshot copies the current entity state into an immutable view. O(entities)
// - call once per tick, never per query; a fresh copy per call is what makes
// concurrent reads free of locks afterward.
func (cs *CollisionSystem) Snapshot() *CollisionSnapshot {
	snap := &CollisionSnapshot{
		tileChecker: cs.tileChecker,
		tileSize:    cs.tileSize,
		entities:    make(map[string]EntitySnapshot, len(cs.entities)),
	}
	for id, e := range cs.entities {
		snap.entities[id] = EntitySnapshot{
			Box:           *e.BoundingBox,
			CollisionType: e.CollisionType,
			Solid:         e.Solid,
		}
	}
	return snap
}

// CanMoveTo mirrors CollisionSystem.CanMoveTo, reading only the frozen view.
func (cs *CollisionSnapshot) CanMoveTo(entityID string, newX, newY float64) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}
	tempBox := NewBoundingBox(newX, newY, entity.Box.Width, entity.Box.Height)
	if !tilesAllowPosition(cs.tileChecker, cs.tileSize, tempBox) {
		return false
	}
	return cs.canMoveToEntityPosition(entityID, entity.CollisionType, tempBox)
}

// CanMoveToWithHabitat mirrors CollisionSystem.CanMoveToWithHabitat, reading
// only the frozen view.
func (cs *CollisionSnapshot) CanMoveToWithHabitat(entityID string, newX, newY float64, habitatPrefs []string, flying bool) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}
	tempBox := NewBoundingBox(newX, newY, entity.Box.Width, entity.Box.Height)
	if !tilesAllowPositionWithHabitat(cs.tileChecker, cs.tileSize, tempBox, habitatPrefs, flying) {
		return false
	}
	return cs.canMoveToEntityPosition(entityID, entity.CollisionType, tempBox)
}

// CanOccupyTilesWithHabitat mirrors CollisionSystem.CanOccupyTilesWithHabitat
// (tiles only, no entity check) - see that method's doc for why.
func (cs *CollisionSnapshot) CanOccupyTilesWithHabitat(entityID string, x, y float64, habitatPrefs []string, flying bool) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}
	tempBox := NewBoundingBox(x, y, entity.Box.Width, entity.Box.Height)
	return tilesAllowPositionWithHabitat(cs.tileChecker, cs.tileSize, tempBox, habitatPrefs, flying)
}

// CheckLineOfSight mirrors CollisionSystem.CheckLineOfSight. Rays only ever
// consult tiles (never entities), so this was already race-free against the
// live system too - implemented here for interface parity and so callers
// don't need to special-case which collision source they're holding.
func (cs *CollisionSnapshot) CheckLineOfSight(x1, y1, x2, y2 float64) bool {
	hit, _ := castRayTiles(cs.tileChecker, cs.tileSize, x1, y1, x2, y2, true)
	return !hit.Hit
}

// canMoveToEntityPosition is CollisionSystem.canMoveToEntityPosition adapted to
// the snapshot's value-copy entities (no *Entity pointers to share).
func (cs *CollisionSnapshot) canMoveToEntityPosition(movingID string, movingType CollisionType, box *BoundingBox) bool {
	for id, other := range cs.entities {
		if id == movingID || !other.Solid {
			continue
		}
		if shouldIgnoreCollisionTypes(movingType, other.CollisionType) {
			continue
		}
		otherBox := other.Box
		if box.Intersects(&otherBox) {
			return false
		}
	}
	return true
}
