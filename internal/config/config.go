package config

import (
	"fmt"
	"math"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all game configuration values
type Config struct {
	Display         DisplayConfig         `yaml:"display"`
	Engine          EngineConfig          `yaml:"engine"`
	World           WorldConfig           `yaml:"world"`
	Movement        MovementConfig        `yaml:"movement"`
	Combat          CombatConfig          `yaml:"combat"`
	Camera          CameraConfig          `yaml:"camera"`
	UI              UIConfig              `yaml:"ui"`
	WorldGeneration WorldGenerationConfig `yaml:"world_generation"`
	Monsters        MonsterConfig         `yaml:"monsters"`
	Characters      CharacterConfig       `yaml:"characters"`
	MonsterAI       MonsterAIConfig       `yaml:"monster_ai"`
	SkillTeaching   SkillTeachingConfig   `yaml:"skill_teaching"`
	Graphics        GraphicsConfig        `yaml:"graphics"`
	Tiles           TileConfig            `yaml:"tiles"`
}

type DisplayConfig struct {
	ScreenWidth       int    `yaml:"screen_width"`
	ScreenHeight      int    `yaml:"screen_height"`
	WindowTitle       string `yaml:"window_title"`
	Resizable         bool   `yaml:"resizable"`
	DisableVsyncOnMac bool   `yaml:"disable_vsync_on_mac"`
}

type EngineConfig struct {
	TPS int `yaml:"tps"`
}

type WorldConfig struct {
	TileSize  int `yaml:"tile_size"`
	MapWidth  int `yaml:"map_width"`
	MapHeight int `yaml:"map_height"`
}

type MovementConfig struct {
	MoveSpeed     float64 `yaml:"move_speed"`
	RotationSpeed float64 `yaml:"rotation_speed"`
}

type CombatConfig struct {
	// Melee weapons (legacy config from config.yaml)
	Sword  MeleeWeaponConfig `yaml:"sword"`
	Dagger MeleeWeaponConfig `yaml:"dagger"`
	Axe    MeleeWeaponConfig `yaml:"axe"`
	Mace   MeleeWeaponConfig `yaml:"mace"`
	Spear  MeleeWeaponConfig `yaml:"spear"`
	Staff  MeleeWeaponConfig `yaml:"staff"`

	// Ranged weapons
	Bow ArrowConfig `yaml:"bow"`
}

// MeleeWeaponConfig for legacy config.yaml melee weapons
type MeleeWeaponConfig struct {
	Speed         float64 `yaml:"speed"`
	Lifetime      int     `yaml:"lifetime"`
	CollisionSize int     `yaml:"collision_size"`
}

// ProjectilePhysicsConfig is the unified config for all projectile physics (spells, arrows, etc.)
// Uses tile-based units for designer-friendly configuration.
// Speed is in tiles per second, range is in tiles, collision is in tiles.
// Lifetime is calculated automatically: lifetime_frames = (range / speed) * tps
type ProjectilePhysicsConfig struct {
	SpeedTiles         float64 `yaml:"speed_tiles"`          // Speed in tiles per second
	RangeTiles         float64 `yaml:"range_tiles"`          // Maximum range in tiles
	CollisionSizeTiles float64 `yaml:"collision_size_tiles"` // Collision box size in tiles (min 0.5)
}

// GetSpeedPixels returns speed in pixels per frame for the game engine
func (p *ProjectilePhysicsConfig) GetSpeedPixels(tileSize float64) float64 {
	// Convert tiles/second to pixels/frame based on target TPS.
	tps := float64(GetTargetTPS())
	return (p.SpeedTiles * tileSize) / tps
}

// GetLifetimeFrames returns lifetime in frames based on range and speed
func (p *ProjectilePhysicsConfig) GetLifetimeFrames() int {
	if p.SpeedTiles <= 0 {
		return GetTargetTPS() // Default 1 second if speed is invalid
	}
	// lifetime = range / speed * tps (frames per second)
	return int((p.RangeTiles / p.SpeedTiles) * float64(GetTargetTPS()))
}

// GetCollisionSizePixels returns collision size in pixels for the game engine
// Enforces minimum of 0.5 tiles
func (p *ProjectilePhysicsConfig) GetCollisionSizePixels(tileSize float64) float64 {
	collisionTiles := p.CollisionSizeTiles
	if collisionTiles < 0.5 {
		collisionTiles = 0.5 // Minimum 0.5 tiles
	}
	return collisionTiles * tileSize
}

// MeleeAttackConfig for instant melee weapons
type MeleeAttackConfig struct {
	ArcAngle        int `yaml:"arc_angle"`        // Swing arc in degrees
	AnimationFrames int `yaml:"animation_frames"` // Frames for animation
	HitDelay        int `yaml:"hit_delay"`        // Frames before damage applies
}

// WeaponGraphicsConfig for weapon visual effects
type WeaponGraphicsConfig struct {
	SlashColor  [3]int `yaml:"slash_color"`  // RGB color for slash effect
	SlashWidth  int    `yaml:"slash_width"`  // Width of slash line
	SlashLength int    `yaml:"slash_length"` // Length of slash line

	// Legacy fields for compatibility with existing projectile system
	MaxSize  int    `yaml:"max_size"`
	MinSize  int    `yaml:"min_size"`
	BaseSize int    `yaml:"base_size"`
	Color    [3]int `yaml:"color"`
}

type ArrowConfig struct {
	Speed         float64 `yaml:"speed"`
	Lifetime      int     `yaml:"lifetime"`
	CollisionSize int     `yaml:"collision_size"` // Size for collision detection
}

type CameraConfig struct {
	FieldOfView  float64 `yaml:"field_of_view"`
	ViewDistance float64 `yaml:"view_distance"`
}

type UIConfig struct {
	SpellInputCooldown  int `yaml:"spell_input_cooldown"`
	PartyPortraitHeight int `yaml:"party_portrait_height"`
	CompassRadius       int `yaml:"compass_radius"`
	DamageBlinkFrames   int `yaml:"damage_blink_frames"`
}

type WorldGenerationConfig struct {
	Forest            ForestConfig            `yaml:"forest"`
	AncientTrees      AncientTreesConfig      `yaml:"ancient_trees"`
	MagicalFeatures   MagicalFeaturesConfig   `yaml:"magical_features"`
	NaturalFormations NaturalFormationsConfig `yaml:"natural_formations"`
	Clearings         ClearingsConfig         `yaml:"clearings"`
	Water             WaterConfig             `yaml:"water"`
	Undergrowth       UndergrowthConfig       `yaml:"undergrowth"`
}

type ForestConfig struct {
	TreeClusters   int `yaml:"tree_clusters"`
	ClusterSizeMin int `yaml:"cluster_size_min"`
	ClusterSizeMax int `yaml:"cluster_size_max"`
	ClusterSpread  int `yaml:"cluster_spread"`
}

type AncientTreesConfig struct {
	CountMin          int `yaml:"count_min"`
	CountMax          int `yaml:"count_max"`
	PlacementAttempts int `yaml:"placement_attempts"`
	ClearRadius       int `yaml:"clear_radius"`
}

type MagicalFeaturesConfig struct {
	MushroomRings FeatureCountConfig `yaml:"mushroom_rings"`
	FireflySwarms FeatureCountConfig `yaml:"firefly_swarms"`
}

type FeatureCountConfig struct {
	CountMin int `yaml:"count_min"`
	CountMax int `yaml:"count_max"`
}

type NaturalFormationsConfig struct {
	MossRocks FeatureCountConfig `yaml:"moss_rocks"`
}

type ClearingsConfig struct {
	CountMin  int `yaml:"count_min"`
	CountMax  int `yaml:"count_max"`
	RadiusMin int `yaml:"radius_min"`
	RadiusMax int `yaml:"radius_max"`
}

type WaterConfig struct {
	StreamStartYFraction float64 `yaml:"stream_start_y_fraction"`
	StreamWanderRange    int     `yaml:"stream_wander_range"`
	PondSize             int     `yaml:"pond_size"`
	PondXFraction        float64 `yaml:"pond_x_fraction"`
	PondYFraction        float64 `yaml:"pond_y_fraction"`
}

type UndergrowthConfig struct {
	FernPatches FernPatchesConfig `yaml:"fern_patches"`
}

type FernPatchesConfig struct {
	CountMin          int     `yaml:"count_min"`
	CountMax          int     `yaml:"count_max"`
	TreeSearchRadius  int     `yaml:"tree_search_radius"`
	RandomSpawnChance float64 `yaml:"random_spawn_chance"`
}

type MonsterConfig struct {
	Common  MonsterSpawnConfig   `yaml:"common"`
	Rare    MonsterSpawnConfig   `yaml:"rare"`
	Special SpecialMonsterConfig `yaml:"special"`
}

type MonsterSpawnConfig struct {
	CountMin          int `yaml:"count_min"`
	CountMax          int `yaml:"count_max"`
	PlacementAttempts int `yaml:"placement_attempts"`
}

type SpecialMonsterConfig struct {
	PixieMushroomRingChance float64 `yaml:"pixie_mushroom_ring_chance"`
}

type CharacterConfig struct {
	StartingGold int                   `yaml:"starting_gold"`
	StartingFood int                   `yaml:"starting_food"`
	HitPoints    HitPointsConfig       `yaml:"hit_points"`
	SpellPoints  SpellPointsConfig     `yaml:"spell_points"`
	Classes      map[string]ClassStats `yaml:"classes"`
}

type HitPointsConfig struct {
	EnduranceMultiplier int `yaml:"endurance_multiplier"`
	LevelMultiplier     int `yaml:"level_multiplier"`
}

type SpellPointsConfig struct {
	LevelMultiplier int `yaml:"level_multiplier"`
}

type ClassStats struct {
	Might       int `yaml:"might"`
	Intellect   int `yaml:"intellect"`
	Personality int `yaml:"personality"`
	Endurance   int `yaml:"endurance"`
	Accuracy    int `yaml:"accuracy"`
	Speed       int `yaml:"speed"`
	Luck        int `yaml:"luck"`
}

// SpellSystemConfig contains the complete unified spell system configuration
type SpellSystemConfig struct {
	Spells map[string]*SpellDefinitionConfig `yaml:"spells"`
}

// SpellDefinitionConfig represents a complete spell definition with embedded physics and graphics
type SpellDefinitionConfig struct {
	// Basic spell properties
	Name               string  `yaml:"name"`
	Description        string  `yaml:"description"`
	School             string  `yaml:"school"`
	Level              int     `yaml:"level"`
	SpellPointsCost    int     `yaml:"spell_points_cost"`
	Duration           int     `yaml:"duration"` // Duration in seconds (for buff spells)
	Damage             int     `yaml:"damage"`
	DisintegrateChance float64 `yaml:"disintegrate_chance,omitempty"`
	ProjectileSize     int     `yaml:"projectile_size"`
	IsProjectile       bool    `yaml:"is_projectile"`
	IsUtility          bool    `yaml:"is_utility"`
	VisualEffect       string  `yaml:"visual_effect"`
	StatusIcon         string  `yaml:"status_icon,omitempty"`

	// Utility spell specific fields
	HealAmount  int     `yaml:"heal_amount,omitempty"`
	StatBonus   int     `yaml:"stat_bonus,omitempty"`
	VisionBonus float64 `yaml:"vision_bonus,omitempty"`

	// Effect configuration
	TargetSelf     bool   `yaml:"target_self,omitempty"`
	Awaken         bool   `yaml:"awaken,omitempty"`
	WaterWalk      bool   `yaml:"water_walk,omitempty"`
	WaterBreathing bool   `yaml:"water_breathing,omitempty"`
	Message        string `yaml:"message,omitempty"`

	// Embedded physics configuration (for projectile spells) - uses tile-based units
	Physics *ProjectilePhysicsConfig `yaml:"physics,omitempty"`

	// Embedded graphics configuration (for projectile spells)
	Graphics *ProjectileRenderConfig `yaml:"graphics,omitempty"`

	// Legacy fields for backward compatibility - will be removed
	Range           int     `yaml:"range,omitempty"`            // Deprecated: use physics.range_tiles
	ProjectileSpeed float64 `yaml:"projectile_speed,omitempty"` // Deprecated: use physics.speed_tiles
	Lifetime        int     `yaml:"lifetime,omitempty"`         // Deprecated: calculated from physics
}

type MonsterAIConfig struct {
	// New AI behavior timers (in frames, 60fps)
	IdlePatrolTimer      int `yaml:"idle_patrol_timer"`
	PatrolDirectionTimer int `yaml:"patrol_direction_timer"`
	PatrolIdleTimer      int `yaml:"patrol_idle_timer"`
	AlertTimeout         int `yaml:"alert_timeout"`
	AttackCooldown       int `yaml:"attack_cooldown"`
	FleeDuration         int `yaml:"flee_duration"`

	// Behavior chances (0.0 to 1.0)
	IdleToPatrolChance    float64 `yaml:"idle_to_patrol_chance"`
	PatrolDirectionChance float64 `yaml:"patrol_direction_chance"`

	// Movement parameters
	NormalSpeedMultiplier float64 `yaml:"normal_speed_multiplier"`
	FleeSpeedMultiplier   float64 `yaml:"flee_speed_multiplier"`

	// Vision and pathfinding
	PatrolVisionDistance    float64 `yaml:"patrol_vision_distance"`
	FleeVisionDistance      float64 `yaml:"flee_vision_distance"`
	DirectionVisionDistance float64 `yaml:"direction_vision_distance"`

	// AI frequency checks (in frames)
	PathCheckFrequency   int `yaml:"path_check_frequency"`
	FleeCheckFrequency   int `yaml:"flee_check_frequency"`
	MaxDirectionAttempts int `yaml:"max_direction_attempts"`

	PushbackDistance float64 `yaml:"pushback_distance"`
}

type SkillTeachingConfig struct {
	ExpertCost      int `yaml:"expert_cost"`
	MasterCost      int `yaml:"master_cost"`
	GrandmasterCost int `yaml:"grandmaster_cost"`
	NoviceCost      int `yaml:"novice_cost"`
}

type GraphicsConfig struct {
	RaysPerScreenWidth int                 `yaml:"rays_per_screen_width"`
	Colors             ColorsConfig        `yaml:"colors"`
	Sprite             SpriteConfig        `yaml:"sprite"`
	BrightnessMin      float64             `yaml:"brightness_min"`
	Monster            MonsterRenderConfig `yaml:"monster"`
	NPC                NPCRenderConfig     `yaml:"npc"`
	Projectiles        ProjectilesConfig   `yaml:"projectiles"`
}

type ColorsConfig struct {
	Sky      [3]int `yaml:"sky"`
	Ground   [3]int `yaml:"ground"`
	ForestBg [3]int `yaml:"forest_bg"`
}

type SpriteConfig struct {
	PlaceholderSize      int     `yaml:"placeholder_size"`
	TreeHeightMultiplier float64 `yaml:"tree_height_multiplier"`
	TreeWidthMultiplier  float64 `yaml:"tree_width_multiplier"`
}

type MonsterRenderConfig struct {
	MaxSpriteSize          int `yaml:"max_sprite_size"`
	MinSpriteSize          int `yaml:"min_sprite_size"`
	SizeDistanceMultiplier int `yaml:"size_distance_multiplier"`
}

type NPCRenderConfig struct {
	MaxSpriteSize          int `yaml:"max_sprite_size"`
	MinSpriteSize          int `yaml:"min_sprite_size"`
	SizeDistanceMultiplier int `yaml:"size_distance_multiplier"`
}

type ProjectilesConfig struct {
	// Dynamic spell graphics configurations (replaces hardcoded Fireball, FireBolt, Lightning!)
	Spells map[string]*ProjectileRenderConfig `yaml:"spells"`

	// Melee weapons
	Sword  ProjectileRenderConfig `yaml:"sword"`
	Dagger ProjectileRenderConfig `yaml:"dagger"`
	Axe    ProjectileRenderConfig `yaml:"axe"`
	Mace   ProjectileRenderConfig `yaml:"mace"`
	Spear  ProjectileRenderConfig `yaml:"spear"`
	Staff  ProjectileRenderConfig `yaml:"staff"`

	// Ranged weapons
	Bow ProjectileRenderConfig `yaml:"bow"`
}

type ProjectileRenderConfig struct {
	MaxSize  int    `yaml:"max_size"`
	MinSize  int    `yaml:"min_size"`
	BaseSize int    `yaml:"base_size"`
	Color    [3]int `yaml:"color"`
}

type TileConfig struct {
	TileData map[string]TileData `yaml:"tiles"`
}

type TileData struct {
	Name             string                 `yaml:"name"`
	Type             string                 `yaml:"type,omitempty"`
	Solid            bool                   `yaml:"solid"`
	Transparent      bool                   `yaml:"transparent"`
	Walkable         bool                   `yaml:"walkable"`
	HeightMultiplier float64                `yaml:"height_multiplier"`
	Sprite           string                 `yaml:"sprite"`
	RenderType       string                 `yaml:"render_type"`
	FloorColor       [3]int                 `yaml:"floor_color"`
	FloorNearColor   [3]int                 `yaml:"floor_near_color"`
	WallColor        [3]int                 `yaml:"wall_color"`
	Letter           string                 `yaml:"letter"`
	Biomes           []string               `yaml:"biomes,omitempty"`
	Properties       map[string]interface{} `yaml:"properties,omitempty"`
	Effects          map[string]string      `yaml:"effects,omitempty"`
}

type SpecialTileConfig struct {
	SpecialTileData map[string]TileData `yaml:"special_tiles"`
}

type MapConfig struct {
	Name              string  `yaml:"name"`
	File              string  `yaml:"file"`
	Biome             string  `yaml:"biome"`
	SkyColor          [3]int  `yaml:"sky_color"`
	DefaultFloorColor [3]int  `yaml:"default_floor_color"`
	AmbientLight      float64 `yaml:"ambient_light"`
}

type MapConfigs struct {
	Maps map[string]MapConfig `yaml:"maps"`
}

// WeaponSystemConfig contains the complete weapon system configuration
type WeaponSystemConfig struct {
	Weapons map[string]*WeaponDefinitionConfig `yaml:"weapons"`
}

// WeaponDefinitionConfig represents a complete weapon definition with embedded physics and graphics
type WeaponDefinitionConfig struct {
	// Basic weapon properties
	Name               string             `yaml:"name"`
	Description        string             `yaml:"description"`
	Category           string             `yaml:"category"`
	Damage             int                `yaml:"damage"`
	Range              int                `yaml:"range"` // Range in tiles (for melee reach)
	BonusStat          string             `yaml:"bonus_stat"`
	BonusStatSecondary string             `yaml:"bonus_stat_secondary"`
	DamageType         string             `yaml:"damage_type"`
	MaxProjectiles     int                `yaml:"max_projectiles"`
	HitBonus           int                `yaml:"hit_bonus"`
	CritChance         int                `yaml:"crit_chance"`
	StunChance         float64            `yaml:"stun_chance"`
	StunTurns          int                `yaml:"stun_turns"`
	DisintegrateChance float64            `yaml:"disintegrate_chance,omitempty"`
	Rarity             string             `yaml:"rarity"`
	Value              int                `yaml:"value,omitempty"`
	BonusVs            map[string]float64 `yaml:"bonus_vs,omitempty"`

	// Embedded physics configuration (for projectile weapons like bows) - uses tile-based units
	Physics *ProjectilePhysicsConfig `yaml:"physics"`

	// Embedded melee configuration (for instant melee weapons)
	Melee *MeleeAttackConfig `yaml:"melee"`

	// Embedded graphics configuration
	Graphics *WeaponGraphicsConfig `yaml:"graphics"`
}

const defaultTPS = 120

func (c *Config) GetTPS() int {
	if c != nil && c.Engine.TPS > 0 {
		return c.Engine.TPS
	}
	return defaultTPS
}

func GetTargetTPS() int {
	if GlobalConfig != nil {
		return GlobalConfig.GetTPS()
	}
	return defaultTPS
}

var GlobalConfig *Config
var GlobalSpells *SpellSystemConfig
var GlobalWeapons *WeaponSystemConfig
var GlobalItems *ItemSystemConfig
var GlobalLoots *LootTablesConfig

// LoadConfig loads the configuration from config.yaml
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	// Set global config for easy access
	GlobalConfig = &config

	return &config, nil
}

