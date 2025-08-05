package spells

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SpellID represents dynamic spell identifiers (replaces hardcoded SpellType enum!)
type SpellID string

// Dynamic spell ID constants (loaded from config at runtime)
var (
	// Core spell IDs - these are loaded dynamically from config.yaml
	SpellIDTorchLight  SpellID = "torch_light"
	SpellIDFireBolt    SpellID = "firebolt"
	SpellIDFireball    SpellID = "fireball"
	SpellIDLightning   SpellID = "lightning"
	SpellIDHeal        SpellID = "heal"
	SpellIDHealOther   SpellID = "heal_other"
	SpellIDBless       SpellID = "bless"
	SpellIDWizardEye   SpellID = "wizard_eye"
	SpellIDAwaken      SpellID = "awaken"
	SpellIDWalkOnWater SpellID = "walk_on_water"
)

// Legacy SpellType for backward compatibility during migration
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

// Convert legacy SpellType to dynamic SpellID
func SpellTypeToID(spellType SpellType) SpellID {
	switch spellType {
	case SpellTypeTorchLight:
		return SpellIDTorchLight
	case SpellTypeFireBolt:
		return SpellIDFireBolt
	case SpellTypeFireball:
		return SpellIDFireball
	case SpellTypeLightning:
		return SpellIDLightning
	case SpellTypeHeal:
		return SpellIDHeal
	case SpellTypeHealOther:
		return SpellIDHealOther
	case SpellTypeBless:
		return SpellIDBless
	case SpellTypeWizardEye:
		return SpellIDWizardEye
	case SpellTypeAwaken:
		return SpellIDAwaken
	case SpellTypeWalkOnWater:
		return SpellIDWalkOnWater
	default:
		return SpellIDFireball // Default fallback
	}
}

// Convert dynamic SpellID to legacy SpellType (for compatibility)
func SpellIDToType(spellID SpellID) SpellType {
	switch spellID {
	case SpellIDTorchLight:
		return SpellTypeTorchLight
	case SpellIDFireBolt:
		return SpellTypeFireBolt
	case SpellIDFireball:
		return SpellTypeFireball
	case SpellIDLightning:
		return SpellTypeLightning
	case SpellIDHeal:
		return SpellTypeHeal
	case SpellIDHealOther:
		return SpellTypeHealOther
	case SpellIDBless:
		return SpellTypeBless
	case SpellIDWizardEye:
		return SpellTypeWizardEye
	case SpellIDAwaken:
		return SpellTypeAwaken
	case SpellIDWalkOnWater:
		return SpellTypeWalkOnWater
	default:
		return SpellTypeFireball // Default fallback
	}
}

// String returns the string representation of a spell ID
func (s SpellID) String() string {
	return string(s)
}

// Legacy String function for SpellType (backward compatibility)
func (s SpellType) String() string {
	return SpellTypeToID(s).String()
}

// SpellDefinition represents the complete definition of a spell
type SpellDefinition struct {
	Type            SpellType
	Name            string
	Description     string
	School          string
	Level           int // Spell level (1-9)
	SpellPointsCost int
	Duration        int // Duration in seconds (0 for instant spells)
	Damage          int
	Range           int // In tiles
	ProjectileSpeed float64
	ProjectileSize  int
	LifeTime        int // In frames for projectiles
	IsProjectile    bool
	IsUtility       bool
	VisualEffect    string
	StatBonus       int // Stat bonus for buff spells like Bless
}

// Global config reference for dynamic spell lookups
var globalConfig *config.Config

// SetGlobalConfig initializes the spell system with dynamic configuration
func SetGlobalConfig(cfg *config.Config) {
	globalConfig = cfg
}

// GetSpellDefinition returns spell definition dynamically from config (replaces hardcoded map!)
func GetSpellDefinition(spellType SpellType) SpellDefinition {
	// Convert legacy SpellType to dynamic SpellID
	spellID := SpellTypeToID(spellType)
	return GetSpellDefinitionByID(spellID)
}

// GetSpellDefinitionByID retrieves spell definition using dynamic SpellID
func GetSpellDefinitionByID(spellID SpellID) SpellDefinition {
	if globalConfig == nil {
		// Fallback for tests or uninitialized config
		return getHardcodedFallback(spellID)
	}

	// Dynamic lookup from config!
	if configDef, exists := globalConfig.GetSpellDefinition(string(spellID)); exists {
		return SpellDefinition{
			Type:            SpellIDToType(spellID), // Legacy compatibility
			Name:            configDef.Name,
			Description:     configDef.Description,
			School:          configDef.School,
			Level:           configDef.Level,
			SpellPointsCost: configDef.SpellPointsCost,
			Duration:        configDef.Duration,
			Damage:          configDef.Damage,
			Range:           configDef.Range,
			ProjectileSpeed: configDef.ProjectileSpeed,
			ProjectileSize:  configDef.ProjectileSize,
			LifeTime:        configDef.Lifetime,
			IsProjectile:    configDef.IsProjectile,
			IsUtility:       configDef.IsUtility,
			VisualEffect:    configDef.VisualEffect,
			StatBonus:       configDef.StatBonus,
		}
	}

	// Fallback if spell not found in config
	return getHardcodedFallback(spellID)
}

