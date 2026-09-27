package config

import "fmt"

// StatusDamageConfig is target-side, so every burn source follows the same rule.
type StatusDamageConfig struct {
	BurnPercent     int `yaml:"burn_percent"`
	BossBurnPercent int `yaml:"boss_burn_percent"`
}

func BurnPercent(boss bool) int {
	if GlobalConfig == nil {
		panic("status_damage: config not loaded")
	}
	if boss {
		return GlobalConfig.StatusDamage.BossBurnPercent
	}
	return GlobalConfig.StatusDamage.BurnPercent
}

func (s StatusDamageConfig) validate() error {
	if s.BurnPercent < 1 || s.BurnPercent > 100 || s.BossBurnPercent < 1 || s.BossBurnPercent > 100 {
		return fmt.Errorf("status_damage: burn_percent and boss_burn_percent are required and must be 1..100")
	}
	return nil
}