// MustLoadConfig loads the configuration and panics on error
func MustLoadConfig(filename string) *Config {
	config, err := LoadConfig(filename)
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}
	return config
}

// LoadSpellConfig loads the spell configuration from spells.yaml
func LoadSpellConfig(filename string) (*SpellSystemConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var spellConfig SpellSystemConfig
	err = yaml.Unmarshal(data, &spellConfig)
	if err != nil {
		return nil, err
	}

	// Set global spell config for easy access
	GlobalSpells = &spellConfig

	return &spellConfig, nil
}

// MustLoadSpellConfig loads the spell configuration and panics on error
func MustLoadSpellConfig(filename string) *SpellSystemConfig {
	spellConfig, err := LoadSpellConfig(filename)
	if err != nil {
		panic("Failed to load spell config: " + err.Error())
	}
	return spellConfig
}

// LoadWeaponConfig loads the weapon configuration from weapons.yaml
func LoadWeaponConfig(filename string) (*WeaponSystemConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var weaponConfig WeaponSystemConfig
	err = yaml.Unmarshal(data, &weaponConfig)
	if err != nil {
		return nil, err
	}

	// Set global weapon config for easy access
	GlobalWeapons = &weaponConfig

	// Set up weapon accessor for items package to avoid circular imports
	setupWeaponAccessor()

	return &weaponConfig, nil
}

