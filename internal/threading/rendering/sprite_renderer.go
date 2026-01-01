package rendering

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// SpriteRenderer handles batched sprite rendering.
// Note: Ebiten requires all GPU operations to be sequential, so parallel rendering
// provides no benefit. This implementation focuses on efficient batching instead.
type ParallelSpriteRenderer struct {
	sprites []*SpriteRenderJob
	mutex   sync.Mutex
}

// SpriteRenderJob represents a sprite rendering task
type SpriteRenderJob struct {
	Image      *ebiten.Image
	X, Y       int
	ScaleX     float64
	ScaleY     float64
	ColorScale struct{ R, G, B, A float32 }
}

// NewParallelSpriteRenderer creates a new sprite renderer
func NewParallelSpriteRenderer() *ParallelSpriteRenderer {
	return &ParallelSpriteRenderer{
		sprites: make([]*SpriteRenderJob, 0, 64), // Pre-allocate for typical frame
	}
}

// AddSprite adds a sprite to the render queue (single sprite, takes lock)
func (psr *ParallelSpriteRenderer) AddSprite(job *SpriteRenderJob) {
	psr.mutex.Lock()
	psr.sprites = append(psr.sprites, job)
	psr.mutex.Unlock()
}

// AddSprites adds multiple sprites to the render queue in a single lock acquisition.
// This is more efficient than calling AddSprite multiple times.
func (psr *ParallelSpriteRenderer) AddSprites(jobs []*SpriteRenderJob) {
	if len(jobs) == 0 {
		return
	}
	psr.mutex.Lock()
	psr.sprites = append(psr.sprites, jobs...)
	psr.mutex.Unlock()
}

// RenderAll renders all queued sprites sequentially.
// Ebiten requires GPU operations to be sequential, so no parallelism is used.
func (psr *ParallelSpriteRenderer) RenderAll(screen *ebiten.Image) {
	psr.mutex.Lock()
	sprites := psr.sprites
	// Reset slice but keep capacity for next frame
	psr.sprites = psr.sprites[:0]
	psr.mutex.Unlock()

	if len(sprites) == 0 {
		return
	}

	// Sequential drawing (required by Ebiten - GPU operations aren't thread-safe)
	for _, job := range sprites {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(job.ScaleX, job.ScaleY)
		opts.GeoM.Translate(float64(job.X), float64(job.Y))
		opts.ColorScale.Scale(job.ColorScale.R, job.ColorScale.G, job.ColorScale.B, job.ColorScale.A)
		screen.DrawImage(job.Image, opts)
	}
}

// Clear discards all queued sprites without rendering
func (psr *ParallelSpriteRenderer) Clear() {
	psr.mutex.Lock()
	psr.sprites = psr.sprites[:0]
	psr.mutex.Unlock()
}

// Stop is a no-op for compatibility (no worker pool to stop)
func (psr *ParallelSpriteRenderer) Stop() {
	// No-op: no worker pool in this implementation
}
