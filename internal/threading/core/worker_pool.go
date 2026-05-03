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

// worker pulls jobs off the queue until quit is closed.
func (wp *WorkerPool) worker() {
	for {
		select {
		case job := <-wp.jobQueue:
			wp.runJob(job)
		case <-wp.quit:
			return
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

// Submit enqueues a job. Blocks if the queue is full. Jobs submitted after
// Stop() are dropped — without this guard, Submit would deadlock waiting for
// a worker that already exited via the quit channel.
func (wp *WorkerPool) Submit(job func()) {
	if wp.stopped.Load() {
		return
	}
	wp.wg.Add(1)
	wp.jobQueue <- job
}

// Wait blocks until all submitted jobs have completed.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Stop signals workers to exit. In-flight jobs finish first. Idempotent.
func (wp *WorkerPool) Stop() {
	if !wp.stopped.CompareAndSwap(false, true) {
		return
	}
	close(wp.quit)
}

// GetNumWorkers returns the configured worker count.
func (wp *WorkerPool) GetNumWorkers() int {
	return wp.numWorkers
}
