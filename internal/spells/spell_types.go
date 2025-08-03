package spells

import "ugataima/internal/items"

// SpellType represents different types of spells
type SpellType int

const (
	SpellTypeTorchLight SpellType = iota
	SpellTypeFireBolt
	SpellTypeFireball
	SpellTypeLightning
	SpellTypeHeal
	SpellTypeHealOther
	SpellTypeBless
	SpellTypeWizardEye
	SpellTypeAwaken
	SpellTypeWalkOnWater
)

// String returns the string representation of a spell type
func (s SpellType) String() string {
	switch s {
	case SpellTypeTorchLight:
		return "SpellTypeTorchLight"
	case SpellTypeFireBolt:
		return "SpellTypeFireBolt"
	case SpellTypeFireball:
		return "SpellTypeFireball"
	case SpellTypeLightning:
		return "SpellTypeLightning"
	case SpellTypeHeal:
		return "SpellTypeHeal"
	case SpellTypeHealOther:
		return "SpellTypeHealOther"
	case SpellTypeBless:
		return "SpellTypeBless"
	case SpellTypeWizardEye:
		return "SpellTypeWizardEye"
	case SpellTypeAwaken:
		return "SpellTypeAwaken"
	case SpellTypeWalkOnWater:
		return "SpellTypeWalkOnWater"
	default:
		return "Unknown"
	}
}

// SpellEffect represents what happens when a spell is cast
type SpellEffect struct {
	Type            SpellType
	Name            string
	Description     string
	School          string
	SpellPointsCost int
	Damage          int
	Range           int // In tiles
	ProjectileSpeed float64
	ProjectileSize  int
	LifeTime        int // In frames
	IsProjectile    bool
	IsUtility       bool
	VisualEffect    string
}

// GetSpellEffect returns the spell effect data for a given spell type
func GetSpellEffect(spellType SpellType) SpellEffect {
	effects := map[SpellType]SpellEffect{
		SpellTypeTorchLight: {
			Type:            SpellTypeTorchLight,
			Name:            "Torch Light",
			Description:     "Creates a magical light that illuminates the surroundings",
			School:          "fire",
			SpellPointsCost: 1,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "light",
		},
		SpellTypeFireBolt: {
			Type:            SpellTypeFireBolt,
			Name:            "Fire Bolt",
			Description:     "Launches a small, fast bolt of fire",
			School:          "fire",
			SpellPointsCost: 2,
			Damage:          6,
			Range:           8, // 8 tiles
			ProjectileSpeed: 1.5,
			ProjectileSize:  10,
			LifeTime:        64, // 8 tiles at 1.5x speed
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "fire_bolt",
		},
		SpellTypeFireball: {
			Type:            SpellTypeFireball,
			Name:            "Fireball",
			Description:     "Hurls a powerful ball of fire that explodes on impact",
			School:          "fire",
			SpellPointsCost: 4,
			Damage:          12,
			Range:           12, // 12 tiles
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96, // 12 tiles at 1.0x speed
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "fireball",
		},
		SpellTypeLightning: {
			Type:            SpellTypeLightning,
			Name:            "Lightning Bolt",
			Description:     "Strikes with a bolt of lightning",
			School:          "air",
			SpellPointsCost: 3,
			Damage:          8,
			Range:           10,
			ProjectileSpeed: 2.0,
			ProjectileSize:  8,
			LifeTime:        48,
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "lightning",
		},
		SpellTypeHeal: {
			Type:            SpellTypeHeal,
			Name:            "Heal",
			Description:     "Restores health to the caster",
			School:          "body",
			SpellPointsCost: 2,
			Damage:          15, // Heal amount
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "heal",
		},
		SpellTypeHealOther: {
			Type:            SpellTypeHealOther,
			Name:            "Heal Other",
			Description:     "Restores health to a target party member",
			School:          "body",
			SpellPointsCost: 3,
			Damage:          20, // Heal amount
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "heal",
		},
		SpellTypeBless: {
			Type:            SpellTypeBless,
			Name:            "Bless",
			Description:     "Increases party's combat effectiveness",
			School:          "spirit",
			SpellPointsCost: 1,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "bless",
		},
		SpellTypeWizardEye: {
			Type:            SpellTypeWizardEye,
			Name:            "Wizard Eye",
			Description:     "Reveals the surrounding area",
			School:          "air",
			SpellPointsCost: 1,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "eye",
		},
		SpellTypeAwaken: {
			Type:            SpellTypeAwaken,
			Name:            "Awaken",
			Description:     "Awakens fallen party members",
			School:          "water",
			SpellPointsCost: 1,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "awaken",
		},
		SpellTypeWalkOnWater: {
			Type:            SpellTypeWalkOnWater,
			Name:            "Walk on Water",
			Description:     "Allows the party to walk across water surfaces",
			School:          "water",
			SpellPointsCost: 3,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 0,
			ProjectileSize:  0,
			LifeTime:        0,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "water_walk",
		},
	}

	return effects[spellType]
}

// CreateSpellItem creates an item from a spell effect
func CreateSpellItem(spellType SpellType) items.Item {
	effect := GetSpellEffect(spellType)

	var itemType items.ItemType
	var spellEffect items.SpellEffect

	if effect.IsUtility {
		itemType = items.ItemUtilitySpell
		switch spellType {
		case SpellTypeHeal:
			spellEffect = items.SpellEffectHealSelf
		case SpellTypeHealOther:
			spellEffect = items.SpellEffectHealOther
		default:
			spellEffect = items.SpellEffectPartyBuff
		}
	} else {
		itemType = items.ItemBattleSpell
		switch spellType {
		case SpellTypeFireBolt:
			spellEffect = items.SpellEffectFireBolt
		case SpellTypeFireball:
			spellEffect = items.SpellEffectFireball
		case SpellTypeTorchLight:
			spellEffect = items.SpellEffectTorchLight
		case SpellTypeLightning:
			spellEffect = items.SpellEffectLightning
		default:
			spellEffect = items.SpellEffectFireball
		}
	}

	return items.Item{
		Name:        effect.Name,
		Type:        itemType,
		Description: effect.Description,
		Damage:      effect.Damage,
		Range:       effect.SpellPointsCost, // Use spell points cost as range for equipped spells
		SpellSchool: effect.School,
		SpellCost:   effect.SpellPointsCost,
		SpellEffect: spellEffect,
		Attributes:  make(map[string]int),
	}
}

// GetSpellTypeByName returns the spell type for a given spell name
func GetSpellTypeByName(name string) SpellType {
	nameMap := map[string]SpellType{
		"Torch Light":    SpellTypeTorchLight,
		"Fire Bolt":      SpellTypeFireBolt,
		"Fireball":       SpellTypeFireball,
		"Lightning Bolt": SpellTypeLightning,
		"Heal":           SpellTypeHeal,
		"Heal Other":     SpellTypeHealOther,
		"Bless":          SpellTypeBless,
		"Wizard Eye":     SpellTypeWizardEye,
		"Awaken":         SpellTypeAwaken,
	}

	if spellType, exists := nameMap[name]; exists {
		return spellType
	}

	return SpellTypeFireball // Default fallback
}
