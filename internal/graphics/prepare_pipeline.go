package graphics

import "context"

// WorkerGate bounds how many preparations run at once. Its limit can change
// while work is queued: a loading screen raises it, play lowers it again. The
// slots are a PreparationBudget counted in preparations instead of bytes.
type WorkerGate struct {
	capacity int
	slots    *PreparationBudget
}

// NewWorkerGate admits limit preparations at once; SetLimit never exceeds
// capacity, the number of workers a pipeline starts for this gate.
func NewWorkerGate(capacity, limit int) *WorkerGate {
	g := &WorkerGate{capacity: max(1, capacity)}
	g.slots = NewPreparationBudget(g.bound(limit))
	return g
}

func (g *WorkerGate) bound(limit int) int64 {
	return int64(min(max(1, limit), g.capacity))
}

func (g *WorkerGate) SetLimit(limit int) { g.slots.SetLimit(g.bound(limit)) }

// Limit is how many preparations the gate admits at once. Test-only: the
// loading pace tests of the game package read it.
func (g *WorkerGate) Limit() int { return int(g.slots.Limit()) }

// enter waits for a free slot; releasing the lease returns it.
func (g *WorkerGate) enter(ctx context.Context) (*PreparationLease, bool) {
	return g.slots.Acquire(ctx, 1)
}

// PrepareOrdered runs prepare for every job on the gate's workers (one worker
// without a gate) and delivers the results in job order. Jobs also start in
// job order, each after its reserve and once the gate admits it, so a cheap
// late job can neither overtake an earlier one nor take the byte reservation
// or the worker the earlier one is waiting for; a false reserve ends the
// pipeline. The one-result buffer and the reservations bound queued pixels
// while the owner commits earlier work. prepare receives the job's lease,
// which is released on cancellation once prepare has returned. The pipeline
// clears each job's slot when it starts, so jobs must not be read afterwards.
func PrepareOrdered[J, R any](ctx context.Context, jobs []J, gate *WorkerGate,
	reserve func(context.Context, J) (*PreparationLease, bool),
	prepare func(J, *PreparationLease) R) <-chan R {
	if ctx == nil {
		ctx = context.Background()
	}
	if gate == nil {
		gate = NewWorkerGate(1, 1)
	}
	results := make(chan R, 1)
	type queued struct {
		job J
		// lease is the job's byte reservation, slot its place at the gate.
		lease, slot *PreparationLease
		reply       chan R
	}
	workers := gate.capacity
	work := make(chan queued)
	ordered := make(chan chan R, workers)
	for range min(workers, len(jobs)) {
		go func() {
			for next := range work {
				if ctx.Err() != nil {
					next.slot.Release()
					next.lease.Release()
					continue
				}
				prepared := prepare(next.job, next.lease)
				next.slot.Release()
				next.lease.ReleaseOnCancel(ctx)
				// Each reply has one slot. A later worker never blocks while
				// the owner waits for an earlier job, even on cancellation.
				next.reply <- prepared
			}
		}()
	}
	go func() {
		defer close(work)
		defer close(ordered)
		for i, job := range jobs {
			lease, ok := reserve(ctx, job)
			if !ok {
				return
			}
			slot, ok := gate.enter(ctx)
			if !ok {
				lease.Release()
				return
			}
			var none J
			jobs[i] = none
			reply := make(chan R, 1)
			select {
			case work <- queued{job: job, lease: lease, slot: slot, reply: reply}:
			case <-ctx.Done():
				slot.Release()
				lease.Release()
				return
			}
			select {
			case ordered <- reply:
			case <-ctx.Done():
				return // The worker owns the lease after dispatch.
			}
		}
	}()
	go func() {
		defer close(results)
		for reply := range ordered {
			select {
			case prepared := <-reply:
				select {
				case results <- prepared:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return results
}
