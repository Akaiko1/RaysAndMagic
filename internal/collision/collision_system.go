package collision

import (
	"fmt"
	"math"
)

// DebugCanMoveTo runs the same checks as CanMoveTo but returns a human-readable reason
// when movement is blocked. Intended for temporary runtime debugging.
func (cs *CollisionSystem) DebugCanMoveTo(entityID string, newX, newY float64) (bool, string) {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false, "entity missing"
	}
	if cs.tileChecker == nil {
		return false, "tileChecker nil"
	}

	// Create a temporary bounding box at the new position
	tempBox := NewBoundingBox(newX, newY, entity.BoundingBox.Width, entity.BoundingBox.Height)

	// World tiles
	width, height := cs.tileChecker.GetWorldBounds()
	minX, minY, maxX, maxY := tempBox.GetBounds()
	startTileX := int(minX / cs.tileSize)
	startTileY := int(minY / cs.tileSize)
	endTileX := int(maxX / cs.tileSize)
	endTileY := int(maxY / cs.tileSize)

	for tileY := startTileY; tileY <= endTileY; tileY++ {
		for tileX := startTileX; tileX <= endTileX; tileX++ {
			if tileX < 0 || tileX >= width || tileY < 0 || tileY >= height {
				return false, fmt.Sprintf("out of bounds tile=(%d,%d) world=(%d,%d)", tileX, tileY, width, height)
			}
			if cs.tileChecker.IsTileBlocking(tileX, tileY) {
				return false, fmt.Sprintf("blocked tile=(%d,%d)", tileX, tileY)
			}
		}
	}

	// Other entities
	for id, other := range cs.entities {
		if id == entityID {
			continue
		}
		if !other.Solid {
			continue
		}
		if shouldIgnoreEntityCollision(entity, other) {
			continue
		}
		if tempBox.Intersects(other.BoundingBox) {
			return false, fmt.Sprintf("collides with entity=%s", id)
		}
	}

	return true, "ok"
}

// GetAllEntities returns a slice of all entities in the collision system
func (cs *CollisionSystem) GetAllEntities() []*Entity {
	entities := make([]*Entity, 0, len(cs.entities))
	for _, e := range cs.entities {
		entities = append(entities, e)
	}
	return entities
}

// TileChecker interface for checking if tiles block movement and sight
type TileChecker interface {
	IsTileBlocking(tileX, tileY int) bool
	IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string, flying bool) bool
	IsTileOpaque(tileX, tileY int) bool
	GetWorldBounds() (width, height int)
}

// CollisionSystem manages all collision detection in the game.
//
// CONCURRENCY CONTRACT (deliberately lock-free): the parallel PROJECTILE
// updater calls UpdateEntity/CanMoveTo* directly on the live system from worker
// goroutines. That is safe only while (1) updates are stop-the-world - no other
// game mutation runs concurrently, (2) each worker touches a DISJOINT set of
// entities (chunked partitioning) AND never reads another worker's entities -
// projectile movement only checks TILES (world.CanProjectileMoveTo), never
// other entities, so this holds - and (3) the entities map itself is never
// mutated (Register/Unregister) inside a parallel phase. Breaking any of these
// requires adding a lock here first.
//
// The parallel MONSTER updater does NOT qualify for the above: a monster's
// movement/AI decision reads OTHER entities' bounding boxes and collision types
// (CanMoveToWithHabitat -> canMoveToEntityPosition scans the whole map), which
// violates (2) - those other entities are concurrently being written by their
// OWN workers. It instead uses Snapshot(): each worker reads an immutable,
// frozen CollisionSnapshot (taken once, single-threaded, before the parallel
// phase) and the live system is only touched afterward, serially, once all
// workers have finished. See CollisionSnapshot's doc and
// game.MonsterWrapper/entities.EntityUpdater.UpdateMonstersParallel.
type CollisionSystem struct {
	tileChecker TileChecker
	entities    map[string]*Entity
	// engagedPosts indexes the (few) CollisionTypeMonsterEngaged entities so
	// attack-post reservation queries scan a handful, not every entity.
	// Maintained by RegisterEntity/UnregisterEntity/SetEntityCollisionType.
	engagedPosts map[string]*Entity
	tileSize     float64
}

// NewCollisionSystem creates a new collision system
func NewCollisionSystem(tileChecker TileChecker, tileSize float64) *CollisionSystem {
	return &CollisionSystem{
		tileChecker:  tileChecker,
		entities:     make(map[string]*Entity),
		engagedPosts: make(map[string]*Entity),
		tileSize:     tileSize,
	}
}

