package graphics

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The gate admits its limit and no more; a raised limit admits waiters at
// once, a lowered one applies to the next entry, and neither passes capacity.
func TestWorkerGateLimit(t *testing.T) {
	gate := NewWorkerGate(4, 2)
	ctx := context.Background()
	first, ok1 := gate.enter(ctx)
	second, ok2 := gate.enter(ctx)
	if !ok1 || !ok2 {
		t.Fatal("gate refused work under its limit")
	}
	short, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	if _, ok := gate.enter(short); ok {
		t.Fatal("gate admitted a third preparation at limit 2")
	}
	admitted := make(chan *PreparationLease, 1)
	go func() {
		slot, _ := gate.enter(ctx)
		admitted <- slot
	}()
	gate.SetLimit(3)
	var third *PreparationLease
	select {
	case third = <-admitted:
		if third == nil {
			t.Fatal("waiter was refused after the limit rose")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a raised limit did not admit the waiter")
	}
	gate.SetLimit(100)
	if got := gate.Limit(); got != 4 {
		t.Fatalf("limit %d passed the gate's capacity 4", got)
	}
	gate.SetLimit(1)
	first.Release()
	second.Release()
	third.Release()
	if _, ok := gate.enter(ctx); !ok {
		t.Fatal("an empty gate refused work")
	}
	short2, stop2 := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop2()
	if _, ok := gate.enter(short2); ok {
		t.Fatal("gate admitted a second preparation after its limit fell to 1")
	}
}

// Results keep job order whatever order the workers finish in, no more
// preparations run at once than the gate admits, and reservations are made in
// job order.
func TestPrepareOrderedKeepsJobOrder(t *testing.T) {
	for _, limit := range []int{1, 3} {
		const jobs = 12
		list := make([]int, jobs)
		for i := range list {
			list[i] = i
		}
		var running, peak atomic.Int32
		var mu sync.Mutex
		var reserved []int
		results := PrepareOrdered(context.Background(), list, NewWorkerGate(4, limit),
			func(_ context.Context, job int) (*PreparationLease, bool) {
				mu.Lock()
				reserved = append(reserved, job)
				mu.Unlock()
				return nil, true
			},
			func(job int, _ *PreparationLease) int {
				now := running.Add(1)
				for {
					seen := peak.Load()
					if now <= seen || peak.CompareAndSwap(seen, now) {
						break
					}
				}
				// Early jobs take longest: later ones finish first.
				time.Sleep(time.Duration(jobs-job) * 200 * time.Microsecond)
				running.Add(-1)
				return job
			})
		next := 0
		for got := range results {
			if got != next {
				t.Fatalf("limit %d: result %d arrived in place of %d", limit, got, next)
			}
			next++
		}
		if next != jobs {
			t.Fatalf("limit %d: %d of %d results arrived", limit, next, jobs)
		}
		if got := int(peak.Load()); got > limit {
			t.Fatalf("limit %d: %d preparations ran at once", limit, got)
		}
		for i, job := range reserved {
			if job != i {
				t.Fatalf("limit %d: job %d reserved in place of %d", limit, job, i)
			}
		}
	}
}

// A larger limit admits a reservation that was waiting for room.
func TestPreparationBudgetSetLimitAdmitsWaiter(t *testing.T) {
	budget := NewPreparationBudget(8)
	first, _ := budget.Acquire(context.Background(), 8)
	admitted := make(chan *PreparationLease, 1)
	go func() {
		lease, _ := budget.Acquire(context.Background(), 8)
		admitted <- lease
	}()
	select {
	case <-admitted:
		t.Fatal("a full budget admitted a second reservation")
	case <-time.After(20 * time.Millisecond):
	}
	budget.SetLimit(16)
	select {
	case lease := <-admitted:
		lease.Release()
	case <-time.After(5 * time.Second):
		t.Fatal("a raised limit did not admit the waiting reservation")
	}
	first.Release()
	if used, _ := budget.Usage(); used != 0 {
		t.Fatalf("released budget still reserves %d bytes", used)
	}
}
