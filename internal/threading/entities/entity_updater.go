package entities

import (
	"ugataima/internal/threading/core"
)

// EntityUpdater runs the per-tick monster update, on its worker pool when the
// batch is large enough to pay for waking it.
//
// Concurrency model: callers run UpdateMonstersParallel from the main game
// tick. Wait() blocks the caller until every chunk finishes, so the main
// goroutine cannot mutate shared world state (camera, world tiles) while
// workers are running. This "stop-the-world" tick lets each entity's Update()
// safely read shared state without locks. The invariant holds only as long as
// no other goroutine writes during this window - keep it that way.
//
// Two-phase apply: Update() must never WRITE shared cross-entity state (the
// live collision system, screen shake, particle effects) - only read an
// immutable, frozen view of it (see collision.CollisionSnapshot) and mutate the
// entity's OWN fields. A shared write it wants to make instead gets computed
// during Update() and applied by ApplyCollisionUpdate() in a plain serial loop
// AFTER Wait() returns, on the calling goroutine, once every worker has
// finished. This keeps Update() itself (the AI/movement work) fully parallel
// while the handful of shared writes it triggers stay race-free without any
// locking. A small batch runs the same two phases on the calling goroutine.
type EntityUpdater struct {
	workerPool *core.WorkerPool
	// minParallel is the smallest batch sent to the pool; below it the wake-up
	// of the workers costs more than the update itself.
	minParallel int
}

// NewEntityUpdater creates an entity updater that uses its pool for batches
// of at least minParallel entities.
func NewEntityUpdater(minParallel int) *EntityUpdater {
	return &EntityUpdater{workerPool: core.CreateDefaultWorkerPool(), minParallel: minParallel}
}

// NewEntityUpdaterWithWorkers sends every batch to a pool of an explicit size.
// Test-only: game tests pin the pool size, and the pool field is unexported.
func NewEntityUpdaterWithWorkers(workers int) *EntityUpdater {
	pool := core.NewWorkerPool(workers)
	pool.Start()
	return &EntityUpdater{workerPool: pool}
}

// MonsterUpdateInterface defines the interface for monsters that can be
// updated. Update() computes the tick (AI/movement + the desired collision
// state) reading only a frozen collision.CollisionSnapshot; ApplyCollisionUpdate
// writes that result (position + collision type) to the LIVE collision system
// - see the type doc's two-phase apply note. game.MonsterWrapper is the
// canonical implementation.
type MonsterUpdateInterface interface {
	Update()
	ApplyCollisionUpdate()
	IsAlive() bool
}

// ProjectileUpdateInterface defines the interface for projectiles. OnCollision
// only records that an impact happened (touches the projectile's own fields
// only); ApplyCollisionEffects spawns the resulting hit effects/screen shake
// once every projectile of the tick has moved - see UpdateProjectiles.
type ProjectileUpdateInterface interface {
	IsActive() bool
	GetPosition() (float64, float64)
	SetPosition(x, y float64)
	GetVelocity() (float64, float64)
	GetLifetime() int
	SetLifetime(lifetime int)
	// IgnoresWalls: the projectile flies through solid terrain (a mortar's
	// display bolt on its ballistic arc) - the mover skips the wall check.
	IgnoresWalls() bool
	OnCollision(hitX, hitY float64)
	ApplyCollisionEffects()
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

// runChunked covers [0, total) with fn and returns when all of it has run: in
// per-chunk pool jobs, or as one call on this goroutine for a batch under
// minParallel. fn receives the half-open range [start, end) for its chunk. If
// the pool is stopped (post-shutdown call), refused chunks run inline -
// degraded to serial, never dropped or hung.
func (eu *EntityUpdater) runChunked(total int, fn func(start, end int)) {
	if total == 0 {
		return
	}
	if total < eu.minParallel {
		fn(0, total)
		return
	}
	chunk := chunkSizeFor(total, eu.workerPool.GetNumWorkers())
	for i := 0; i < total; i += chunk {
		start := i
		end := start + chunk
		if end > total {
			end = total
		}
		if !eu.workerPool.Submit(func() { fn(start, end) }) {
			fn(start, end)
		}
	}
	eu.workerPool.Wait()
}

// UpdateMonstersParallel updates all monsters, on the worker pool for a large
// batch, then applies each one's collision-system write serially. See type
// doc for the stop-the-world invariant and the two-phase apply rationale.
func (eu *EntityUpdater) UpdateMonstersParallel(monsters []MonsterUpdateInterface) {
	eu.runChunked(len(monsters), func(start, end int) {
		for j := start; j < end; j++ {
			m := monsters[j]
			if m.IsAlive() {
				m.Update()
			}
		}
	})
	// Phase 2: apply the position/collision-type each monster computed against
	// its frozen snapshot. Serial and safe - every worker above has already
	// returned (runChunked's Wait()), so nothing else touches the live system.
	for _, m := range monsters {
		if m.IsAlive() {
			m.ApplyCollisionUpdate()
		}
	}
}

// UpdateProjectiles moves every projectile one tick, then applies the hit
// effects/screen shake of the impacts the moves recorded. A plain loop: one
// projectile's move is a few additions and a tile lookup, far cheaper than
// handing it to a worker.
func UpdateProjectiles(projectiles []ProjectileUpdateInterface, canMoveTo func(x, y float64) bool) {
	for _, p := range projectiles {
		if !p.IsActive() {
			continue
		}
		x, y := p.GetPosition()
		vx, vy := p.GetVelocity()
		newX, newY := x+vx, y+vy
		if p.IgnoresWalls() || canMoveTo(newX, newY) {
			p.SetPosition(newX, newY)
		} else {
			p.OnCollision(x, y)
		}
		p.SetLifetime(p.GetLifetime() - 1)
	}
	for _, p := range projectiles {
		p.ApplyCollisionEffects()
	}
}

// Stop shuts down the entity updater.
func (eu *EntityUpdater) Stop() {
	eu.workerPool.Stop()
}