// setupWeaponAccessor configures the global weapon accessor for items package
func setupWeaponAccessor() {
	// This will be imported by items package
	// For now we'll define this in a separate function
}

// MustLoadWeaponConfig loads the weapon configuration and panics on error
func MustLoadWeaponConfig(filename string) *WeaponSystemConfig {
	weaponConfig, err := LoadWeaponConfig(filename)
	if err != nil {
		panic("Failed to load weapon config: " + err.Error())
	}
	return weaponConfig
}

// ---------------- Items (non-weapon, non-spell) ----------------

type ItemSystemConfig struct {
	Items map[string]*ItemDefinitionConfig `yaml:"items"`
}

type ItemDefinitionConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // armor|accessory|consumable|quest
	ArmorType   string `yaml:"armor_category,omitempty"`
	Description string `yaml:"description"`          // Gameplay-neutral summary (optional)
	Flavor      string `yaml:"flavor,omitempty"`     // Short artistic line for tooltip
	EquipSlot   string `yaml:"equip_slot,omitempty"` // Preferred equip slot (armor|helmet|boots|belt|amulet|ring)
	Value       int    `yaml:"value,omitempty"`      // Gold value
	Rarity      string `yaml:"rarity,omitempty"`
	OpensMap    bool   `yaml:"opens_map,omitempty"` // Quest items that open the map overlay
	// Optional numeric stats to un-hardcode item effects
	ArmorClassBase            int `yaml:"armor_class_base,omitempty"`
	EnduranceScalingDivisor   int `yaml:"endurance_scaling_divisor,omitempty"`
	IntellectScalingDivisor   int `yaml:"intellect_scaling_divisor,omitempty"`
	PersonalityScalingDivisor int `yaml:"personality_scaling_divisor,omitempty"`
	BonusMight                int `yaml:"bonus_might,omitempty"`
	BonusIntellect            int `yaml:"bonus_intellect,omitempty"`
	BonusPersonality          int `yaml:"bonus_personality,omitempty"`
	BonusEndurance            int `yaml:"bonus_endurance,omitempty"`
	BonusAccuracy             int `yaml:"bonus_accuracy,omitempty"`
	BonusSpeed                int `yaml:"bonus_speed,omitempty"`
	BonusLuck                 int `yaml:"bonus_luck,omitempty"`
	// Optional consumable attributes
	HealBase             int  `yaml:"heal_base,omitempty"`
	HealEnduranceDivisor int  `yaml:"heal_endurance_divisor,omitempty"`
	SummonDistanceTiles  int  `yaml:"summon_distance_tiles,omitempty"`
	Revive               bool `yaml:"revive,omitempty"`
	FullHeal             bool `yaml:"full_heal,omitempty"`
}

