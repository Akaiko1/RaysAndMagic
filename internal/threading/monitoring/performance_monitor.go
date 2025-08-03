package monitoring

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// PerformanceMonitor tracks various performance metrics
type PerformanceMonitor struct {
	// Frame metrics
	frameCount atomic.Uint64
	frameTime  atomic.Uint64 // nanoseconds

	// Rendering metrics
	raycastTime      atomic.Uint64
	spriteRenderTime atomic.Uint64
	entityUpdateTime atomic.Uint64

	// Threading metrics
	activeWorkers atomic.Int32
	queuedJobs    atomic.Int32
	completedJobs atomic.Uint64

	// Game-specific metrics
	monstersUpdated    atomic.Uint64
	projectilesActive  atomic.Int32
	collisionsDetected atomic.Uint64

	// Statistics
	mutex           sync.RWMutex
	avgFrameTime    float64
	avgRaycastTime  float64
	peakMemoryUsage uint64
	startTime       time.Time

	// Configuration
	enableDetailed bool
	sampleInterval time.Duration
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{
		startTime:      time.Now(),
		enableDetailed: true,
		sampleInterval: time.Second,
	}
}

// FrameTimer helps measure frame timing
type FrameTimer struct {
	monitor   *PerformanceMonitor
	startTime time.Time
}

// StartFrame begins frame timing
func (pm *PerformanceMonitor) StartFrame() *FrameTimer {
	return &FrameTimer{
		monitor:   pm,
		startTime: time.Now(),
	}
}

// EndFrame completes frame timing
func (ft *FrameTimer) EndFrame() {
	frameTime := time.Since(ft.startTime)
	ft.monitor.frameTime.Store(uint64(frameTime.Nanoseconds()))
	ft.monitor.frameCount.Add(1)

	// Update average frame time
	if ft.monitor.enableDetailed {
		ft.monitor.mutex.Lock()
		count := ft.monitor.frameCount.Load()
		if count > 0 {
			ft.monitor.avgFrameTime = float64(ft.monitor.frameTime.Load()) / float64(count)
		}
		ft.monitor.mutex.Unlock()
	}
}

// RaycastTimer helps measure raycasting performance
type RaycastTimer struct {
	monitor   *PerformanceMonitor
	startTime time.Time
}

// StartRaycast begins raycast timing
func (pm *PerformanceMonitor) StartRaycast() *RaycastTimer {
	return &RaycastTimer{
		monitor:   pm,
		startTime: time.Now(),
	}
}

// EndRaycast completes raycast timing
func (rt *RaycastTimer) EndRaycast() {
	raycastTime := time.Since(rt.startTime)
	rt.monitor.raycastTime.Store(uint64(raycastTime.Nanoseconds()))

	if rt.monitor.enableDetailed {
		rt.monitor.mutex.Lock()
		rt.monitor.avgRaycastTime = float64(rt.monitor.raycastTime.Load())
		rt.monitor.mutex.Unlock()
	}
}

// WorkerMetrics tracks worker pool performance
type WorkerMetrics struct {
	ActiveWorkers  int32
	QueuedJobs     int32
	CompletedJobs  uint64
	AverageJobTime time.Duration
}

// UpdateWorkerMetrics updates threading metrics
func (pm *PerformanceMonitor) UpdateWorkerMetrics(active, queued int32, completed uint64) {
	pm.activeWorkers.Store(active)
	pm.queuedJobs.Store(queued)
	pm.completedJobs.Store(completed)
}

// IncrementActiveWorkers atomically increments active worker count
func (pm *PerformanceMonitor) IncrementActiveWorkers() {
	pm.activeWorkers.Add(1)
}

// DecrementActiveWorkers atomically decrements active worker count
func (pm *PerformanceMonitor) DecrementActiveWorkers() {
	pm.activeWorkers.Add(-1)
}

// AddQueuedJob atomically adds to queued job count
func (pm *PerformanceMonitor) AddQueuedJob() {
	pm.queuedJobs.Add(1)
}

// CompleteJob atomically marks a job as complete
func (pm *PerformanceMonitor) CompleteJob() {
	pm.queuedJobs.Add(-1)
	pm.completedJobs.Add(1)
}

// GameMetrics tracks game-specific performance data
type GameMetrics struct {
	MonstersUpdated    uint64
	ProjectilesActive  int32
	CollisionsDetected uint64
	FramesPerSecond    float64
	MemoryUsageMB      uint64
}

// UpdateGameMetrics updates game-specific metrics
func (pm *PerformanceMonitor) UpdateGameMetrics(monsters uint64, projectiles int32, collisions uint64) {
	pm.monstersUpdated.Store(monsters)
	pm.projectilesActive.Store(projectiles)
	pm.collisionsDetected.Store(collisions)
}

// GetCurrentMetrics returns current performance metrics
func (pm *PerformanceMonitor) GetCurrentMetrics() GameMetrics {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Calculate FPS
	frameTime := pm.frameTime.Load()
	fps := 0.0
	if frameTime > 0 {
		fps = 1000000000.0 / float64(frameTime) // Convert nanoseconds to FPS
	}

	// Get memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryMB := memStats.Alloc / 1024 / 1024

	return GameMetrics{
		MonstersUpdated:    pm.monstersUpdated.Load(),
		ProjectilesActive:  pm.projectilesActive.Load(),
		CollisionsDetected: pm.collisionsDetected.Load(),
		FramesPerSecond:    fps,
		MemoryUsageMB:      memoryMB,
	}
}

