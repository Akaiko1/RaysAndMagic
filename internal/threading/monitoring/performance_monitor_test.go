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

func TestNewPerformanceMonitor(t *testing.T) {
	pm := NewPerformanceMonitor()

	if pm == nil {
		t.Fatal("NewPerformanceMonitor returned nil")
	}

	if pm.enableDetailed != true {
		t.Error("Expected enableDetailed to be true")
	}

	if pm.sampleInterval != time.Second {
		t.Error("Expected sampleInterval to be 1 second")
	}

	// Check that start time is recent
	if time.Since(pm.startTime) > time.Second {
		t.Error("Start time should be recent")
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

	// Test worker metrics
	pm.UpdateWorkerMetrics(5, 10, 100)
	if pm.activeWorkers.Load() != 5 {
		t.Errorf("Expected active workers to be 5, got %d", pm.activeWorkers.Load())
	}

	// Test individual operations
	pm.IncrementActiveWorkers()
	if pm.activeWorkers.Load() != 6 {
		t.Errorf("Expected active workers to be 6 after increment, got %d", pm.activeWorkers.Load())
	}

	pm.CompleteJob()
	if pm.completedJobs.Load() != 101 {
		t.Errorf("Expected completed jobs to be 101, got %d", pm.completedJobs.Load())
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
				pm.IncrementActiveWorkers()
				pm.CompleteJob()
				pm.DecrementActiveWorkers()
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
	if pm.completedJobs.Load() == 0 {
		t.Error("Expected some jobs to be completed")
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

func TestWorkerPoolJobExecution(t *testing.T) {
	wp := core.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	var counter int32
	var wg sync.WaitGroup

	// Submit multiple jobs
	for i := 0; i < 10; i++ {
		wg.Add(1)
		wp.Submit(func() {
			atomic.AddInt32(&counter, 1)
			wg.Done()
		})
	}

	wg.Wait()

	if counter != 10 {
		t.Errorf("Expected counter to be 10, got %d", counter)
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

func TestParallelRenderer(t *testing.T) {
	renderer := rendering.NewParallelRenderer()
	if renderer == nil {
		t.Fatal("NewParallelRenderer returned nil")
	}

	// Test parallel raycast
	numRays := 100
	results := renderer.RenderRaycast(numRays, mockRaycastFunc)

	if len(results) != numRays {
		t.Errorf("Expected %d results, got %d", numRays, len(results))
	}

	// Verify some results
	for i := 0; i < min(10, len(results)); i++ {
		expectedDistance, _ := mockRaycastFunc(i)

		if results[i].Distance != expectedDistance {
			t.Errorf("Ray %d: expected distance %.2f, got %.2f", i, expectedDistance, results[i].Distance)
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
			results := renderer.RenderRaycast(numRays, mockRaycastFunc)

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

// ApplyCollisionUpdate is a no-op here — this mock only exercises that
// Update() runs on every monster, not the two-phase collision apply.
func (m *MockMonster) ApplyCollisionUpdate() {}

func TestEntityUpdater(t *testing.T) {
	updater := entities.NewEntityUpdater()
	if updater == nil {
		t.Fatal("NewEntityUpdater returned nil")
	}

	// Create test monsters
	monsters := make([]entities.MonsterUpdateInterface, 10)
	for i := range monsters {
		monsters[i] = &MockMonster{
			id:    i,
			x:     float64(i),
			y:     float64(i * 2),
			alive: true,
		}
	}

	// Update monsters in parallel
	updater.UpdateMonstersParallel(monsters)

	// Verify all monsters were updated
	for i, monster := range monsters {
		mockMonster := monster.(*MockMonster)
		if !mockMonster.IsUpdated() {
			t.Errorf("Monster %d was not updated", i)
		}
	}
}

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
// INTEGRATION TESTS
// =============================================================================

func TestFullParallelPipeline(t *testing.T) {
	// Test the complete parallel processing pipeline
	pm := NewPerformanceMonitor()
	wp := core.NewWorkerPool(4)
	wp.Start()
	defer wp.Stop()

	renderer := rendering.NewParallelRenderer()
	updater := entities.NewEntityUpdater()

	// Simulate a game frame
	frameTimer := pm.StartFrame()

	// Create entities
	monsters := make([]entities.MonsterUpdateInterface, 20)
	for i := range monsters {
		monsters[i] = &MockMonster{
			id:    i,
			x:     float64(i * 10),
			y:     float64(i * 15),
			alive: true,
		}
	}

	// Parallel entity updates
	updater.UpdateMonstersParallel(monsters)

	// Parallel rendering
	results := renderer.RenderRaycast(50, mockRaycastFunc)

	// Some parallel work via the worker pool
	var workCounter int64
	for i := 0; i < 100; i++ {
		wp.Submit(func() {
			atomic.AddInt64(&workCounter, 1)
			time.Sleep(time.Microsecond * 50)
		})
	}
	wp.Wait()

	frameTimer.EndFrame()

	// Verify everything completed
	if len(results) != 50 {
		t.Errorf("Expected 50 raycast results, got %d", len(results))
	}

	if atomic.LoadInt64(&workCounter) != 100 {
		t.Errorf("Expected 100 work units, got %d", workCounter)
	}

	for i, monster := range monsters {
		mockMonster := monster.(*MockMonster)
		if !mockMonster.IsUpdated() {
			t.Errorf("Monster %d was not updated", i)
		}
	}

	// Check performance metrics
	metrics := pm.GetCurrentMetrics()
	if metrics.FramesPerSecond <= 0 {
		t.Error("Expected positive FPS")
	}

	if pm.frameCount.Load() != 1 {
		t.Errorf("Expected 1 frame, got %d", pm.frameCount.Load())
	}
}

func TestHighLoadConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high-load test in short mode")
	}

	// Stress test with high concurrency
	numWorkers := runtime.NumCPU() * 2
	wp := core.NewWorkerPool(numWorkers)
	wp.Start()
	defer wp.Stop()

	pm := NewPerformanceMonitor()

	var totalWork int64
	numIterations := 1000

	startTime := time.Now()

	// Submit lots of concurrent work
	for i := 0; i < numIterations; i++ {
		wp.Submit(func() {
			// Simulate various types of work
			frameTimer := pm.StartFrame()

			// Some CPU work
			sum := 0
			for j := 0; j < 1000; j++ {
				sum += j
			}

			// Update metrics
			pm.IncrementActiveWorkers()
			pm.UpdateGameMetrics(uint64(sum), 1, 1)
			pm.DecrementActiveWorkers()

			frameTimer.EndFrame()
			atomic.AddInt64(&totalWork, 1)
		})
	}

	wp.Wait()
	duration := time.Since(startTime)

	t.Logf("Completed %d work units in %v", totalWork, duration)
	t.Logf("Average time per work unit: %v", duration/time.Duration(totalWork))

	if atomic.LoadInt64(&totalWork) != int64(numIterations) {
		t.Errorf("Expected %d work units, got %d", numIterations, totalWork)
	}

	// Check that we didn't have any race conditions
	metrics := pm.GetCurrentMetrics()
	if metrics.FramesPerSecond < 0 {
		t.Error("Invalid FPS value, possible race condition")
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
		renderer.RenderRaycast(100, mockRaycastFunc)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
