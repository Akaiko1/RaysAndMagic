package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EffectLines returns user-facing description lines for the non-base
// special effects of a weapon (damage type override, stun, disintegrate,
// AoE splash, max airborne projectiles, per-monster bonus multipliers).
// Single source of truth — both the in-game tooltip and the map-viewer
// card pull from here so any new effect surfaces everywhere automatically.
//
// Base attributes (Damage, Range, BonusStat, CritChance) are shown
// separately by each consumer because their formatting differs — e.g.
// the in-game tooltip renders crit as "Critical Chance: total% (Base: X,
// Luck: +N)" using character context, while the map-viewer card shows
// the raw "Crit Chance: X%". Listing crit here too would render it
// twice in every in-game tooltip.
func (w *WeaponDefinitionConfig) EffectLines() []string {
	if w == nil {
		return nil
	}
	var lines []string
	if w.DamageType != "" && w.DamageType != "physical" {
		lines = append(lines, fmt.Sprintf("Damage Type: %s", titleCaseLower(w.DamageType)))
	}
	if w.StunChance > 0 {
		turns := w.StunTurns
		if turns <= 0 {
			turns = 1
		}
		lines = append(lines, fmt.Sprintf("Stun Chance: %.0f%% (%d turns)", w.StunChance*100, turns))
	}
	if w.DisintegrateChance > 0 {
		lines = append(lines, fmt.Sprintf("Disintegrate Chance: %.0f%%", w.DisintegrateChance*100))
	}
	if w.AoeRadiusTiles > 0 {
		lines = append(lines, fmt.Sprintf("AoE radius: %.1f tiles (splashes all nearby monsters)", w.AoeRadiusTiles))
	}
	if w.MaxProjectiles > 0 {
		lines = append(lines, fmt.Sprintf("Max Airborne: %d", w.MaxProjectiles))
	}
	if len(w.BonusVs) > 0 {
		keys := make([]string, 0, len(w.BonusVs))
		for k := range w.BonusVs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("Bonus vs %s: x%.1f", titleCaseLower(k), w.BonusVs[k]))
		}
	}
	return lines
}

