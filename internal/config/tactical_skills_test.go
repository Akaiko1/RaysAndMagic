package config

import "testing"

func TestTacticalSkillValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TacticalSkillsConfig)
		bad  bool
	}{
		{"disabled", func(*TacticalSkillsConfig) {}, false},
		{"enabled", func(c *TacticalSkillsConfig) { c.AutoDrinkThresholdPct = 35; c.AutoDrinkSeconds = 1 }, false},
		{"zero interval", func(c *TacticalSkillsConfig) { c.AutoDrinkThresholdPct = 35 }, true},
		{"chance", func(c *TacticalSkillsConfig) { c.OverwatchChance[3] = 101 }, true},
		{"negative range", func(c *TacticalSkillsConfig) { c.BallisticsRangeTiles[1] = -1 }, true},
		{"negative delay", func(c *TacticalSkillsConfig) { c.OverwatchReadySeconds = -1 }, true},
		{"poison", func(c *TacticalSkillsConfig) { c.MedicinePoisonReductionPct[0] = 101 }, true},
		{"mark crit", func(c *TacticalSkillsConfig) { c.DesignationCritPct[0] = -1 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c TacticalSkillsConfig
			tc.edit(&c)
			if (c.Validate() != nil) != tc.bad {
				t.Fatal("unexpected validation result")
			}
		})
	}
}
