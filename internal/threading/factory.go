package threading

import (
	"ugataima/internal/threading/entities"
	"ugataima/internal/threading/monitoring"
	"ugataima/internal/threading/rendering"
)

// ThreadingComponents holds all threading-related components
type ThreadingComponents struct {
	ParallelRenderer   *rendering.ParallelRenderer
	EntityUpdater      *entities.EntityUpdater
	WallSliceCache     *rendering.WallSliceCache
	PerformanceMonitor *monitoring.PerformanceMonitor
}

// NewThreadingComponents creates and initializes all threading components
// This eliminates the repetitive initialization code
func NewThreadingComponents(config interface{}) *ThreadingComponents {
	return &ThreadingComponents{
		ParallelRenderer:   rendering.NewParallelRenderer(),
		EntityUpdater:      entities.NewEntityUpdater(),
		WallSliceCache:     rendering.NewWallSliceCache(),
		PerformanceMonitor: monitoring.NewPerformanceMonitor(),
	}
}

// Shutdown gracefully shuts down all threading components
func (tc *ThreadingComponents) Shutdown() {
	if tc.ParallelRenderer != nil {
		tc.ParallelRenderer.Stop()
	}
	if tc.EntityUpdater != nil {
		tc.EntityUpdater.Stop()
	}
	if tc.PerformanceMonitor != nil {
		tc.PerformanceMonitor.Reset()
	}
}

// GetPerformanceMetrics returns current performance metrics
func (tc *ThreadingComponents) GetPerformanceMetrics() interface{} {
	if tc.PerformanceMonitor != nil {
		return tc.PerformanceMonitor.GetCurrentMetrics()
	}
	return nil
}

// GetDetailedPerformanceStats returns detailed performance statistics
func (tc *ThreadingComponents) GetDetailedPerformanceStats() map[string]interface{} {
	if tc.PerformanceMonitor != nil {
		return tc.PerformanceMonitor.GetDetailedStats()
	}
	return nil
}

// CheckPerformanceAlerts returns any performance warnings
func (tc *ThreadingComponents) CheckPerformanceAlerts() interface{} {
	if tc.PerformanceMonitor != nil {
		return tc.PerformanceMonitor.CheckPerformanceAlerts()
	}
	return nil
}