func LoadItemConfig(filename string) (*ItemSystemConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var itemCfg ItemSystemConfig
	if err := yaml.Unmarshal(data, &itemCfg); err != nil {
		return nil, err
	}
	// Validate per-type required attributes for single source of truth
	if err := validateItemConfig(&itemCfg); err != nil {
		return nil, err
	}
	GlobalItems = &itemCfg
	return &itemCfg, nil
}

func MustLoadItemConfig(filename string) *ItemSystemConfig {
	cfg, err := LoadItemConfig(filename)
	if err != nil {
		panic("Failed to load item config: " + err.Error())
	}
	return cfg
}

// validateItemConfig enforces per-type required attributes for consumables
func validateItemConfig(cfg *ItemSystemConfig) error {
	for key, def := range cfg.Items {
		switch def.Type {
		case "consumable":
			// If heal_base is set, heal_endurance_divisor must be set and positive (unless revive)
			if def.HealBase > 0 && def.HealEnduranceDivisor <= 0 && !def.Revive {
				return fmt.Errorf("consumable '%s' missing heal_endurance_divisor", key)
			}
			if def.HealEnduranceDivisor > 0 && def.HealBase <= 0 && !def.Revive {
				return fmt.Errorf("consumable '%s' missing heal_base", key)
			}
			// If no known consumable attributes are present, warn but allow
			if def.HealBase == 0 && def.SummonDistanceTiles == 0 && !def.Revive {
				// Allow “vanilla” consumables for future behaviors; no hard error
			}
		}
	}
	return nil
}

