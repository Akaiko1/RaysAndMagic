package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// ChampionTier is one rung of the shared arena difficulty ladder: the character
// level and skill mastery the build is assembled at, the boss HP pool, and the
// victory rewards.
type ChampionTier struct {
	Level       int    `yaml:"level"`
	Mastery     string `yaml:"mastery"` // novice/expert/master/grandmaster - applied to EVERY authored skill
	HP          int    `yaml:"hp"`
	Experience  int    `yaml:"experience"`
	ArenaPoints int    `yaml:"arena_points"`
}

// ChampionDefinition is one champions.yaml build: a real character (class,
// skills, per-tier equipment) that rides the monster AI. The character is
// constructed per tier via character.BuildChampion, leveled with the party's
// own auto stat distribution, and drives the mob's combat numbers
// (game.mirrorChampionStats + the live damage path).
type ChampionDefinition struct {
	Name      string              `yaml:"name"`
	Class     string              `yaml:"class"` // config.yaml characters.classes key
	Race      string              `yaml:"race,omitempty"`
	Skills    []string            `yaml:"skills"`           // skill keys; mastery comes from the tier
	Equipment map[string][]string `yaml:"equipment"`        // tier -> weapon/item keys; first weapon is main hand
	Ranged    bool                `yaml:"ranged,omitempty"` // fires its main-hand weapon as a projectile
}

// ChampionSystemConfig is the full champions.yaml document.
type ChampionSystemConfig struct {
	Tiers     map[string]*ChampionTier       `yaml:"tiers"`
	Champions map[string]*ChampionDefinition `yaml:"champions"`
}

// ChampionDefaultTier is the tier assumed when a champion mob carries no
// explicit tier (pre-placed map spawns, older saves).
const ChampionDefaultTier = "impossible"

// GlobalChampionConfig is loaded once at startup (LoadChampionConfig).
var GlobalChampionConfig *ChampionSystemConfig

// GetChampionDefinition returns the build for key, or nil if unknown.
func GetChampionDefinition(key string) *ChampionDefinition {
	if GlobalChampionConfig == nil {
		return nil
	}
	return GlobalChampionConfig.Champions[key]
}

// GetChampionTier is a PURE lookup: nil for an unknown name. Defaulting for
// tierless mobs is championTierOf's job (the one owner) - a silent fallback
// here would let a typo'd dialogue tier: duel at the default difficulty.
func GetChampionTier(name string) *ChampionTier {
	if GlobalChampionConfig == nil {
		return nil
	}
	return GlobalChampionConfig.Tiers[name]
}

// ChampionKeys returns every champion key (the duel registry to roll from),
// sorted - the accessor owns determinism, like WeaponKeysByRarity.
func ChampionKeys() []string {
	if GlobalChampionConfig == nil {
		return nil
	}
	keys := make([]string, 0, len(GlobalChampionConfig.Champions))
	for k := range GlobalChampionConfig.Champions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// LoadChampionConfig reads and structurally validates assets/champions.yaml.
// Deeper validation (class/skill/item keys, buildability, equip gates) happens
// when the characters are built at startup - it needs the class, weapon and
// item catalogs.
func LoadChampionConfig(filename string) (*ChampionSystemConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read champion config: %w", err)
	}
	var cfg ChampionSystemConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse champion config: %w", err)
	}
	if err := validateChampionConfig(&cfg); err != nil {
		return nil, err
	}
	GlobalChampionConfig = &cfg
	return &cfg, nil
}

// MustLoadChampionConfig follows the Must* startup convention.
func MustLoadChampionConfig(filename string) *ChampionSystemConfig {
	cfg, err := LoadChampionConfig(filename)
	if err != nil {
		panic(err)
	}
	return cfg
}

func validateChampionConfig(cfg *ChampionSystemConfig) error {
	if len(cfg.Champions) == 0 {
		return fmt.Errorf("champion config defines no champions")
	}
	if len(cfg.Tiers) == 0 {
		return fmt.Errorf("champion config defines no tiers")
	}
	if cfg.Tiers[ChampionDefaultTier] == nil {
		return fmt.Errorf("champion tiers must include %q (the default for tierless spawns)", ChampionDefaultTier)
	}
	for name, t := range cfg.Tiers {
		if t == nil || t.Level <= 0 || t.Mastery == "" || t.HP <= 0 {
			return fmt.Errorf("champion tier %q: level, mastery and hp are required", name)
		}
		if t.Experience <= 0 || t.ArenaPoints <= 0 {
			return fmt.Errorf("champion tier %q: experience and arena_points must be positive", name)
		}
	}
	for key, c := range cfg.Champions {
		if c == nil || c.Name == "" {
			return fmt.Errorf("champion %q: missing name", key)
		}
		if c.Class == "" {
			return fmt.Errorf("champion %q: missing class", key)
		}
		if len(c.Skills) == 0 {
			return fmt.Errorf("champion %q: no skills", key)
		}
		for tierName := range cfg.Tiers {
			equipment := c.Equipment[tierName]
			if len(equipment) == 0 {
				return fmt.Errorf("champion %q: no equipment for tier %q", key, tierName)
			}
			// Every tier MUST wield a real weapon: the first weapon key becomes
			// the main hand and every combat number derives from it.
			var mainHand *WeaponDefinitionConfig
			for _, ek := range equipment {
				if wd, ok := GetWeaponDefinition(ek); ok && wd != nil {
					mainHand = wd
					break
				}
			}
			if mainHand == nil {
				return fmt.Errorf("champion %q tier %q: equipment lists no weapon", key, tierName)
			}
			if c.Ranged && mainHand.Physics == nil {
				return fmt.Errorf("champion %q tier %q: ranged but main-hand weapon %q has no projectile physics", key, tierName, mainHand.Name)
			}
		}
	}
	return nil
}
