package entities

import (
	"math"
	"sync"
	"ugataima/internal/threading/core"
)

// EntityUpdater manages parallel updates for game entities
type EntityUpdater struct {
	workerPool *core.WorkerPool
	// mutex      sync.RWMutex
}

// NewEntityUpdater creates a new entity updater
func NewEntityUpdater() *EntityUpdater {
	return &EntityUpdater{
		workerPool: core.CreateDefaultWorkerPool(),
	}
}

// MonsterUpdateInterface defines the interface for monsters that can be updated
type MonsterUpdateInterface interface {
	Update()
	IsAlive() bool
	GetPosition() (float64, float64)
	SetPosition(x, y float64)
}

// UpdateMonstersParallel updates all monsters in parallel
func (eu *EntityUpdater) UpdateMonstersParallel(monsters []MonsterUpdateInterface) {
	if len(monsters) == 0 {
		return
	}

	core.ParallelForEach(monsters, func(monster MonsterUpdateInterface) {
		if monster.IsAlive() {
			monster.Update()
		}
	})
}

// ProjectileUpdateInterface defines the interface for projectiles
type ProjectileUpdateInterface interface {
	Update()
	IsActive() bool
	GetPosition() (float64, float64)
	SetPosition(x, y float64)
	GetVelocity() (float64, float64)
	SetVelocity(vx, vy float64)
	GetLifetime() int
	SetLifetime(lifetime int)
	OnCollision(hitX, hitY float64)
}

// UpdateProjectilesParallel updates all projectiles in parallel
func (eu *EntityUpdater) UpdateProjectilesParallel(projectiles []ProjectileUpdateInterface, canMoveTo func(x, y float64) bool) {
	if len(projectiles) == 0 {
		return
	}

	core.ParallelForEach(projectiles, func(projectile ProjectileUpdateInterface) {
		if !projectile.IsActive() {
			return
		}

		// Update position
		x, y := projectile.GetPosition()
		vx, vy := projectile.GetVelocity()

		newX := x + vx
		newY := y + vy

		// Check collision (this needs to be thread-safe)
		if canMoveTo(newX, newY) {
			projectile.SetPosition(newX, newY)
		} else {
			// Projectile hit something, deactivate it
			projectile.OnCollision(x, y)
			projectile.SetLifetime(0)
		}

		// Update lifetime
		lifetime := projectile.GetLifetime()
		projectile.SetLifetime(lifetime - 1)
	})
}

// SpatialHash provides thread-safe spatial partitioning for collision detection
type SpatialHash struct {
	cellSize int
	cells    map[CellKey][]EntityID
	entities map[EntityID]*SpatialEntity
	mutex    sync.RWMutex
}

// CellKey represents a cell in the spatial hash
type CellKey struct {
	X, Y int
}

// EntityID is a unique identifier for entities
type EntityID int

// SpatialEntity represents an entity in the spatial hash
type SpatialEntity struct {
	ID       EntityID
	X, Y     float64
	Radius   float64
	Category string // "monster", "projectile", "player"
}

// NewSpatialHash creates a new spatial hash with the given cell size
func NewSpatialHash(cellSize int) *SpatialHash {
	return &SpatialHash{
		cellSize: cellSize,
		cells:    make(map[CellKey][]EntityID),
		entities: make(map[EntityID]*SpatialEntity),
	}
}

// AddEntity adds an entity to the spatial hash
func (sh *SpatialHash) AddEntity(entity *SpatialEntity) {
	sh.mutex.Lock()
	defer sh.mutex.Unlock()

	// Remove from old position if exists
	if oldEntity, exists := sh.entities[entity.ID]; exists {
		sh.removeFromCells(oldEntity)
	}

	// Add to new position
	sh.entities[entity.ID] = entity
	sh.addToCells(entity)
}

// RemoveEntity removes an entity from the spatial hash
func (sh *SpatialHash) RemoveEntity(entityID EntityID) {
	sh.mutex.Lock()
	defer sh.mutex.Unlock()

	if entity, exists := sh.entities[entityID]; exists {
		sh.removeFromCells(entity)
		delete(sh.entities, entityID)
	}
}

