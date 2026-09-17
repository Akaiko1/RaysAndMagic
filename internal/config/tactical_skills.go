package config

import "fmt"

// Mastery tables are authored Novice, Expert, Master, Grandmaster.
type TacticalSkillsConfig struct {
	OverwatchChance            [4]int            `yaml:"overwatch_chance"`
	OverwatchReadySeconds      float64           `yaml:"overwatch_ready_seconds"`
	BallisticsSpeedPct         [4]int            `yaml:"ballistics_speed_pct"`
	BallisticsRangeTiles       [4]int            `yaml:"ballistics_range_tiles"`
	MedicineRestorePct         [4]int            `yaml:"medicine_restore_pct"`
	MedicinePoisonReductionPct [4]int            `yaml:"medicine_poison_reduction_pct"`
	DesignationCritPct         [4]int            `yaml:"designation_crit_pct"`
	DesignationSeconds         [4]int            `yaml:"designation_seconds"`
	AutoDrinkThresholdPct      int               `yaml:"auto_drink_threshold_pct"`
	AutoDrinkSeconds           float64           `yaml:"auto_drink_seconds"`
	Descriptions               map[string]string `yaml:"descriptions"`
}

func (c TacticalSkillsConfig) Validate() error {
	for name, values := range map[string][4]int{"overwatch_chance": c.OverwatchChance, "medicine_poison_reduction_pct": c.MedicinePoisonReductionPct, "designation_crit_pct": c.DesignationCritPct} {
		for _, value := range values {
			if value < 0 || value > 100 {
				return fmt.Errorf("characters.tactics.%s must be between 0 and 100", name)
			}
		}
	}
	for name, values := range map[string][4]int{"ballistics_speed_pct": c.BallisticsSpeedPct, "ballistics_range_tiles": c.BallisticsRangeTiles, "medicine_restore_pct": c.MedicineRestorePct, "designation_seconds": c.DesignationSeconds} {
		for _, value := range values {
			if value < 0 {
				return fmt.Errorf("characters.tactics.%s must be nonnegative", name)
			}
		}
	}
	if c.OverwatchReadySeconds < 0 || c.AutoDrinkSeconds < 0 || c.AutoDrinkThresholdPct < 0 || c.AutoDrinkThresholdPct > 100 {
		return fmt.Errorf("invalid characters.tactics timing or threshold")
	}
	if c.AutoDrinkThresholdPct > 0 && c.AutoDrinkSeconds <= 0 {
		return fmt.Errorf("auto drinking requires a positive interval")
	}
	return nil
}

func TacticalSkills() TacticalSkillsConfig {
	if GlobalConfig == nil {
		return TacticalSkillsConfig{}
	}
	return GlobalConfig.Characters.Tactics
}
