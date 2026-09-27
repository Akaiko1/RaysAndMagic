package config

import "fmt"

type AutoStatsConfig struct {
	Speed     int    `yaml:"speed"`
	Endurance int    `yaml:"endurance"`
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary,omitempty"`
}
type ClassItemConfig struct {
	Key   string `yaml:"key"`
	Count int    `yaml:"count"`
}

func validateClassProgression(key string, c ClassStats) error {
	a := c.AutoStats
	allowed := map[string]bool{"might": true, "intellect": true, "personality": true, "endurance": true, "accuracy": true, "speed": true, "luck": true}
	if !allowed[a.Primary] || (a.Secondary != "" && (!allowed[a.Secondary] || a.Secondary == a.Primary)) || a.Speed < 1 || a.Speed > 99 || a.Endurance < 1 || a.Endurance > 99 {
		return fmt.Errorf("class %q: invalid auto_stats", key)
	}
	for _, item := range c.Items {
		if item.Key == "" || item.Count < 1 || item.Count > 999 {
			return fmt.Errorf("class %q: invalid starting item", key)
		}
	}
	return nil
}

// ValidateClassItems runs once after loading item definitions, before any hero
// is created. Class supply errors belong to content loading, not recruitment.
func ValidateClassItems(cfg *Config) error {
	for key, c := range cfg.Characters.Classes {
		for _, entry := range c.Items {
			d, ok := GetItemDefinition(entry.Key)
			if !ok || d == nil || d.Type != "consumable" || entry.Count < 1 || entry.Count > 999 {
				return fmt.Errorf("class %q: invalid starting consumable %q", key, entry.Key)
			}
		}
	}
	return nil
}