func titleCaseLower(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Config holds all game configuration values
type Config struct {
	Display    DisplayConfig   `yaml:"display"`
	Engine     EngineConfig    `yaml:"engine"`
	World      WorldConfig     `yaml:"world"`
	Movement   MovementConfig  `yaml:"movement"`
	Camera     CameraConfig    `yaml:"camera"`
	UI         UIConfig        `yaml:"ui"`
	Characters CharacterConfig `yaml:"characters"`
	MonsterAI  MonsterAIConfig `yaml:"monster_ai"`
	Graphics   GraphicsConfig  `yaml:"graphics"`
	Tiles      TileConfig      `yaml:"tiles"`
}

type DisplayConfig struct {
	ScreenWidth       int    `yaml:"screen_width"`
	ScreenHeight      int    `yaml:"screen_height"`
	WindowTitle       string `yaml:"window_title"`
	Resizable         bool   `yaml:"resizable"`
	Fullscreen        bool   `yaml:"fullscreen"`
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
	// RunMultiplier scales real-time translation speed while the run key (Shift)
	// is held. <= 1 (or absent) falls back to RunMultiplierDefault.
	RunMultiplier float64 `yaml:"run_multiplier"`
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
	// lifetime = range / speed * tps (frames per second). Round (not truncate) so
	// the projectile travels as close to range_tiles as discrete frames allow —
	// truncation left a few weapons ~1 frame short of their stated range.
	return int((p.RangeTiles/p.SpeedTiles)*float64(GetTargetTPS()) + 0.5)
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

// WeaponGraphicsConfig for melee slash effects and projectile weapon rendering.
type WeaponGraphicsConfig struct {
	SlashColor  [3]int `yaml:"slash_color"`  // RGB color for slash effect
	SlashWidth  int    `yaml:"slash_width"`  // Width of slash line
	SlashLength int    `yaml:"slash_length"` // Length of slash line

	MaxSize  int    `yaml:"max_size"`
	MinSize  int    `yaml:"min_size"`
	BaseSize int    `yaml:"base_size"`
	Color    [3]int `yaml:"color"`
}

type CameraConfig struct {
	FieldOfView  float64 `yaml:"field_of_view"`
	ViewDistance float64 `yaml:"view_distance"`
}

type UIConfig struct {
	SpellInputCooldown  int `yaml:"spell_input_cooldown"`
	PartyPortraitHeight int `yaml:"party_portrait_height"`
	PartyPortraitWidth  int `yaml:"party_portrait_width"`
	CompassRadius       int `yaml:"compass_radius"`
	DamageBlinkFrames   int `yaml:"damage_blink_frames"`
}

type CharacterConfig struct {
	StartingGold  int                   `yaml:"starting_gold"`
	StartingFood  int                   `yaml:"starting_food"`
	HitPoints     HitPointsConfig       `yaml:"hit_points"`
	SpellPoints   SpellPointsConfig     `yaml:"spell_points"`
	Classes       map[string]ClassStats `yaml:"classes"`
	StartingParty []RosterEntry         `yaml:"starting_party,omitempty"`
	Captives      []RosterEntry         `yaml:"captives,omitempty"`
}

// RosterEntry defines one starting hero (active party or imprisoned captive).
// Class is a class key (knight/paladin/...); Name is the display name (and,
// lowercased, the portrait sprite key — falls back to the class sprite).
type RosterEntry struct {
	Name  string `yaml:"name"`
	Class string `yaml:"class"`
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
	Name            string `yaml:"name"`
	Description     string `yaml:"description"`
	School          string `yaml:"school"`
	Level           int    `yaml:"level"`
	SpellPointsCost int    `yaml:"spell_points_cost"`
	// CooldownSeconds is the real-time cast cooldown for this spell at the
	// reference Speed (see SpellCooldownSpeedRefSpeed); Speed scales it. 0 =
	// fall back to SpellCooldownDefaultSecondsForLevel(level).
	CooldownSeconds    float64 `yaml:"cooldown_seconds,omitempty"`
	Duration           int     `yaml:"duration"` // Duration in seconds (for buff spells)
	DisintegrateChance float64 `yaml:"disintegrate_chance,omitempty"`
	// AoeRadiusTiles, when > 0, makes a projectile spell splash damage to
	// every monster within this many tiles of the primary target. The splash
	// uses the same base damage as the direct hit (each splash victim still
	// applies its own armor reduction). Single source of truth for combat
	// math and tooltip text.
	AoeRadiusTiles float64 `yaml:"aoe_radius_tiles,omitempty"`
	ProjectileSize int     `yaml:"projectile_size"`
	IsProjectile   bool    `yaml:"is_projectile"`
	IsUtility      bool    `yaml:"is_utility"`
	StatusIcon     string  `yaml:"status_icon,omitempty"`
	// MonsterOnly spells are cast by monsters (as projectile_spell) but never
	// offered to the player — excluded from every learnable school list.
	MonsterOnly bool `yaml:"monster_only,omitempty"`

	// Damage-formula modifiers (data-driven; default behaviour when unset).
	DamageCostMultiplier  int  `yaml:"damage_cost_multiplier,omitempty"`  // base = cost × SpellDamagePerSP × this (default 1)
	ScalesWithPersonality bool `yaml:"scales_with_personality,omitempty"` // also add Personality/divisor to spell damage

	// AoE-stun effect (e.g. Darkness): when StunRadiusTiles > 0 the spell stuns
	// every monster within that radius of the caster — no damage. RT uses
	// seconds×TPS frames, TB uses StunDurationTurns.
	StunRadiusTiles     float64 `yaml:"stun_radius_tiles,omitempty"`
	StunDurationSeconds int     `yaml:"stun_duration_seconds,omitempty"`
	StunDurationTurns   int     `yaml:"stun_duration_turns,omitempty"`

	// DealsNoDamage zeroes the projectile's direct damage (e.g. Disintegrate,
	// whose only effect is its disintegrate_chance instakill roll on hit).
	DealsNoDamage bool `yaml:"deals_no_damage,omitempty"`

	// Party combat buffs (applied for `duration` seconds). Day of the Gods sets
	// ResistBuffPct; Hour of Power sets OutgoingDamageBonus + IncomingDamageReduction.
	ResistBuffPct           int `yaml:"resist_buff_pct,omitempty"`           // % reduction of all incoming party damage
	OutgoingDamageBonus     int `yaml:"outgoing_damage_bonus,omitempty"`     // flat add to all party outgoing damage
	IncomingDamageReduction int `yaml:"incoming_damage_reduction,omitempty"` // flat reduction of incoming damage (floors at 0)

	// Bind Undead: on hit, takes control of an UNDEAD target for the duration — it
	// hunts other monsters and ignores the party. No effect on non-undead. Dies
	// (party XP, no loot) when the party leaves the map.
	BindUndead          bool `yaml:"bind_undead,omitempty"`
	BindDurationSeconds int  `yaml:"bind_duration_seconds,omitempty"`

	// Resurrect: restores a fallen ally (incl. eradicated, unlike a revival
	// potion). FullHeal restores them to maximum HP.
	Revive   bool `yaml:"revive,omitempty"`
	FullHeal bool `yaml:"full_heal,omitempty"`
	// Raise Dead: revives the first fallen ally (Unconscious/Dead, NOT eradicated)
	// to this percentage of max HP. Distinct from Resurrect (Revive+FullHeal).
	ReviveHpPct int `yaml:"revive_hp_pct,omitempty"`

	// HealParty makes a heal_amount spell restore EVERY party member (Mass Heal),
	// not just the caster/target.
	HealParty bool `yaml:"heal_party,omitempty"`

	// StunChance (0..1): on a projectile hit, chance to also stun the struck
	// monster for StunDurationSeconds/Turns (Psychic Shock). Single-target.
	StunChance float64 `yaml:"stun_chance,omitempty"`

	// Charm: pacifies a LIVING target — it simply STOPS attacking (does not fight
	// others) and breaks free on any hit it takes. Undead-only Bind Undead is the
	// separate spell above; the two never mix.
	Pacify                bool `yaml:"pacify,omitempty"`
	PacifyDurationSeconds int  `yaml:"pacify_duration_seconds,omitempty"`

	// PartyAoeRadiusTiles > 0 makes the spell an instant nova centered on the party
	// that damages every monster AND every party member within the radius (Inferno).
	// Damage = SpellPointsCost × SpellDamagePerSP.
	PartyAoeRadiusTiles float64 `yaml:"party_aoe_radius_tiles,omitempty"`

	// StarburstFx triggers the falling-star impact VFX: a star drops into each tile
	// within the spell's AoE radius (Starburst). Purely visual; damage uses AoeRadiusTiles.
	StarburstFx bool `yaml:"starburst_fx,omitempty"`

	// Persistent damage zone (Hot Steam): on cast, spawns a fixed zone of
	// ZoneRadiusTiles centered on the party that lasts `duration` seconds and deals
	// ZoneTickDamage to monsters inside it — every turn in TB, every ZoneTickSeconds in RT.
	ZoneRadiusTiles float64 `yaml:"zone_radius_tiles,omitempty"`
	ZoneTickDamage  int     `yaml:"zone_tick_damage,omitempty"`
	ZoneTickSeconds float64 `yaml:"zone_tick_seconds,omitempty"`

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
}

type MonsterAIConfig struct {
	// AI behavior timers (in frames, 60fps)
	IdlePatrolTimer int `yaml:"idle_patrol_timer"`
	PatrolIdleTimer int `yaml:"patrol_idle_timer"`
	AttackCooldown  int `yaml:"attack_cooldown"`
	FleeDuration    int `yaml:"flee_duration"`

	// Behavior chance (0.0 to 1.0)
	IdleToPatrolChance float64 `yaml:"idle_to_patrol_chance"`

	// Movement parameters
	NormalSpeedMultiplier float64 `yaml:"normal_speed_multiplier"`
	FleeSpeedMultiplier   float64 `yaml:"flee_speed_multiplier"`

	// Vision distance used while fleeing
	FleeVisionDistance float64 `yaml:"flee_vision_distance"`

	// AI frequency check (in frames)
	PathCheckFrequency int `yaml:"path_check_frequency"`

	// Detection tuning. Distances are in tiles; the rest are multipliers.
	DefaultAlertRadiusTiles      float64 `yaml:"default_alert_radius_tiles"`      // fallback when a monster omits alert_radius
	AlertOutsideTetherMultiplier float64 `yaml:"alert_outside_tether_multiplier"` // wider detection when lured away from spawn
	AlertLosBlockedMultiplier    float64 `yaml:"alert_los_blocked_multiplier"`    // reduced detection through trees/walls
	DisengageDistanceMultiplier  float64 `yaml:"disengage_distance_multiplier"`   // lose engagement at detection × this (hysteresis)
	AttackEnterRangeFraction     float64 `yaml:"attack_enter_range_fraction"`     // enter attack at ≤ range × this (exit at > range)

	// Flee cycle: after this many consecutive attacks, roll this chance to flee
	FleeAfterAttacks       int     `yaml:"flee_after_attacks"`
	FleeAfterAttacksChance float64 `yaml:"flee_after_attacks_chance"`
}

type GraphicsConfig struct {
	RaysPerScreenWidth int                  `yaml:"rays_per_screen_width"`
	Colors             ColorsConfig         `yaml:"colors"`
	Sprite             SpriteConfig         `yaml:"sprite"`
	BrightnessMin      float64              `yaml:"brightness_min"`
	Monster            MonsterRenderConfig  `yaml:"monster"`
	NPC                NPCRenderConfig      `yaml:"npc"`
	ImpassableAura     ImpassableAuraConfig `yaml:"impassable_aura"`
	ColorKey           ColorKeyConfig       `yaml:"color_key"`
	// Standee renders monsters, NPCs and scenery objects as flat two-sided
	// tokens standing in the world (board-game standees) instead of
	// camera-facing billboards. Monsters turn with their travel direction.
	Standee StandeeConfig `yaml:"standee"`
}

// StandeeConfig tunes the board-game standee rendering mode.
type StandeeConfig struct {
	Enabled bool `yaml:"enabled"`
	// ThicknessTiles is the token slab's thickness in tiles: the gap between
	// the front and back sticker faces, with the core layer visible between
	// them at viewing angles.
	ThicknessTiles float64 `yaml:"thickness_tiles"`
	// TurnSpeedDegPerSec caps how fast a monster token turns toward its travel
	// direction (degrees per second), so tokens swivel smoothly instead of
	// snapping.
	TurnSpeedDegPerSec float64 `yaml:"turn_speed_deg_per_sec"`
	// NPCSpinDegPerSec slowly spins NPC tokens (people, statues, valves,
	// buildings) in place, showcase-style; 0 disables. Scenery tile tokens
	// (grass, trees, rocks) never spin.
	NPCSpinDegPerSec float64 `yaml:"npc_spin_deg_per_sec"`
	// EnvFaceDegPerSec makes scenery tile tokens (grass, ferns, rocks) slowly
	// turn to face the camera at this rate; 0 keeps them at a fixed diagonal.
	EnvFaceDegPerSec float64 `yaml:"env_face_deg_per_sec"`
	// MinViewAngleDeg keeps a monster token's plane at least this far from the
	// camera's sight line, so a monster crossing the view never degenerates
	// into an invisible edge-on sliver. 0 disables the clamp.
	MinViewAngleDeg float64 `yaml:"min_view_angle_deg"`
}

// ColorKeyConfig makes a key color (default magenta) transparent at sprite load,
// clearing stray magenta from imperfect background removal.
type ColorKeyConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Color     [3]int `yaml:"color"`     // RGB of the key color; [0,0,0]/absent → magenta (255,0,255)
	Tolerance int    `yaml:"tolerance"` // per-channel max abs difference for the transparent core (0 = exact)
	Despill   bool   `yaml:"despill"`   // fringe pixels: subtract the cast, keep the base tone opaque
}

// ImpassableAuraConfig tunes the rising "bubble" particles drawn along the
// ground edges of impassable billboard tiles (rocks/cliffs) so the player can
// tell which tiles block movement. Zero/absent numeric fields fall back to
// in-code defaults; Enabled defaults off unless set in config.yaml.
type ImpassableAuraConfig struct {
	Enabled        bool    `yaml:"enabled"`
	RadiusTiles    int     `yaml:"radius_tiles"`     // scan radius around the camera, in tiles
	BubblesPerEdge int     `yaml:"bubbles_per_edge"` // particle columns per walkable-facing edge
	Alpha          float64 `yaml:"alpha"`            // base glow alpha (0..1)
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
	// MaxSpriteSize bounds the PERSPECTIVE-SCALED COLLISION boxes in combat
	// (projectile hits); rendering is uncapped — a render-side pixel cap makes
	// sprites sink at close range as the floor anchor outgrows the capped size.
	MaxSpriteSize          int `yaml:"max_sprite_size"`
	MinSpriteSize          int `yaml:"min_sprite_size"`
	SizeDistanceMultiplier int `yaml:"size_distance_multiplier"`
}

type NPCRenderConfig struct {
	MinSpriteSize          int `yaml:"min_sprite_size"`
	SizeDistanceMultiplier int `yaml:"size_distance_multiplier"`
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

type TileLightConfig struct {
	Enabled     bool    `yaml:"enabled"`
	RadiusTiles float64 `yaml:"radius_tiles"`
	Intensity   float64 `yaml:"intensity"`
}

type TileData struct {
	Name             string  `yaml:"name"`
	Type             string  `yaml:"type,omitempty"`
	Solid            bool    `yaml:"solid"`
	Transparent      bool    `yaml:"transparent"`
	Walkable         bool    `yaml:"walkable"`
	HeightMultiplier float64 `yaml:"height_multiplier"`
	Sprite           string  `yaml:"sprite"`
	RenderType       string  `yaml:"render_type"`
	FloorColor       [3]int  `yaml:"floor_color"`
	FloorNearColor   [3]int  `yaml:"floor_near_color"`
	// FloorTextureGroup selects which named group from the current biome's
	// floor_texture_groups (see BiomeConfig) supplies the floor texture for
	// this tile type. Empty = no texture overlay (renderer falls back to base
	// color). The "beach" group is picked dynamically for empty tiles
	// bordering water — see the renderer.
	FloorTextureGroup string   `yaml:"floor_texture_group,omitempty"`
	WallColor         [3]int   `yaml:"wall_color"`
	Letter            string   `yaml:"letter"`
	Biomes            []string `yaml:"biomes,omitempty"`
	// ImpassableAura forces the rising "impassable" bubble glow on a FLOOR tile
	// (render_type floor_only) that blocks movement but reads like walkable
	// ground — e.g. a chasm pit. Wall/billboard blockers get the aura
	// automatically; ordinary impassable floors (water) leave this false.
	ImpassableAura      bool                   `yaml:"impassable_aura,omitempty"`
	Light               *TileLightConfig       `yaml:"light,omitempty"`
	AlphaFromBrightness float64                `yaml:"alpha_from_brightness,omitempty"`
	Properties          map[string]interface{} `yaml:"properties,omitempty"`
}

type SpecialTileConfig struct {
	SpecialTileData map[string]TileData `yaml:"special_tiles"`
}

type MapConfig struct {
	Name              string `yaml:"name"`
	File              string `yaml:"file"`
	Biome             string `yaml:"biome"`
	SkyColor          [3]int `yaml:"sky_color"`
	SkyTexture        string `yaml:"sky_texture,omitempty"`
	DefaultFloorColor [3]int `yaml:"default_floor_color"`
	// AmbientLight scales the map's base (distance) brightness: 1.0 (or absent)
	// = normal daylight, low values make a dungeon genuinely dark so torch
	// light and spell glow become essential. Point lights add on top.
	AmbientLight float64 `yaml:"ambient_light,omitempty"`
	// ClearEncounter: a single map-wide encounter — ALL monsters on the map
	// share it and the reward fires when the last one dies.
	ClearEncounter *MapClearEncounterConfig `yaml:"clear_encounter,omitempty"`
	// ClearEncounters: multiple independent encounters on one map. Each
	// monster is assigned to the encounter whose treasure-chest tile is
	// nearest, so spatially-clustered groups (e.g. bandits per oasis) each
	// trigger their own reward. Takes precedence over ClearEncounter.
	ClearEncounters []MapClearEncounterConfig `yaml:"clear_encounters,omitempty"`
}

// BiomeConfig holds the data-driven appearance shared by every map of a
// biome. Floor textures live here (keyed by group name; tiles select a
// group via TileData.FloorTextureGroup) so all maps of the same biome
// render identical ground without re-declaring texture lists per map.
type BiomeConfig struct {
	FloorTextureGroups map[string][]string `yaml:"floor_texture_groups,omitempty"`
}

type MapClearEncounterConfig struct {
	Rewards *MapEncounterRewardsConfig `yaml:"rewards,omitempty"`
	// Monsters declares which pre-placed map monsters belong to this
	// encounter (by type + count). At load the engine binds the `count`
	// monsters of each `type` nearest to this encounter's chest. Used only
	// by the multi-encounter `clear_encounters` form; the single
	// `clear_encounter` binds every monster on the map.
	Monsters []MapEncounterMonsterReq `yaml:"monsters,omitempty"`
}

// MapEncounterMonsterReq binds a count of a monster type to an encounter.
type MapEncounterMonsterReq struct {
	Type  string `yaml:"type"`
	Count int    `yaml:"count"`
}

type MapEncounterRewardsConfig struct {
	Gold              int                            `yaml:"gold"`
	Experience        int                            `yaml:"experience"`
	CompletionMessage string                         `yaml:"completion_message,omitempty"`
	TreasureChest     *MapTreasureChestRewardConfig  `yaml:"treasure_chest,omitempty"`
	TreasureChests    []MapTreasureChestRewardConfig `yaml:"treasure_chests,omitempty"`
}

type MapTreasureChestRewardConfig struct {
	ID                string   `yaml:"id,omitempty"`
	Map               string   `yaml:"map,omitempty"`
	TileX             int      `yaml:"tile_x"`
	TileY             int      `yaml:"tile_y"`
	Sprite            string   `yaml:"sprite,omitempty"`
	SizeMultiplier    float64  `yaml:"size_multiplier,omitempty"` // Visual scale; defaults to 0.45 if unset
	RandomWeaponCount int      `yaml:"random_weapon_count,omitempty"`
	Items             []string `yaml:"items,omitempty"`
	Weapons           []string `yaml:"weapons,omitempty"`
	Gold              int      `yaml:"gold,omitempty"`
	CompletionMessage string   `yaml:"completion_message,omitempty"`
}

type MapConfigs struct {
	Maps   map[string]MapConfig   `yaml:"maps"`
	Biomes map[string]BiomeConfig `yaml:"biomes"`
}

// WeaponSystemConfig contains the complete weapon system configuration
type WeaponSystemConfig struct {
	Weapons map[string]*WeaponDefinitionConfig `yaml:"weapons"`
	// WeaponCooldownMultipliers is the per-weapon-TYPE real-time attack-cooldown
	// multiplier (1.0 = baseline sword), keyed by the canonical weapon-skill
	// noun (sword/dagger/axe/spear/bow/mace/staff). Types not listed default to
	// 1.0. A single weapon may override via its own `cooldown_multiplier`.
	WeaponCooldownMultipliers map[string]float64 `yaml:"weapon_cooldown_multipliers"`
}

// WeaponCooldownMultiplierForSkill returns the attack-cooldown multiplier for a
// weapon-skill noun (see SkillType.WeaponNoun), or 1.0 if unset/unknown — so a
// weapon whose category maps to no skill (e.g. the alien blaster) is neutral.
func WeaponCooldownMultiplierForSkill(skillNoun string) float64 {
	if GlobalWeapons != nil {
		if m, ok := GlobalWeapons.WeaponCooldownMultipliers[skillNoun]; ok && m > 0 {
			return m
		}
	}
	return 1.0
}

// WeaponDefinitionConfig represents a complete weapon definition with embedded physics and graphics
type WeaponDefinitionConfig struct {
	// Basic weapon properties
	Name               string  `yaml:"name"`
	Description        string  `yaml:"description"`
	Category           string  `yaml:"category"`
	Damage             int     `yaml:"damage"`
	Range              int     `yaml:"range"` // Range in tiles (for melee reach)
	BonusStat          string  `yaml:"bonus_stat"`
	BonusStatSecondary string  `yaml:"bonus_stat_secondary"`
	DamageType         string  `yaml:"damage_type"`
	MaxProjectiles     int     `yaml:"max_projectiles"`
	CritChance         int     `yaml:"crit_chance"`
	StunChance         float64 `yaml:"stun_chance"`
	StunTurns          int     `yaml:"stun_turns"`
	DisintegrateChance float64 `yaml:"disintegrate_chance,omitempty"`
	// AoeRadiusTiles, when > 0, makes the weapon's projectile splash damage
	// to every other monster within this radius (in tiles) of the primary
	// hit. Same semantics as the spell field of the same name: splash uses
	// the base damage, applies the victim's armor reduction, and skips
	// crits/disintegrate/stun.
	AoeRadiusTiles float64            `yaml:"aoe_radius_tiles,omitempty"`
	Rarity         string             `yaml:"rarity"`
	Value          int                `yaml:"value,omitempty"`
	BonusVs        map[string]float64 `yaml:"bonus_vs,omitempty"`
	// CooldownMultiplier overrides the weapon's category attack-cooldown
	// multiplier in real time (the per-skill defaults live in weapons.yaml
	// `weapon_cooldown_multipliers`, read via WeaponCooldownMultiplierForSkill).
	// Used for legendaries with a bespoke cadence (e.g. Bow of Hellfire = 1.7, slow).
	// 0 = use the category default.
	CooldownMultiplier float64 `yaml:"cooldown_multiplier,omitempty"`
	// SpellCooldownMultiplier, when > 0, scales the cooldown of EVERY spell the
	// wielder casts while this weapon is in their main hand (e.g. Archmage
	// Staff = 0.8 → −20% spell cooldown). 0 = no effect.
	SpellCooldownMultiplier float64 `yaml:"spell_cooldown_multiplier,omitempty"`
	// ProjectileSchool, when set ("arcane"/"dark"/...), makes a ranged weapon's
	// projectile render as a glowing spell-style orb of that school instead of a
	// plain arrow. Cosmetic only; damage stays weapon-based.
	ProjectileSchool string `yaml:"projectile_school,omitempty"`

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
	if err := validateWeaponConfig(&weaponConfig); err != nil {
		return nil, err
	}

	// Set global weapon config for easy access
	GlobalWeapons = &weaponConfig

	// Pre-compute display-name index so GetWeaponDefinitionByName is O(1).
	weaponDefByName = make(map[string]*WeaponDefinitionConfig, len(weaponConfig.Weapons))
	weaponKeyByName = make(map[string]string, len(weaponConfig.Weapons))
	for key, def := range weaponConfig.Weapons {
		weaponDefByName[def.Name] = def
		weaponKeyByName[def.Name] = key
	}

	// Set up weapon accessor for items package to avoid circular imports
	setupWeaponAccessor()

	return &weaponConfig, nil
}

// weapon name → definition / yaml key, populated at config load.
var (
	weaponDefByName map[string]*WeaponDefinitionConfig
	weaponKeyByName map[string]string
)

// setupWeaponAccessor configures the global weapon accessor for items package
func setupWeaponAccessor() {
	// This will be imported by items package
	// For now we'll define this in a separate function
}

func validateWeaponConfig(cfg *WeaponSystemConfig) error {
	for key, def := range cfg.Weapons {
		if def == nil {
			return fmt.Errorf("weapon '%s' has empty definition", key)
		}
		if isProjectileWeapon(def) {
			if def.Physics == nil {
				return fmt.Errorf("projectile weapon '%s' missing physics configuration", key)
			}
			if def.Graphics == nil || def.Graphics.BaseSize <= 0 || def.Graphics.MaxSize <= 0 || def.Graphics.MinSize <= 0 {
				return fmt.Errorf("projectile weapon '%s' missing projectile graphics configuration", key)
			}
		} else {
			if def.Physics != nil {
				return fmt.Errorf("melee weapon '%s' should not define projectile physics", key)
			}
			if def.Melee == nil {
				return fmt.Errorf("melee weapon '%s' missing melee configuration", key)
			}
			if def.Graphics == nil || def.Graphics.SlashWidth <= 0 || def.Graphics.SlashLength <= 0 {
				return fmt.Errorf("melee weapon '%s' missing melee graphics configuration", key)
			}
		}
	}
	return nil
}

func isProjectileWeapon(def *WeaponDefinitionConfig) bool {
	category := strings.ToLower(strings.TrimSpace(def.Category))
	return def.Range > 3 ||
		strings.Contains(category, "bow") ||
		strings.Contains(category, "throwing") ||
		strings.Contains(category, "blaster")
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
	Name         string `yaml:"name"`
	Type         string `yaml:"type"` // armor|accessory|consumable|quest
	ArmorType    string `yaml:"armor_category,omitempty"`
	Description  string `yaml:"description"`          // Gameplay-neutral summary (optional)
	Flavor       string `yaml:"flavor,omitempty"`     // Short artistic line for tooltip
	EquipSlot    string `yaml:"equip_slot,omitempty"` // Preferred equip slot (armor|helmet|boots|belt|amulet|ring)
	Value        int    `yaml:"value,omitempty"`      // Gold value
	Rarity       string `yaml:"rarity,omitempty"`
	OpensMap     bool   `yaml:"opens_map,omitempty"`     // Quest items that open the map overlay
	PromotesLich bool   `yaml:"promotes_lich,omitempty"` // using this item offers a member the Lich path
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
	// Per-school % damage resistance the wearer gains (e.g. {fire: 100}); physical stacks after armor.
	Resistances map[string]int `yaml:"resistances,omitempty"`
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

// RunMultiplierDefault is the fallback run/sprint speed multiplier when the
// config omits (or zeroes) movement.run_multiplier.
const RunMultiplierDefault = 2.0

// GetRunMultiplier returns the real-time sprint multiplier (Shift held),
// falling back to RunMultiplierDefault when unset.
func (c *Config) GetRunMultiplier() float64 {
	if c.Movement.RunMultiplier > 1 {
		return c.Movement.RunMultiplier
	}
	return RunMultiplierDefault
}

func (c *Config) GetRotSpeed() float64 {
	return c.Movement.RotationSpeed
}

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

// GetSpellsBySchool returns all spells for a given magic school
func GetSpellsBySchool(schoolKey string) []string {
	if GlobalSpells == nil {
		return nil
	}
	var spells []string
	for key, def := range GlobalSpells.Spells {
		if def.School == schoolKey && !def.MonsterOnly {
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

// GetWeaponDefinition retrieves weapon definition from global weapon config
func GetWeaponDefinition(weaponKey string) (*WeaponDefinitionConfig, bool) {
	if GlobalWeapons == nil {
		return nil, false
	}
	def, exists := GlobalWeapons.Weapons[weaponKey]
	return def, exists
}

// GetWeaponDefinitionByName retrieves weapon definition by display name in O(1).
func GetWeaponDefinitionByName(name string) (*WeaponDefinitionConfig, string, bool) {
	def, ok := weaponDefByName[name]
	if !ok {
		return nil, "", false
	}
	return def, weaponKeyByName[name], true
}
