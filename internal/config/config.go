package config

import (
	"math"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all game configuration values
type Config struct {
	Display         DisplayConfig         `yaml:"display"`
	World           WorldConfig           `yaml:"world"`
	Movement        MovementConfig        `yaml:"movement"`
	Combat          CombatConfig          `yaml:"combat"`
	Camera          CameraConfig          `yaml:"camera"`
	UI              UIConfig              `yaml:"ui"`
	WorldGeneration WorldGenerationConfig `yaml:"world_generation"`
	Monsters        MonsterConfig         `yaml:"monsters"`
	Characters      CharacterConfig       `yaml:"characters"`
	Spells          SpellConfig           `yaml:"spells"`
	MonsterAI       MonsterAIConfig       `yaml:"monster_ai"`
	SkillTeaching   SkillTeachingConfig   `yaml:"skill_teaching"`
	Graphics        GraphicsConfig        `yaml:"graphics"`
	Tiles           TileConfig            `yaml:"tiles"`
}

type DisplayConfig struct {
	ScreenWidth  int    `yaml:"screen_width"`
	ScreenHeight int    `yaml:"screen_height"`
	WindowTitle  string `yaml:"window_title"`
	Resizable    bool   `yaml:"resizable"`
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
	Fireball    FireballConfig    `yaml:"fireball"`
	SwordAttack SwordAttackConfig `yaml:"sword_attack"`
}

type FireballConfig struct {
	Speed     float64 `yaml:"speed"`
	Lifetime  int     `yaml:"lifetime"`
	HitRadius int     `yaml:"hit_radius"`
}

type SwordAttackConfig struct {
	Speed     float64 `yaml:"speed"`
	Lifetime  int     `yaml:"lifetime"`
	HitRadius int     `yaml:"hit_radius"`
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

type SpellConfig struct {
	FireballCost   int `yaml:"fireball_cost"`
	HealCost       int `yaml:"heal_cost"`
	HealBaseAmount int `yaml:"heal_base_amount"`
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

	// Legacy fields (kept for backward compatibility)
	IdleTimeMin      int     `yaml:"idle_time_min"`
	IdleTimeMax      int     `yaml:"idle_time_max"`
	PatrolTimeMin    int     `yaml:"patrol_time_min"`
	PatrolTimeMax    int     `yaml:"patrol_time_max"`
	AlertTimeMin     int     `yaml:"alert_time_min"`
	AlertTimeMax     int     `yaml:"alert_time_max"`
	FleeTimeMin      int     `yaml:"flee_time_min"`
	FleeTimeMax      int     `yaml:"flee_time_max"`
	AttackDistance   float64 `yaml:"attack_distance"`
	PushbackDistance float64 `yaml:"pushback_distance"`
	TrollRegenChance float64 `yaml:"troll_regen_chance"`
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
	Fireball    ProjectileRenderConfig `yaml:"fireball"`
	SwordAttack ProjectileRenderConfig `yaml:"sword_attack"`
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
	Name             string  `yaml:"name"`
	Solid            bool    `yaml:"solid"`
	Transparent      bool    `yaml:"transparent"`
	Walkable         bool    `yaml:"walkable"`
	HeightMultiplier float64 `yaml:"height_multiplier"`
	Sprite           string  `yaml:"sprite"`
	RenderType       string  `yaml:"render_type"`
	FloorColor       [3]int  `yaml:"floor_color"`
	FloorNearColor   [3]int  `yaml:"floor_near_color"`
	WallColor        [3]int  `yaml:"wall_color"`
	Letter           string  `yaml:"letter"`
}

var GlobalConfig *Config

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

func (c *Config) GetFireballSpeed() float64 {
	return c.Combat.Fireball.Speed
}

func (c *Config) GetFireballLifetime() int {
	return c.Combat.Fireball.Lifetime
}

func (c *Config) GetFireballHitRadius() float64 {
	return float64(c.Combat.Fireball.HitRadius)
}

func (c *Config) GetSwordAttackSpeed() float64 {
	return c.Combat.SwordAttack.Speed
}

func (c *Config) GetSwordAttackLifetime() int {
	return c.Combat.SwordAttack.Lifetime
}

func (c *Config) GetSwordHitRadius() float64 {
	return float64(c.Combat.SwordAttack.HitRadius)
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
