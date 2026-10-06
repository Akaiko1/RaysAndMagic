package monitoring

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"ugataima/internal/threading/core"
	"ugataima/internal/threading/entities"
	"ugataima/internal/threading/rendering"
)

// =============================================================================
// PERFORMANCE MONITOR TESTS (Consolidated)
// =============================================================================

// Percentiles come from the presented-frame interval ring: known intervals in,
// exact order statistics out; empty and reset states report not-ok.
func TestFrameTimePercentiles(t *testing.T) {
	pm := NewPerformanceMonitor()

	if _, _, _, ok := pm.FrameTimePercentilesMs(); ok {
		t.Error("expected ok=false before any presented frame")
	}
	pm.RecordPresentedFrame()
	if _, _, _, ok := pm.FrameTimePercentilesMs(); ok {
		t.Error("expected ok=false after a single draw (no interval yet)")
	}

	// Inject 100 known intervals (1..100 ms) directly into the ring.
	pm.mutex.Lock()
	for i := 0; i < 100; i++ {
		pm.frameTimes[i] = uint64(i+1) * 1e6
	}
	pm.frameTimesLen = 100
	pm.frameTimesIdx = 100 % frameTimeWindow
	pm.mutex.Unlock()

	p50, p95, p99, ok := pm.FrameTimePercentilesMs()
	if !ok {
		t.Fatal("expected ok=true with a filled window")
	}
	if p50 != 50 || p95 != 95 || p99 != 99 {
		t.Errorf("percentiles = %.1f/%.1f/%.1f, want 50/95/99", p50, p95, p99)
	}

	pm.Reset()
	if _, _, _, ok := pm.FrameTimePercentilesMs(); ok {
		t.Error("expected ok=false after Reset")
	}
}

func TestPerformanceMonitorFrameTiming(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Test frame timing
	frameTimer := pm.StartFrame()
	time.Sleep(10 * time.Millisecond) // Simulate some work
	frameTimer.EndFrame()

	// Check that frame count was incremented
	if pm.frameCount.Load() != 1 {
		t.Errorf("Expected frame count to be 1, got %d", pm.frameCount.Load())
	}

	// Check that frame time was recorded
	frameTime := pm.frameTime.Load()
	if frameTime == 0 {
		t.Error("Expected frame time to be recorded")
	}

	// Frame time should be at least 10ms (in nanoseconds)
	minExpectedTime := uint64(10 * time.Millisecond)
	if frameTime < minExpectedTime {
		t.Errorf("Expected frame time to be at least %d ns, got %d ns", minExpectedTime, frameTime)
	}
}

func TestPerformanceMonitorMetrics(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Test game metrics
	pm.UpdateGameMetrics(25, 5, 10)
	if pm.monstersUpdated.Load() != 25 {
		t.Errorf("Expected monsters updated to be 25, got %d", pm.monstersUpdated.Load())
	}

}

func TestPerformanceMonitorConcurrency(t *testing.T) {
	pm := NewPerformanceMonitor()
	done := make(chan bool, 10)

	// Multiple goroutines doing concurrent operations
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				frameTimer := pm.StartFrame()
				time.Sleep(time.Microsecond * 100)
				frameTimer.EndFrame()
				pm.UpdateGameMetrics(uint64(j), 1, 1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify some work was done
	if pm.frameCount.Load() == 0 {
		t.Error("Expected some frames to be recorded")
	}
	if pm.monstersUpdated.Load() == 0 {
		t.Error("Expected the game metrics to be recorded")
	}
}

// =============================================================================
// WORKER POOL TESTS
// =============================================================================

func TestWorkerPoolCreation(t *testing.T) {
	// Test default creation
	wp := core.NewWorkerPool(0)
	if wp.GetNumWorkers() != runtime.NumCPU() {
		t.Errorf("Expected %d workers (CPU count), got %d", runtime.NumCPU(), wp.GetNumWorkers())
	}

	// Test specific worker count
	wp2 := core.NewWorkerPool(4)
	if wp2.GetNumWorkers() != 4 {
		t.Errorf("Expected 4 workers, got %d", wp2.GetNumWorkers())
	}
}

func TestWorkerPoolConcurrentAccess(t *testing.T) {
	wp := core.NewWorkerPool(4)
	wp.Start()
	defer wp.Stop()

	var counter int64
	numGoroutines := 10
	jobsPerGoroutine := 50

	var wg sync.WaitGroup

	// Multiple goroutines submitting jobs concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < jobsPerGoroutine; j++ {
				wp.Submit(func() {
					atomic.AddInt64(&counter, 1)
				})
			}
		}()
	}

	wg.Wait()
	wp.Wait() // Wait for all jobs to complete

	expected := int64(numGoroutines * jobsPerGoroutine)
	if atomic.LoadInt64(&counter) != expected {
		t.Errorf("Expected counter to be %d, got %d", expected, counter)
	}
}

