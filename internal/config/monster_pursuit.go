package config

import (
	"fmt"
	"math"
)

type MonsterPursuitConfig struct {
	MaxRadiusTiles float64 `yaml:"max_radius_tiles"`
	// Timer exception: the retaliation leash is an authored RT/TB pair, not
	// the 1 round = 3 s conversion.
	RetaliationSeconds     float64 `yaml:"retaliation_seconds"`
	RetaliationTurns       int     `yaml:"retaliation_turns"`
	RetaliationMarginTiles float64 `yaml:"retaliation_margin_tiles"`
}

func (c MonsterPursuitConfig) validate() error {
	for _, field := range []struct {
		key   string
		value float64
	}{
		{"max_radius_tiles", c.MaxRadiusTiles},
		{"retaliation_seconds", c.RetaliationSeconds},
		{"retaliation_turns", float64(c.RetaliationTurns)},
		{"retaliation_margin_tiles", c.RetaliationMarginTiles},
	} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value <= 0 {
			return fmt.Errorf("monster_ai.pursuit.%s must be finite and positive", field.key)
		}
	}
	return nil
}
