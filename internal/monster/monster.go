package monster

import (
	"math"
	"math/rand"
	"strconv"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// EncounterRewards represents rewards for completing an encounter
type EncounterRewards struct {
	Gold              int                  `yaml:"gold"`
	Experience        int                  `yaml:"experience"`
	CompletionMessage string               `yaml:"completion_message"`       // Configurable message shown when encounter is completed
	TreasureChest     *TreasureChestReward `yaml:"treasure_chest,omitempty"` // Optional chest spawned when encounter is completed
	QuestID           string               `yaml:"-"`                        // Quest ID linked to this encounter (set at runtime, not from YAML)
}

// TreasureChestReward describes a chest spawned after an encounter is cleared.
type TreasureChestReward struct {
	ID                string  `yaml:"id,omitempty"`
	Map               string  `yaml:"map,omitempty"`
	TileX             int     `yaml:"tile_x"`
	TileY             int     `yaml:"tile_y"`
	Sprite            string  `yaml:"sprite,omitempty"`
	SizeMultiplier    float64 `yaml:"size_multiplier,omitempty"`
	RandomWeaponCount int     `yaml:"random_weapon_count,omitempty"`
	Gold              int     `yaml:"gold,omitempty"`
	CompletionMessage string  `yaml:"completion_message,omitempty"`
}

// Global counter for unique monster IDs
var nextMonsterID int = 1

// generateUniqueMonsterID creates a unique ID for a monster
func generateUniqueMonsterID() string {
	id := "monster_" + strconv.Itoa(nextMonsterID)
	nextMonsterID++
	return id
}

type Monster3D struct {
	X, Y         float64
	Name         string
	Key          string // YAML monster key (e.g. "bandit"); used to match encounter monster requirements
	MonsterType  string // creature category from YAML, e.g. "undead" (empty = generic)
	Level        int
	HitPoints    int
	MaxHitPoints int
	ArmorClass   int
	PerfectDodge int // Chance (0-100) to completely avoid an attack
	Experience   int
	ID           string // Unique identifier for collision tracking

	// Combat stats
	DamageMin int
	DamageMax int
	// Light emission (torch-like)
	LightRadius    float64
	LightIntensity float64

	// AI behavior
	State        MonsterState
	AlertRadius  float64
	AttackRadius float64
	Speed        float64
	Direction    float64
	StateTimer   int
	AttackCount  int // Number of attacks made in current engagement

	// Pathfinding state - prevents oscillation when stuck between obstacles
	LastChosenDir float64 // Last direction chosen by pathfinding
	StuckCounter  int     // Counts consecutive frames where monster couldn't move
	LastX, LastY  float64 // Position last frame to detect stuck state
	LastMoveTick  int64   // Last game frame when the monster moved (for animations)

	// Pursuit pathfinding state (tile-based A*)
	PathTiles        []TileCoord
	PathIndex        int
	PathTargetTileX  int
	PathTargetTileY  int
	LastPathCalcTick int
	pathScratch      pathScratch

	// RT movement target selection for non-pursuit movement (patrol/flee)
	MoveTargetTileX int
	MoveTargetTileY int
	MoveTargetState MonsterState
	HasMoveTarget   bool

	// Tethering system - monsters stay within 3 tiles of spawn unless engaging player
	SpawnX, SpawnY           float64 // Original spawn position
	TetherRadius             float64 // Maximum distance from spawn point (default 4 tiles = 256 pixels)
	IsEngagingPlayer         bool    // True when actively pursuing/fighting player
	WasAttacked              bool    // True when monster was hit - prevents disengagement
	HitTintFrames            int     // Frames remaining for red hit tint
	AttackAnimFrames         int     // Frames remaining for attack animation (TB mode)
	StunTurnsRemaining       int     // Turn-based stun duration (monster skips turns)
	StunFramesRemaining      int     // Real-time stun duration in frames
	Charmed                  bool    // Bound to the party (bind_undead): fights other monsters, ignores party
	CharmFramesRemaining     int     // Real-time charm duration in frames (0 in TB = lasts the encounter)
	CharmAttackCD            int     // RT cadence counter between charmed attacks
	Flying                   bool    // Whether the monster should be rendered above ground
	RangedAttackRange        float64 // Optional ranged attack range override (pixels)
	AttacksPerRound          int     // Turn-based melee attacks per monster round
	PassiveUntilAttacked     bool    // True when the monster should not aggro until hit
	AttackCooldownMultiplier float64 // Real-time attack cooldown multiplier (0.5 = twice as often)
	FireburstChance          float64 // Chance to cast fireburst instead of normal attack
	FireburstDamageMin       int     // Fireburst damage min
	FireburstDamageMax       int     // Fireburst damage max
	PoisonChance             float64 // Chance to apply poison on hit
	PoisonDurationSec        int     // Poison duration in seconds

	// Loot
	Gold  int
	Items []items.Item

	// Resistances and immunities
	Resistances map[DamageType]int

	// Habitat preferences - tiles this monster can walk on even if normally blocked
	HabitatPrefs []string

	// Ranged attack configuration
	ProjectileSpell  string
	ProjectileWeapon string

	// Pounce/leap: PounceRangePixels > 0 enables it. Runtime cooldowns are
	// tracked separately per mode (frames in real-time, turns in turn-based).
	PounceRangePixels     float64
	PounceCooldownSeconds float64
	PounceCDFrames        int // real-time cooldown countdown (frames)
	PounceCDTurns         int // turn-based cooldown countdown (turns)

	// Encounter system
	IsEncounterMonster bool              // True if this monster is part of an encounter
	EncounterRewards   *EncounterRewards // Rewards for defeating this encounter monster

	// Configuration reference
	config *config.Config
}

// NewMonster3DFromConfig creates a monster from YAML configuration
func NewMonster3DFromConfig(x, y float64, monsterKey string, cfg *config.Config) *Monster3D {
	if MonsterConfig == nil {
		panic("Monster configuration not loaded. Call monster.MustLoadMonsterConfig() first.")
	}

	def, err := MonsterConfig.GetMonsterByKey(monsterKey)
	if err != nil {
		panic("Monster not found in config: " + monsterKey + ". Check assets/monsters.yaml")
	}

	monster := &Monster3D{
		X:           x,
		Y:           y,
		Key:         monsterKey,
		State:       StatePatrolling, // Start patrolling immediately to avoid "vibing" at spawn
		Speed:       1.0,
		Direction:   rand.Float64() * 2 * math.Pi,
		StateTimer:  0,
		Resistances: make(map[DamageType]int),
		config:      cfg,
		ID:          generateUniqueMonsterID(), // Assign unique ID

		// Initialize tethering system
		SpawnX:           x,
		SpawnY:           y,
		TetherRadius:     256.0, // 4 tiles * 64 pixels per tile
		IsEngagingPlayer: false,
	}

	// Setup from YAML config
	monster.SetupMonsterFromConfig(def)
	monster.HitPoints = monster.MaxHitPoints

	return monster
}

func (m *Monster3D) TakeDamage(damage int, damageType DamageType, playerX, playerY float64) int {
	// Apply resistance
	if resistance, exists := m.Resistances[damageType]; exists {
		damage = damage * (100 - resistance) / 100
		if damage < 0 {
			damage = 0
		}
	}

	// Apply damage
	m.HitPoints -= damage
	if m.HitPoints < 0 {
		m.HitPoints = 0
	}

	// Mark as attacked - prevents AI from disengaging due to distance
	m.WasAttacked = true

	// If already engaging player, just return damage (don't change AI state)
	if m.IsEngagingPlayer {
		return damage
	}

	// Being attacked always triggers engagement - chase the attacker
	m.IsEngagingPlayer = true
	m.State = StateAlert
	m.StateTimer = 0

	return damage
}

func (m *Monster3D) IsAlive() bool {
	return m.HitPoints > 0
}

func (m *Monster3D) GetAttackDamage() int {
	if m.DamageMin >= m.DamageMax {
		return m.DamageMin
	}
	return m.DamageMin + rand.Intn(m.DamageMax-m.DamageMin+1)
}

func (m *Monster3D) GetTurnBasedAttackCount() int {
	if m.AttacksPerRound < 1 {
		return 1
	}
	return m.AttacksPerRound
}

func (m *Monster3D) HasRangedAttack() bool {
	return m.ProjectileSpell != "" || m.ProjectileWeapon != ""
}

// CanPounce reports whether the monster has the leap ability configured.
func (m *Monster3D) CanPounce() bool {
	return m.PounceRangePixels > 0
}

// GetAttackRangePixels returns the effective attack range in pixels.
// For ranged monsters, this uses the projectile range from config when available.
func (m *Monster3D) GetAttackRangePixels() float64 {
	attackRange := m.AttackRadius
	if !m.HasRangedAttack() || m.config == nil {
		return attackRange
	}

	if m.RangedAttackRange > 0 {
		return m.RangedAttackRange
	}

	tileSize := m.config.GetTileSize()

	if m.ProjectileSpell != "" {
		if physics, err := m.config.GetSpellConfig(m.ProjectileSpell); err == nil && physics != nil {
			if physics.RangeTiles > 0 {
				rangePixels := physics.RangeTiles * tileSize
				if rangePixels > attackRange {
					attackRange = rangePixels
				}
			}
		}
	}

	if m.ProjectileWeapon != "" {
		if def, exists := config.GetWeaponDefinition(m.ProjectileWeapon); exists && def.Physics != nil {
			if def.Physics.RangeTiles > 0 {
				rangePixels := def.Physics.RangeTiles * tileSize
				if rangePixels > attackRange {
					attackRange = rangePixels
				}
			}
		}
	}

	return attackRange
}

func (m *Monster3D) GetSpriteType() string {
	// Resolve by the monster's own KEY (always set by NewMonster3DFromConfig),
	// never by name: several monsters can share a display Name (the 4 elemental
	// dragons are all "Dragon"), and a name scan returns a random match per call
	// (Go map order) — which made dragons flicker through every color each frame.
	if MonsterConfig != nil {
		if def, err := MonsterConfig.GetMonsterByKey(m.Key); err == nil {
			return def.GetSpriteFromConfig()
		}
	}

	// Fallback if config not loaded or key unknown.
	return "goblin"
}

func (m *Monster3D) GetSize() (width, height float64) {
	// Get size from config
	if MonsterConfig != nil {
		for _, def := range MonsterConfig.Monsters {
			if def.Name == m.Name {
				return def.GetSizeFromConfig()
			}
		}
	}

	// Fallback size if config not loaded or monster not found
	return 32.0, 32.0
}

// GetSizeGameMultiplier returns the visual size multiplier from config
func (m *Monster3D) GetSizeGameMultiplier() float64 {
	// Get size multiplier from config
	if MonsterConfig != nil {
		for _, def := range MonsterConfig.Monsters {
			if def.Name == m.Name {
				return def.GetSizeGameMultiplier()
			}
		}
	}

	// Fallback multiplier if config not loaded or monster not found
	return 1.0
}

// distance calculates the Euclidean distance between two 2D points.
func distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// GetDistanceFromSpawn calculates the distance from the monster's spawn point
func (m *Monster3D) GetDistanceFromSpawn() float64 {
	return distance(m.X, m.Y, m.SpawnX, m.SpawnY)
}

// IsWithinTetherRadius checks if the monster is within its tether radius from spawn
func (m *Monster3D) IsWithinTetherRadius() bool {
	return m.GetDistanceFromSpawn() <= m.TetherRadius
}

// GetDirectionToSpawn returns the direction (in radians) from current position back to spawn
func (m *Monster3D) GetDirectionToSpawn() float64 {
	dx := m.SpawnX - m.X
	dy := m.SpawnY - m.Y
	return math.Atan2(dy, dx)
}

// CanMoveWithinTether checks if moving in a direction would keep monster within tether
func (m *Monster3D) CanMoveWithinTether(newX, newY float64) bool {
	return distance(newX, newY, m.SpawnX, m.SpawnY) <= m.TetherRadius
}
