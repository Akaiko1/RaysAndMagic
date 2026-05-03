package entities

import (
	"ugataima/internal/threading/core"
)

// EntityUpdater manages parallel updates for game entities.
//
// Concurrency model: callers run UpdateMonstersParallel / UpdateProjectilesParallel
// from the main game tick. Wait() blocks the caller until every chunk finishes,
// so the main goroutine cannot mutate shared world state (camera, world tiles,
// projectile slices) while workers are running. This "stop-the-world" tick lets
// each entity's Update() safely read shared state without locks. The invariant
// holds only as long as no other goroutine writes during this window — keep it
// that way.
type EntityUpdater struct {
	workerPool *core.WorkerPool
}

// NewEntityUpdater creates a new entity updater.
func NewEntityUpdater() *EntityUpdater {
	return &EntityUpdater{
		workerPool: core.CreateDefaultWorkerPool(),
	}
}

// MonsterUpdateInterface defines the interface for monsters that can be updated.
type MonsterUpdateInterface interface {
	Update()
	IsAlive() bool
	GetPosition() (float64, float64)
	SetPosition(x, y float64)
}

// ProjectileUpdateInterface defines the interface for projectiles.
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

// chunkSizeFor returns a ceiling chunk size that distributes total items evenly
// across numWorkers, with a minimum of 1 to avoid zero-sized chunks.
func chunkSizeFor(total, numWorkers int) int {
	if numWorkers <= 0 {
		return total
	}
	chunk := (total + numWorkers - 1) / numWorkers
	if chunk < 1 {
		chunk = 1
	}
	return chunk
}

// runChunked submits per-chunk jobs covering [0, total) and waits for them all.
// fn receives the half-open range [start, end) for its chunk.
func (eu *EntityUpdater) runChunked(total int, fn func(start, end int)) {
	if total == 0 {
		return
	}
	chunk := chunkSizeFor(total, eu.workerPool.GetNumWorkers())
	for i := 0; i < total; i += chunk {
		start := i
		end := start + chunk
		if end > total {
			end = total
		}
		eu.workerPool.Submit(func() { fn(start, end) })
	}
	eu.workerPool.Wait()
}

// UpdateMonstersParallel updates all monsters in parallel using the reusable
// worker pool. See type doc for the stop-the-world invariant.
func (eu *EntityUpdater) UpdateMonstersParallel(monsters []MonsterUpdateInterface) {
	eu.runChunked(len(monsters), func(start, end int) {
		for j := start; j < end; j++ {
			m := monsters[j]
			if m.IsAlive() {
				m.Update()
			}
		}
	})
}

// UpdateProjectilesParallel updates all projectiles in parallel. canMoveTo must
// be safe for concurrent reads. See type doc for the stop-the-world invariant.
func (eu *EntityUpdater) UpdateProjectilesParallel(projectiles []ProjectileUpdateInterface, canMoveTo func(x, y float64) bool) {
	eu.runChunked(len(projectiles), func(start, end int) {
		for j := start; j < end; j++ {
			p := projectiles[j]
			if !p.IsActive() {
				continue
			}
			x, y := p.GetPosition()
			vx, vy := p.GetVelocity()
			newX, newY := x+vx, y+vy
			if canMoveTo(newX, newY) {
				p.SetPosition(newX, newY)
			} else {
				p.OnCollision(x, y)
				p.SetLifetime(0)
			}
			p.SetLifetime(p.GetLifetime() - 1)
		}
	})
}

// Stop shuts down the entity updater.
func (eu *EntityUpdater) Stop() {
	eu.workerPool.Stop()
}
