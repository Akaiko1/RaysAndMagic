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
// serial) writes to - see MonsterWrapper's doc.
func CreateMonsterWrapper(m *monster.Monster3D, collisionSystem *collision.CollisionSystem, snapshot *collision.CollisionSnapshot, game *MMGame) entities.MonsterUpdateInterface {
	wrapper := monsterWrapperPool.Get().(*MonsterWrapper)
	wrapper.Monster = m
	wrapper.collisionSystem = collisionSystem
	wrapper.snapshot = snapshot
	wrapper.frame = game.monsterFrameContext()
	wrapper.result = monsterFrameResult{}
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
	totalCount := g.projectileCount()
	if cap(g.reusableProjectileWrappers) < totalCount {
		g.reusableProjectileWrappers = make([]entities.ProjectileUpdateInterface, 0, totalCount)
	}

	// Convert magic projectiles
	for i := range g.magicProjectiles {
		g.reusableProjectileWrappers = append(g.reusableProjectileWrappers,
			CreateMagicProjectileWrapper(&g.magicProjectiles[i], g.collisionSystem, g.magicProjectiles[i].ID, g))
	}

	for i := range g.flasks {
		g.reusableProjectileWrappers = append(g.reusableProjectileWrappers, createFlaskWrapper(&g.flasks[i], g.collisionSystem, g))
	}

	// Convert arrows
	for i := range g.arrows {
		if g.arrows[i].Backwash != nil {
			g.combat.prepareBackwashCharge(&g.arrows[i])
		}
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
	// dispatch) - every monster's worker reads this same snapshot instead of the
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
	case *FlaskProjectileWrapper:
		flaskWrapperPool.Put(w)
	}
}

// RemoveInactiveEntities drops landed and expired shots of every family in
// place (no per-frame allocation) with their collision entities.
func (g *MMGame) RemoveInactiveEntities() {
	g.magicProjectiles = keepFlying(g.collisionSystem, g.magicProjectiles)
	g.arrows = keepFlying(g.collisionSystem, g.arrows)
	g.flasks = keepFlying(g.collisionSystem, g.flasks)
}

// projectileCount is every shot in flight: spells, arrows and flasks.
func (g *MMGame) projectileCount() int {
	return len(g.magicProjectiles) + len(g.arrows) + len(g.flasks)
}

// clearProjectiles drops every shot and its collision entity.
// switchToMap keeps the collision system, so stale entities must go.
func (g *MMGame) clearProjectiles() {
	g.magicProjectiles = dropShots(g.collisionSystem, g.magicProjectiles)
	g.arrows = dropShots(g.collisionSystem, g.arrows)
	g.flasks = dropShots(g.collisionSystem, g.flasks)
}

// inFlight is the lifecycle view every projectile family shares.
type inFlight interface {
	collisionID() string
	flying() bool
}

func (p *MagicProjectile) collisionID() string { return p.ID }
func (p *MagicProjectile) flying() bool        { return p.Active && p.LifeTime > 0 }
func (p *Arrow) collisionID() string           { return p.ID }
func (p *Arrow) flying() bool                  { return p.Active && p.LifeTime > 0 }
func (p *FlaskProjectile) collisionID() string { return p.ID }
func (p *FlaskProjectile) flying() bool        { return p.Active && p.LifeTime > 0 }

// keepFlying filters shots in place, unregistering each one that landed or
// expired.
func keepFlying[T any, P interface {
	*T
	inFlight
}](cs *collision.CollisionSystem, shots []T) []T {
	kept := 0
	for i := range shots {
		if P(&shots[i]).flying() {
			if kept != i {
				shots[kept] = shots[i]
			}
			kept++
		} else if cs != nil {
			cs.UnregisterEntity(P(&shots[i]).collisionID())
		}
	}
	clear(shots[kept:]) // the spare capacity holds no stale actors
	return shots[:kept]
}

// dropShots unregisters every shot and empties the slice, keeping its capacity.
func dropShots[T any, P interface {
	*T
	inFlight
}](cs *collision.CollisionSystem, shots []T) []T {
	if cs != nil {
		for i := range shots {
			cs.UnregisterEntity(P(&shots[i]).collisionID())
		}
	}
	clear(shots)
	return shots[:0]
}
