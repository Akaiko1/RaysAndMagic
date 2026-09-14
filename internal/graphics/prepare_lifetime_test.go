package graphics

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise context's public AfterFunc integration without relying on private
// runtime fields. Value hides the underlying cancelCtx so registrations use
// this context's AfterFunc hook and can be counted.
type countedPreparationContext struct {
	context.Context
	pending atomic.Int64
}

func (c *countedPreparationContext) Value(any) any { return nil }
func (c *countedPreparationContext) AfterFunc(f func()) func() bool {
	c.pending.Add(1)
	var once sync.Once
	done := func() { once.Do(func() { c.pending.Add(-1) }) }
	stop := context.AfterFunc(c.Context, func() { done(); f() })
	return func() bool {
		if stop() {
			done()
			return true
		}
		return false
	}
}
func waitPreparationReleased(t *testing.T, b *PreparationBudget, c *countedPreparationContext) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		used, _ := b.Usage()
		if used == 0 && c.pending.Load() == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	used, _ := b.Usage()
	t.Fatalf("released preparation retains bytes=%d callbacks=%d", used, c.pending.Load())
}

func TestPreparationLeaseLifetimeCells(t *testing.T) {
	for _, order := range []string{"complete", "release_before_register", "cancel_before_register", "cancel_after_register", "repeat", "concurrent"} {
		t.Run(order, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &countedPreparationContext{Context: base}
			b := NewPreparationBudget(32)
			lease, ok := b.Acquire(ctx, 16)
			if !ok {
				t.Fatal("acquire failed")
			}
			switch order {
			case "complete":
				lease.ReleaseOnCancel(ctx)
				lease.Release()
			case "release_before_register":
				lease.Release()
				lease.ReleaseOnCancel(ctx)
			case "cancel_before_register":
				cancel()
				if used, _ := b.Usage(); used != 16 {
					t.Fatal("decoder scratch released before preparation ended")
				}
				lease.ReleaseOnCancel(ctx)
			case "cancel_after_register":
				lease.ReleaseOnCancel(ctx)
				cancel()
			case "repeat":
				lease.ReleaseOnCancel(ctx)
				lease.ReleaseOnCancel(ctx)
				if ctx.pending.Load() != 1 {
					t.Fatal("duplicate cancellation registration")
				}
				lease.Release()
				lease.Release()
			case "concurrent":
				var wg sync.WaitGroup
				for i := 0; i < 30; i++ {
					wg.Add(3)
					go func() { defer wg.Done(); lease.ReleaseOnCancel(ctx) }()
					go func() { defer wg.Done(); lease.Release() }()
					go func() { defer wg.Done(); cancel() }()
				}
				wg.Wait()
			}
			waitPreparationReleased(t, b, ctx)
		})
	}
}

func TestPreparedSpriteCommitDetachesCallback(t *testing.T) {
	for _, outcome := range []string{"commit", "decode_failure", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			sm := NewSpriteManager()
			path := filepath.Join(t.TempDir(), "source.png")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
			if outcome == "decode_failure" {
				data, _ := os.ReadFile(path)
				if err = os.WriteFile(path, data[:33], 0600); err != nil {
					t.Fatal(err)
				}
			}
			sm.spritePaths = map[string]string{"fixture": path}
			b := NewPreparationBudget(4096)
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &countedPreparationContext{Context: base}
			results := sm.PrepareResources(ctx, []SpriteResourceRequest{{Name: "fixture"}}, b)
			var prepared PreparedSpriteResource
			select {
			case prepared = <-results:
			case <-time.After(time.Second):
				t.Fatal("preparation stalled")
			}
			if prepared.QueueLease == nil || ctx.pending.Load() != 1 {
				t.Fatal("preparation did not register a leased result")
			}
			if outcome == "cancel" {
				cancel()
			} else {
				if prepared.Found != (outcome == "commit") {
					t.Fatalf("Found=%v", prepared.Found)
				}
				sm.CommitPreparedResource(prepared)
				defer sm.EvictResource("fixture", "")
			}
			waitPreparationReleased(t, b, ctx)
		})
	}
}
