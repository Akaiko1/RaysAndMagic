package rendering

import (
	"sync"
	"ugataima/internal/mathutil"
	"ugataima/internal/threading/core"
)

// ParallelRenderer handles multi-threaded rendering operations
type ParallelRenderer struct {
	workerPool *core.WorkerPool
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
func (pr *ParallelRenderer) RenderRaycast(numRays int, raycastFunc func(int, float64) (float64, interface{}),
	angleFunc func(int, int) float64) []RaycastResult {

	// Single allocation for results
	results := make([]RaycastResult, numRays)

	// Very small workloads: process inline to avoid synchronization overhead
	if numRays <= 8 {
		for rayIndex := 0; rayIndex < numRays; rayIndex++ {
			angle := angleFunc(rayIndex, numRays)
			distance, tileType := raycastFunc(rayIndex, angle)
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
				angle := angleFunc(rayIndex, numRays)
				distance, tileType := raycastFunc(rayIndex, angle)
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
