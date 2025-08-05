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

// CreateProjectile creates a projectile based on spell type and caster stats (now dynamic!)
// CalculateSpellDamage calculates damage for a spell using centralized formula
func CalculateSpellDamage(spellType SpellType, casterIntellect int) (baseDamage, intellectBonus, totalDamage int) {
	def := GetSpellDefinition(spellType)

	// Original formula: baseDamage = spellPoints * 3
	baseDamage = def.SpellPointsCost * 3

	// Original formula: intellectBonus = character.Intellect / 2
	intellectBonus = casterIntellect / 2

	totalDamage = baseDamage + intellectBonus
	return
}

// CalculateHealingAmount calculates healing for utility spells using centralized formula
func CalculateHealingAmount(spellType SpellType, casterPersonality int) (baseHealing, personalityBonus, totalHealing int) {
	def := GetSpellDefinition(spellType)

	// Healing formula: baseHealing = def.Damage (from config)
	baseHealing = def.Damage

	// Personality bonus: casterPersonality / 2
	personalityBonus = casterPersonality / 2

	totalHealing = baseHealing + personalityBonus
	return
}

func (cs *CastingSystem) CreateProjectile(spellType SpellType, casterX, casterY, angle float64, casterIntellect int) ProjectileData {
	def := GetSpellDefinition(spellType)
	spellID := SpellTypeToID(spellType)

	// Use centralized damage calculation
	_, _, damage := CalculateSpellDamage(spellType, casterIntellect)

	// Calculate velocity using dynamic spell config
	spellConfig := cs.config.GetSpellConfig(string(spellID))
	baseSpeed := spellConfig.Speed
	velocity := baseSpeed * def.ProjectileSpeed

	return ProjectileData{
		X:         casterX,
		Y:         casterY,
		VelX:      math.Cos(angle) * velocity,
		VelY:      math.Sin(angle) * velocity,
		Damage:    damage,
		LifeTime:  def.LifeTime,
		Active:    true,
		SpellType: spellType,
		Size:      def.ProjectileSize,
	}
}

// ApplyUtilitySpell applies utility spell effects using centralized spell definitions (now dynamic!)
func (cs *CastingSystem) ApplyUtilitySpell(spellType SpellType, casterPersonality int) UtilitySpellResult {
	def := GetSpellDefinition(spellType)
	spellID := SpellTypeToID(spellType)

	// Dynamic utility spell effect application based on spell properties
	result := UtilitySpellResult{
		Type:    spellType,
		Success: true,
		Message: def.Description, // Use spell description as default message
	}

	// Apply dynamic effects based on spell ID pattern (no more hardcoded switches!)
	switch string(spellID) {
	case "torch_light":
		result.Message = "A magical light illuminates the area!"
		result.VisionBonus = 50.0
		result.Duration = def.Duration * 60
	case "heal":
		_, _, healAmount := CalculateHealingAmount(spellType, casterPersonality)
		result.Message = "You feel renewed!"
		result.HealAmount = healAmount
		result.TargetSelf = true
	case "heal_other":
		_, _, healAmount := CalculateHealingAmount(spellType, casterPersonality)
		result.Message = "Healing energy flows to your ally!"
		result.HealAmount = healAmount
		result.TargetSelf = false
	case "bless":
		result.Message = "The party feels blessed! (+20 to all stats)"
		result.StatBonus = def.StatBonus  // Use configurable stat bonus
		result.Duration = def.Duration * 60
	case "wizard_eye":
		result.Message = "Your vision extends beyond normal limits!"
		result.VisionBonus = 100.0
		result.Duration = def.Duration * 60
	case "awaken":
		result.Message = "Awakening energy spreads through the party!"
		result.Awaken = true
	case "walk_on_water":
		result.Message = "The party gains the ability to walk on water!"
		result.Duration = def.Duration * 60
		result.WaterWalk = true
	default:
		// For unknown utility spells, try to infer effects from spell definition
		if def.Damage > 0 {
			// Healing spell
			_, _, healAmount := CalculateHealingAmount(spellType, casterPersonality)
			result.Message = "SPELL CONFIG FAILED - Healing energy flows through you!"
			result.HealAmount = healAmount
			result.TargetSelf = true
		} else if def.Duration > 0 {
			// Duration-based buff
			result.Message = "SPELL CONFIG FAILED - You feel empowered!"
			result.Duration = def.Duration * 60
		} else {
			// Unknown spell
			result.Success = false
			result.Message = "SPELL CONFIG FAILED - The spell fizzles out..."
		}
	}

	return result
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

// GetProjectileColor returns the color for a projectile based on spell type (now dynamic!)
func GetProjectileColor(spellType SpellType) [3]int {
	spellID := SpellTypeToID(spellType)

	// Try to get color from config first
	if globalConfig != nil {
		if graphicsConfig := globalConfig.GetSpellGraphicsConfig(string(spellID)); graphicsConfig != nil {
			return graphicsConfig.Color
		}
	}

	// Fallback to hardcoded colors for compatibility
	switch spellType {
	case SpellTypeFireBolt:
		return [3]int{0, 0, 0} // BLACK - CONFIG FAILED!
	case SpellTypeFireball:
		return [3]int{0, 0, 0} // BLACK - CONFIG FAILED!
	case SpellTypeLightning:
		return [3]int{0, 0, 0} // BLACK - CONFIG FAILED!
	default:
		return [3]int{0, 0, 0} // BLACK - CONFIG FAILED!
	}
}

// GetProjectileVisualEffect returns visual effect parameters for projectiles (now dynamic!)
func GetProjectileVisualEffect(spellType SpellType) ProjectileVisualEffect {
	def := GetSpellDefinition(spellType)

	// Create default effect based on spell properties
	effect := ProjectileVisualEffect{
		TrailLength:  3,
		TrailOpacity: 150,
		GlowRadius:   5,
		Sparkles:     false,
	}

	// Use visual_effect field from spell definition for specific effects
	switch def.VisualEffect {
	case "fireball":
		effect.TrailLength = 5
		effect.TrailOpacity = 200
		effect.GlowRadius = 8
		effect.Sparkles = true
	case "fire_bolt":
		effect.TrailLength = 3
		effect.TrailOpacity = 150
		effect.GlowRadius = 5
		effect.Sparkles = false
	case "lightning":
		effect.TrailLength = 2
		effect.TrailOpacity = 255
		effect.GlowRadius = 6
		effect.Sparkles = true
	default:
		// Fallback to school-based effects if no specific visual effect defined
		switch def.School {
		case "fire":
			effect.TrailOpacity = 200
			effect.Sparkles = true
		case "water":
			effect.TrailLength = 4
			effect.GlowRadius = 6
		case "air":
			effect.TrailLength = 2
			effect.TrailOpacity = 255
			effect.Sparkles = true
		}
	}

	return effect
}

// ProjectileVisualEffect defines visual parameters for projectiles
type ProjectileVisualEffect struct {
	TrailLength  int
	TrailOpacity int
	GlowRadius   int
	Sparkles     bool
}
