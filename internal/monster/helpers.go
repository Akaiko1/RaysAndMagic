package monster

import "math"

// defaultTileSize covers monsters built without a config (tests).
const defaultTileSize = 64.0

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

func (m *Monster3D) worldToTileCenter(x, y float64) (float64, float64) {
	return m.tileToWorldCenter(m.worldToTile(x), m.worldToTile(y))
}
