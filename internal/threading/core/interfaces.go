package core

import (
	"runtime"
	"sync"
)

// Common interfaces and utilities for threading operations

// Updatable represents an entity that can be updated
type Updatable interface {
	Update()
}

// Positionable represents an entity with position
type Positionable interface {
	GetPosition() (float64, float64)
	SetPosition(x, y float64)
}

// Movable represents an entity that can move with velocity
type Movable interface {
	Positionable
	GetVelocity() (float64, float64)
	SetVelocity(vx, vy float64)
}

// Mortal represents an entity with lifetime/health
type Mortal interface {
	IsAlive() bool
}

// TimeLimited represents an entity with a lifetime
type TimeLimited interface {
	GetLifetime() int
	SetLifetime(lifetime int)
	IsActive() bool
}

// MonsterEntity combines monster-specific interfaces
type MonsterEntity interface {
	Updatable
	Positionable
	Mortal
}

// ProjectileEntity combines projectile-specific interfaces
type ProjectileEntity interface {
	Updatable
	Movable
	TimeLimited
}

// CreateDefaultWorkerPool creates a worker pool with default CPU count
func CreateDefaultWorkerPool() *WorkerPool {
	pool := NewWorkerPool(0) // 0 means use CPU count
	pool.Start()
	return pool
}

// ParallelForEach executes a function in parallel for each item in a slice
func ParallelForEach[T any](items []T, fn func(T)) {
	if len(items) == 0 {
		return
	}

	numWorkers := min(runtime.NumCPU(), len(items))
	var wg sync.WaitGroup

	chunkSize := max(1, len(items)/numWorkers)

	for i := 0; i < len(items); i += chunkSize {
		end := min(i+chunkSize, len(items))
		chunk := items[i:end]

		wg.Add(1)
		go func(chunk []T) {
			defer wg.Done()
			for _, item := range chunk {
				fn(item)
			}
		}(chunk)
	}

	wg.Wait()
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
