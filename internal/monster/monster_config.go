package monster

import (
	"fmt"
	"math/rand"
	"os"
	"slices"
	"sort"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/strictyaml"

	"gopkg.in/yaml.v3"
)

// MonsterDefinition holds the configuration for a monster type from YAML
type MonsterDefinition struct {
	// Abilities are the monster's authored combat abilities (abilities.go).
	Abilities Abilities       `yaml:"abilities,omitempty"`
	Arboreal  *ArborealConfig `yaml:"arboreal,omitempty"`
	// LegacyKeys and LegacyNames are a renamed monster's former key and display
	// name; saves written before the rename (keyed, or name-only legacy saves)
	// restore it under its current key.
	LegacyKeys  []string `yaml:"legacy_keys,omitempty"`
	LegacyNames []string `yaml:"legacy_names,omitempty"`

	Disposition  string   `yaml:"disposition,omitempty"`
	Prey         []string `yaml:"prey,omitempty"`
	PreyRadius   float64  `yaml:"prey_radius,omitempty"`
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type,omitempty"` // creature category, e.g. "undead" (empty = generic, for now)
	Level        int      `yaml:"level"`
	MaxHitPoints int      `yaml:"max_hit_points"`
	ArmorClass   int      `yaml:"armor_class"`
	PerfectDodge int      `yaml:"perfect_dodge"` // Chance (0-100) to completely avoid an attack
	Experience   int      `yaml:"experience"`
	DamageMin    int      `yaml:"damage_min"`
	DamageMax    int      `yaml:"damage_max"`
	TrueDamage   int      `yaml:"true_damage,omitempty"` // typed per-attack damage: resistance applies; armor/flat/dodge do not
	AlertRadius  float64  `yaml:"alert_radius"`
	AttackRadius float64  `yaml:"attack_radius"`
	Speed        float64  `yaml:"speed"` // the one movement speed: RT pace and fractional, accumulating TB steps (turn_based_cadence.go)
	Sprite       string   `yaml:"sprite"`
	Letter       string   `yaml:"letter"`
	Biomes       []string `yaml:"biomes,omitempty"`
	// AnimateWhenIdle loops the walking sheet at rest, independently of speed.
	AnimateWhenIdle bool `yaml:"animate_when_idle,omitempty"`
	// SizeClass (small/medium/person/large/huge) picks the sprite height from
	// config graphics.size_classes and the collision body from world.monster_bodies.
	SizeClass string `yaml:"size_class"`
	// StandeeCore picks the standee slab's core colour: empty uses the
	// chroma-weighted art average; silhouette_edge uses the mean colour of the
	// silhouette border, for art whose small saturated accents tint the side.
	StandeeCore string `yaml:"standee_core,omitempty"`
	// Champion, when set, names a champions.yaml build (a real character on the
	// monster AI). game.mirrorChampionStats mirrors that character's weapon
	// damage, attack cadence, HP and armor onto this monster at spawn. Its melee
	// damage school comes from that weapon. Ordinary melee uses a shared profile.
	Champion string `yaml:"champion,omitempty"`
	// size_game, size_multiplier and box_w/box_h are retired. The fields exist
	// only so content still authoring them FAILS LOUD in validation instead of
	// silently rendering or colliding at the wrong scale.
	DeprecatedSizeGame        float64             `yaml:"size_game,omitempty"`
	DeprecatedSizeMultiplier  float64             `yaml:"size_multiplier,omitempty"`
	DeprecatedBoxW            float64             `yaml:"box_w,omitempty"`
	DeprecatedBoxH            float64             `yaml:"box_h,omitempty"`
	Resistances               map[string]int      `yaml:"resistances"`
	WalkableTileOverrides     []string            `yaml:"walkable_tile_overrides,omitempty"`
	ProjectileSpell           string              `yaml:"projectile_spell"`
	ProjectileWeapon          string              `yaml:"projectile_weapon"`
	Flying                    bool                `yaml:"flying"`
	RangedAttackRange         float64             `yaml:"ranged_attack_range"`
	AttackCooldownMult        float64             `yaml:"attack_cooldown_multiplier"` // the one attack speed: RT cooldown scale and fractional, accumulating TB swings (turn_based_cadence.go)
	PassiveUntilHit           bool                `yaml:"passive_until_attacked"`
	DeprecatedMeleeDamageType string              `yaml:"melee_damage_type,omitempty"`
	Light                     *MonsterLightConfig `yaml:"light,omitempty"`
	// Boss is the explicit content classification. It drives boss AI, rewards,
	// and exclusions from ambient systems; boss-only mechanics below require it.
	Boss bool `yaml:"boss,omitempty"`
	// Elite classifies a strong ordinary monster (no rule reads it: an
	// instakill still erases it). A boss is not also elite.
	Elite bool `yaml:"elite,omitempty"`
	// Boss behaviour (data-driven; see the Golden Thief Bug). All optional.
	PassiveUntilQuest string  `yaml:"passive_until_quest,omitempty"`   // while this quest is incomplete the boss does not attack: it evades (if evade_radius_tiles set) or just holds dormant; turns aggressive once complete
	EvadeRadiusTiles  float64 `yaml:"evade_radius_tiles,omitempty"`    // >0 = evasive boss: blink when the party is within this many tiles (needs boss_cooldown_seconds). Omit for a dormant boss that just holds.
	BossCooldownSecs  float64 `yaml:"boss_cooldown_seconds,omitempty"` // RT cooldown between evasive blinks (required with evade_radius_tiles); Timer exception: may be a non-whole-round span, its TB rounds round up
	// Idol-ward (deep-jungle warlord): the boss is invulnerable + rooted (holds its
	// plaza) while any WarlordIdol monster lives; idols are immobile and never attack.
	WardedByIdols     bool    `yaml:"warded_by_idols,omitempty"` // boss: warded while any idol lives
	AggroWholeMap     bool    `yaml:"aggro_whole_map,omitempty"`
	RallyOnAggroTiles float64 `yaml:"rally_on_aggro_tiles,omitempty"` // alarm bell: on aggro, wake nearby monsters once
	RallyMaxTargets   int     `yaml:"rally_max_targets,omitempty"`    // alarm bell: 0 = no cap; otherwise wake at most this many calm monsters
	// boss: UNIQUE - once active, relentlessly chases from anywhere (else relentless only after normal aggro)
	DeathRalliesType string `yaml:"death_rallies_type,omitempty"` // on this monster's death, every live map monster of this Type goes relentless (revenge)
	WarlordIdol      bool   `yaml:"warlord_idol,omitempty"`       // this monster is a ward idol
	// Banding: while calm, same-type banding mobs stack onto one tile (rendered as
	// a small fanned pile, centred) and patrol as a flock; on aggro/being hit they
	// scatter to a ring of nearby tiles. BandGroup opts into a named mixed
	// party with shared aggro. See [[project_monster_banding]].
	Banding   bool   `yaml:"banding,omitempty"`
	BandGroup string `yaml:"band_group,omitempty"`

	// Reject the former misleading key rather than silently losing terrain permissions.
	DeprecatedHabitatPreferences yaml.Node `yaml:"habitat_preferences,omitempty"`
}

