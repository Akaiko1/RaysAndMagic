package core

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of worker goroutines for parallel processing.
type WorkerPool struct {
	numWorkers int
	jobQueue   chan func()
	wg         sync.WaitGroup
	quit       chan struct{}
	stopped    atomic.Bool
}

// NewWorkerPool creates a new worker pool. Pass numWorkers <= 0 to use
// runtime.NumCPU() as the worker count.
func NewWorkerPool(numWorkers int) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	return &WorkerPool{
		numWorkers: numWorkers,
		jobQueue:   make(chan func(), numWorkers*2),
		quit:       make(chan struct{}),
	}
}

// Start launches all worker goroutines.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.numWorkers; i++ {
		go wp.worker()
	}
}

// worker pulls jobs off the queue until quit is closed, then DRAINS whatever
// is still queued before exiting. Every queued job has already wg.Add(1)'d in
// Submit — abandoning it would leak the WaitGroup counter and hang any Wait().
func (wp *WorkerPool) worker() {
	for {
		select {
		case job := <-wp.jobQueue:
			wp.runJob(job)
		case <-wp.quit:
			for {
				select {
				case job := <-wp.jobQueue:
					wp.runJob(job)
				default:
					return
				}
			}
		}
	}
}

// runJob executes a single job in isolation. wg.Done() runs even if the job
// panics — otherwise a panic would leak the WaitGroup counter and Wait() would
// hang the whole frame. Panics are logged and swallowed so a single bad job
// can't take down the pool.
func (wp *WorkerPool) runJob(job func()) {
	defer wp.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[worker_pool] job panic: %v\n%s\n", r, debug.Stack())
		}
	}()
	job()
}

// Submit enqueues a job and reports whether the pool accepted it. Blocks if
// the queue is full. After Stop() it refuses (returns false) WITHOUT touching
// the WaitGroup — the job is NOT run; the caller must run it inline (all
// callers do), so submitted work always completes and no caller-side
// WaitGroup can hang on a silently dropped job.
func (wp *WorkerPool) Submit(job func()) bool {
	if wp.stopped.Load() {
		return false
	}
	wp.wg.Add(1)
	wp.jobQueue <- job
	return true
}

// Wait blocks until all submitted jobs have completed.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Stop signals workers to exit and waits for every accepted job (in-flight
// AND still queued — workers drain the queue on quit) to finish. Idempotent.
// Lifecycle contract: Submit must not be called CONCURRENTLY with Stop — the
// pool has a single owner (the main goroutine submits during ticks, stops at
// shutdown), and a truly concurrent Submit could slip its job into the queue
// after the drain and hang the Wait below. Submit called AFTER Stop returns
// is fine: it refuses and the caller runs the job inline.
func (wp *WorkerPool) Stop() {
	if !wp.stopped.CompareAndSwap(false, true) {
		return
	}
	close(wp.quit)
	wp.wg.Wait()
}

// GetNumWorkers returns the configured worker count.
func (wp *WorkerPool) GetNumWorkers() int {
	return wp.numWorkers
}
