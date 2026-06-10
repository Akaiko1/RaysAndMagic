package rendering

import (
	"sync"
	"ugataima/internal/mathutil"
	"ugataima/internal/threading/core"
)

// ParallelRenderer handles multi-threaded rendering operations
type ParallelRenderer struct {
	workerPool *core.WorkerPool
	// results is reused across frames (callers consume it before the next
	// RenderRaycast call) — allocating numRays results per frame was steady
	// GC churn for nothing.
	results []RaycastResult
}

// RaycastResult holds the result of a raycast operation
type RaycastResult struct {
	Distance  float64
	TileType  interface{} // Will be world.TileType3D from game package
	WallSlice []byte      // Pre-rendered wall slice data
}

// NewParallelRenderer creates a new parallel renderer
func NewParallelRenderer() *ParallelRenderer {
	return &ParallelRenderer{
		workerPool: core.CreateDefaultWorkerPool(),
	}
}

// RenderRaycast performs parallel raycasting optimized for 60 FPS with minimal allocations.
// Always uses the worker pool to avoid goroutine creation/destruction overhead every frame.
func (pr *ParallelRenderer) RenderRaycast(numRays int, raycastFunc func(int) (float64, interface{})) []RaycastResult {
	// Reuse the results buffer (grown once to the largest numRays seen).
	if cap(pr.results) < numRays {
		pr.results = make([]RaycastResult, numRays)
	}
	results := pr.results[:numRays]

	// Very small workloads: process inline to avoid synchronization overhead
	if numRays <= 8 {
		for rayIndex := 0; rayIndex < numRays; rayIndex++ {
			distance, tileType := raycastFunc(rayIndex)
			results[rayIndex] = RaycastResult{
				Distance: distance,
				TileType: tileType,
			}
		}
		return results
	}

	// Use worker pool for all parallel workloads to avoid goroutine churn
	// This prevents goroutine creation/destruction overhead every frame
	numWorkers := pr.workerPool.GetNumWorkers()
	batchSize := numRays / numWorkers
	if batchSize < 4 {
		batchSize = 4 // Minimum batch size for efficiency
	}
	if batchSize > 32 {
		batchSize = 32 // Cap batch size
	}

	var wg sync.WaitGroup

	for i := 0; i < numRays; i += batchSize {
		start := i
		end := mathutil.IntMin(i+batchSize, numRays)

		wg.Add(1)
		pr.workerPool.Submit(func() {
			defer wg.Done()
			for rayIndex := start; rayIndex < end; rayIndex++ {
				distance, tileType := raycastFunc(rayIndex)
				results[rayIndex] = RaycastResult{
					Distance: distance,
					TileType: tileType,
				}
			}
		})
	}
	wg.Wait()

	return results
}

// Stop shuts down the parallel renderer
func (pr *ParallelRenderer) Stop() {
	pr.workerPool.Stop()
}