func GetItemDefinition(itemKey string) (*ItemDefinitionConfig, bool) {
	if GlobalItems == nil {
		return nil, false
	}
	def, ok := GlobalItems.Items[itemKey]
	return def, ok
}

func GetItemDefinitionByName(name string) (*ItemDefinitionConfig, string, bool) {
	if GlobalItems == nil {
		return nil, "", false
	}
	for key, def := range GlobalItems.Items {
		if def.Name == name {
			return def, key, true
		}
	}
	return nil, "", false
}

// ---------------- Loot Tables ----------------

type LootTablesConfig struct {
	Loots map[string][]LootEntry `yaml:"loots"`
}

type LootEntry struct {
	Type   string  `yaml:"type"` // weapon|item
	Key    string  `yaml:"key"`
	Chance float64 `yaml:"chance"`
}

func LoadLootTables(filename string) (*LootTablesConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var loots LootTablesConfig
	if err := yaml.Unmarshal(data, &loots); err != nil {
		return nil, err
	}
	GlobalLoots = &loots
	return &loots, nil
}

func MustLoadLootTables(filename string) *LootTablesConfig {
	lt, err := LoadLootTables(filename)
	if err != nil {
		panic("Failed to load loot tables: " + err.Error())
	}
	return lt
}

func GetLootTable(monsterKey string) []LootEntry {
	if GlobalLoots == nil {
		return nil
	}
	return GlobalLoots.Loots[monsterKey]
}

