package config

import "fmt"

// MonsterPhaseShiftConfig tunes the Phase Shift planner (abilities.phase_shift).
type MonsterPhaseShiftConfig struct {
	DetourFactor int `yaml:"detour_factor"`  // blink only when the pursuit route exceeds this multiple of the plan
	WindowTiles  int `yaml:"window_tiles"`   // search margin around the party and the monster
	MaxWalkTiles int `yaml:"max_walk_tiles"` // farthest walk to a launch tile
	ReplanFrames int `yaml:"replan_frames"`  // real-time plans refresh at least this often
}

func (c MonsterPhaseShiftConfig) validate() error {
	for _, field := range []struct {
		key   string
		value int
	}{
		{"detour_factor", c.DetourFactor},
		{"window_tiles", c.WindowTiles},
		{"max_walk_tiles", c.MaxWalkTiles},
		{"replan_frames", c.ReplanFrames},
	} {
		if field.value <= 0 {
			return fmt.Errorf("monster_ai.phase_shift.%s must be positive", field.key)
		}
	}
	return nil
}
