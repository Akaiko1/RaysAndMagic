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
	Gold       int `yaml:"gold"`
	Experience int `yaml:"experience"`
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
	Level        int
	HitPoints    int
	MaxHitPoints int
	ArmorClass   int
	PerfectDodge int // Chance (0-100) to completely avoid an attack
	Experience   int
	ID           string // Unique identifier for collision tracking

	// Combat stats
	AttackBonus int
	DamageMin   int
	DamageMax   int

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
	SpawnX, SpawnY   float64 // Original spawn position
	TetherRadius     float64 // Maximum distance from spawn point (default 4 tiles = 256 pixels)
	IsEngagingPlayer bool    // True when actively pursuing/fighting player
	WasAttacked      bool    // True when monster was hit - prevents disengagement

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

func (m *Monster3D) HasRangedAttack() bool {
	return m.ProjectileSpell != "" || m.ProjectileWeapon != ""
}

func (m *Monster3D) GetSpriteType() string {
	// Get sprite from config
	if MonsterConfig != nil {
		for _, def := range MonsterConfig.Monsters {
			if def.Name == m.Name {
				return def.GetSpriteFromConfig()
			}
		}
	}

	// Fallback if config not loaded or monster not found
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
