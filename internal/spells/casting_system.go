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

// CreateProjectile builds the PHYSICS of a spell projectile (velocity,
// lifetime, size). Damage is authored by the caller (CombatSystem owns every
// number: stats, mastery, crits; monsters use their own attack damage) — this
// layer deliberately computes none.
func (cs *CastingSystem) CreateProjectile(spellID SpellID, casterX, casterY, angle float64) (ProjectileData, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return ProjectileData{}, err
	}

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
		LifeTime: lifetime,
		Active:   true,
		SpellID:  spellID,
		Size:     def.ProjectileSize,
	}, nil
}

// ApplyUtilitySpell resolves the EFFECT FLAGS of a utility spell from YAML
// (vision/water flags + cast message). All numbers — heal totals, durations,
// stat bonuses — are computed by CombatSystem (stats, mastery), never here.
func (cs *CastingSystem) ApplyUtilitySpell(spellID SpellID) (UtilitySpellResult, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return UtilitySpellResult{}, err
	}
	return UtilitySpellResult{
		Success:        true,
		Message:        def.Message,
		VisionBonus:    def.VisionBonus,
		WaterWalk:      def.WaterWalk,
		WaterBreathing: def.WaterBreathing,
	}, nil
}

// UtilitySpellResult represents the result of casting a utility spell
type UtilitySpellResult struct {
	Success        bool
	Message        string
	VisionBonus    float64
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