// Helper functions for easy access to commonly used values
func (c *Config) GetScreenWidth() int {
	return c.Display.ScreenWidth
}

func (c *Config) GetScreenHeight() int {
	return c.Display.ScreenHeight
}

func (c *Config) GetTileSize() float64 {
	return float64(c.World.TileSize)
}

func (c *Config) GetMapWidth() int {
	return c.World.MapWidth
}

func (c *Config) GetMapHeight() int {
	return c.World.MapHeight
}

func (c *Config) GetMoveSpeed() float64 {
	return c.Movement.MoveSpeed
}

func (c *Config) GetRotSpeed() float64 {
	return c.Movement.RotationSpeed
}

func (c *Config) GetArrowSpeed() float64 {
	return c.Combat.Bow.Speed
}

func (c *Config) GetArrowLifetime() int {
	return c.Combat.Bow.Lifetime
}

func (c *Config) GetArrowCollisionSize() float64 {
	return float64(c.Combat.Bow.CollisionSize)
}

// Legacy weapon config methods removed - use YAML embedded configs

// GetSpellConfig retrieves spell combat configuration from embedded physics
func (c *Config) GetSpellConfig(spellType string) (*ProjectilePhysicsConfig, error) {
	if GlobalSpells == nil {
		return nil, fmt.Errorf("spell system not initialized")
	}
	spellDef, exists := GlobalSpells.Spells[spellType]
	if !exists {
		return nil, fmt.Errorf("spell '%s' not found in spells.yaml", spellType)
	}
	if spellDef.Physics == nil {
		return nil, fmt.Errorf("spell '%s' has no physics configuration", spellType)
	}
	return spellDef.Physics, nil
}

