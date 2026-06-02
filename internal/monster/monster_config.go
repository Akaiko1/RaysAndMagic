package monster

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MonsterDefinition holds the configuration for a monster type from YAML
type MonsterDefinition struct {
	Name               string            `yaml:"name"`
	Type               string            `yaml:"type,omitempty"` // creature category, e.g. "undead" (empty = generic, for now)
	Level              int               `yaml:"level"`
	MaxHitPoints       int               `yaml:"max_hit_points"`
	ArmorClass         int               `yaml:"armor_class"`
	PerfectDodge       int               `yaml:"perfect_dodge"` // Chance (0-100) to completely avoid an attack
	Experience         int               `yaml:"experience"`
	DamageMin          int               `yaml:"damage_min"`
	DamageMax          int               `yaml:"damage_max"`
	AlertRadius        float64           `yaml:"alert_radius"`
	AttackRadius       float64           `yaml:"attack_radius"`
	Speed              float64           `yaml:"speed"`
	GoldMin            int               `yaml:"gold_min"`
	GoldMax            int               `yaml:"gold_max"`
	Sprite             string            `yaml:"sprite"`
	Letter             string            `yaml:"letter"`
	Biomes             []string          `yaml:"biomes,omitempty"`
	BoxW               float64           `yaml:"box_w"`
	BoxH               float64           `yaml:"box_h"`
	SizeGame           float64           `yaml:"size_game"`
	Resistances        map[string]int    `yaml:"resistances"`
	HabitatPrefs       []string          `yaml:"habitat_preferences"`
	HabitatNear        []HabitatNearRule `yaml:"habitat_near"`
	ProjectileSpell    string            `yaml:"projectile_spell"`
	ProjectileWeapon   string            `yaml:"projectile_weapon"`
	Flying             bool              `yaml:"flying"`
	RangedAttackRange  float64           `yaml:"ranged_attack_range"`
	AttacksPerRound    int               `yaml:"attacks_per_round"`
	AttackCooldownMult float64           `yaml:"attack_cooldown_multiplier"`
	PassiveUntilHit    bool              `yaml:"passive_until_attacked"`
	FireburstChance    float64           `yaml:"fireburst_chance"`
	FireburstDamageMin int               `yaml:"fireburst_damage_min"`
	FireburstDamageMax int               `yaml:"fireburst_damage_max"`
	PoisonChance       float64           `yaml:"poison_chance"`
	PoisonDurationSec  int               `yaml:"poison_duration_seconds"`
	// PounceRangeTiles > 0 gives the monster a leap: from within this range
	// (but beyond melee) it closes to melee instantly and attacks. Cooldown
	// (real-time only) throttles repeats.
	PounceRangeTiles      float64             `yaml:"pounce_range_tiles"`
	PounceCooldownSeconds float64             `yaml:"pounce_cooldown_seconds"`
	Light                 *MonsterLightConfig `yaml:"light,omitempty"`
}

// HabitatNearRule defines a rule for placing monsters near certain tile types
type HabitatNearRule struct {
	Type   string `yaml:"type"`
	Radius int    `yaml:"radius"`
}

type MonsterLightConfig struct {
	Enabled     bool    `yaml:"enabled"`
	RadiusTiles float64 `yaml:"radius_tiles"`
	Intensity   float64 `yaml:"intensity"`
}

// MonsterYAMLConfig holds the complete monster configuration from YAML
type MonsterYAMLConfig struct {
	Monsters    map[string]MonsterDefinition `yaml:"monsters"`
	DamageTypes map[string]int               `yaml:"damage_types"`
	TileTypes   map[string]int               `yaml:"tile_types"`
}

// Global monster configuration
var MonsterConfig *MonsterYAMLConfig

