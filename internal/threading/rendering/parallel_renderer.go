package rendering

import (
	"sync"
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

// RenderRaycast performs parallel raycasting optimized for 60 FPS with minimal allocations
func (pr *ParallelRenderer) RenderRaycast(numRays int, raycastFunc func(int, float64) (float64, interface{}),
	angleFunc func(int, int) float64) []RaycastResult {

	// Single allocation for results
	results := make([]RaycastResult, numRays)

	// For 60 FPS, use direct goroutines for smaller workloads, worker pool for larger
	if numRays <= 64 {
		// Small workload: use direct goroutines (faster than worker pool overhead)
		numWorkers := min(numRays, pr.workerPool.GetNumWorkers())
		raysPerWorker := numRays / numWorkers
		var wg sync.WaitGroup

		for i := 0; i < numWorkers; i++ {
			start := i * raysPerWorker
			end := start + raysPerWorker
			if i == numWorkers-1 {
				end = numRays // Last worker gets remaining rays
			}

			wg.Add(1)
			go func(s, e int) {
				defer wg.Done()
				for rayIndex := s; rayIndex < e; rayIndex++ {
					angle := angleFunc(rayIndex, numRays)
					distance, tileType := raycastFunc(rayIndex, angle)
					results[rayIndex] = RaycastResult{
						Distance: distance,
						TileType: tileType,
					}
				}
			}(start, end)
		}
		wg.Wait()
	} else {
		// Large workload: use worker pool with optimal batching
		numWorkers := pr.workerPool.GetNumWorkers()
		batchSize := max(8, numRays/numWorkers) // Larger batches for efficiency
		if batchSize > 32 {
			batchSize = 32 // Cap batch size
		}

		var wg sync.WaitGroup

		for i := 0; i < numRays; i += batchSize {
			start := i
			end := min(i+batchSize, numRays)

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
	}

	return results
}

// Stop shuts down the parallel renderer
func (pr *ParallelRenderer) Stop() {
	pr.workerPool.Stop()
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