// RegisterEntity adds an entity to the collision system
func (cs *CollisionSystem) RegisterEntity(entity *Entity) {
	cs.entities[entity.ID] = entity
	if entity.CollisionType == CollisionTypeMonsterEngaged {
		cs.engagedPosts[entity.ID] = entity
	} else {
		delete(cs.engagedPosts, entity.ID)
	}
}

// UnregisterEntity removes an entity from the collision system
func (cs *CollisionSystem) UnregisterEntity(id string) {
	delete(cs.entities, id)
	delete(cs.engagedPosts, id)
}

// SetEntityCollisionType flips an entity's collision type, keeping the
// engaged-post index in sync. All type changes must come through here - a
// direct field write would leave IsMonsterAttackPostReserved blind to the
// claim. Single-threaded like every other live-system mutation.
func (cs *CollisionSystem) SetEntityCollisionType(id string, t CollisionType) {
	entity, exists := cs.entities[id]
	if !exists {
		return
	}
	entity.CollisionType = t
	if t == CollisionTypeMonsterEngaged {
		cs.engagedPosts[id] = entity
	} else {
		delete(cs.engagedPosts, id)
	}
}

// UpdateEntity updates an entity's position in the collision system
func (cs *CollisionSystem) UpdateEntity(id string, x, y float64) {
	if entity, exists := cs.entities[id]; exists {
		entity.BoundingBox.MoveTo(x, y)
	}
}

// UpdateTileChecker updates the tile checker (used when switching maps)
func (cs *CollisionSystem) UpdateTileChecker(tileChecker TileChecker) {
	cs.tileChecker = tileChecker
}

// CanMoveTo checks if an entity can move to the specified position
func (cs *CollisionSystem) CanMoveTo(entityID string, newX, newY float64) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}

	// Create a temporary bounding box at the new position
	tempBox := NewBoundingBox(newX, newY, entity.BoundingBox.Width, entity.BoundingBox.Height)

	// Check collision with world tiles
	if !cs.canMoveToWorldPosition(tempBox) {
		return false
	}

	// Check collision with other entities
	if !cs.canMoveToEntityPosition(entityID, entity, tempBox) {
		return false
	}

	return true
}

// CanMoveToWithHabitat checks if an entity can move to a position, allowing habitat tiles for monsters.
func (cs *CollisionSystem) CanMoveToWithHabitat(entityID string, newX, newY float64, habitatPrefs []string, flying bool) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}

	// Create a temporary bounding box at the new position
	tempBox := NewBoundingBox(newX, newY, entity.BoundingBox.Width, entity.BoundingBox.Height)

	// Check collision with world tiles (habitat-aware)
	if !cs.canMoveToWorldPositionWithHabitat(tempBox, habitatPrefs, flying) {
		return false
	}

	// Check collision with other entities
	if !cs.canMoveToEntityPosition(entityID, entity, tempBox) {
		return false
	}

	return true
}

// canMoveToWorldPosition checks collision with world tiles
func (cs *CollisionSystem) canMoveToWorldPosition(boundingBox *BoundingBox) bool {
	return tilesAllowPosition(cs.tileChecker, cs.tileSize, boundingBox)
}

// tilesAllowPosition is the tile-only half of CanMoveTo: a pure function of the
// (immutable, per-map) tile checker - never touches cs.entities - so it is
// shared verbatim by the live CollisionSystem and by CollisionSnapshot, which
// queries it from parallel workers against a frozen entity view.
func tilesAllowPosition(tileChecker TileChecker, tileSize float64, boundingBox *BoundingBox) bool {
	width, height := tileChecker.GetWorldBounds()

	// Get the tile range that the bounding box covers
	minX, minY, maxX, maxY := boundingBox.GetBounds()

	// Convert to tile coordinates
	startTileX := int(minX / tileSize)
	startTileY := int(minY / tileSize)
	endTileX := int(maxX / tileSize)
	endTileY := int(maxY / tileSize)

	// Check all tiles that the bounding box overlaps
	for tileY := startTileY; tileY <= endTileY; tileY++ {
		for tileX := startTileX; tileX <= endTileX; tileX++ {
			// Check bounds
			if tileX < 0 || tileX >= width || tileY < 0 || tileY >= height {
				return false
			}

			// Check if any overlapping tile blocks movement
			if tileChecker.IsTileBlocking(tileX, tileY) {
				return false
			}
		}
	}

	return true
}

