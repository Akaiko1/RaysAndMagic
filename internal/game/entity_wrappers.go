package game

import (
	"sync"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

// Wrapper pools to reduce GC pressure - objects are reused instead of allocated each frame
var (
	monsterWrapperPool = sync.Pool{
		New: func() interface{} {
			return &MonsterWrapper{}
		},
	}
	magicProjectileWrapperPool = sync.Pool{
		New: func() interface{} {
			return &MagicProjectileWrapper{}
		},
	}
	arrowWrapperPool = sync.Pool{
		New: func() interface{} {
			return &ArrowWrapper{}
		},
	}
)

// Helper functions to reduce code duplication in wrapper creation and common operations

// CreateMagicProjectileWrapper creates a wrapper for magic projectile entities using pool
func CreateMagicProjectileWrapper(magicProjectile *MagicProjectile, collisionSystem *collision.CollisionSystem, projectileID string, game *MMGame) entities.ProjectileUpdateInterface {
	wrapper := magicProjectileWrapperPool.Get().(*MagicProjectileWrapper)
	wrapper.MagicProjectile = magicProjectile
	wrapper.collisionSystem = collisionSystem
	wrapper.projectileID = projectileID
	wrapper.game = game
	wrapper.pendingImpact = false // defensive: Phase 2 always clears this before recycling, but don't trust it across pool reuse
	return wrapper
}

// CreateArrowWrapper creates a wrapper for arrow entities using pool
func CreateArrowWrapper(arrow *Arrow, collisionSystem *collision.CollisionSystem, projectileID string, game *MMGame) entities.ProjectileUpdateInterface {
	wrapper := arrowWrapperPool.Get().(*ArrowWrapper)
	wrapper.Arrow = arrow
	wrapper.collisionSystem = collisionSystem
	wrapper.projectileID = projectileID
	wrapper.game = game
	wrapper.pendingImpact = false // defensive: Phase 2 always clears this before recycling, but don't trust it across pool reuse
	return wrapper
}

// CreateMonsterWrapper creates a wrapper for monster entities using pool.
// snapshot is the frozen collision view Update() (Phase 1, parallel) must read
// from; collisionSystem is the live system ApplyCollisionUpdate (Phase 2,
// serial) writes to — see MonsterWrapper's doc.
func CreateMonsterWrapper(m *monster.Monster3D, collisionSystem *collision.CollisionSystem, snapshot *collision.CollisionSnapshot, game *MMGame) entities.MonsterUpdateInterface {
	wrapper := monsterWrapperPool.Get().(*MonsterWrapper)
	wrapper.Monster = m
	wrapper.collisionSystem = collisionSystem
	wrapper.snapshot = snapshot
	wrapper.game = game
	return wrapper
}

// ConvertProjectilesToWrappers converts projectile slices to wrapper interfaces
// Uses pre-allocated slice to reduce GC pressure
func (g *MMGame) ConvertProjectilesToWrappers() []entities.ProjectileUpdateInterface {
	// Return previous wrappers to pool before reusing slice
	for _, wrapper := range g.reusableProjectileWrappers {
		recycleProjectileWrapper(wrapper)
	}

	// Reuse slice by resetting length to 0 (keeps capacity)
	g.reusableProjectileWrappers = g.reusableProjectileWrappers[:0]

	// Ensure capacity is sufficient
	totalCount := len(g.magicProjectiles) + len(g.arrows)
	if cap(g.reusableProjectileWrappers) < totalCount {
		g.reusableProjectileWrappers = make([]entities.ProjectileUpdateInterface, 0, totalCount)
	}

	// Convert magic projectiles
	for i := range g.magicProjectiles {
		g.reusableProjectileWrappers = append(g.reusableProjectileWrappers,
			CreateMagicProjectileWrapper(&g.magicProjectiles[i], g.collisionSystem, g.magicProjectiles[i].ID, g))
	}

	// Convert arrows
	for i := range g.arrows {
		g.reusableProjectileWrappers = append(g.reusableProjectileWrappers,
			CreateArrowWrapper(&g.arrows[i], g.collisionSystem, g.arrows[i].ID, g))
	}

	return g.reusableProjectileWrappers
}

// ConvertMonstersToWrappers converts monster slice to wrapper interfaces
// Uses pre-allocated slice to reduce GC pressure
func (g *MMGame) ConvertMonstersToWrappers() []entities.MonsterUpdateInterface {
	// Return previous wrappers to pool before reusing slice
	for _, wrapper := range g.reusableMonsterWrappers {
		monsterWrapperPool.Put(wrapper)
	}

	// Reuse slice by resetting length to 0 (keeps capacity)
	g.reusableMonsterWrappers = g.reusableMonsterWrappers[:0]

	// Ensure capacity is sufficient
	if cap(g.reusableMonsterWrappers) < len(g.world.Monsters) {
		g.reusableMonsterWrappers = make([]entities.MonsterUpdateInterface, 0, len(g.world.Monsters))
	}

	// ONE frozen view for the whole tick, taken here (single-threaded, before
	// dispatch) — every monster's worker reads this same snapshot instead of the
	// live system, so no synchronization is needed between them. See
	// collision.CollisionSnapshot's doc.
	snapshot := g.collisionSystem.Snapshot()
	for _, m := range g.world.Monsters {
		g.reusableMonsterWrappers = append(g.reusableMonsterWrappers,
			CreateMonsterWrapper(m, g.collisionSystem, snapshot, g))
	}
	return g.reusableMonsterWrappers
}

// recycleProjectileWrapper returns a projectile wrapper to the appropriate pool
func recycleProjectileWrapper(wrapper entities.ProjectileUpdateInterface) {
	switch w := wrapper.(type) {
	case *MagicProjectileWrapper:
		magicProjectileWrapperPool.Put(w)
	case *ArrowWrapper:
		arrowWrapperPool.Put(w)
	}
}

// RemoveInactiveEntities removes inactive projectiles from slices using in-place filtering
// This avoids allocating new slices each frame to reduce GC pressure
func (g *MMGame) RemoveInactiveEntities() {
	// Remove inactive magic projectiles using in-place filtering
	writeIdx := 0
	for readIdx := range g.magicProjectiles {
		if g.magicProjectiles[readIdx].Active && g.magicProjectiles[readIdx].LifeTime > 0 {
			if writeIdx != readIdx {
				g.magicProjectiles[writeIdx] = g.magicProjectiles[readIdx]
			}
			writeIdx++
		} else {
			g.collisionSystem.UnregisterEntity(g.magicProjectiles[readIdx].ID)
		}
	}
	g.magicProjectiles = g.magicProjectiles[:writeIdx]

	// Remove inactive arrows using in-place filtering
	writeIdx = 0
	for readIdx := range g.arrows {
		if g.arrows[readIdx].Active && g.arrows[readIdx].LifeTime > 0 {
			if writeIdx != readIdx {
				g.arrows[writeIdx] = g.arrows[readIdx]
			}
			writeIdx++
		} else {
			g.collisionSystem.UnregisterEntity(g.arrows[readIdx].ID)
		}
	}
	g.arrows = g.arrows[:writeIdx]
}
