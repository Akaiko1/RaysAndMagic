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
	Type         MonsterType3D
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

	// Loot
	Gold  int
	Items []items.Item

	// Resistances and immunities
	Resistances map[DamageType]int

	// Configuration reference
	config *config.Config
}

// Legacy NewMonster3D function removed - use NewMonster3DFromConfig instead

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
		Type:        MonsterGoblin, // Legacy field, no longer used for logic
		State:       StateIdle,
		Speed:       1.0,
		Direction:   rand.Float64() * 2 * math.Pi,
		StateTimer:  0,
		Resistances: make(map[DamageType]int),
		config:      cfg,
		ID:          generateUniqueMonsterID(), // Assign unique ID
	}

	// Setup from YAML config
	monster.SetupMonsterFromConfig(def)
	monster.HitPoints = monster.MaxHitPoints

	return monster
}

func (m *Monster3D) TakeDamage(damage int, damageType DamageType) int {
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

	// Become alert when damaged
	if m.State == StateIdle {
		m.State = StateAlert
		m.StateTimer = 0
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