// canMoveToWorldPositionWithHabitat checks collision with world tiles using habitat preferences.
func (cs *CollisionSystem) canMoveToWorldPositionWithHabitat(boundingBox *BoundingBox, habitatPrefs []string, flying bool) bool {
	return tilesAllowPositionWithHabitat(cs.tileChecker, cs.tileSize, boundingBox, habitatPrefs, flying)
}

// tilesAllowPositionWithHabitat is the habitat-aware counterpart of
// tilesAllowPosition - same sharing rationale (see its doc comment).
func tilesAllowPositionWithHabitat(tileChecker TileChecker, tileSize float64, boundingBox *BoundingBox, habitatPrefs []string, flying bool) bool {
	width, height := tileChecker.GetWorldBounds()

	// Get the tile range that the bounding box covers
	minX, minY, maxX, maxY := boundingBox.GetBounds()

	// Convert to tile coordinates
	startTileX := int(minX / tileSize)
	startTileY := int(minY / tileSize)
	endTileX := int(maxX / tileSize)
	endTileY := int(maxY / tileSize)

	// Check all tiles that the bounding box overlaps
	for tileY := startTileY; tileY <= endTileY; tileY++ {
		for tileX := startTileX; tileX <= endTileX; tileX++ {
			// Check bounds
			if tileX < 0 || tileX >= width || tileY < 0 || tileY >= height {
				return false
			}

			// Check if any overlapping tile blocks movement (habitat-aware)
			if tileChecker.IsTileBlockingForHabitat(tileX, tileY, habitatPrefs, flying) {
				return false
			}
		}
	}

	return true
}

// canMoveToEntityPosition checks collision with other entities
func (cs *CollisionSystem) canMoveToEntityPosition(movingEntityID string, movingEntity *Entity, boundingBox *BoundingBox) bool {
	for id, entity := range cs.entities {
		// Skip self
		if id == movingEntityID {
			continue
		}

		// Skip non-solid entities
		if !entity.Solid {
			continue
		}
		if shouldIgnoreEntityCollision(movingEntity, entity) {
			continue
		}

		// Check intersection
		if boundingBox.Intersects(entity.BoundingBox) {
			return false
		}
	}

	return true
}

// shouldIgnoreCollisionTypes is the type-only decision behind
// shouldIgnoreEntityCollision, factored out so CollisionSnapshot (which holds
// value copies, not *Entity pointers) can share it. CollisionTypeMonsterEngaged
// identifies a logical combat attack post; whether it blocks physically remains
// the entity's separate Solid flag. The normal gameplay path keeps these posts
// non-solid so transit mobs and the party can pass through them.
func shouldIgnoreCollisionTypes(moving, other CollisionType) bool {
	// A normal monster and a logical attack-post marker must both cross the
	// party while they are routing toward a different combat target. Attack
	// posts are deliberately non-solid in gameplay, but this keeps snapshots
	// and explicit solid-entity callers consistent with that rule.
	if (moving == CollisionTypeMonster || moving == CollisionTypeMonsterEngaged) &&
		other == CollisionTypePlayer {
		return true
	}

	// Allow only non-engaged monsters to walk through each other to prevent
	// pathfinding deadlocks in generic solid-entity scenarios.
	if (moving == CollisionTypeMonster || moving == CollisionTypeMonsterEngaged) &&
		(other == CollisionTypeMonster || other == CollisionTypeMonsterEngaged) {
		return moving == CollisionTypeMonster && other == CollisionTypeMonster
	}
	return false
}

// IsMonsterAttackPostReserved reports whether another monster has claimed the
// logical tile containing (x,y) as its combat attack post. The marker is
// CollisionTypeMonsterEngaged even though the entity is intentionally non-solid.
// Movement may pass through the tile; only settling to attack must avoid it.
func (cs *CollisionSystem) IsMonsterAttackPostReserved(entityID string, x, y float64) bool {
	if cs == nil || cs.tileSize <= 0 {
		return false
	}
	tileX, tileY := bucketCoord(x, cs.tileSize), bucketCoord(y, cs.tileSize)
	for id, entity := range cs.engagedPosts {
		if id == entityID || entity.BoundingBox == nil {
			continue
		}
		if bucketCoord(entity.BoundingBox.X, cs.tileSize) == tileX &&
			bucketCoord(entity.BoundingBox.Y, cs.tileSize) == tileY {
			return true
		}
	}
	return false
}

