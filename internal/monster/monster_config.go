package monster

import (
	"fmt"
	"math/rand"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// MonsterDefinition holds the configuration for a monster type from YAML
type MonsterDefinition struct {
	Name         string            `yaml:"name"`
	Level        int               `yaml:"level"`
	MaxHitPoints int               `yaml:"max_hit_points"`
	ArmorClass   int               `yaml:"armor_class"`
	Experience   int               `yaml:"experience"`
	AttackBonus  int               `yaml:"attack_bonus"`
	DamageMin    int               `yaml:"damage_min"`
	DamageMax    int               `yaml:"damage_max"`
	AlertRadius  float64           `yaml:"alert_radius"`
	AttackRadius float64           `yaml:"attack_radius"`
	Speed        float64           `yaml:"speed"`
	GoldMin      int               `yaml:"gold_min"`
	GoldMax      int               `yaml:"gold_max"`
	Sprite       string            `yaml:"sprite"`
	Letter       string            `yaml:"letter"`
	BoxW         float64           `yaml:"box_w"`
	BoxH         float64           `yaml:"box_h"`
	SizeGame     float64           `yaml:"size_game"`
	Resistances  map[string]int    `yaml:"resistances"`
	HabitatPrefs []string          `yaml:"habitat_preferences"`
	HabitatNear  []HabitatNearRule `yaml:"habitat_near"`
}

// HabitatNearRule defines a rule for placing monsters near certain tile types
type HabitatNearRule struct {
	Type   string `yaml:"type"`
	Radius int    `yaml:"radius"`
}

// MonsterPlacementConfig holds monster placement configuration
type MonsterPlacementConfig struct {
	Common  PlacementRules `yaml:"common"`
	Special SpecialRules   `yaml:"special"`
}

type PlacementRules struct {
	CountMin int `yaml:"count_min"`
	CountMax int `yaml:"count_max"`
}

type SpecialRules struct {
	TreantChance  float64 `yaml:"treant_chance"`
	PixieCountMax int     `yaml:"pixie_count_max"`
	DragonChance  float64 `yaml:"dragon_chance"`
	TrollChance   float64 `yaml:"troll_chance"`
}

// MonsterYAMLConfig holds the complete monster configuration from YAML
type MonsterYAMLConfig struct {
	Monsters    map[string]MonsterDefinition `yaml:"monsters"`
	Placement   MonsterPlacementConfig       `yaml:"placement"`
	DamageTypes map[string]int               `yaml:"damage_types"`
	TileTypes   map[string]int               `yaml:"tile_types"`
}

// Global monster configuration
var MonsterConfig *MonsterYAMLConfig

// validateMonsterConfiguration checks for conflicts in monster letters
func validateMonsterConfiguration(config *MonsterYAMLConfig) error {
	letterToMonsters := make(map[string][]string)
	
	// Group monsters by their letters
	for key, monster := range config.Monsters {
		letter := monster.Letter
		if letter == "" {
			continue // Skip monsters without letters (might be special spawns)
		}
		letterToMonsters[letter] = append(letterToMonsters[letter], key)
	}
	
	// Check for conflicts
	var conflicts []string
	for letter, monsterKeys := range letterToMonsters {
		if len(monsterKeys) > 1 {
			conflicts = append(conflicts, fmt.Sprintf("Letter '%s' is used by multiple monsters: %v", letter, monsterKeys))
		}
	}
	
	if len(conflicts) > 0 {
		return fmt.Errorf("monster configuration conflicts detected:\n%s", strings.Join(conflicts, "\n"))
	}
	
	return nil
}

// LoadMonsterConfig loads monster configuration from YAML file
func LoadMonsterConfig(filename string) (*MonsterYAMLConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read monster config file: %w", err)
	}

	var config MonsterYAMLConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse monster config YAML: %w", err)
	}

	// Validate configuration for conflicts
	if err := validateMonsterConfiguration(&config); err != nil {
		return nil, err
	}

	// Set global config for easy access
	MonsterConfig = &config

	return &config, nil
}

