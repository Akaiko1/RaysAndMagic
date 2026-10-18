package monster

import (
	"math"

	"ugataima/internal/config"
)

// defaultTileSize covers monsters built without a config (tests).
const defaultTileSize = 64.0

// tps is the simulation rate the monster's clocks are armed at.
func (m *Monster3D) tps() int {
	if m.config != nil {
		return m.config.GetTPS()
	}
	return config.GlobalConfig.GetTPS()
}

// tileSize returns the configured tile size for this monster's world; no
// package-level mutable state.
func (m *Monster3D) tileSize() float64 {
	if m.config != nil {
		if ts := m.config.GetTileSize(); ts > 0 {
			return ts
		}
	}
	return defaultTileSize
}

// Helper: tile coordinate conversions (search: tile-center).
func (m *Monster3D) worldToTile(pos float64) int {
	return int(math.Floor(pos / m.tileSize()))
}

func (m *Monster3D) tileToWorldCenter(tileX, tileY int) (float64, float64) {
	ts := m.tileSize()
	return float64(tileX)*ts + ts/2, float64(tileY)*ts + ts/2
}
