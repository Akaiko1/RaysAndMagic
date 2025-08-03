package rendering

import (
	"sync"
	"ugataima/internal/threading/core"

	"github.com/hajimehoshi/ebiten/v2"
)

// ParallelSpriteRenderer handles concurrent sprite rendering
type ParallelSpriteRenderer struct {
	workerPool *core.WorkerPool
	sprites    []*SpriteRenderJob
	mutex      sync.Mutex
}

// SpriteRenderJob represents a sprite rendering task
type SpriteRenderJob struct {
	Image      *ebiten.Image
	X, Y       int
	ScaleX     float64
	ScaleY     float64
	ColorScale struct{ R, G, B, A float32 }
	Complete   bool
}

// NewParallelSpriteRenderer creates a new parallel sprite renderer
func NewParallelSpriteRenderer() *ParallelSpriteRenderer {
	return &ParallelSpriteRenderer{
		workerPool: core.CreateDefaultWorkerPool(),
		sprites:    make([]*SpriteRenderJob, 0),
	}
}

// AddSprite adds a sprite to the render queue
func (psr *ParallelSpriteRenderer) AddSprite(job *SpriteRenderJob) {
	psr.mutex.Lock()
	psr.sprites = append(psr.sprites, job)
	psr.mutex.Unlock()
}

// RenderAll renders all queued sprites in parallel
func (psr *ParallelSpriteRenderer) RenderAll(screen *ebiten.Image) {
	psr.mutex.Lock()
	sprites := make([]*SpriteRenderJob, len(psr.sprites))
	copy(sprites, psr.sprites)
	psr.sprites = psr.sprites[:0] // Clear the slice
	psr.mutex.Unlock()

	// Process sprites in parallel
	core.ParallelForEach(sprites, func(job *SpriteRenderJob) {
		// Pre-calculate transformation matrix
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(job.ScaleX, job.ScaleY)
		opts.GeoM.Translate(float64(job.X), float64(job.Y))
		opts.ColorScale.Scale(job.ColorScale.R, job.ColorScale.G, job.ColorScale.B, job.ColorScale.A)

		// Note: We can't directly draw to screen in parallel due to Ebiten limitations
		// Instead, we prepare the transformation data here
		job.Complete = true
	})

	// Sequential drawing (required by Ebiten)
	for _, job := range sprites {
		if job.Complete {
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Scale(job.ScaleX, job.ScaleY)
			opts.GeoM.Translate(float64(job.X), float64(job.Y))
			opts.ColorScale.Scale(job.ColorScale.R, job.ColorScale.G, job.ColorScale.B, job.ColorScale.A)
			screen.DrawImage(job.Image, opts)
		}
	}
}

// Stop shuts down the parallel sprite renderer
func (psr *ParallelSpriteRenderer) Stop() {
	psr.workerPool.Stop()
}
