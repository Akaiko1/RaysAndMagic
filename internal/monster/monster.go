package monster

import (
	"math"
	"math/rand"
	"strconv"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

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
	AttackCount  int   // Number of attacks made in current engagement

	// Tethering system - monsters stay within 3 tiles of spawn unless engaging player
	SpawnX, SpawnY float64 // Original spawn position
	TetherRadius   float64 // Maximum distance from spawn point (default 3 tiles = 192 pixels)
	IsEngagingPlayer bool  // True when actively pursuing/fighting player

	// Loot
	Gold  int
	Items []items.Item

	// Resistances and immunities
	Resistances map[DamageType]int

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
		State:       StateIdle,
		Speed:       1.0,
		Direction:   rand.Float64() * 2 * math.Pi,
		StateTimer:  0,
		Resistances: make(map[DamageType]int),
		config:      cfg,
		ID:          generateUniqueMonsterID(), // Assign unique ID
		
		// Initialize tethering system
		SpawnX:           x,
		SpawnY:           y,
		TetherRadius:     192.0, // 3 tiles * 64 pixels per tile
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

	// If already engaging player, just return damage (don't change AI state)
	if m.IsEngagingPlayer {
		return damage
	}

	// Calculate distance to player who attacked
	dx := playerX - m.X
	dy := playerY - m.Y
	distanceToPlayer := math.Sqrt(dx*dx + dy*dy)
	
	// React based on distance to attacker (only if not already engaged)
	if distanceToPlayer <= 512.0 { // 8 tiles (8 * 64 pixels)
		// Player is within 8 tiles - engage and pursue
		m.IsEngagingPlayer = true
		m.State = StateAlert
		m.StateTimer = 0
	} else {
		// Player is 9+ tiles away - flee
		m.IsEngagingPlayer = false
		m.State = StateFleeing
		m.StateTimer = 0
		// Flee in opposite direction from attacker
		m.Direction = math.Atan2(-dy, -dx) // Opposite direction
	}

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

// GetDistanceFromSpawn calculates the distance from the monster's spawn point
func (m *Monster3D) GetDistanceFromSpawn() float64 {
	dx := m.X - m.SpawnX
	dy := m.Y - m.SpawnY
	return math.Sqrt(dx*dx + dy*dy)
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
	dx := newX - m.SpawnX
	dy := newY - m.SpawnY
	distance := math.Sqrt(dx*dx + dy*dy)
	return distance <= m.TetherRadius
}
