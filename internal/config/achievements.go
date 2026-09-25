package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AchievementDef declares presentation and lifetime counter unlock rules.
type AchievementDef struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Icon is an optional sprite key (assets/sprites/<icon>.png). Empty falls
	// back to a procedural placeholder in the UI.
	Icon   string   `yaml:"icon,omitempty"`
	AnyOf  []string `yaml:"any_of"`
	Target int64    `yaml:"target"`
}

// AchievementsConfig is the root of assets/achievements.yaml.
type AchievementsConfig struct {
	Achievements []AchievementDef `yaml:"achievements"`
}

// GlobalAchievements holds the loaded achievement definitions (nil if the file
// was absent/unreadable).
var GlobalAchievements *AchievementsConfig

// LoadAchievementConfig reads achievement definitions. Missing/empty config is
// not fatal; a malformed rule is rejected before replacing the live catalog.
func LoadAchievementConfig(filename string) (*AchievementsConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg AchievementsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, def := range cfg.Achievements {
		if def.Key == "" || seen[def.Key] || len(def.AnyOf) == 0 || def.Target <= 0 {
			return nil, fmt.Errorf("invalid achievement rule: %q", def.Key)
		}
		seen[def.Key] = true
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