// shouldIgnoreEntityCollision returns true when collision checks should skip the
// pair. See shouldIgnoreCollisionTypes for the actual decision.
func shouldIgnoreEntityCollision(moving *Entity, other *Entity) bool {
	if moving == nil || other == nil {
		return false
	}
	return shouldIgnoreCollisionTypes(moving.CollisionType, other.CollisionType)
}

// CanOccupyTilesWithHabitat checks only world tiles (no entity collision).
// Used by the monster separation pass: two overlapping monsters veto each
// other's every move through the normal check, so pushing them apart must
// consult terrain alone.
func (cs *CollisionSystem) CanOccupyTilesWithHabitat(entityID string, x, y float64, habitatPrefs []string, flying bool) bool {
	entity, exists := cs.entities[entityID]
	if !exists {
		return false
	}
	tempBox := NewBoundingBox(x, y, entity.BoundingBox.Width, entity.BoundingBox.Height)
	return cs.canMoveToWorldPositionWithHabitat(tempBox, habitatPrefs, flying)
}

// GetCollisions returns all current collisions between entities
func (cs *CollisionSystem) GetCollisions() []CollisionPair {
	var collisions []CollisionPair

	// Convert entities to slice for indexed iteration
	entities := make([]*Entity, 0, len(cs.entities))
	for _, entity := range cs.entities {
		entities = append(entities, entity)
	}

	// Check all pairs
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			if entities[i].BoundingBox.Intersects(entities[j].BoundingBox) {
				collisions = append(collisions, CollisionPair{
					Entity1: entities[i],
					Entity2: entities[j],
				})
			}
		}
	}

	return collisions
}

// GetNearbyEntities returns entities within a certain distance of a point
func (cs *CollisionSystem) GetNearbyEntities(x, y, radius float64, excludeID string) []*Entity {
	var nearby []*Entity
	searchPoint := Point{X: x, Y: y}

	for id, entity := range cs.entities {
		if id == excludeID {
			continue
		}

		if entity.BoundingBox.DistanceToPoint(searchPoint) <= radius {
			nearby = append(nearby, entity)
		}
	}

	return nearby
}

// RaycastHit represents the result of a raycast operation
type RaycastHit struct {
	Hit   bool    // Whether the ray hit an opaque tile
	TileX int     // X coordinate of the hit tile
	TileY int     // Y coordinate of the hit tile
	Dist  float64 // Distance to the hit point
	HitX  float64 // World X coordinate of the hit point
	HitY  float64 // World Y coordinate of the hit point
}

// CastRay performs a DDA-based raycast between two points
func (cs *CollisionSystem) CastRay(x1, y1, x2, y2 float64, sightOnly bool) (RaycastHit, bool) {
	return castRayTiles(cs.tileChecker, cs.tileSize, x1, y1, x2, y2, sightOnly)
}

