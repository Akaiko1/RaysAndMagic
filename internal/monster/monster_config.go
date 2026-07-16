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
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type,omitempty"` // creature category, e.g. "undead" (empty = generic, for now)
	Level        int      `yaml:"level"`
	MaxHitPoints int      `yaml:"max_hit_points"`
	ArmorClass   int      `yaml:"armor_class"`
	PerfectDodge int      `yaml:"perfect_dodge"` // Chance (0-100) to completely avoid an attack
	Experience   int      `yaml:"experience"`
	DamageMin    int      `yaml:"damage_min"`
	DamageMax    int      `yaml:"damage_max"`
	TrueDamage   int      `yaml:"true_damage,omitempty"` // added per attack, bypasses ALL mitigation (armor/resist/flat/dodge)
	AlertRadius  float64  `yaml:"alert_radius"`
	AttackRadius float64  `yaml:"attack_radius"`
	Speed        float64  `yaml:"speed"`
	GoldMin      int      `yaml:"gold_min"`
	GoldMax      int      `yaml:"gold_max"`
	Sprite       string   `yaml:"sprite"`
	Letter       string   `yaml:"letter"`
	Biomes       []string `yaml:"biomes,omitempty"`
	BoxW         float64  `yaml:"box_w"`
	BoxH         float64  `yaml:"box_h"`
	// SizeClass picks a quantized sprite height (small/medium/person/large/huge);
	// the tile-height value per class lives in config graphics.monster_size_classes.
	SizeClass string `yaml:"size_class"`
	// Champion, when set, names a champions.yaml build (a real character on the
	// monster AI). game.mirrorChampionStats mirrors that character's weapon
	// damage, attack cadence, HP and armor onto this monster at spawn.
	Champion string `yaml:"champion,omitempty"`
	// size_game and size_multiplier are retired. The fields exist only so content
	// still authoring them FAILS LOUD in validation instead of silently rendering
	// at the wrong scale.
	DeprecatedSizeGame       float64           `yaml:"size_game,omitempty"`
	DeprecatedSizeMultiplier float64           `yaml:"size_multiplier,omitempty"`
	Resistances              map[string]int    `yaml:"resistances"`
	HabitatPrefs             []string          `yaml:"habitat_preferences"`
	HabitatNear              []HabitatNearRule `yaml:"habitat_near"`
	ProjectileSpell          string            `yaml:"projectile_spell"`
	ProjectileWeapon         string            `yaml:"projectile_weapon"`
	Flying                   bool              `yaml:"flying"`
	RangedAttackRange        float64           `yaml:"ranged_attack_range"`
	AttacksPerRound          int               `yaml:"attacks_per_round"`
	AttackCooldownMult       float64           `yaml:"attack_cooldown_multiplier"`
	PassiveUntilHit          bool              `yaml:"passive_until_attacked"`
	FireburstChance          float64           `yaml:"fireburst_chance"`
	FireburstDamageMin       int               `yaml:"fireburst_damage_min"`
	FireburstDamageMax       int               `yaml:"fireburst_damage_max"`
	DragonBreathChance       float64           `yaml:"dragon_breath_chance,omitempty"`
	DragonBreathType         string            `yaml:"dragon_breath_damage_type,omitempty"`
	PiercingShotChance       float64           `yaml:"piercing_shot_chance,omitempty"`
	PiercingShotTargets      int               `yaml:"piercing_shot_targets,omitempty"`
	AllyHealChance           float64           `yaml:"ally_heal_chance,omitempty"`
	AllyHealAmount           int               `yaml:"ally_heal_amount,omitempty"`
	AllyHealRadius           float64           `yaml:"ally_heal_radius_tiles,omitempty"`
	PoisonChance             float64           `yaml:"poison_chance"`
	PoisonDurationSec        int               `yaml:"poison_duration_seconds"`
	IgniteChance             float64           `yaml:"ignite_chance,omitempty"`
	IgniteDurationSec        int               `yaml:"ignite_duration_seconds,omitempty"`
	StunCharChance           float64           `yaml:"stun_char_chance,omitempty"`
	StunCharSeconds          int               `yaml:"stun_char_seconds,omitempty"`
	StunCharTurns            int               `yaml:"stun_char_turns,omitempty"`
	DispelChance             float64           `yaml:"dispel_chance,omitempty"`
	// PounceRangeTiles > 0 gives the monster a leap: from within this range
	// (but beyond melee) it closes to melee instantly and attacks. Cooldown
	// (real-time only) throttles repeats.
	PounceRangeTiles      float64             `yaml:"pounce_range_tiles"`
	PounceCooldownSeconds float64             `yaml:"pounce_cooldown_seconds"`
	Light                 *MonsterLightConfig `yaml:"light,omitempty"`
	// Boss behaviour knobs (data-driven; see the Golden Thief Bug). All optional.
	IgnoresArmor      bool    `yaml:"ignores_armor,omitempty"`         // melee bypasses party armor class
	InfernoChance     float64 `yaml:"inferno_chance,omitempty"`        // 0..1 chance per action to cast a party-nova Inferno
	InfernoDamage     int     `yaml:"inferno_damage,omitempty"`        // fire damage of that nova, pre-mitigation (required with inferno_chance)
	TeleportAtHP      int     `yaml:"teleport_at_hp,omitempty"`        // when HP <= this, may blink to a random tile
	TeleportChance    float64 `yaml:"teleport_chance,omitempty"`       // 0..1 chance per action to blink (only below TeleportAtHP)
	PassiveUntilQuest string  `yaml:"passive_until_quest,omitempty"`   // while this quest is incomplete the boss does not attack: it evades (if evade_radius_tiles set) or just holds dormant; turns aggressive once complete
	EvadeRadiusTiles  float64 `yaml:"evade_radius_tiles,omitempty"`    // >0 = evasive boss: blink when the party is within this many tiles (needs boss_cooldown_seconds). Omit for a dormant boss that just holds.
	BossCooldownSecs  float64 `yaml:"boss_cooldown_seconds,omitempty"` // RT cadence between evasive blinks (required with evade_radius_tiles)
	// Summon: an aggressive boss rallies adds on its action.
	SummonChance          float64  `yaml:"summon_chance,omitempty"`           // 0..1 chance per action to summon (needs summon_monsters)
	SummonFirstGuaranteed bool     `yaml:"summon_first_guaranteed,omitempty"` // first successful summon ignores summon_chance; refill uses chance
	SummonMonsters        []string `yaml:"summon_monsters,omitempty"`         // monster keys to pick from
	SummonCount           int      `yaml:"summon_count,omitempty"`            // adds per summon (default 1)
	SummonMax             int      `yaml:"summon_max,omitempty"`              // cap on simultaneously-live summons (0 = uncapped)
	// Enrage: at/below enrage_at_hp the boss hits harder/faster (at least one mult).
	EnrageAtHP         int     `yaml:"enrage_at_hp,omitempty"`
	EnrageDamageMult   float64 `yaml:"enrage_damage_mult,omitempty"`
	EnrageCooldownMult float64 `yaml:"enrage_cooldown_mult,omitempty"`
	// Idol-ward (deep-jungle warlord): the boss is invulnerable + rooted (holds its
	// plaza) while any WarlordIdol monster lives; idols are immobile and never attack.
	WardedByIdols     bool    `yaml:"warded_by_idols,omitempty"` // boss: warded while any idol lives
	AggroWholeMap     bool    `yaml:"aggro_whole_map,omitempty"`
	RallyOnAggroTiles float64 `yaml:"rally_on_aggro_tiles,omitempty"` // alarm bell: on aggro, wake every monster within N tiles (once)    // boss: UNIQUE - once active, relentlessly chases from anywhere (else relentless only after normal aggro)
	DeathRalliesType  string  `yaml:"death_rallies_type,omitempty"`   // on this monster's death, every live map monster of this Type goes relentless (revenge)
	WarlordIdol       bool    `yaml:"warlord_idol,omitempty"`         // this monster is a ward idol
	// Banding: while calm, same-type banding mobs stack onto one tile (rendered as
	// a small fanned pile, centred) and patrol as a flock; on aggro/being hit they
	// scatter to a ring of nearby tiles. See [[project_monster_banding]].
	Banding bool `yaml:"banding,omitempty"`
	// Persistent sprite colour cast [r,g,b] (multipliers, ~0..1.5) - marks an elite
	// or variant apart from a base mob that shares its sprite.
	TintColor []float64 `yaml:"tint_color,omitempty"`
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
// ASCII map contract: lowercase a-z is reserved for monster spawns; tiles and
// props use uppercase letters (or a letterless [tile:] token). Monster letters
// may be reused by biome-scoped definitions; the map loader resolves
// biome-specific monsters before universal monsters.
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
	// Effect flags travel in pairs: a chance without its magnitude (or an evasive
	// phase without its trigger tuning) would silently fall back to zero in code.
	for key, monster := range config.Monsters {
		if monster.Letter != "" && (len(monster.Letter) != 1 || monster.Letter[0] < 'a' || monster.Letter[0] > 'z') {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has map letter %q - monster spawns must use one lowercase ASCII letter (a-z)", key, monster.Letter))
		}
		if monster.DeprecatedSizeGame != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed key size_game - use size_class instead", key))
		}
		if monster.DeprecatedSizeMultiplier != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed key size_multiplier - use size_class (small/medium/person/large/huge)", key))
		}
		if !ValidSizeClasses[monster.SizeClass] {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has invalid size_class %q - want one of small/medium/person/large/huge", key, monster.SizeClass))
		}
		if monster.InfernoChance > 0 && monster.InfernoDamage <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has inferno_chance but no inferno_damage", key))
		}
		if monster.PiercingShotChance > 0 && monster.PiercingShotTargets < 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has negative piercing_shot_targets", key))
		}
		if monster.AllyHealChance > 0 && monster.AllyHealAmount <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has ally_heal_chance but no ally_heal_amount", key))
		}
		if monster.TeleportChance > 0 && monster.TeleportAtHP <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has teleport_chance but no teleport_at_hp", key))
		}
		if monster.TeleportAtHP > 0 && monster.TeleportChance <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has teleport_at_hp but no teleport_chance", key))
		}
		if monster.PoisonChance > 0 && monster.PoisonDurationSec <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has poison_chance but no poison_duration_seconds", key))
		}
		if monster.IgniteChance > 0 && monster.IgniteDurationSec <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has ignite_chance but no ignite_duration_seconds", key))
		}
		if monster.StunCharChance > 0 && monster.StunCharSeconds <= 0 && monster.StunCharTurns <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has stun_char_chance but no stun_char_seconds/turns", key))
		}
		if monster.DragonBreathChance > 0 && strings.TrimSpace(monster.DragonBreathType) == "" {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has dragon_breath_chance but no dragon_breath_damage_type", key))
		}
		breathType := strings.ToLower(strings.TrimSpace(monster.DragonBreathType))
		if breathType != "" && config.DamageTypes[breathType] == 0 && breathType != DamageSchoolPhysical {
			if _, err := config.ConvertDamageType(breathType); err != nil {
				conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has unknown dragon_breath_damage_type %q", key, monster.DragonBreathType))
			}
		}
		// An evasive boss (blinks away while its quest is unfinished) needs a blink
		// cadence. A dormant boss (passive_until_quest with no evade_radius_tiles)
		// just holds until the quest completes, so it needs neither.
		if monster.EvadeRadiusTiles > 0 && monster.BossCooldownSecs <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has evade_radius_tiles but no boss_cooldown_seconds", key))
		}
		if monster.SummonChance > 0 && len(monster.SummonMonsters) == 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has summon_chance but no summon_monsters", key))
		}
		if monster.EnrageAtHP > 0 && monster.EnrageDamageMult <= 0 && monster.EnrageCooldownMult <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has enrage_at_hp but neither enrage_damage_mult nor enrage_cooldown_mult", key))
		}
		if len(monster.TintColor) != 0 && len(monster.TintColor) != 3 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' tint_color must be [r,g,b] (3 values), got %d", key, len(monster.TintColor)))
		}
	}
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
	damageTypeStr = strings.ToLower(strings.TrimSpace(damageTypeStr))
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
	// Size never changes after setup; cached so the per-tick callers (monster
	// separation runs GetSize per overlapping pair) skip the config scan.
	m.cachedSizeW, m.cachedSizeH = def.GetSizeFromConfig()
	m.cachedSizeMult = def.GetSizeGameMultiplier()
	m.Level = def.Level
	m.MaxHitPoints = def.MaxHitPoints
	m.ArmorClass = def.ArmorClass
	m.PerfectDodge = def.PerfectDodge
	m.Experience = def.Experience
	m.DamageMin = def.DamageMin
	m.DamageMax = def.DamageMax
	m.TrueDamage = def.TrueDamage
	// Convert tile-based radii to pixels
	tileSize := m.tileSize()
	m.AlertRadius = def.AlertRadius * tileSize
	m.AttackRadius = def.AttackRadius * tileSize
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
	m.HatesTraits = HatesTable[m.Key] // party traits that enrage this passive monster (hates.yaml)
	if def.RangedAttackRange > 0 {
		m.RangedAttackRange = def.RangedAttackRange * tileSize
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
	m.DragonBreathChance = def.DragonBreathChance
	m.DragonBreathDamageType = def.DragonBreathType
	m.PiercingShotChance = def.PiercingShotChance
	m.PiercingShotTargets = def.PiercingShotTargets
	m.AllyHealChance = def.AllyHealChance
	m.AllyHealAmount = def.AllyHealAmount
	if def.AllyHealRadius > 0 {
		m.AllyHealRadiusPixels = def.AllyHealRadius * tileSize
	}
	if def.PoisonChance > 0 {
		m.PoisonChance = def.PoisonChance
	}
	if def.PoisonDurationSec > 0 {
		m.PoisonDurationSec = def.PoisonDurationSec
	}
	m.IgniteChance = def.IgniteChance
	m.IgniteDurationSec = def.IgniteDurationSec
	m.StunCharChance = def.StunCharChance
	m.StunCharSeconds = def.StunCharSeconds
	m.StunCharTurns = def.StunCharTurns
	m.DispelChance = def.DispelChance
	if def.PounceRangeTiles > 0 {
		m.PounceRangePixels = def.PounceRangeTiles * tileSize
		m.PounceCooldownSeconds = def.PounceCooldownSeconds
	}
	m.IgnoresArmor = def.IgnoresArmor
	m.InfernoChance = def.InfernoChance
	m.InfernoDamage = def.InfernoDamage
	m.TeleportAtHP = def.TeleportAtHP
	m.TeleportChance = def.TeleportChance
	m.PassiveUntilQuest = def.PassiveUntilQuest
	m.EvadeRadiusTiles = def.EvadeRadiusTiles
	m.BossCooldownSecs = def.BossCooldownSecs
	m.SummonChance = def.SummonChance
	m.SummonFirstGuaranteed = def.SummonFirstGuaranteed
	m.SummonMonsters = def.SummonMonsters
	m.SummonCount = def.SummonCount
	m.SummonMax = def.SummonMax
	m.EnrageAtHP = def.EnrageAtHP
	m.EnrageDamageMult = def.EnrageDamageMult
	m.EnrageCooldownMult = def.EnrageCooldownMult
	m.WardedByIdols = def.WardedByIdols
	m.AggroWholeMap = def.AggroWholeMap
	m.RallyOnAggroTiles = def.RallyOnAggroTiles
	m.DeathRalliesType = def.DeathRalliesType
	m.WarlordIdol = def.WarlordIdol
	m.Banding = def.Banding
	if len(def.TintColor) == 3 {
		m.TintR = float32(def.TintColor[0])
		m.TintG = float32(def.TintColor[1])
		m.TintB = float32(def.TintColor[2])
	}

	m.LightRadius = 0
	m.LightIntensity = 0
	if def.Light != nil && def.Light.Enabled {
		m.LightRadius = def.Light.RadiusTiles * tileSize
		m.LightIntensity = def.Light.Intensity
	}

	// Champion mobs derive combat stats from a character build (game champion paths).
	m.ChampionKey = def.Champion
}

