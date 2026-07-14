package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// AchievementDef is one data-driven achievement definition. Unlock tracking is
// not wired yet - the entry-menu screen renders these as a graphics-ready,
// all-locked list. Add real unlock logic later without touching the schema.
type AchievementDef struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Icon is an optional sprite key (assets/sprites/<icon>.png). Empty falls
	// back to a procedural placeholder in the UI.
	Icon string `yaml:"icon,omitempty"`
}

// AchievementsConfig is the root of assets/achievements.yaml.
type AchievementsConfig struct {
	Achievements []AchievementDef `yaml:"achievements"`
}

// GlobalAchievements holds the loaded achievement definitions (nil if the file
// was absent/unreadable - the UI treats nil as "no achievements yet").
var GlobalAchievements *AchievementsConfig

// LoadAchievementConfig reads achievement definitions. Missing/empty config is
// not fatal - achievements are an optional, stubbed feature.
func LoadAchievementConfig(filename string) (*AchievementsConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg AchievementsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	GlobalAchievements = &cfg
	return &cfg, nil
}

// GetAchievements returns the loaded achievement definitions (nil-safe).
func GetAchievements() []AchievementDef {
	if GlobalAchievements == nil {
		return nil
	}
	return GlobalAchievements.Achievements
}
