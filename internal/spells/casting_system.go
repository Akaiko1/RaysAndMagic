package spells

import (
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
	SpellType  SpellType
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

// CreateProjectile creates a projectile based on spell type and caster stats
func (cs *CastingSystem) CreateProjectile(spellType SpellType, casterX, casterY, angle float64, casterIntellect int) ProjectileData {
	effect := GetSpellEffect(spellType)

	// Calculate damage with caster's intellect bonus
	damage := effect.Damage
	switch spellType {
	case SpellTypeFireBolt:
		damage += casterIntellect / 4 // Light intellect scaling
	case SpellTypeFireball:
		damage += casterIntellect / 2 // Standard intellect scaling
	case SpellTypeLightning:
		damage += casterIntellect / 3 // Medium intellect scaling
	}

	// Calculate velocity based on spell effect
	baseSpeed := cs.config.GetFireballSpeed()
	velocity := baseSpeed * effect.ProjectileSpeed

	return ProjectileData{
		X:         casterX,
		Y:         casterY,
		VelX:      math.Cos(angle) * velocity,
		VelY:      math.Sin(angle) * velocity,
		Damage:    damage,
		LifeTime:  effect.LifeTime,
		Active:    true,
		SpellType: spellType,
		Size:      effect.ProjectileSize,
	}
}

// ApplyUtilitySpell applies utility spell effects
func (cs *CastingSystem) ApplyUtilitySpell(spellType SpellType, casterPersonality int) UtilitySpellResult {
	effect := GetSpellEffect(spellType)

	switch spellType {
	case SpellTypeTorchLight:
		return UtilitySpellResult{
			Type:        spellType,
			Success:     true,
			Message:     "A magical light illuminates the area!",
			VisionBonus: 50.0, // Increase vision range by 50 units
			Duration:    1800, // 30 seconds at 60 FPS
		}
	case SpellTypeHeal:
		healAmount := effect.Damage + casterPersonality/2
		return UtilitySpellResult{
			Type:       spellType,
			Success:    true,
			Message:    "You feel renewed!",
			HealAmount: healAmount,
			TargetSelf: true,
		}
	case SpellTypeHealOther:
		healAmount := effect.Damage + casterPersonality/2
		return UtilitySpellResult{
			Type:       spellType,
			Success:    true,
			Message:    "Healing energy flows to your ally!",
			HealAmount: healAmount,
			TargetSelf: false,
		}
	case SpellTypeBless:
		return UtilitySpellResult{
			Type:      spellType,
			Success:   true,
			Message:   "The party feels blessed!",
			StatBonus: 2,   // +2 to combat stats
			Duration:  600, // 10 seconds
		}
	case SpellTypeWizardEye:
		return UtilitySpellResult{
			Type:        spellType,
			Success:     true,
			Message:     "Your vision extends beyond normal limits!",
			VisionBonus: 100.0, // Large vision increase
			Duration:    900,   // 15 seconds
		}
	case SpellTypeAwaken:
		return UtilitySpellResult{
			Type:    spellType,
			Success: true,
			Message: "Awakening energy spreads through the party!",
			Awaken:  true,
		}
	case SpellTypeWalkOnWater:
		return UtilitySpellResult{
			Type:      spellType,
			Success:   true,
			Message:   "The party gains the ability to walk on water!",
			Duration:  18000, // 5 minutes at 60 FPS (300 seconds * 60)
			WaterWalk: true,
		}
	default:
		return UtilitySpellResult{
			Type:    spellType,
			Success: false,
			Message: "The spell fizzles out...",
		}
	}
}

// UtilitySpellResult represents the result of casting a utility spell
type UtilitySpellResult struct {
	Type        SpellType
	Success     bool
	Message     string
	HealAmount  int
	TargetSelf  bool
	StatBonus   int
	VisionBonus float64
	Duration    int // In frames
	Awaken      bool
	WaterWalk   bool
}

// GetProjectileColor returns the color for a projectile based on spell type
func GetProjectileColor(spellType SpellType) [3]int {
	switch spellType {
	case SpellTypeFireBolt:
		return [3]int{255, 150, 0} // Orange
	case SpellTypeFireball:
		return [3]int{255, 100, 0} // Red-orange
	case SpellTypeLightning:
		return [3]int{200, 200, 255} // Light blue
	default:
		return [3]int{255, 100, 0} // Default to fireball color
	}
}

// GetProjectileVisualEffect returns visual effect parameters for projectiles
func GetProjectileVisualEffect(spellType SpellType) ProjectileVisualEffect {
	switch spellType {
	case SpellTypeFireBolt:
		return ProjectileVisualEffect{
			TrailLength:  3,
			TrailOpacity: 150,
			GlowRadius:   5,
			Sparkles:     false,
		}
	case SpellTypeFireball:
		return ProjectileVisualEffect{
			TrailLength:  5,
			TrailOpacity: 200,
			GlowRadius:   8,
			Sparkles:     true,
		}
	case SpellTypeLightning:
		return ProjectileVisualEffect{
			TrailLength:  2,
			TrailOpacity: 255,
			GlowRadius:   6,
			Sparkles:     true,
		}
	default:
		return ProjectileVisualEffect{
			TrailLength:  3,
			TrailOpacity: 150,
			GlowRadius:   5,
			Sparkles:     false,
		}
	}
}

// ProjectileVisualEffect defines visual parameters for projectiles
type ProjectileVisualEffect struct {
	TrailLength  int
	TrailOpacity int
	GlowRadius   int
	Sparkles     bool
}