type MonsterLightConfig struct {
	Enabled     bool    `yaml:"enabled"`
	RadiusTiles float64 `yaml:"radius_tiles"`
	Intensity   float64 `yaml:"intensity"`
}

// MonsterYAMLConfig holds the complete monster configuration from YAML
type MonsterYAMLConfig struct {
	Monsters map[string]MonsterDefinition `yaml:"monsters"`
}

// StandeeCoreSilhouetteEdge is the one non-default standee_core value.
const StandeeCoreSilhouetteEdge = "silhouette_edge"

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
	var conflicts []string

	// A former key or name must never shadow a live monster or belong to two.
	if err := validateMonsterRenames(config); err != nil {
		return err
	}

	// Creature types gate immunities and revenge rallies by exact comparison,
	// so they are canonical (lowercase, trimmed) from load on.
	for key, monster := range config.Monsters {
		monster.Type = CanonicalMonsterType(monster.Type)
		monster.DeathRalliesType = CanonicalMonsterType(monster.DeathRalliesType)
		config.Monsters[key] = monster
	}
	for key, monster := range config.Monsters {
		// Formless is a boss-only type: it shields a boss from type-keyed
		// control (Bind Undead, Charm) and nothing else uses it.
		if monster.Type == TypeFormless && !monster.Boss {
			conflicts = append(conflicts, fmt.Sprintf("monster %q: type %q is reserved for bosses", key, TypeFormless))
		}
		if monster.AttackCooldownMult < 0 {
			conflicts = append(conflicts, fmt.Sprintf("monster %q: attack_cooldown_multiplier must be positive (or omitted for the default speed)", key))
		}
		if monster.Arboreal != nil {
			if err := monster.Arboreal.validate(); err != nil {
				conflicts = append(conflicts, fmt.Sprintf("monster %q: %v", key, err))
			}
			if monster.Disposition != DispositionWildlife || monster.Flying || len(monster.WalkableTileOverrides) != 0 || len(monster.Prey) != 0 {
				conflicts = append(conflicts, fmt.Sprintf("monster %q: arboreal movement requires ground wildlife without prey or terrain overrides", key))
			}
		}
		if monster.Disposition != "" && monster.Disposition != DispositionWildlife && monster.Disposition != DispositionCaravan && monster.Disposition != DispositionFish {
			conflicts = append(conflicts, fmt.Sprintf("monster %q has invalid disposition", key))
		}
		if len(monster.Prey) > 0 && (monster.Disposition != DispositionWildlife || monster.PreyRadius <= 0) {
			conflicts = append(conflicts, fmt.Sprintf("monster %q has invalid prey rules", key))
		}
		for _, prey := range monster.Prey {
			if _, ok := config.Monsters[prey]; !ok {
				conflicts = append(conflicts, fmt.Sprintf("monster %q has unknown prey %q", key, prey))
			}
		}
		if monster.DeprecatedHabitatPreferences.Kind != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed habitat_preferences - use walkable_tile_overrides for blocked tile exceptions", key))
		}
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

	// Effect flags travel in pairs: a chance without its magnitude (or an evasive
	// phase without its trigger tuning) would silently fall back to zero in code.
	for key, monster := range config.Monsters {
		if monster.BandGroup != "" && !monster.Banding {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' band_group requires banding", key))
		}
		if monster.Letter != "" && (len(monster.Letter) != 1 || monster.Letter[0] < 'a' || monster.Letter[0] > 'z') {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has map letter %q - monster spawns must use one lowercase ASCII letter (a-z)", key, monster.Letter))
		}
		if monster.DeprecatedSizeGame != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed key size_game - use size_class instead", key))
		}
		if monster.DeprecatedSizeMultiplier != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed key size_multiplier - use size_class (small/medium/person/large/huge)", key))
		}
		if monster.DeprecatedBoxW != 0 || monster.DeprecatedBoxH != 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed keys box_w/box_h - the collision body comes from size_class (config world.monster_bodies)", key))
		}
		if monster.StandeeCore != "" && monster.StandeeCore != StandeeCoreSilhouetteEdge {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has invalid standee_core %q - want %s or omit it", key, monster.StandeeCore, StandeeCoreSilhouetteEdge))
		}
		if !ValidSizeClasses[monster.SizeClass] {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has invalid size_class %q - want one of small/medium/person/large/huge", key, monster.SizeClass))
		}
		if monster.DeprecatedMeleeDamageType != "" {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' uses removed melee_damage_type - ordinary melee is physical; elemental attacks come from biome rules", key))
		}
		if monster.Elite && monster.Boss {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' is both boss and elite", key))
		}
		if monster.requiresBossClassification() && !monster.Boss {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' authors boss-only behavior but lacks boss: true", key))
		}
		// Abilities name their own numbers: combat has no hidden fallbacks, so
		// the monster card always shows what the ability does.
		for _, problem := range monster.abilityProblems() {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' ability %s", key, problem))
		}
		for i := range monster.Abilities.PartyAttacks {
			attack := &monster.Abilities.PartyAttacks[i]
			if school, err := ParseDamageType(attack.School.String()); err == nil {
				attack.School = school
			}
		}
		if monster.Resistances != nil {
			resistances := make(map[string]int, len(monster.Resistances))
			originals := make(map[string]string, len(monster.Resistances))
			valid := true
			for rawSchool, resistance := range monster.Resistances {
				damageType, err := ParseDamageType(rawSchool)
				if err != nil {
					conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has unknown resistance school %q", key, rawSchool))
					valid = false
					continue
				}
				school := damageType.String()
				if previous, exists := originals[school]; exists {
					conflicts = append(conflicts, fmt.Sprintf(
						"Monster '%s' resistance schools %q and %q normalize to the same key %q",
						key,
						previous,
						rawSchool,
						school,
					))
					valid = false
					continue
				}
				originals[school] = rawSchool
				resistances[school] = resistance
			}
			if valid {
				monster.Resistances = resistances
			}
		}
		config.Monsters[key] = monster
		// An evasive boss (blinks away while its quest is unfinished) needs a blink
		// cooldown. A dormant boss (passive_until_quest with no evade_radius_tiles)
		// just holds until the quest completes, so it needs neither.
		if monster.EvadeRadiusTiles > 0 && monster.BossCooldownSecs <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has evade_radius_tiles but no boss_cooldown_seconds", key))
		}
		if monster.RallyMaxTargets < 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has negative rally_max_targets", key))
		}
		if monster.RallyMaxTargets > 0 && monster.RallyOnAggroTiles <= 0 {
			conflicts = append(conflicts, fmt.Sprintf("Monster '%s' has rally_max_targets but no rally_on_aggro_tiles", key))
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

// validateMonsterRenames applies the catalogs' one rename rule; monster
// display names may repeat (several "Dragon"s).
func validateMonsterRenames(c *MonsterYAMLConfig) error {
	renamed := make([]config.Renamed, 0, len(c.Monsters))
	for key, def := range c.Monsters {
		renamed = append(renamed, config.Renamed{Key: key, Name: def.Name, LegacyNames: def.LegacyNames, LegacyKeys: def.LegacyKeys})
	}
	_, err := config.ValidateRenames("monster", renamed, false)
	return err
}

// requiresBossClassification lists the boss behaviours whose scheduling and AI
// use the explicit Boss classification; boss abilities say so in their own row
// (abilityRows bossOnly). A new boss-only knob fails loud until classified.
func (m MonsterDefinition) requiresBossClassification() bool {
	return m.PassiveUntilQuest != "" || m.EvadeRadiusTiles > 0 || m.BossCooldownSecs > 0 ||
		m.WardedByIdols || m.AggroWholeMap || m.DeathRalliesType != ""
}

// ValidateCatalogReferences checks monster links that can only resolve after
// the spell, weapon and loot catalogs have loaded.
func ValidateCatalogReferences(monsters *MonsterYAMLConfig, gameConfig *config.Config) error {
	if monsters == nil {
		return fmt.Errorf("monster catalog is nil")
	}
	if gameConfig != nil {
		for _, pack := range gameConfig.DayNight.Packs {
			for _, night := range []bool{false, true} {
				for _, member := range pack.PhaseMembers(night) {
					keys := []string{member.Monster}
					if member.Replacement != nil {
						keys = append(keys, member.Replacement.Monster)
					}
					for _, key := range keys {
						if _, ok := monsters.Monsters[key]; !ok {
							return fmt.Errorf("day_night pack %q references unknown monster %q", pack.Map, key)
						}
					}
				}
			}
		}
	}
	// Catalog entries that spawn a monster by key: a typo would otherwise panic
	// in NewMonster3DFromConfig on the first summon, mid-fight.
	if config.GlobalSpells != nil {
		for id, spell := range config.GlobalSpells.Spells {
			if spell == nil || spell.SummonMonster == "" {
				continue
			}
			if _, known := monsters.Monsters[spell.SummonMonster]; !known {
				return fmt.Errorf("spell %q summon_monster references unknown monster %q", id, spell.SummonMonster)
			}
		}
	}
	if config.GlobalItems != nil {
		for key, item := range config.GlobalItems.Items {
			if item == nil || item.CardSummonMonster == "" {
				continue
			}
			if _, known := monsters.Monsters[item.CardSummonMonster]; !known {
				return fmt.Errorf("item %q card_summon_monster references unknown monster %q", key, item.CardSummonMonster)
			}
		}
	}
	for key, def := range monsters.Monsters {
		if def.ProjectileSpell != "" {
			if spell, ok := config.GetSpellDefinition(def.ProjectileSpell); !ok || spell == nil {
				return fmt.Errorf("monster %q references unknown projectile_spell %q", key, def.ProjectileSpell)
			}
			if gameConfig == nil {
				return fmt.Errorf("monster %q projectile_spell %q cannot be validated without game config", key, def.ProjectileSpell)
			}
			if _, err := gameConfig.GetSpellConfig(def.ProjectileSpell); err != nil {
				return fmt.Errorf("monster %q projectile_spell %q has no projectile physics: %w", key, def.ProjectileSpell, err)
			}
		}
		if def.ProjectileWeapon != "" {
			weapon, ok := config.GetWeaponDefinition(def.ProjectileWeapon)
			if !ok || weapon == nil {
				return fmt.Errorf("monster %q references unknown projectile_weapon %q", key, def.ProjectileWeapon)
			}
			if weapon.Physics == nil {
				return fmt.Errorf("monster %q projectile_weapon %q has no projectile physics", key, def.ProjectileWeapon)
			}
		}
		for i, summonKey := range def.Abilities.Summon.Monsters {
			if _, ok := monsters.Monsters[summonKey]; !ok {
				return fmt.Errorf("monster %q abilities.summon.monsters[%d] references unknown monster %q", key, i, summonKey)
			}
		}
	}
	if config.GlobalLoots != nil {
		for monsterKey := range config.GlobalLoots.Loots {
			if _, ok := monsters.Monsters[monsterKey]; !ok {
				return fmt.Errorf("loots.%s has no matching monster definition", monsterKey)
			}
		}
	}
	return nil
}

// LoadMonsterConfig loads monster configuration from YAML file
func LoadMonsterConfig(filename string) (*MonsterYAMLConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read monster config file: %w", err)
	}

	// Strict: an unknown field (a typo, or a key an ability rework moved) fails
	// the load instead of silently switching the mechanic off.
	var config MonsterYAMLConfig
	if err := strictyaml.Decode(data, &config); err != nil {
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
// CanonicalKey maps a renamed monster's former key (legacy_keys) to its
// current key; any other key is returned unchanged.
func (c *MonsterYAMLConfig) CanonicalKey(key string) string {
	if _, ok := c.Monsters[key]; ok {
		return key
	}
	for current, def := range c.Monsters {
		if slices.Contains(def.LegacyKeys, key) {
			return current
		}
	}
	return key
}

func (c *MonsterYAMLConfig) GetMonsterByKey(key string) (*MonsterDefinition, error) {
	monster, exists := c.Monsters[key]
	if !exists {
		return nil, fmt.Errorf("monster with key '%s' not found", key)
	}
	return &monster, nil
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

// SetupMonsterFromConfig configures a monster from YAML definition
func (m *Monster3D) SetupMonsterFromConfig(def *MonsterDefinition) {
	m.Name = def.Name
	m.Disposition, m.Prey, m.PreyRadius = def.Disposition, def.Prey, def.PreyRadius*m.tileSize()
	m.Arboreal = def.Arboreal
	m.MonsterType = def.Type
	// Render/collision identity never changes after setup; cache it so hot
	// frame/tick callers do not copy or scan the YAML definition.
	m.cachedSprite = def.GetSpriteFromConfig()
	m.cachedBody = m.bodyPixels(def.SizeClass)
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
	m.AnimateWhenIdle = def.AnimateWhenIdle

	// Gold comes from the monster's loot table, rolled once at spawn.
	m.Gold = 0
	if gold, ok := config.MonsterGold(m.Key); ok {
		m.Gold = gold.Min + rand.Intn(gold.Max-gold.Min+1)
	}

	// Set resistances from the canonical runtime school catalog. YAML keys were
	// validated at load, so this no longer depends on the global config pointer.
	for damageTypeStr, resistance := range def.Resistances {
		if damageType, err := ParseDamageType(damageTypeStr); err == nil {
			m.Resistances[damageType] = resistance
		}
	}

	// Set walkable tile overrides - tiles this monster can walk on even if normally blocked
	m.WalkableTileOverrides = def.WalkableTileOverrides

	// Set ranged attack configuration
	m.ProjectileSpell = def.ProjectileSpell
	m.ProjectileWeapon = def.ProjectileWeapon
	m.Flying = def.Flying
	m.AttackCooldownMultiplier = def.AttackCooldownMult
	m.PassiveUntilAttacked = def.PassiveUntilHit
	m.HatesTraits = HatesTable[m.Key] // party traits that enrage this passive monster (hates.yaml)
	if def.RangedAttackRange > 0 {
		m.RangedAttackRange = def.RangedAttackRange * tileSize
	}
	// The monster owns its copy: a champion stamps its striking weapon's riders
	// onto it before each hit, auto-level scales its damage.
	m.Abilities = def.Abilities.Clone()
	m.Boss = def.Boss
	m.PassiveUntilQuest = def.PassiveUntilQuest
	m.EvadeRadiusTiles = def.EvadeRadiusTiles
	m.BossCooldownSecs = def.BossCooldownSecs
	m.WardedByIdols = def.WardedByIdols
	m.AggroWholeMap = def.AggroWholeMap
	m.RallyOnAggroTiles = def.RallyOnAggroTiles
	m.RallyMaxTargets = def.RallyMaxTargets
	m.DeathRalliesType = def.DeathRalliesType
	m.WarlordIdol = def.WarlordIdol
	m.Banding = def.Banding
	m.BandGroup = def.BandGroup

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

// bodyPixels resolves the square collision body of a size class from config
// world.monster_bodies. LoadConfig guarantees every class; only hand-built
// test configs and monsters fall back to the cap.
func (m *Monster3D) bodyPixels(class string) float64 {
	tiles, ok := m.config.MonsterBodyTiles(class)
	if !ok {
		tiles = config.MaxMonsterBodyTiles
	}
	return tiles * m.tileSize()
}

// ValidSizeClasses is the fixed set of monster size-class names. The tile-height
// per class is data-driven (config graphics.monster_size_classes); the NAMES are
// the enum, so content validation can reject a typo without the config loaded.
var ValidSizeClasses = func() map[string]bool {
	classes := make(map[string]bool)
	for _, name := range config.ActorSizeClassNames() {
		classes[name] = true
	}
	return classes
}()

// sizeClassHeights maps size class -> sprite height in tiles, wired from config
// at boot via SetSizeClassHeights (monster package must not import the loaded
// config instance, so the values are pushed in).
var sizeClassHeights = map[string]float64{}

// SetSizeClassHeights installs the monster view of the shared class -> tile
// span table from config graphics.size_classes. Call once after loading config.
func SetSizeClassHeights(m map[string]float64) {
	sizeClassHeights = map[string]float64{}
	for k, v := range m {
		sizeClassHeights[k] = v
	}
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
	if h, ok := sizeClassHeights[config.SizeClassPerson]; ok {
		return h
	}
	return 0.8
}