// Legacy weapon graphics config methods removed - use YAML embedded graphics

// GetSpellGraphicsConfig retrieves spell graphics configuration from embedded graphics
func (c *Config) GetSpellGraphicsConfig(spellType string) (*ProjectileRenderConfig, error) {
	if GlobalSpells == nil {
		return nil, fmt.Errorf("spell system not initialized")
	}
	spellDef, exists := GlobalSpells.Spells[spellType]
	if !exists {
		return nil, fmt.Errorf("spell '%s' not found in spells.yaml", spellType)
	}
	if spellDef.Graphics == nil {
		return nil, fmt.Errorf("spell '%s' has no graphics configuration", spellType)
	}
	return spellDef.Graphics, nil
}

// GetSpellDefinition retrieves spell definition from global spell config
func GetSpellDefinition(spellKey string) (*SpellDefinitionConfig, bool) {
	if GlobalSpells == nil {
		return nil, false
	}
	def, exists := GlobalSpells.Spells[spellKey]
	return def, exists
}

// GetSpellDefinitionByName retrieves spell definition by display name
func GetSpellDefinitionByName(name string) (*SpellDefinitionConfig, string, bool) {
	if GlobalSpells == nil {
		return nil, "", false
	}
	for key, def := range GlobalSpells.Spells {
		if def.Name == name {
			return def, key, true
		}
	}
	return nil, "", false
}

