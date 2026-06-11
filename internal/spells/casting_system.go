package spells

import (
	"fmt"
	"math"
	"ugataima/internal/config"
)

// ProjectileData represents a projectile in the game world
type ProjectileData struct {
	X, Y       float64
	VelX, VelY float64
	Damage     int
	LifeTime   int
	Active     bool
	SpellID    SpellID
	Size       int
}

// CastingSystem handles spell casting and effects
type CastingSystem struct {
	config *config.Config
}

// NewCastingSystem creates a new spell casting system
func NewCastingSystem(config *config.Config) *CastingSystem {
	return &CastingSystem{
		config: config,
	}
}

// CreateProjectile creates a projectile based on spell type and caster stats (now dynamic!)
// CalculateSpellDamageByID calculates damage using SpellID (YAML-based)
func CalculateSpellDamageByID(spellID SpellID, casterIntellect int) (baseDamage, intellectBonus, totalDamage int) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0, 0, 0
	}

	mult := def.DamageCostMultiplier
	if mult < 1 {
		mult = 1
	}
	baseDamage = def.SpellPointsCost * SpellDamagePerSP * mult
	intellectBonus = casterIntellect / SpellIntellectDivisor

	totalDamage = baseDamage + intellectBonus
	return
}

// CalculateHealingAmountByID calculates healing using SpellID (YAML-based)
func CalculateHealingAmountByID(spellID SpellID, casterPersonality int) (baseHealing, personalityBonus, totalHealing int) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0, 0, 0
	}

	// Healing spells MUST set heal_amount; we no longer fall back to the
	// damage field because the damage field has been removed from the YAML
	// schema (offensive damage is derived from spell_points_cost × N).
	baseHealing = def.HealAmount
	personalityBonus = casterPersonality / HealingPersonalityDivisor
	totalHealing = baseHealing + personalityBonus
	return
}

func (cs *CastingSystem) CreateProjectile(spellID SpellID, casterX, casterY, angle float64, casterIntellect int) (ProjectileData, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return ProjectileData{}, err
	}

	// Use centralized damage calculation
	_, _, damage := CalculateSpellDamageByID(spellID, casterIntellect)

	// Get physics config (tile-based)
	physics, err := cs.config.GetSpellConfig(string(spellID))
	if err != nil {
		return ProjectileData{}, err
	}

	// Convert tile-based speed to pixels per frame
	tileSize := cs.config.GetTileSize()
	velocity := physics.GetSpeedPixels(tileSize)

	// Get lifetime from physics (calculated from range/speed)
	lifetime := physics.GetLifetimeFrames()

	return ProjectileData{
		X:        casterX,
		Y:        casterY,
		VelX:     math.Cos(angle) * velocity,
		VelY:     math.Sin(angle) * velocity,
		Damage:   damage,
		LifeTime: lifetime,
		Active:   true,
		SpellID:  spellID,
		Size:     def.ProjectileSize,
	}, nil
}

// ApplyUtilitySpell applies utility spell effects using YAML configuration
func (cs *CastingSystem) ApplyUtilitySpell(spellID SpellID, casterPersonality int) (UtilitySpellResult, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return UtilitySpellResult{}, err
	}

	// Dynamic utility spell effect application based on YAML properties
	result := UtilitySpellResult{
		Success: true,
		Message: def.Message, // Use message from YAML
	}

	tps := config.GetTargetTPS()

	// Apply effects directly from YAML configuration - no hardcoded logic!
	if def.Duration > 0 {
		result.Duration = def.Duration * tps // Convert to frames
	}

	if def.HealAmount > 0 {
		// Calculate actual healing based on caster stats
		_, _, healAmount := CalculateHealingAmountByID(spellID, casterPersonality)
		result.HealAmount = healAmount
		result.TargetSelf = def.TargetSelf
	}

	if def.VisionBonus > 0 {
		result.VisionBonus = def.VisionBonus
	}

	if def.StatBonus > 0 {
		result.StatBonus = def.StatBonus
	}
	if len(def.StatBonuses) > 0 {
		result.StatBonuses = def.StatBonuses
	}

	if def.Awaken {
		result.Awaken = true
	}

	if def.WaterWalk {
		result.WaterWalk = true
		result.Duration = def.Duration * tps // Convert to frames
	}

	if def.WaterBreathing {
		result.WaterBreathing = true
		result.Duration = def.Duration * tps // Convert to frames
	}

	return result, nil
}

// UtilitySpellResult represents the result of casting a utility spell
type UtilitySpellResult struct {
	Success        bool
	Message        string
	HealAmount     int
	TargetSelf     bool
	StatBonus      int
	StatBonuses    map[string]int
	VisionBonus    float64
	Duration       int // In frames
	Awaken         bool
	WaterWalk      bool
	WaterBreathing bool
}

// GetProjectileColor returns the color for a projectile based on spell ID
func GetProjectileColor(spellID SpellID) ([3]int, error) {
	if config.GlobalConfig != nil {
		graphicsConfig, err := config.GlobalConfig.GetSpellGraphicsConfig(string(spellID))
		if err != nil {
			return [3]int{}, err
		}
		return graphicsConfig.Color, nil
	}

	return [3]int{}, fmt.Errorf("no spell configuration available for '%s'", spellID)
}
