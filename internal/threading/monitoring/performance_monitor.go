package monitoring

import (
	"math"
	"runtime"
	"slices"
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

	// Game-specific metrics
	monstersUpdated    atomic.Uint64
	projectilesActive  atomic.Int32
	collisionsDetected atomic.Uint64

	// Statistics
	mutex     sync.RWMutex
	startTime time.Time
	// Ring of the last frameTimeWindow presented-frame intervals, for the
	// overlay's p50/p95/p99. An average hides hitches; percentiles don't.
	frameTimes    [frameTimeWindow]uint64
	frameTimesIdx int
	frameTimesLen int
	lastPresented time.Time
}

// frameTimeWindow is ~2-4s of frames: long enough for stable percentiles,
// short enough that a spike ages out and the overlay reflects NOW.
const frameTimeWindow = 240

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{
		startTime: time.Now(),
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
}

// RecordPresentedFrame notes one presented frame (call it once per Draw); the
// interval since the previous call feeds the percentile window. Wall-clock
// between draws is the pacing the player actually sees - it includes update
// ticks, render, GPU sync and prewarm, which the update-only FrameTimer misses.
func (pm *PerformanceMonitor) RecordPresentedFrame() {
	now := time.Now()
	pm.mutex.Lock()
	if !pm.lastPresented.IsZero() {
		pm.frameTimes[pm.frameTimesIdx] = uint64(now.Sub(pm.lastPresented).Nanoseconds())
		pm.frameTimesIdx = (pm.frameTimesIdx + 1) % frameTimeWindow
		if pm.frameTimesLen < frameTimeWindow {
			pm.frameTimesLen++
		}
	}
	pm.lastPresented = now
	pm.mutex.Unlock()
}

// FrameTimePercentilesMs returns p50/p95/p99 of the recent presented-frame
// window in milliseconds. ok=false until at least two draws were recorded.
func (pm *PerformanceMonitor) FrameTimePercentilesMs() (p50, p95, p99 float64, ok bool) {
	pm.mutex.RLock()
	n := pm.frameTimesLen
	window := make([]uint64, n)
	copy(window, pm.frameTimes[:n])
	pm.mutex.RUnlock()
	if n == 0 {
		return 0, 0, 0, false
	}
	slices.Sort(window)
	at := func(q float64) float64 {
		i := int(math.Ceil(q*float64(n))) - 1
		if i < 0 {
			i = 0
		}
		return float64(window[i]) / 1e6
	}
	return at(0.50), at(0.95), at(0.99), true
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
		"uptime_seconds":             uptime.Seconds(),
		"frame_count":                pm.frameCount.Load(),
		"last_frame_time_ms":         float64(pm.frameTime.Load()) / 1000000,
		"last_raycast_time_ms":       float64(pm.raycastTime.Load()) / 1000000,
		"last_sprite_render_time_ms": float64(pm.spriteRenderTime.Load()) / 1000000,
		"last_entity_update_time_ms": float64(pm.entityUpdateTime.Load()) / 1000000,
		"current_fps":                1000000000.0 / float64(pm.frameTime.Load()),
		"memory_alloc_mb":            memStats.Alloc / 1024 / 1024,
		"memory_sys_mb":              memStats.Sys / 1024 / 1024,
		"gc_cycles":                  memStats.NumGC,
		"monsters_updated":           pm.monstersUpdated.Load(),
		"projectiles_active":         pm.projectilesActive.Load(),
		"collisions_detected":        pm.collisionsDetected.Load(),
		"cpu_cores":                  runtime.NumCPU(),
		"goroutines":                 runtime.NumGoroutine(),
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

	return alerts
}

// Reset resets all performance counters
func (pm *PerformanceMonitor) Reset() {
	pm.frameCount.Store(0)
	pm.frameTime.Store(0)
	pm.raycastTime.Store(0)
	pm.spriteRenderTime.Store(0)
	pm.entityUpdateTime.Store(0)
	pm.monstersUpdated.Store(0)
	pm.projectilesActive.Store(0)
	pm.collisionsDetected.Store(0)

	pm.mutex.Lock()
	pm.frameTimesIdx = 0
	pm.frameTimesLen = 0
	pm.lastPresented = time.Time{}
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
