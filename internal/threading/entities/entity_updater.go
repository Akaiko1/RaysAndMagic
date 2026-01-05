package entities

import (
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

// UpdateMonstersParallel updates all monsters in parallel using the reusable worker pool.
func (eu *EntityUpdater) UpdateMonstersParallel(monsters []MonsterUpdateInterface) {
	if len(monsters) == 0 {
		return
	}

	numWorkers := eu.workerPool.GetNumWorkers()
	chunkSize := (len(monsters) + numWorkers - 1) / numWorkers
	if chunkSize < 1 {
		chunkSize = 1
	}

	for i := 0; i < len(monsters); i += chunkSize {
		start := i
		end := start + chunkSize
		if end > len(monsters) {
			end = len(monsters)
		}

		eu.workerPool.Submit(func() {
			for j := start; j < end; j++ {
				monster := monsters[j]
				if monster.IsAlive() {
					monster.Update()
				}
			}
		})
	}

	eu.workerPool.Wait()
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

// UpdateProjectilesParallel updates all projectiles in parallel using the reusable worker pool.
func (eu *EntityUpdater) UpdateProjectilesParallel(projectiles []ProjectileUpdateInterface, canMoveTo func(x, y float64) bool) {
	if len(projectiles) == 0 {
		return
	}

	numWorkers := eu.workerPool.GetNumWorkers()
	chunkSize := (len(projectiles) + numWorkers - 1) / numWorkers
	if chunkSize < 1 {
		chunkSize = 1
	}

	for i := 0; i < len(projectiles); i += chunkSize {
		start := i
		end := start + chunkSize
		if end > len(projectiles) {
			end = len(projectiles)
		}

		eu.workerPool.Submit(func() {
			for j := start; j < end; j++ {
				projectile := projectiles[j]
				if !projectile.IsActive() {
					continue
				}

				// Update position
				x, y := projectile.GetPosition()
				vx, vy := projectile.GetVelocity()

				newX := x + vx
				newY := y + vy

				// Check collision (canMoveTo must be thread-safe)
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
			}
		})
	}

	eu.workerPool.Wait()
}

// Stop shuts down the entity updater
func (eu *EntityUpdater) Stop() {
	eu.workerPool.Stop()
}
