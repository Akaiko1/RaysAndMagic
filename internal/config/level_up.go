package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LevelUpChoice defines a single selectable option for a level-up choice.
// Type can be: "spell", "weapon_mastery", "armor_mastery", "magic_mastery".
type LevelUpChoice struct {
	Type   string `yaml:"type"`
	Skill  string `yaml:"skill,omitempty"`  // weapon/armor skill key (e.g., "sword", "plate")
	Spell  string `yaml:"spell,omitempty"`  // spell id (e.g., "fireball")
	School string `yaml:"school,omitempty"` // magic school key (e.g., "fire", "any")
}

// LevelUpLevel defines the choices available at a specific level.
type LevelUpLevel struct {
	Level   int             `yaml:"level"`
	Choices []LevelUpChoice `yaml:"choices"`
}

// LevelUpClassConfig defines level-up choices for a class.
type LevelUpClassConfig struct {
	Levels []LevelUpLevel `yaml:"levels"`
}

// LevelUpConfig is the root config for level-up choices.
type LevelUpConfig struct {
	LevelUps map[string]LevelUpClassConfig `yaml:"level_ups"`
}

var levelUpConfig *LevelUpConfig

// LoadLevelUpConfig loads level-up choices from YAML.
func LoadLevelUpConfig(filename string) (*LevelUpConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read level up config: %w", err)
	}

	var cfg LevelUpConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse level up config: %w", err)
	}
	if err := validateLevelUpConfig(&cfg); err != nil {
		return nil, err
	}
	levelUpConfig = &cfg
	return &cfg, nil
}

func validateLevelUpConfig(cfg *LevelUpConfig) error {
	for classKey, classCfg := range cfg.LevelUps {
		for levelIndex := range classCfg.Levels {
			level := &classCfg.Levels[levelIndex]
			for choiceIndex := range level.Choices {
				choice := &level.Choices[choiceIndex]
				rawSchool := strings.TrimSpace(choice.School)
				if rawSchool == "" {
					continue
				}
				if strings.EqualFold(rawSchool, "any") {
					choice.School = "any"
					continue
				}
				school, err := canonicalMagicSchool(rawSchool)
				if err != nil {
					return fmt.Errorf(
						"level_up %q level %d choice %d: unsupported school %q",
						classKey,
						level.Level,
						choiceIndex,
						choice.School,
					)
				}
				choice.School = school
			}
		}
		cfg.LevelUps[classKey] = classCfg
	}
	return nil
}

// MustLoadLevelUpConfig loads level-up config or panics.
func MustLoadLevelUpConfig(filename string) *LevelUpConfig {
	cfg, err := LoadLevelUpConfig(filename)
	if err != nil {
		panic(err)
	}
	return cfg
}

// GetLevelUpChoices returns the choices for a class at a given level.
func GetLevelUpChoices(classKey string, level int) []LevelUpChoice {
	if levelUpConfig == nil {
		return nil
	}
	classCfg, ok := levelUpConfig.LevelUps[classKey]
	if !ok {
		return nil
	}
	for _, lvl := range classCfg.Levels {
		if lvl.Level == level {
			return lvl.Choices
		}
	}
	return nil
}
