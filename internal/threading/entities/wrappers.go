package entities

// Generic wrappers that eliminate code duplication

// GenericProjectileWrapper provides a generic wrapper for projectile-like entities
// This eliminates the need for separate FireballWrapper and SwordAttackWrapper types
type GenericProjectileWrapper[T any] struct {
	Entity       *T
	UpdateFunc   func(*T)
	IsActiveFunc func(*T) bool
	GetPosFunc   func(*T) (float64, float64)
	SetPosFunc   func(*T, float64, float64)
	GetVelFunc   func(*T) (float64, float64)
	SetVelFunc   func(*T, float64, float64)
	GetLifeFunc  func(*T) int
	SetLifeFunc  func(*T, int)
}

func (w *GenericProjectileWrapper[T]) Update() {
	w.UpdateFunc(w.Entity)
}

func (w *GenericProjectileWrapper[T]) IsActive() bool {
	return w.IsActiveFunc(w.Entity)
}

func (w *GenericProjectileWrapper[T]) GetPosition() (float64, float64) {
	return w.GetPosFunc(w.Entity)
}

func (w *GenericProjectileWrapper[T]) SetPosition(x, y float64) {
	w.SetPosFunc(w.Entity, x, y)
}

func (w *GenericProjectileWrapper[T]) GetVelocity() (float64, float64) {
	return w.GetVelFunc(w.Entity)
}

func (w *GenericProjectileWrapper[T]) SetVelocity(vx, vy float64) {
	w.SetVelFunc(w.Entity, vx, vy)
}

func (w *GenericProjectileWrapper[T]) GetLifetime() int {
	return w.GetLifeFunc(w.Entity)
}

func (w *GenericProjectileWrapper[T]) SetLifetime(lifetime int) {
	w.SetLifeFunc(w.Entity, lifetime)
}

// GenericMonsterWrapper provides a generic wrapper for monster-like entities
type GenericMonsterWrapper[T any] struct {
	Entity      *T
	UpdateFunc  func(*T)
	IsAliveFunc func(*T) bool
	GetPosFunc  func(*T) (float64, float64)
	SetPosFunc  func(*T, float64, float64)
}

func (w *GenericMonsterWrapper[T]) Update() {
	w.UpdateFunc(w.Entity)
}

func (w *GenericMonsterWrapper[T]) IsAlive() bool {
	return w.IsAliveFunc(w.Entity)
}

func (w *GenericMonsterWrapper[T]) GetPosition() (float64, float64) {
	return w.GetPosFunc(w.Entity)
}

func (w *GenericMonsterWrapper[T]) SetPosition(x, y float64) {
	w.SetPosFunc(w.Entity, x, y)
}
