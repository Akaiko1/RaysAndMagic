package config

import "fmt"

type AutoDrinkConfig struct {
	ThresholdPct    int     `yaml:"threshold_pct"`
	IntervalSeconds float64 `yaml:"interval_seconds"`
}

func (c AutoDrinkConfig) Validate() error {
	if c.ThresholdPct < 0 || c.ThresholdPct > 100 || c.IntervalSeconds < 0 {
		return fmt.Errorf("invalid characters.auto_drink timing or threshold")
	}
	if c.ThresholdPct > 0 && c.IntervalSeconds <= 0 {
		return fmt.Errorf("automatic drinking requires a positive interval")
	}
	return nil
}

func AutoDrinkSettings() AutoDrinkConfig {
	if GlobalConfig == nil {
		return AutoDrinkConfig{}
	}
	return GlobalConfig.Characters.AutoDrink
}