// GetDetailedStats returns detailed performance statistics
func (pm *PerformanceMonitor) GetDetailedStats() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(pm.startTime)

	return map[string]interface{}{
		"uptime_seconds":      uptime.Seconds(),
		"frame_count":         pm.frameCount.Load(),
		"avg_frame_time_ms":   pm.avgFrameTime / 1000000, // Convert to milliseconds
		"avg_raycast_time_ms": pm.avgRaycastTime / 1000000,
		"current_fps":         1000000000.0 / float64(pm.frameTime.Load()),
		"active_workers":      pm.activeWorkers.Load(),
		"queued_jobs":         pm.queuedJobs.Load(),
		"completed_jobs":      pm.completedJobs.Load(),
		"memory_alloc_mb":     memStats.Alloc / 1024 / 1024,
		"memory_sys_mb":       memStats.Sys / 1024 / 1024,
		"gc_cycles":           memStats.NumGC,
		"monsters_updated":    pm.monstersUpdated.Load(),
		"projectiles_active":  pm.projectilesActive.Load(),
		"collisions_detected": pm.collisionsDetected.Load(),
		"cpu_cores":           runtime.NumCPU(),
		"goroutines":          runtime.NumGoroutine(),
	}
}

// PerformanceAlert represents a performance warning
type PerformanceAlert struct {
	Type      string
	Message   string
	Value     float64
	Threshold float64
	Timestamp time.Time
}

// CheckPerformanceAlerts checks for performance issues and returns alerts
func (pm *PerformanceMonitor) CheckPerformanceAlerts() []PerformanceAlert {
	alerts := make([]PerformanceAlert, 0)
	currentTime := time.Now()

	// Check frame rate
	frameTime := pm.frameTime.Load()
	if frameTime > 0 {
		fps := 1000000000.0 / float64(frameTime)
		if fps < 30 { // Alert if FPS drops below 30
			alerts = append(alerts, PerformanceAlert{
				Type:      "low_fps",
				Message:   "Frame rate is below 30 FPS",
				Value:     fps,
				Threshold: 30,
				Timestamp: currentTime,
			})
		}
	}

	// Check memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryMB := float64(memStats.Alloc) / 1024 / 1024
	if memoryMB > 500 { // Alert if memory usage exceeds 500MB
		alerts = append(alerts, PerformanceAlert{
			Type:      "high_memory",
			Message:   "Memory usage is above 500MB",
			Value:     memoryMB,
			Threshold: 500,
			Timestamp: currentTime,
		})
	}

	// Check worker queue
	queuedJobs := pm.queuedJobs.Load()
	if queuedJobs > 100 { // Alert if job queue is backing up
		alerts = append(alerts, PerformanceAlert{
			Type:      "queue_backlog",
			Message:   "Worker queue has more than 100 pending jobs",
			Value:     float64(queuedJobs),
			Threshold: 100,
			Timestamp: currentTime,
		})
	}

	return alerts
}

// EnableDetailedLogging enables/disables detailed performance logging
func (pm *PerformanceMonitor) EnableDetailedLogging(enabled bool) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.enableDetailed = enabled
}

// Reset resets all performance counters
func (pm *PerformanceMonitor) Reset() {
	pm.frameCount.Store(0)
	pm.frameTime.Store(0)
	pm.raycastTime.Store(0)
	pm.spriteRenderTime.Store(0)
	pm.entityUpdateTime.Store(0)
	pm.activeWorkers.Store(0)
	pm.queuedJobs.Store(0)
	pm.completedJobs.Store(0)
	pm.monstersUpdated.Store(0)
	pm.projectilesActive.Store(0)
	pm.collisionsDetected.Store(0)

	pm.mutex.Lock()
	pm.avgFrameTime = 0
	pm.avgRaycastTime = 0
	pm.peakMemoryUsage = 0
	pm.startTime = time.Now()
	pm.mutex.Unlock()
}

// ProfiledFunction wraps a function with performance timing
func (pm *PerformanceMonitor) ProfiledFunction(name string, fn func()) time.Duration {
	start := time.Now()
	fn()
	duration := time.Since(start)

	// Store timing based on function name
	switch name {
	case "raycast":
		pm.raycastTime.Store(uint64(duration.Nanoseconds()))
	case "sprite_render":
		pm.spriteRenderTime.Store(uint64(duration.Nanoseconds()))
	case "entity_update":
		pm.entityUpdateTime.Store(uint64(duration.Nanoseconds()))
	}

	return duration
}

// GetAverageJobTime calculates average job completion time
func (pm *PerformanceMonitor) GetAverageJobTime() time.Duration {
	completedJobs := pm.completedJobs.Load()
	if completedJobs == 0 {
		return 0
	}

	totalTime := pm.frameTime.Load()
	avgNanos := totalTime / completedJobs
	return time.Duration(avgNanos)
}
