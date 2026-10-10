package config

import (
	"fmt"
	"math"
)

type MonsterPursuitConfig struct {
	MaxRadiusTiles         float64 `yaml:"max_radius_tiles"`
	RetaliationSeconds     float64 `yaml:"retaliation_seconds"`
	RetaliationTurns       int     `yaml:"retaliation_turns"`
	RetaliationMarginTiles float64 `yaml:"retaliation_margin_tiles"`
}

func (c MonsterPursuitConfig) validate() error {
	for key, value := range map[string]float64{
		"max_radius_tiles":         c.MaxRadiusTiles,
		"retaliation_seconds":      c.RetaliationSeconds,
		"retaliation_turns":        float64(c.RetaliationTurns),
		"retaliation_margin_tiles": c.RetaliationMarginTiles,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return fmt.Errorf("monster_ai.pursuit.%s must be finite and positive", key)
		}
	}
	return nil
}