// validateMonsterConfiguration checks for conflicts in monster letters.
// Monster letters are allowed to be reused by biome-scoped definitions; the map
// loader resolves biome-specific monsters before universal monsters.
func validateMonsterConfiguration(config *MonsterYAMLConfig) error {
	universalLetters := make(map[string][]string)
	biomeLetters := make(map[string]map[string][]string)

	for key, monster := range config.Monsters {
		letter := monster.Letter
		if letter == "" {
			continue
		}
		if len(monster.Biomes) == 0 {
			universalLetters[letter] = append(universalLetters[letter], key)
			continue
		}
		if biomeLetters[letter] == nil {
			biomeLetters[letter] = make(map[string][]string)
		}
		for _, biome := range monster.Biomes {
			biomeLetters[letter][biome] = append(biomeLetters[letter][biome], key)
		}
	}

	var conflicts []string
	for letter, monsterKeys := range universalLetters {
		if len(monsterKeys) > 1 {
			sort.Strings(monsterKeys)
			conflicts = append(conflicts, fmt.Sprintf("Letter '%s' is used by multiple monsters: %v", letter, monsterKeys))
		}
	}
	for letter, byBiome := range biomeLetters {
		for biome, monsterKeys := range byBiome {
			if len(monsterKeys) > 1 {
				sort.Strings(monsterKeys)
				conflicts = append(conflicts, fmt.Sprintf("Letter '%s' is used by multiple monsters in biome '%s': %v", letter, biome, monsterKeys))
			}
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
	return c.GetMonsterByLetterForBiome(letter, "")
}

// GetMonsterByLetterForBiome resolves a monster spawn marker for a map biome.
// Biome-specific definitions win over universal fallback definitions.
func (c *MonsterYAMLConfig) GetMonsterByLetterForBiome(letter string, biome string) (*MonsterDefinition, string, error) {
	if biome != "" {
		for key, monster := range c.Monsters {
			if monster.Letter == letter && monsterSupportsBiome(monster.Biomes, biome) {
				return &monster, key, nil
			}
		}
	}
	for key, monster := range c.Monsters {
		if monster.Letter == letter && len(monster.Biomes) == 0 {
			return &monster, key, nil
		}
	}
	return nil, "", fmt.Errorf("monster with letter '%s' not found", letter)
}

func monsterSupportsBiome(biomes []string, biome string) bool {
	for _, supported := range biomes {
		if supported == biome {
			return true
		}
	}
	return false
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
	m.MonsterType = def.Type
	m.Level = def.Level
	m.MaxHitPoints = def.MaxHitPoints
	m.ArmorClass = def.ArmorClass
	m.PerfectDodge = def.PerfectDodge
	m.Experience = def.Experience
	m.DamageMin = def.DamageMin
	m.DamageMax = def.DamageMax
	// Convert tile-based radii to pixels (1 tile = 64 pixels)
	m.AlertRadius = def.AlertRadius * 64.0
	m.AttackRadius = def.AttackRadius * 64.0
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

	// Set habitat preferences - tiles this monster can walk on even if normally blocked
	m.HabitatPrefs = def.HabitatPrefs

	// Set ranged attack configuration
	m.ProjectileSpell = def.ProjectileSpell
	m.ProjectileWeapon = def.ProjectileWeapon
	m.Flying = def.Flying
	m.AttacksPerRound = def.AttacksPerRound
	m.AttackCooldownMultiplier = def.AttackCooldownMult
	m.PassiveUntilAttacked = def.PassiveUntilHit
	if def.RangedAttackRange > 0 {
		m.RangedAttackRange = def.RangedAttackRange * 64.0
	}
	if def.FireburstChance > 0 {
		m.FireburstChance = def.FireburstChance
	}
	if def.FireburstDamageMin > 0 {
		m.FireburstDamageMin = def.FireburstDamageMin
	}
	if def.FireburstDamageMax > 0 {
		m.FireburstDamageMax = def.FireburstDamageMax
	}
	if def.PoisonChance > 0 {
		m.PoisonChance = def.PoisonChance
	}
	if def.PoisonDurationSec > 0 {
		m.PoisonDurationSec = def.PoisonDurationSec
	}
	if def.PounceRangeTiles > 0 {
		m.PounceRangePixels = def.PounceRangeTiles * 64.0
		m.PounceCooldownSeconds = def.PounceCooldownSeconds
	}

	m.LightRadius = 0
	m.LightIntensity = 0
	if def.Light != nil && def.Light.Enabled {
		m.LightRadius = def.Light.RadiusTiles * 64.0
		m.LightIntensity = def.Light.Intensity
	}
}

// GetSpriteFromConfig returns sprite type from config
func (def *MonsterDefinition) GetSpriteFromConfig() string {
	return def.Sprite
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
