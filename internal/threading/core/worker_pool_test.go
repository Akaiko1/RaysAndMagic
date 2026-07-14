package core

import (
	"sync/atomic"
	"testing"
	"time"
)

// Stop must complete every ACCEPTED job - in-flight and still queued - before
// returning (workers drain the queue on quit). Pre-fix, closing quit could
// strand queued jobs with their wg.Add leaked, hanging any Wait().
func TestWorkerPool_StopDrainsQueuedJobs(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()

	var ran atomic.Int32
	// More jobs than workers, each slow enough that a burst is still queued
	// when Stop fires.
	const jobs = 16
	for i := 0; i < jobs; i++ {
		if !pool.Submit(func() {
			time.Sleep(time.Millisecond)
			ran.Add(1)
		}) {
			t.Fatal("running pool refused a job")
		}
	}

	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() hung - queued jobs were not drained")
	}
	if got := ran.Load(); got != jobs {
		t.Fatalf("Stop() abandoned queued jobs: %d of %d ran", got, jobs)
	}
}

// After Stop, Submit must REFUSE (return false) without touching the
// WaitGroup, so callers can run the job inline; Wait() must not hang.
func TestWorkerPool_SubmitAfterStopRefuses(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	pool.Stop()
	pool.Stop() // idempotent

	if pool.Submit(func() {}) {
		t.Fatal("stopped pool accepted a job")
	}

	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() hung after a refused Submit - WaitGroup counter leaked")
	}
}
