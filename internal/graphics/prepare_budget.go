package graphics

import (
	"context"
	"image"
	"os"
	"sync"
)

// PreparationBudget bounds reserved bytes for decoding and queued results.
// Published/committing pixels have a separate lifetime and accounting. A single
// resource larger than the limit is admitted only into an empty budget so large
// art cannot starve. Scratch allocations inside PNG decoding are not measured.
type PreparationBudget struct {
	mu                sync.Mutex
	limit, used, peak int64
	changed           chan struct{}
}

type PreparationLease struct {
	once    sync.Once
	release func()
}

func NewPreparationBudget(limit int64) *PreparationBudget {
	return &PreparationBudget{limit: max(int64(1), limit), changed: make(chan struct{})}
}

func (b *PreparationBudget) Acquire(ctx context.Context, bytes int64) (*PreparationLease, bool) {
	if b == nil || bytes <= 0 {
		return nil, true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		b.mu.Lock()
		if b.used == 0 || bytes <= b.limit-b.used {
			b.used += bytes
			b.peak = max(b.peak, b.used)
			b.mu.Unlock()
			lease := &PreparationLease{release: func() {
				b.mu.Lock()
				b.used -= bytes
				close(b.changed)
				b.changed = make(chan struct{})
				b.mu.Unlock()
			}}
			return lease, true
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, false
		}
	}
}

func (lease *PreparationLease) Release() {
	if lease != nil {
		lease.once.Do(lease.release)
	}
}

func (b *PreparationBudget) Usage() (used, peak int64) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.peak
}

func preparationBudget(budgets []*PreparationBudget) *PreparationBudget {
	if len(budgets) > 0 {
		return budgets[0]
	}
	return nil
}

// ReservePNGPreparation checks dimensions before decoding. Twelve bytes per
// pixel covers the decoded image, its keyed RGBA and copied animation frames.
func ReservePNGPreparation(ctx context.Context, path string, budget *PreparationBudget) (*PreparationLease, bool) {
	if budget == nil {
		return nil, true
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, true
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, true
	}
	return budget.Acquire(ctx, int64(cfg.Width)*int64(cfg.Height)*12)
}

// ReleaseOnCancel attaches cleanup only AFTER CPU preparation finishes. A
// cancelled decoder must retain its reservation while it still owns scratch.
func (lease *PreparationLease) ReleaseOnCancel(ctx context.Context) {
	if lease != nil && ctx != nil {
		context.AfterFunc(ctx, lease.Release)
	}
}
