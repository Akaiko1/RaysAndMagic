package graphics

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPreparationBudgetCells(t *testing.T) {
	for _, bytes := range []int64{4, 64} {
		t.Run(fmt.Sprintf("bytes=%d", bytes), func(t *testing.T) {
			budget := NewPreparationBudget(8)
			ctx, cancel := context.WithCancel(context.Background())
			first, ok := budget.Acquire(ctx, bytes)
			if !ok {
				t.Fatal("empty budget refused progress")
			}
			if used, _ := budget.Usage(); used != bytes {
				t.Fatalf("used=%d", used)
			}
			// An oversized image owns the entire queue; another image waits.
			waitCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer stop()
			second, allowed := budget.Acquire(waitCtx, 8)
			if allowed {
				second.Release()
				t.Fatal("budget admitted another image beyond its limit")
			}
			first.Release()
			first.Release()
			cancel()
			next, ok := budget.Acquire(context.Background(), 1)
			if !ok {
				t.Fatal("released capacity was lost")
			}
			next.Release()
			if used, _ := budget.Usage(); used != 0 {
				t.Fatalf("released queue still reserves %d bytes", used)
			}
		})
	}
}

func TestSpriteResourceOriginIndexSurvivesLazyWinnerAndEviction(t *testing.T) {
	for _, animation := range []string{"", "walking_r"} {
		t.Run(animation, func(t *testing.T) {
			sm := NewSpriteManager()
			req := SpriteResourceRequest{Name: "origin-test", AnimationType: animation}
			cpu := image.NewRGBA(image.Rect(0, 0, 16, 16))
			prepared := PreparedSpriteResource{Request: req, CPU: cpu, Image: cpu, Found: true}
			if animation != "" {
				prepared.Frames = []*image.RGBA{cpu, cpu}
			}
			pending := sm.BeginPreparedResourceCommit(prepared)
			sm.CommitPreparedResource(prepared) // synchronous load wins
			for {
				_, done := pending.Advance(16)
				if done {
					break
				}
			}
			images := sm.ResourceImages(req)
			if len(images) == 0 {
				t.Fatal("source never published")
			}
			for _, img := range images {
				if got, ok := sm.ResourceForImage(img); !ok || got != req {
					t.Fatal("published origin missing")
				}
			}
			sm.EvictResource(req.Name, req.AnimationType)
			for _, img := range images {
				if _, ok := sm.ResourceForImage(img); ok {
					t.Fatal("evicted image retained in origin index")
				}
			}
		})
	}
}

func TestPrepareResourcesHonorsSharedByteBudget(t *testing.T) {
	sm := NewSpriteManager()
	path := filepath.Join(t.TempDir(), "source.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	sm.spritePaths = map[string]string{"one": path, "two": path}
	budget := NewPreparationBudget(8 * 8 * 12)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := sm.PrepareResources(ctx, []SpriteResourceRequest{{Name: "one"}, {Name: "two"}}, budget)
	var first PreparedSpriteResource
	select {
	case first = <-results:
	case <-time.After(time.Second):
		t.Fatal("first decode stalled")
	}
	if used, _ := budget.Usage(); used != 8*8*12 {
		t.Fatalf("queued decode reservation=%d", used)
	}
	select {
	case <-results:
		t.Fatal("second decode escaped the occupied byte budget")
	case <-time.After(20 * time.Millisecond):
	}
	sm.CommitPreparedResource(first)
	select {
	case second := <-results:
		sm.CommitPreparedResource(second)
	case <-time.After(time.Second):
		t.Fatal("committing the first source did not unblock the second")
	}
	if used, _ := budget.Usage(); used != 0 {
		t.Fatalf("consumed decode queue retained %d bytes", used)
	}
	sm.EvictResource("one", "")
	sm.EvictResource("two", "")
}
