package config

import "fmt"

// AutoStatsConfig is one AUTO build: Speed and Endurance targets, then the
// primary to 99 with the secondary to 50 alongside. Gear names the weapons and
// wear (weapons.yaml or items.yaml keys) the build is made for; Settings shows
// them and the boot check requires each to fit the class and the build.
type AutoStatsConfig struct {
	Speed     int      `yaml:"speed"`
	Endurance int      `yaml:"endurance"`
	Primary   string   `yaml:"primary"`
	Secondary string   `yaml:"secondary,omitempty"`
	Gear      []string `yaml:"gear"`
}

// AutoBuild is one build a class offers AUTO: its default (auto_stats) or an
// authored alternative (auto_builds).
type AutoBuild struct {
	Key             string `yaml:"key"`
	Name            string `yaml:"name"`
	AutoStatsConfig `yaml:",inline"`
}

// DefaultAutoBuildKey names a class's auto_stats among its builds.
const DefaultAutoBuildKey = "auto"

// MaxAutoBuildGear is how many example pieces a build shows.
const MaxAutoBuildGear = 6

// AutoBuildOptions lists the class default first, then its authored builds.
func (c ClassStats) AutoBuildOptions() []AutoBuild {
	return append([]AutoBuild{{Key: DefaultAutoBuildKey, Name: "Auto", AutoStatsConfig: c.AutoStats}}, c.AutoBuilds...)
}

func validAutoStats(a AutoStatsConfig) bool {
	return IsStatName(a.Primary) && (a.Secondary == "" || IsStatName(a.Secondary) && a.Secondary != a.Primary) &&
		a.Speed >= 1 && a.Speed <= 99 && a.Endurance >= 1 && a.Endurance <= 99 && len(a.Gear) >= 1 && len(a.Gear) <= MaxAutoBuildGear
}

type ClassItemConfig struct {
	Key   string `yaml:"key"`
	Count int    `yaml:"count"`
}

func validateClassProgression(key string, c ClassStats) error {
	if !validAutoStats(c.AutoStats) {
		return fmt.Errorf("class %q: invalid auto_stats", key)
	}
	seen := map[string]bool{DefaultAutoBuildKey: true}
	for _, b := range c.AutoBuilds {
		if b.Key == "" || seen[b.Key] || b.Name == "" || !validAutoStats(b.AutoStatsConfig) {
			return fmt.Errorf("class %q: invalid auto build %q", key, b.Key)
		}
		seen[b.Key] = true
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