// Fallback function for missing config or tests
func getHardcodedFallback(spellID SpellID) SpellDefinition {
	switch spellID {
	case SpellIDTorchLight:
		return SpellDefinition{
			Type:            SpellTypeTorchLight,
			Name:            "Torch Light",
			Description:     "Creates a magical light that illuminates the surroundings",
			School:          "fire",
			Level:           1,
			SpellPointsCost: 1,
			Duration:        300,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "light",
		}
	case SpellIDFireBolt:
		return SpellDefinition{
			Type:            SpellTypeFireBolt,
			Name:            "Fire Bolt",
			Description:     "Quick fire attack with moderate damage",
			School:          "fire",
			Level:           1,
			SpellPointsCost: 2,
			Duration:        0,
			Damage:          6,
			Range:           8,
			ProjectileSpeed: 1.5,
			ProjectileSize:  12,
			LifeTime:        64,
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "fire_bolt",
		}
	case SpellIDFireball:
		return SpellDefinition{
			Type:            SpellTypeFireball,
			Name:            "Fireball",
			Description:     "Hurls a powerful ball of fire",
			School:          "fire",
			Level:           1,
			SpellPointsCost: 4,
			Duration:        0,
			Damage:          12,
			Range:           12,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "fireball",
		}
	case SpellIDLightning:
		return SpellDefinition{
			Type:            SpellTypeLightning,
			Name:            "Lightning Bolt",
			Description:     "Strikes the target with a bolt of lightning",
			School:          "air",
			Level:           2,
			SpellPointsCost: 8,
			Duration:        0,
			Damage:          18,
			Range:           6,
			ProjectileSpeed: 2.0,
			ProjectileSize:  8,
			LifeTime:        48,
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "lightning",
		}
	case SpellIDHeal:
		return SpellDefinition{
			Type:            SpellTypeHeal,
			Name:            "First Aid",
			Description:     "Restores health to the caster",
			School:          "body",
			Level:           1,
			SpellPointsCost: 2,
			Duration:        0,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "heal",
		}
	case SpellIDHealOther:
		return SpellDefinition{
			Type:            SpellTypeHealOther,
			Name:            "Heal",
			Description:     "Restores health to another party member",
			School:          "body",
			Level:           2,
			SpellPointsCost: 4,
			Duration:        0,
			Damage:          0,
			Range:           1,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "heal",
		}
	case SpellIDBless:
		return SpellDefinition{
			Type:            SpellTypeBless,
			Name:            "Bless",
			Description:     "Blesses the party with divine protection",
			School:          "spirit",
			Level:           3,
			SpellPointsCost: 6,
			Duration:        600,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "bless",
		}
	case SpellIDWizardEye:
		return SpellDefinition{
			Type:            SpellTypeWizardEye,
			Name:            "Wizard Eye",
			Description:     "Extends your vision beyond normal limits",
			School:          "air",
			Level:           4,
			SpellPointsCost: 8,
			Duration:        300,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "wizard_eye",
		}
	case SpellIDAwaken:
		return SpellDefinition{
			Type:            SpellTypeAwaken,
			Name:            "Awaken",
			Description:     "Awakens unconscious party members",
			School:          "mind",
			Level:           2,
			SpellPointsCost: 4,
			Duration:        0,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "awaken",
		}
	case SpellIDWalkOnWater:
		return SpellDefinition{
			Type:            SpellTypeWalkOnWater,
			Name:            "Walk on Water",
			Description:     "Allows the party to walk on water surfaces",
			School:          "water",
			Level:           3,
			SpellPointsCost: 6,
			Duration:        180,
			Damage:          0,
			Range:           0,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    false,
			IsUtility:       true,
			VisualEffect:    "water_walk",
		}
	default:
		// Ultimate fallback
		return SpellDefinition{
			Type:            SpellTypeFireball,
			Name:            "Unknown Spell",
			Description:     "A mysterious spell",
			School:          "fire",
			Level:           1,
			SpellPointsCost: 1,
			Duration:        0,
			Damage:          1,
			Range:           1,
			ProjectileSpeed: 1.0,
			ProjectileSize:  16,
			LifeTime:        96,
			IsProjectile:    true,
			IsUtility:       false,
			VisualEffect:    "fireball",
		}
	}
}

// CreateSpellItem creates an item from a spell definition
func CreateSpellItem(spellType SpellType) items.Item {
	def := GetSpellDefinition(spellType)

	var itemType items.ItemType
	var spellEffect items.SpellEffect

	// Dynamic spell effect assignment (no more hardcoded switches!)
	spellID := SpellTypeToID(spellType)
	spellEffect = items.SpellIDToSpellEffect(string(spellID))

	if def.IsUtility {
		itemType = items.ItemUtilitySpell
	} else {
		itemType = items.ItemBattleSpell
	}

	return items.Item{
		Name:        def.Name,
		Type:        itemType,
		Description: def.Description,
		Damage:      def.Damage,
		Range:       def.SpellPointsCost,
		SpellSchool: def.School,
		SpellCost:   def.SpellPointsCost,
		SpellEffect: spellEffect,
		Attributes:  make(map[string]int),
	}
}