// GetNearbyEntities returns all entities near the given position
func (sh *SpatialHash) GetNearbyEntities(x, y, radius float64) []*SpatialEntity {
	sh.mutex.RLock()
	defer sh.mutex.RUnlock()

	entities := make([]*SpatialEntity, 0)
	visited := make(map[EntityID]bool)

	// Calculate cell range
	minCellX := int((x - radius) / float64(sh.cellSize))
	maxCellX := int((x + radius) / float64(sh.cellSize))
	minCellY := int((y - radius) / float64(sh.cellSize))
	maxCellY := int((y + radius) / float64(sh.cellSize))

	// Check all relevant cells
	for cellX := minCellX; cellX <= maxCellX; cellX++ {
		for cellY := minCellY; cellY <= maxCellY; cellY++ {
			key := CellKey{X: cellX, Y: cellY}
			if entityIDs, exists := sh.cells[key]; exists {
				for _, entityID := range entityIDs {
					if !visited[entityID] {
						if entity, exists := sh.entities[entityID]; exists {
							// Check actual distance
							dx := entity.X - x
							dy := entity.Y - y
							distance := math.Sqrt(dx*dx + dy*dy)
							if distance <= radius+entity.Radius {
								entities = append(entities, entity)
								visited[entityID] = true
							}
						}
					}
				}
			}
		}
	}

	return entities
}

// addToCells adds an entity to the appropriate cells
func (sh *SpatialHash) addToCells(entity *SpatialEntity) {
	// Calculate cell range based on entity position and radius
	minCellX := int((entity.X - entity.Radius) / float64(sh.cellSize))
	maxCellX := int((entity.X + entity.Radius) / float64(sh.cellSize))
	minCellY := int((entity.Y - entity.Radius) / float64(sh.cellSize))
	maxCellY := int((entity.Y + entity.Radius) / float64(sh.cellSize))

	for cellX := minCellX; cellX <= maxCellX; cellX++ {
		for cellY := minCellY; cellY <= maxCellY; cellY++ {
			key := CellKey{X: cellX, Y: cellY}
			sh.cells[key] = append(sh.cells[key], entity.ID)
		}
	}
}

// removeFromCells removes an entity from all cells
func (sh *SpatialHash) removeFromCells(entity *SpatialEntity) {
	minCellX := int((entity.X - entity.Radius) / float64(sh.cellSize))
	maxCellX := int((entity.X + entity.Radius) / float64(sh.cellSize))
	minCellY := int((entity.Y - entity.Radius) / float64(sh.cellSize))
	maxCellY := int((entity.Y + entity.Radius) / float64(sh.cellSize))

	for cellX := minCellX; cellX <= maxCellX; cellX++ {
		for cellY := minCellY; cellY <= maxCellY; cellY++ {
			key := CellKey{X: cellX, Y: cellY}
			if entityIDs, exists := sh.cells[key]; exists {
				// Remove entity ID from the slice
				for i, id := range entityIDs {
					if id == entity.ID {
						sh.cells[key] = append(entityIDs[:i], entityIDs[i+1:]...)
						break
					}
				}
				// Clean up empty cells
				if len(sh.cells[key]) == 0 {
					delete(sh.cells, key)
				}
			}
		}
	}
}

// ParallelCollisionDetection performs collision detection in parallel
type ParallelCollisionDetection struct {
	spatialHash *SpatialHash
	workerPool  *core.WorkerPool
}

// NewParallelCollisionDetection creates a new parallel collision detection system
func NewParallelCollisionDetection(cellSize int) *ParallelCollisionDetection {
	return &ParallelCollisionDetection{
		spatialHash: NewSpatialHash(cellSize),
		workerPool:  core.CreateDefaultWorkerPool(),
	}
}

// CollisionPair represents a collision between two entities
type CollisionPair struct {
	Entity1, Entity2 *SpatialEntity
	Distance         float64
}

// DetectCollisions detects all collisions in parallel
func (pcd *ParallelCollisionDetection) DetectCollisions(entities []*SpatialEntity) []CollisionPair {
	// Update spatial hash
	for _, entity := range entities {
		pcd.spatialHash.AddEntity(entity)
	}

	collisions := make([]CollisionPair, 0)
	var collisionsMutex sync.Mutex

	// Process entities in parallel
	core.ParallelForEach(entities, func(entity *SpatialEntity) {
		nearby := pcd.spatialHash.GetNearbyEntities(entity.X, entity.Y, entity.Radius*2)

		for _, other := range nearby {
			if entity.ID >= other.ID { // Avoid duplicate pairs
				continue
			}

			dx := entity.X - other.X
			dy := entity.Y - other.Y
			distance := math.Sqrt(dx*dx + dy*dy)

			if distance <= entity.Radius+other.Radius {
				collisionsMutex.Lock()
				collisions = append(collisions, CollisionPair{
					Entity1:  entity,
					Entity2:  other,
					Distance: distance,
				})
				collisionsMutex.Unlock()
			}
		}
	})

	return collisions
}

// Stop shuts down the entity updater
func (eu *EntityUpdater) Stop() {
	eu.workerPool.Stop()
}

// Stop shuts down the parallel collision detection
func (pcd *ParallelCollisionDetection) Stop() {
	pcd.workerPool.Stop()
}
