package monster

import (
	"math"

	"ugataima/internal/config"
)

// defaultTileSize covers monsters built without a config (tests).
const defaultTileSize = 64.0

// defaultMonsterAI backs ai() for a monster built without a config.
var defaultMonsterAI = config.DefaultMonsterAI()

// ai is the monster_ai block this monster runs on: its config's, else
// config.DefaultMonsterAI (the shipped values), never whichever config
// happens to be loaded. Read only.
func (m *Monster3D) ai() *config.MonsterAIConfig {
	if m.config != nil {
		return &m.config.MonsterAI
	}
	return &defaultMonsterAI
}

// tps is the simulation rate every monster clock runs at (cooldowns, DoT
// ticks, movement per tick); never zero.
func (m *Monster3D) tps() int {
	if m.config != nil {
		return m.config.GetTPS()
	}
	return config.GetTargetTPS()
}

// framesForSeconds is a span authored in seconds in this monster's ticks
// (config.SecondsToFrames).
func (m *Monster3D) framesForSeconds(seconds float64) int {
	return config.SecondsToFrames(seconds, m.tps())
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