// =============================================================================
// PARALLEL RENDERER TESTS
// =============================================================================

// Mock raycast function for testing
func mockRaycastFunc(rayIndex int) (float64, interface{}) {
	distance := float64(rayIndex) * 1.5
	tileType := "wall"
	return distance, tileType
}

func TestParallelRendererInto(t *testing.T) {
	renderer := rendering.NewParallelRenderer()
	const numRays = 100
	payloads := make([]int, numRays)

	results := renderer.RenderRaycastInto(numRays, func(rayIndex int, result *rendering.RaycastResult) {
		payloads[rayIndex] = rayIndex * 2
		result.Distance = float64(rayIndex) * 1.5
		result.TileType = &payloads[rayIndex]
	})

	if len(results) != numRays {
		t.Fatalf("got %d results, want %d", len(results), numRays)
	}
	for i := range results {
		if results[i].Distance != float64(i)*1.5 {
			t.Fatalf("ray %d distance = %.2f", i, results[i].Distance)
		}
		got, ok := results[i].TileType.(*int)
		if !ok || got != &payloads[i] || *got != i*2 {
			t.Fatalf("ray %d payload = %#v", i, results[i].TileType)
		}
	}
}

func TestParallelRendererConcurrency(t *testing.T) {
	renderer := rendering.NewParallelRenderer()

	var wg sync.WaitGroup
	numConcurrentRenders := 5

	// Multiple concurrent render operations
	for i := 0; i < numConcurrentRenders; i++ {
		wg.Add(1)
		go func(renderID int) {
			defer wg.Done()
			numRays := 50 + renderID*10 // Different ray counts
			results := renderer.RenderRaycastInto(numRays, func(rayIndex int, result *rendering.RaycastResult) {
				result.Distance, result.TileType = mockRaycastFunc(rayIndex)
			})

			if len(results) != numRays {
				t.Errorf("Render %d: expected %d results, got %d", renderID, numRays, len(results))
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// ENTITY UPDATER TESTS
// =============================================================================

// Mock entities for testing
type MockMonster struct {
	id      int
	x, y    float64
	alive   bool
	updated bool
	mu      sync.Mutex
}

func (m *MockMonster) Update() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated = true
	time.Sleep(time.Microsecond * 100) // Simulate work
}

func (m *MockMonster) IsAlive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alive
}

func (m *MockMonster) GetPosition() (float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.x, m.y
}

func (m *MockMonster) SetPosition(x, y float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.x, m.y = x, y
}

func (m *MockMonster) IsUpdated() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updated
}

// ApplyCollisionUpdate is a no-op here - this mock only exercises that
// Update() runs on every monster, not the two-phase collision apply.
func (m *MockMonster) ApplyCollisionUpdate() {}

func TestEntityUpdaterWithDeadMonsters(t *testing.T) {
	updater := entities.NewEntityUpdater()

	// Create test monsters, some dead
	monsters := make([]entities.MonsterUpdateInterface, 6)
	for i := range monsters {
		monsters[i] = &MockMonster{
			id:    i,
			x:     float64(i),
			y:     float64(i * 2),
			alive: i%2 == 0, // Every other monster is alive
		}
	}

	// Update monsters in parallel
	updater.UpdateMonstersParallel(monsters)

	// Verify only alive monsters were updated
	for i, monster := range monsters {
		mockMonster := monster.(*MockMonster)
		shouldBeUpdated := mockMonster.IsAlive()
		wasUpdated := mockMonster.IsUpdated()

		if shouldBeUpdated != wasUpdated {
			t.Errorf("Monster %d: expected updated=%v, got updated=%v", i, shouldBeUpdated, wasUpdated)
		}
	}
}

// =============================================================================
// BENCHMARK TESTS (Consolidated)
// =============================================================================

func BenchmarkWorkerPoolSubmit(b *testing.B) {
	wp := core.NewWorkerPool(4)
	wp.Start()
	defer wp.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wp.Submit(func() {
			// Minimal work
		})
	}
	wp.Wait()
}

func BenchmarkPerformanceMonitorFrameTiming(b *testing.B) {
	pm := NewPerformanceMonitor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frameTimer := pm.StartFrame()
		frameTimer.EndFrame()
	}
}

func BenchmarkParallelRenderer(b *testing.B) {
	renderer := rendering.NewParallelRenderer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderer.RenderRaycastInto(100, func(rayIndex int, result *rendering.RaycastResult) {
			result.Distance, result.TileType = mockRaycastFunc(rayIndex)
		})
	}
}
