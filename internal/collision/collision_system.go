package collision

import (
	"math"
)

// GetAllEntities returns a slice of all entities in the collision system
func (cs *CollisionSystem) GetAllEntities() []*Entity {
	entities := make([]*Entity, 0, len(cs.entities))
	for _, e := range cs.entities {
		entities = append(entities, e)
	}
	return entities
}

// TileChecker interface for checking if tiles block movement
type TileChecker interface {
	IsTileBlocking(tileX, tileY int) bool
	GetWorldBounds() (width, height int)
}

// CollisionSystem manages all collision detection in the game
type CollisionSystem struct {
	tileChecker TileChecker
	entities    map[string]*Entity
	tileSize    float64
}

// NewCollisionSystem creates a new collision system
func NewCollisionSystem(tileChecker TileChecker, tileSize float64) *CollisionSystem {
	return &CollisionSystem{
		tileChecker: tileChecker,
		entities:    make(map[string]*Entity),
		tileSize:    tileSize,
	}
}

// RegisterEntity adds an entity to the collision system
func (cs *CollisionSystem) RegisterEntity(entity *Entity) {
	cs.entities[entity.ID] = entity
}

// UnregisterEntity removes an entity from the collision system
func (cs *CollisionSystem) UnregisterEntity(id string) {
	delete(cs.entities, id)
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
	if !cs.canMoveToEntityPosition(entityID, tempBox) {
		return false
	}

	return true
}

// canMoveToWorldPosition checks collision with world tiles
func (cs *CollisionSystem) canMoveToWorldPosition(boundingBox *BoundingBox) bool {
	width, height := cs.tileChecker.GetWorldBounds()

	// Get the tile range that the bounding box covers
	minX, minY, maxX, maxY := boundingBox.GetBounds()

	// Convert to tile coordinates
	startTileX := int(minX / cs.tileSize)
	startTileY := int(minY / cs.tileSize)
	endTileX := int(maxX / cs.tileSize)
	endTileY := int(maxY / cs.tileSize)

	// Check all tiles that the bounding box overlaps
	for tileY := startTileY; tileY <= endTileY; tileY++ {
		for tileX := startTileX; tileX <= endTileX; tileX++ {
			// Check bounds
			if tileX < 0 || tileX >= width || tileY < 0 || tileY >= height {
				return false
			}

			// Check if any overlapping tile blocks movement
			if cs.tileChecker.IsTileBlocking(tileX, tileY) {
				return false
			}
		}
	}

	return true
}

// canMoveToEntityPosition checks collision with other entities
func (cs *CollisionSystem) canMoveToEntityPosition(movingEntityID string, boundingBox *BoundingBox) bool {
	for id, entity := range cs.entities {
		// Skip self
		if id == movingEntityID {
			continue
		}

		// Skip non-solid entities
		if !entity.Solid {
			continue
		}

		// Check intersection
		if boundingBox.Intersects(entity.BoundingBox) {
			return false
		}
	}

	return true
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

// CheckLineOfSight checks if there's a clear line of sight between two points
func (cs *CollisionSystem) CheckLineOfSight(x1, y1, x2, y2 float64) bool {
	// Simple raycast implementation
	steps := 50
	dx := (x2 - x1) / float64(steps)
	dy := (y2 - y1) / float64(steps)

	width, height := cs.tileChecker.GetWorldBounds()

	for i := 0; i <= steps; i++ {
		checkX := x1 + dx*float64(i)
		checkY := y1 + dy*float64(i)

		tileX := int(checkX / cs.tileSize)
		tileY := int(checkY / cs.tileSize)

		// Check bounds
		if tileX < 0 || tileX >= width || tileY < 0 || tileY >= height {
			return false
		}

		// Check if tile blocks line of sight
		if cs.tileChecker.IsTileBlocking(tileX, tileY) {
			return false
		}
	}

	return true
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