// castRayTiles is CastRay's body, parametrized over (tileChecker, tileSize) -
// both immutable for the lifetime of a map, so it needs no entity access and is
// shared verbatim by CollisionSnapshot.CheckLineOfSight.
func castRayTiles(tileChecker TileChecker, tileSize float64, x1, y1, x2, y2 float64, sightOnly bool) (RaycastHit, bool) {
	inv := 1.0 / tileSize

	// Current tile
	tx := int(x1 * inv)
	ty := int(y1 * inv)

	// Target tile
	gx := int(x2 * inv)
	gy := int(y2 * inv)

	dx := x2 - x1
	dy := y2 - y1

	// Handle zero distance. A sight ray sees out of its own tile (see the
	// start-tile note below); only movement is blocked by the start tile.
	if math.Abs(dx) < 1e-6 && math.Abs(dy) < 1e-6 {
		if !sightOnly && tileChecker.IsTileBlocking(tx, ty) {
			return RaycastHit{Hit: true, TileX: tx, TileY: ty, Dist: 0, HitX: x1, HitY: y1}, true
		}
		return RaycastHit{Hit: false}, false
	}

	// DDA setup
	var stepX, stepY int
	var tMaxX, tMaxY, tDeltaX, tDeltaY float64

	if dx > 0 {
		stepX = 1
		tMaxX = ((float64(tx)+1)*tileSize - x1) / dx
		tDeltaX = tileSize / dx
	} else if dx < 0 {
		stepX = -1
		tMaxX = (x1 - float64(tx)*tileSize) / -dx
		tDeltaX = tileSize / -dx
	} else {
		stepX = 0
		tMaxX = math.Inf(1)
		tDeltaX = math.Inf(1)
	}

	if dy > 0 {
		stepY = 1
		tMaxY = ((float64(ty)+1)*tileSize - y1) / dy
		tDeltaY = tileSize / dy
	} else if dy < 0 {
		stepY = -1
		tMaxY = (y1 - float64(ty)*tileSize) / -dy
		tDeltaY = tileSize / -dy
	} else {
		stepY = 0
		tMaxY = math.Inf(1)
		tDeltaY = math.Inf(1)
	}

	width, height := tileChecker.GetWorldBounds()
	maxT := math.Hypot(dx, dy) / math.Max(tileSize, 1)

	// Check starting tile. A sight ray always sees OUT of the observer's own
	// tile: the only way to occupy an opaque tile is a flying mob perched on a
	// solid-but-transparent sprite tile (boulder/canopy), which IsTileOpaque
	// reports opaque - bailing here would blind it to its own line of fire and
	// make ranged flyers shuffle instead of shooting. Movement is still blocked
	// by the start tile.
	if !sightOnly && tileChecker.IsTileBlocking(tx, ty) {
		return RaycastHit{Hit: true, TileX: tx, TileY: ty, Dist: 0, HitX: x1, HitY: y1}, true
	}

	t := 0.0
	for steps := 0; steps < int(maxT)+2; steps++ {
		// DDA step
		if tMaxX < tMaxY {
			tx += stepX
			t = tMaxX
			tMaxX += tDeltaX
		} else {
			ty += stepY
			t = tMaxY
			tMaxY += tDeltaY
		}

		// Check bounds
		if tx < 0 || ty < 0 || tx >= width || ty >= height {
			hitX := x1 + dx*t
			hitY := y1 + dy*t
			dist := math.Hypot(hitX-x1, hitY-y1)
			return RaycastHit{Hit: true, TileX: tx, TileY: ty, Dist: dist, HitX: hitX, HitY: hitY}, true
		}

		// Check for hit based on mode
		if sightOnly && tileChecker.IsTileOpaque(tx, ty) {
			hitX := x1 + dx*t
			hitY := y1 + dy*t
			dist := math.Hypot(hitX-x1, hitY-y1)
			return RaycastHit{Hit: true, TileX: tx, TileY: ty, Dist: dist, HitX: hitX, HitY: hitY}, true
		} else if !sightOnly && tileChecker.IsTileBlocking(tx, ty) {
			hitX := x1 + dx*t
			hitY := y1 + dy*t
			dist := math.Hypot(hitX-x1, hitY-y1)
			return RaycastHit{Hit: true, TileX: tx, TileY: ty, Dist: dist, HitX: hitX, HitY: hitY}, true
		}

		// Check if we reached the target tile
		if tx == gx && ty == gy {
			return RaycastHit{Hit: false}, false
		}
	}

	return RaycastHit{Hit: false}, false
}

// CheckLineOfSight checks if there's a clear line of sight between two points
func (cs *CollisionSystem) CheckLineOfSight(x1, y1, x2, y2 float64) bool {
	hit, _ := cs.CastRay(x1, y1, x2, y2, true)
	return !hit.Hit
}

// CollisionPair represents a collision between two entities
type CollisionPair struct {
	Entity1 *Entity
	Entity2 *Entity
}

// GetCollisionDistance returns the overlap distance between two colliding entities
func (cp *CollisionPair) GetCollisionDistance() float64 {
	return cp.Entity1.BoundingBox.Distance(cp.Entity2.BoundingBox)
}

// GetCollisionNormal returns the collision normal vector (normalized)
func (cp *CollisionPair) GetCollisionNormal() (float64, float64) {
	dx := cp.Entity2.BoundingBox.X - cp.Entity1.BoundingBox.X
	dy := cp.Entity2.BoundingBox.Y - cp.Entity1.BoundingBox.Y

	// Normalize
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		return 0, 0
	}

	return dx / length, dy / length
}

// GetEntityByID returns the entity with the given ID, or nil if not found
func (cs *CollisionSystem) GetEntityByID(id string) *Entity {
	if entity, ok := cs.entities[id]; ok {
		return entity
	}
	return nil
}
