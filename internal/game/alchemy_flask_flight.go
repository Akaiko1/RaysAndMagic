package game

import (
	"math"
	"sync"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/threading/entities"
)

// FlaskProjectile is a thrown Alchemist flask in flight. It is not a spell: it
// bursts at the end of its range, on a wall or on a body, and detonateFlask
// resolves its splash.
type FlaskProjectile struct {
	ActorID                  uint64
	CatalysisPercent         int
	CatalysisFrames          int
	Reusable                 bool
	Launch                   projectileLaunch // Runtime-only; save/load discards projectiles.
	ID                       string
	Key                      string // items.yaml flask key
	X, Y                     float64
	VelX, VelY               float64
	Remaining, Radius        float64 // distance left to the range end; splash radius
	PoisonFrames, BurnFrames int
	Damage                   int
	Attacker                 *character.MMCharacter
	LifeTime                 int
	Active                   bool
}

// FlaskProjectileWrapper implements entities.ProjectileUpdateInterface.
type FlaskProjectileWrapper struct {
	Flask           *FlaskProjectile
	collisionSystem *collision.CollisionSystem
	game            *MMGame

	// See MagicProjectileWrapper's pendingImpact fields for the rationale.
	pendingImpact bool
	impactX       float64
	impactY       float64
}

var flaskWrapperPool = sync.Pool{New: func() interface{} { return &FlaskProjectileWrapper{} }}

func createFlaskWrapper(f *FlaskProjectile, collisionSystem *collision.CollisionSystem, game *MMGame) entities.ProjectileUpdateInterface {
	w := flaskWrapperPool.Get().(*FlaskProjectileWrapper)
	w.Flask, w.collisionSystem, w.game = f, collisionSystem, game
	w.pendingImpact = false
	return w
}

func (w *FlaskProjectileWrapper) IsActive() bool { return w.Flask.Active && w.Flask.LifeTime > 0 }

func (w *FlaskProjectileWrapper) GetPosition() (float64, float64) { return w.Flask.X, w.Flask.Y }

// SetPosition stops the bottle at the end of its range and bursts it there.
func (w *FlaskProjectileWrapper) SetPosition(x, y float64) {
	f := w.Flask
	if f.Active {
		distance := math.Hypot(x-f.X, y-f.Y)
		if distance >= f.Remaining && distance > 0 {
			ratio := f.Remaining / distance
			x = f.X + (x-f.X)*ratio
			y = f.Y + (y-f.Y)*ratio
			f.Active = false
			w.pendingImpact = true
			w.impactX, w.impactY = x, y
		}
		f.Remaining = max(0, f.Remaining-distance)
	}
	f.X, f.Y = x, y
	if w.collisionSystem != nil {
		w.collisionSystem.UpdateEntity(f.ID, x, y)
	}
}

func (w *FlaskProjectileWrapper) GetVelocity() (float64, float64) { return w.Flask.VelX, w.Flask.VelY }

func (w *FlaskProjectileWrapper) GetLifetime() int { return w.Flask.LifeTime }

func (w *FlaskProjectileWrapper) SetLifetime(lifetime int) { w.Flask.LifeTime = lifetime }

// IgnoresWalls: a flask bursts on solid terrain.
func (w *FlaskProjectileWrapper) IgnoresWalls() bool { return false }

// OnCollision runs in a worker goroutine and touches only this flask.
func (w *FlaskProjectileWrapper) OnCollision(hitX, hitY float64) {
	if !w.Flask.Active {
		return
	}
	w.Flask.Active = false
	w.pendingImpact = true
	w.impactX, w.impactY = hitX, hitY
}

// ApplyCollisionEffects bursts the flask serially, after every worker returned.
func (w *FlaskProjectileWrapper) ApplyCollisionEffects() {
	if !w.pendingImpact {
		return
	}
	w.pendingImpact = false
	if w.game != nil && w.game.combat != nil {
		w.game.combat.detonateFlask(w.Flask, w.impactX, w.impactY)
	}
}