// GetAllSpellKeys returns all available spell keys
func GetAllSpellKeys() []string {
	if GlobalSpells == nil {
		return nil
	}
	keys := make([]string, 0, len(GlobalSpells.Spells))
	for key := range GlobalSpells.Spells {
		keys = append(keys, key)
	}
	return keys
}

// GetSpellsBySchool returns all spells for a given magic school
func GetSpellsBySchool(schoolKey string) []string {
	if GlobalSpells == nil {
		return nil
	}
	var spells []string
	for key, def := range GlobalSpells.Spells {
		if def.School == schoolKey {
			spells = append(spells, key)
		}
	}
	return spells
}

func (c *Config) GetCameraFOV() float64 {
	return c.Camera.FieldOfView
}

func (c *Config) GetViewDistance() float64 {
	return c.Camera.ViewDistance
}

// Convenience function for getting PI/3 (60 degrees) FOV
func (c *Config) GetDefaultFOV() float64 {
	return math.Pi / 3
}

// GetWeaponDefinition retrieves weapon definition from global weapon config
func GetWeaponDefinition(weaponKey string) (*WeaponDefinitionConfig, bool) {
	if GlobalWeapons == nil {
		return nil, false
	}
	def, exists := GlobalWeapons.Weapons[weaponKey]
	return def, exists
}

// GetWeaponDefinitionByName retrieves weapon definition by display name
func GetWeaponDefinitionByName(name string) (*WeaponDefinitionConfig, string, bool) {
	if GlobalWeapons == nil {
		return nil, "", false
	}
	for key, def := range GlobalWeapons.Weapons {
		if def.Name == name {
			return def, key, true
		}
	}
	return nil, "", false
}

// GetAllWeaponKeys returns all available weapon keys
func GetAllWeaponKeys() []string {
	if GlobalWeapons == nil {
		return nil
	}
	keys := make([]string, 0, len(GlobalWeapons.Weapons))
	for key := range GlobalWeapons.Weapons {
		keys = append(keys, key)
	}
	return keys
}

// GetWeaponsByCategory returns all weapons for a given category
func GetWeaponsByCategory(category string) []string {
	if GlobalWeapons == nil {
		return nil
	}
	var weapons []string
	for key, def := range GlobalWeapons.Weapons {
		if def.Category == category {
			weapons = append(weapons, key)
		}
	}
	return weapons
}