// MustLoadMonsterConfig loads monster configuration and panics on error
func MustLoadMonsterConfig(filename string) *MonsterYAMLConfig {
	config, err := LoadMonsterConfig(filename)
	if err != nil {
		panic("Failed to load monster config: " + err.Error())
	}
	return config
}

// GetMonsterByKey returns monster definition by key
func (c *MonsterYAMLConfig) GetMonsterByKey(key string) (*MonsterDefinition, error) {
	monster, exists := c.Monsters[key]
	if !exists {
		return nil, fmt.Errorf("monster with key '%s' not found", key)
	}
	return &monster, nil
}

// GetMonsterByLetter returns monster definition by letter marker
func (c *MonsterYAMLConfig) GetMonsterByLetter(letter string) (*MonsterDefinition, string, error) {
	for key, monster := range c.Monsters {
		if monster.Letter == letter {
			return &monster, key, nil
		}
	}
	return nil, "", fmt.Errorf("monster with letter '%s' not found", letter)
}

// GetAllMonsterKeys returns all monster keys
func (c *MonsterYAMLConfig) GetAllMonsterKeys() []string {
	keys := make([]string, 0, len(c.Monsters))
	for key := range c.Monsters {
		keys = append(keys, key)
	}
	return keys
}

// ConvertDamageType converts string damage type to DamageType enum
func (c *MonsterYAMLConfig) ConvertDamageType(damageTypeStr string) (DamageType, error) {
	if typeInt, exists := c.DamageTypes[damageTypeStr]; exists {
		return DamageType(typeInt), nil
	}
	return DamagePhysical, fmt.Errorf("unknown damage type: %s", damageTypeStr)
}

// ConvertTileType converts string tile type to integer
func (c *MonsterYAMLConfig) ConvertTileType(tileTypeStr string) (int, error) {
	if typeInt, exists := c.TileTypes[tileTypeStr]; exists {
		return typeInt, nil
	}
	return 0, fmt.Errorf("unknown tile type: %s", tileTypeStr)
}

// SetupMonsterFromConfig configures a monster from YAML definition
func (m *Monster3D) SetupMonsterFromConfig(def *MonsterDefinition) {
	m.Name = def.Name
	m.Level = def.Level
	m.MaxHitPoints = def.MaxHitPoints
	m.ArmorClass = def.ArmorClass
	m.Experience = def.Experience
	m.AttackBonus = def.AttackBonus
	m.DamageMin = def.DamageMin
	m.DamageMax = def.DamageMax
	m.AlertRadius = def.AlertRadius
	m.AttackRadius = def.AttackRadius
	m.Speed = def.Speed

	// Set random gold within range
	if def.GoldMax > def.GoldMin {
		m.Gold = def.GoldMin + rand.Intn(def.GoldMax-def.GoldMin+1)
	} else {
		m.Gold = def.GoldMin
	}

	// Set resistances
	if MonsterConfig != nil {
		for damageTypeStr, resistance := range def.Resistances {
			if damageType, err := MonsterConfig.ConvertDamageType(damageTypeStr); err == nil {
				m.Resistances[damageType] = resistance
			}
		}
	}
}

// GetSpriteFromConfig returns sprite type from config
func (def *MonsterDefinition) GetSpriteFromConfig() string {
	return def.Sprite
}

// GetRandomMonsterKey returns a random monster key from the configuration
func (c *MonsterYAMLConfig) GetRandomMonsterKey() string {
	keys := c.GetAllMonsterKeys()
	if len(keys) == 0 {
		return "goblin" // fallback
	}
	return keys[rand.Intn(len(keys))]
}

// GetSizeFromConfig returns collision box width and height from config
func (def *MonsterDefinition) GetSizeFromConfig() (width, height float64) {
	return def.BoxW, def.BoxH
}

// GetSizeGameMultiplier returns the visual size multiplier from config
func (def *MonsterDefinition) GetSizeGameMultiplier() float64 {
	if def.SizeGame == 0 {
		return 1.0 // Default multiplier if not set
	}
	return def.SizeGame
}
