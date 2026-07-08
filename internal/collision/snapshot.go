package collision

import "math"

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
	// Spatial index over the SOLID entities: tileSize-sided cells -> indices
	// into solids. Entity passability queries (A* expands thousands of nodes
	// per tick) test only the buckets the probe box overlaps instead of
	// scanning every entity. An entity is inserted into every bucket its box
	// overlaps, so two intersecting boxes always share a bucket - results are
	// identical to the linear scan. Nil when tileSize is 0 (hand-built test
	// snapshots): queries fall back to the linear path.
	solids  []snapEntity
	buckets map[bucketKey][]int32
}

// snapEntity is one solid entity in the snapshot's spatial index.
type snapEntity struct {
	id            string
	box           BoundingBox
	collisionType CollisionType
}

type bucketKey struct{ x, y int32 }

func bucketCoord(v, size float64) int32 {
	return int32(math.Floor(v / size))
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
	if snap.tileSize > 0 {
		snap.solids = make([]snapEntity, 0, len(cs.entities))
		snap.buckets = make(map[bucketKey][]int32, len(cs.entities)*2)
	}
	for id, e := range cs.entities {
		snap.entities[id] = EntitySnapshot{
			Box:           *e.BoundingBox,
			CollisionType: e.CollisionType,
			Solid:         e.Solid,
		}
		if snap.buckets == nil || !e.Solid {
			continue
		}
		idx := int32(len(snap.solids))
		snap.solids = append(snap.solids, snapEntity{id: id, box: *e.BoundingBox, collisionType: e.CollisionType})
		minX, minY, maxX, maxY := e.BoundingBox.GetBounds()
		bx0, by0 := bucketCoord(minX, snap.tileSize), bucketCoord(minY, snap.tileSize)
		bx1, by1 := bucketCoord(maxX, snap.tileSize), bucketCoord(maxY, snap.tileSize)
		for by := by0; by <= by1; by++ {
			for bx := bx0; bx <= bx1; bx++ {
				k := bucketKey{bx, by}
				snap.buckets[k] = append(snap.buckets[k], idx)
			}
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
// the snapshot's value-copy entities (no *Entity pointers to share). Queries
// the spatial index when available; an entity spanning several buckets may be
// tested more than once, which only repeats the same cheap intersect test.
func (cs *CollisionSnapshot) canMoveToEntityPosition(movingID string, movingType CollisionType, box *BoundingBox) bool {
	if cs.buckets == nil {
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

	minX, minY, maxX, maxY := box.GetBounds()
	bx0, by0 := bucketCoord(minX, cs.tileSize), bucketCoord(minY, cs.tileSize)
	bx1, by1 := bucketCoord(maxX, cs.tileSize), bucketCoord(maxY, cs.tileSize)
	for by := by0; by <= by1; by++ {
		for bx := bx0; bx <= bx1; bx++ {
			for _, idx := range cs.buckets[bucketKey{bx, by}] {
				other := &cs.solids[idx]
				if other.id == movingID {
					continue
				}
				if shouldIgnoreCollisionTypes(movingType, other.collisionType) {
					continue
				}
				if box.Intersects(&other.box) {
					return false
				}
			}
		}
	}
	return true
}