// GetSpellTypeByName returns the spell type for a given spell name (now dynamic!)
func GetSpellTypeByName(name string) SpellType {
	if globalConfig == nil {
		return getHardcodedSpellTypeByName(name)
	}

	// Dynamic lookup from config!
	if _, spellKey, exists := globalConfig.GetSpellDefinitionByName(name); exists {
		spellID := SpellID(spellKey)
		return SpellIDToType(spellID)
	}

	// Fallback
	return getHardcodedSpellTypeByName(name)
}

// GetSpellIDByName returns dynamic SpellID for a given spell name
func GetSpellIDByName(name string) SpellID {
	if globalConfig == nil {
		// Convert legacy lookup to SpellID
		legacyType := getHardcodedSpellTypeByName(name)
		return SpellTypeToID(legacyType)
	}

	// Dynamic lookup from config!
	if _, spellKey, exists := globalConfig.GetSpellDefinitionByName(name); exists {
		return SpellID(spellKey)
	}

	// Fallback
	return SpellIDFireball
}

// Hardcoded fallback for GetSpellTypeByName (compatibility)
func getHardcodedSpellTypeByName(name string) SpellType {
	nameMap := map[string]SpellType{
		"Torch Light":    SpellTypeTorchLight,
		"Fire Bolt":      SpellTypeFireBolt,
		"Fireball":       SpellTypeFireball,
		"Lightning Bolt": SpellTypeLightning,
		"First Aid":      SpellTypeHeal,
		"Heal":           SpellTypeHealOther,
		"Bless":          SpellTypeBless,
		"Wizard Eye":     SpellTypeWizardEye,
		"Awaken":         SpellTypeAwaken,
		"Walk on Water":  SpellTypeWalkOnWater,
	}

	if spellType, exists := nameMap[name]; exists {
		return spellType
	}

	return SpellTypeFireball // Default fallback
}

// GetSpellsBySchool returns all spells available for a given magic school (now dynamic!)
func GetSpellsBySchool(school string) []SpellType {
	if globalConfig == nil {
		return getHardcodedSpellsBySchool(school)
	}

	// Dynamic lookup from config!
	spellKeys := globalConfig.GetSpellsBySchool(school)
	spellTypes := make([]SpellType, 0, len(spellKeys))

	for _, spellKey := range spellKeys {
		spellID := SpellID(spellKey)
		spellType := SpellIDToType(spellID)
		spellTypes = append(spellTypes, spellType)
	}

	return spellTypes
}

// GetSpellIDsBySchool returns all spell IDs for a given magic school (dynamic version)
func GetSpellIDsBySchool(school string) []SpellID {
	if globalConfig == nil {
		// Convert legacy types to IDs
		legacyTypes := getHardcodedSpellsBySchool(school)
		spellIDs := make([]SpellID, 0, len(legacyTypes))
		for _, spellType := range legacyTypes {
			spellIDs = append(spellIDs, SpellTypeToID(spellType))
		}
		return spellIDs
	}

	// Dynamic lookup from config!
	spellKeys := globalConfig.GetSpellsBySchool(school)
	spellIDs := make([]SpellID, 0, len(spellKeys))

	for _, spellKey := range spellKeys {
		spellIDs = append(spellIDs, SpellID(spellKey))
	}

	return spellIDs
}

// Hardcoded fallback for GetSpellsBySchool (compatibility)
func getHardcodedSpellsBySchool(school string) []SpellType {
	var spells []SpellType

	// Get all spell types and filter by school
	allSpellTypes := []SpellType{
		SpellTypeTorchLight, SpellTypeFireBolt, SpellTypeFireball, SpellTypeLightning,
		SpellTypeHeal, SpellTypeHealOther, SpellTypeBless, SpellTypeWizardEye,
		SpellTypeAwaken, SpellTypeWalkOnWater,
	}

	for _, spellType := range allSpellTypes {
		def := GetSpellDefinition(spellType)
		if def.School == school {
			spells = append(spells, spellType)
		}
	}

	return spells
}

// ConvertSpellTypeToCharacterSpell converts a SpellType to character.Spell format
func ConvertSpellTypeToCharacterSpell(spellType SpellType) interface{} {
	def := GetSpellDefinition(spellType)

	// This will be used to create character.Spell structs
	// We return interface{} to avoid circular imports for now
	return map[string]interface{}{
		"Name":        def.Name,
		"School":      def.School,
		"Level":       def.Level,
		"SpellPoints": def.SpellPointsCost,
		"Description": def.Description,
		"Duration":    def.Duration,
	}
}
