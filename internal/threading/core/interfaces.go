package core

import (
	"context"
	"runtime"
	"sync"
	"ugataima/internal/mathutil"
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

// ParallelForEach executes a function in parallel for each item in a slice.
// This is a convenience wrapper around ParallelForEachWithContext using context.Background().
func ParallelForEach[T any](items []T, fn func(T)) {
	ParallelForEachWithContext(context.Background(), items, fn)
}

// ParallelForEachWithContext executes a function in parallel for each item in a slice
// with cancellation support via context. Goroutines check for cancellation between items.
func ParallelForEachWithContext[T any](ctx context.Context, items []T, fn func(T)) {
	if len(items) == 0 {
		return
	}

	numWorkers := mathutil.IntMin(runtime.NumCPU(), len(items))
	var wg sync.WaitGroup

	chunkSize := mathutil.IntMax(1, len(items)/numWorkers)

	for i := 0; i < len(items); i += chunkSize {
		end := mathutil.IntMin(i+chunkSize, len(items))
		chunk := items[i:end]

		wg.Add(1)
		go func(chunk []T) {
			defer wg.Done()
			for _, item := range chunk {
				// Check for cancellation between items
				select {
				case <-ctx.Done():
					return
				default:
					fn(item)
				}
			}
		}(chunk)
	}

	wg.Wait()
}

// ParallelMap executes a function in parallel for each item and collects results.
// Uses per-worker result slices to avoid mutex contention, then merges results.
// This is optimal for map-reduce style operations.
func ParallelMap[T any, R any](items []T, fn func(T) R) []R {
	return ParallelMapWithContext(context.Background(), items, fn)
}

// ParallelMapWithContext executes a function in parallel with cancellation support.
// Results are collected using per-worker slices (no mutex contention).
func ParallelMapWithContext[T any, R any](ctx context.Context, items []T, fn func(T) R) []R {
	if len(items) == 0 {
		return nil
	}

	numWorkers := mathutil.IntMin(runtime.NumCPU(), len(items))
	chunkSize := mathutil.IntMax(1, len(items)/numWorkers)

	// Pre-allocate result slice
	results := make([]R, len(items))
	var wg sync.WaitGroup

	for i := 0; i < len(items); i += chunkSize {
		start := i
		end := mathutil.IntMin(i+chunkSize, len(items))

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for j := start; j < end; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					results[j] = fn(items[j])
				}
			}
		}(start, end)
	}

	wg.Wait()
	return results
}

// ParallelFilter executes a predicate in parallel and returns matching items.
// Uses per-worker result slices to avoid mutex contention.
func ParallelFilter[T any](items []T, predicate func(T) bool) []T {
	return ParallelFilterWithContext(context.Background(), items, predicate)
}

// ParallelFilterWithContext executes a predicate in parallel with cancellation support.
func ParallelFilterWithContext[T any](ctx context.Context, items []T, predicate func(T) bool) []T {
	if len(items) == 0 {
		return nil
	}

	numWorkers := mathutil.IntMin(runtime.NumCPU(), len(items))
	chunkSize := mathutil.IntMax(1, len(items)/numWorkers)

	// Per-worker result slices (no mutex contention)
	workerResults := make([][]T, numWorkers)
	var wg sync.WaitGroup

	workerIdx := 0
	for i := 0; i < len(items); i += chunkSize {
		start := i
		end := mathutil.IntMin(i+chunkSize, len(items))
		idx := workerIdx
		workerIdx++

		wg.Add(1)
		go func(idx, start, end int) {
			defer wg.Done()
			localResults := make([]T, 0, end-start)
			for j := start; j < end; j++ {
				select {
				case <-ctx.Done():
					workerResults[idx] = localResults
					return
				default:
					if predicate(items[j]) {
						localResults = append(localResults, items[j])
					}
				}
			}
			workerResults[idx] = localResults
		}(idx, start, end)
	}

	wg.Wait()

	// Merge results from all workers
	totalLen := 0
	for _, wr := range workerResults {
		totalLen += len(wr)
	}
	results := make([]T, 0, totalLen)
	for _, wr := range workerResults {
		results = append(results, wr...)
	}

	return results
}
