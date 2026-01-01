package core

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of worker goroutines for parallel processing
type WorkerPool struct {
	numWorkers int
	jobQueue   chan func()
	wg         sync.WaitGroup
	quit       chan bool
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(numWorkers int) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	return &WorkerPool{
		numWorkers: numWorkers,
		jobQueue:   make(chan func(), numWorkers*2), // Buffer for better performance
		quit:       make(chan bool),
	}
}

// Start initializes and starts all worker goroutines
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.numWorkers; i++ {
		go wp.worker()
	}
}

// worker is the goroutine that processes jobs from the queue
func (wp *WorkerPool) worker() {
	for {
		select {
		case job := <-wp.jobQueue:
			job()
			wp.wg.Done()
		case <-wp.quit:
			return
		}
	}
}

// Submit adds a job to the worker queue
func (wp *WorkerPool) Submit(job func()) {
	wp.wg.Add(1)
	wp.jobQueue <- job
}

// SubmitAndWait submits a job and waits for it to complete
func (wp *WorkerPool) SubmitAndWait(job func()) {
	wp.Submit(job)
	wp.wg.Wait()
}

// Wait waits for all currently queued jobs to complete
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Stop shuts down the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.quit)
}

// ParallelFor executes a function in parallel for a range of values.
// This is a convenience wrapper around ParallelForWithContext using context.Background().
func (wp *WorkerPool) ParallelFor(start, end int, fn func(int)) {
	wp.ParallelForWithContext(context.Background(), start, end, fn)
}

// ParallelForWithContext executes a function in parallel for a range of values
// with cancellation support via context.
func (wp *WorkerPool) ParallelForWithContext(ctx context.Context, start, end int, fn func(int)) {
	if start >= end {
		return
	}

	// Calculate optimal chunk size based on range and worker count
	totalWork := end - start
	chunkSize := max(1, totalWork/wp.numWorkers)

	for i := start; i < end; i += chunkSize {
		chunkStart := i
		chunkEnd := min(i+chunkSize, end)
		wp.Submit(func() {
			for j := chunkStart; j < chunkEnd; j++ {
				// Check for cancellation between iterations
				select {
				case <-ctx.Done():
					return
				default:
					fn(j)
				}
			}
		})
	}
	wp.Wait()
}

// SubmitWithContext adds a job that respects context cancellation.
// The job will not execute if the context is already cancelled.
func (wp *WorkerPool) SubmitWithContext(ctx context.Context, job func()) {
	wp.wg.Add(1)
	wp.jobQueue <- func() {
		select {
		case <-ctx.Done():
			// Context cancelled, skip job
		default:
			job()
		}
	}
}

// Batch processes items in batches across multiple goroutines
type Batch[T any] struct {
	items []T
	fn    func([]T)
}

// NewBatch creates a new batch processor
func NewBatch[T any](items []T, fn func([]T)) *Batch[T] {
	return &Batch[T]{items: items, fn: fn}
}

// Process executes the batch processing with the specified batch size
func (b *Batch[T]) Process(batchSize int) {
	if len(b.items) == 0 {
		return
	}

	var wg sync.WaitGroup

	for i := 0; i < len(b.items); i += batchSize {
		end := min(i+batchSize, len(b.items))
		batch := b.items[i:end]

		wg.Add(1)
		go func(batch []T) {
			defer wg.Done()
			b.fn(batch)
		}(batch)
	}

	wg.Wait()
}

// RaycastJob represents a single raycasting operation
type RaycastJob struct {
	RayIndex int
	Angle    float64
	Result   *RaycastResult
}

// RaycastResult holds the result of a raycast operation
type RaycastResult struct {
	Distance  float64
	TileType  interface{} // Will be world.TileType3D from game package
	WallSlice []byte      // Pre-rendered wall slice data
}

// MonsterUpdateJob represents a monster AI update operation
type MonsterUpdateJob struct {
	MonsterIndex int
	Monster      interface{} // Will be *monster.Monster3D from entities package
}

// ProjectileJob represents projectile physics calculation
type ProjectileJob struct {
	ProjectileIndex int
	ProjectileType  string // "fireball" or "sword"
	Position        struct{ X, Y float64 }
	Velocity        struct{ X, Y float64 }
}

// SafeCounter provides thread-safe counter operations using lock-free atomics.
// This is more efficient than mutex-based counters for simple increment/decrement operations.
type SafeCounter struct {
	value atomic.Int64
}

// NewSafeCounter creates a new thread-safe counter initialized to zero
func NewSafeCounter() *SafeCounter {
	return &SafeCounter{}
}

// Increment atomically increments the counter and returns the new value
func (c *SafeCounter) Increment() int64 {
	return c.value.Add(1)
}

// Decrement atomically decrements the counter and returns the new value
func (c *SafeCounter) Decrement() int64 {
	return c.value.Add(-1)
}

// Add atomically adds delta to the counter and returns the new value
func (c *SafeCounter) Add(delta int64) int64 {
	return c.value.Add(delta)
}

// Get atomically gets the counter value
func (c *SafeCounter) Get() int64 {
	return c.value.Load()
}

// Set atomically sets the counter value
func (c *SafeCounter) Set(value int64) {
	c.value.Store(value)
}

// CompareAndSwap atomically sets to new if current value equals old. Returns true if swapped.
func (c *SafeCounter) CompareAndSwap(old, new int64) bool {
	return c.value.CompareAndSwap(old, new)
}

// GetNumWorkers returns the number of workers in the pool
func (wp *WorkerPool) GetNumWorkers() int {
	return wp.numWorkers
}