// GetSpriteFromConfig returns sprite type from config
func (def *MonsterDefinition) GetSpriteFromConfig() string {
	return def.Sprite
}

// GetSizeFromConfig returns collision box width and height from config
func (def *MonsterDefinition) GetSizeFromConfig() (width, height float64) {
	return def.BoxW, def.BoxH
}

// ValidSizeClasses is the fixed set of monster size-class names. The tile-height
// per class is data-driven (config graphics.monster_size_classes); the NAMES are
// the enum, so content validation can reject a typo without the config loaded.
var ValidSizeClasses = map[string]bool{"small": true, "medium": true, "person": true, "large": true, "huge": true}

// sizeClassHeights maps size class -> sprite height in tiles, wired from config
// at boot via SetSizeClassHeights (monster package must not import the loaded
// config instance, so the values are pushed in).
var sizeClassHeights = map[string]float64{}

// SetSizeClassHeights installs the class -> tile-height table (config
// graphics.size_classes). Call once after loading config. It is the single
// runtime source both monster AND NPC sizing resolve through (SizeClassTiles).
func SetSizeClassHeights(m map[string]float64) {
	sizeClassHeights = map[string]float64{}
	for k, v := range m {
		sizeClassHeights[k] = v
	}
}

// SizeClassTiles returns the sprite height (tiles) for a size class and whether
// it is defined. The one lookup shared by monster and NPC sizing, so the two
// never resolve the same class differently.
func SizeClassTiles(class string) (float64, bool) {
	h, ok := sizeClassHeights[class]
	return h, ok
}

// ValidateSizeClassHeights fails if the loaded height table omits any canonical
// class, so a config that drops/renames a class is caught at boot instead of
// silently shrinking those entities to the fallback height.
func ValidateSizeClassHeights() error {
	var missing []string
	for name := range ValidSizeClasses {
		if _, ok := sizeClassHeights[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("config graphics.size_classes is missing heights for: %s", strings.Join(missing, ", "))
	}
	return nil
}

// GetSizeGameMultiplier returns the sprite height in tile units for this
// monster's size class (1.0 == a 1-tile wall). Class validity is enforced at
// load; an unresolved class here falls back to person height.
func (def *MonsterDefinition) GetSizeGameMultiplier() float64 {
	if h, ok := sizeClassHeights[def.SizeClass]; ok {
		return h
	}
	if h, ok := sizeClassHeights["person"]; ok {
		return h
	}
	return 0.8
}
