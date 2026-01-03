package spells

import (
	"fmt"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SpellID represents dynamic spell identifiers loaded from YAML
type SpellID string

// String returns the string representation of a spell ID
func (s SpellID) String() string {
	return string(s)
}

// SpellDefinition represents the complete definition of a spell loaded from YAML
type SpellDefinition struct {
	ID                 SpellID
	Name               string
	Description        string
	School             string
	Level              int // Spell level (1-9)
	SpellPointsCost    int
	Duration           int // Duration in seconds (0 for instant spells)
	Damage             int
	DisintegrateChance float64
	Range              int // In tiles
	ProjectileSpeed    float64
	ProjectileSize     int
	LifeTime           int // In frames for projectiles
	IsProjectile       bool
	IsUtility          bool
	VisualEffect       string
	StatusIcon         string
	StatBonus          int // Stat bonus for buff spells like Bless
	// Effect configuration
	HealAmount     int     // For healing spells
	VisionBonus    float64 // For vision enhancement spells
	TargetSelf     bool    // Whether spell targets self or others
	Awaken         bool    // For awaken spell
	WaterWalk      bool    // For water walking spell
	WaterBreathing bool    // For water breathing spell
	Message        string  // Effect message to display
}

// SetGlobalConfig initializes the spell system with dynamic configuration (legacy)
func SetGlobalConfig(cfg *config.Config) {
	// No longer needed - spells load directly from global config
}

// GetSpellDefinitionByID retrieves spell definition from YAML config
func GetSpellDefinitionByID(spellID SpellID) (SpellDefinition, error) {
	configDef, exists := config.GetSpellDefinition(string(spellID))
	if !exists {
		return SpellDefinition{}, fmt.Errorf("spell '%s' not found in spells.yaml", spellID)
	}

	return SpellDefinition{
		ID:                 spellID,
		Name:               configDef.Name,
		Description:        configDef.Description,
		School:             configDef.School,
		Level:              configDef.Level,
		SpellPointsCost:    configDef.SpellPointsCost,
		Duration:           configDef.Duration,
		Damage:             configDef.Damage,
		DisintegrateChance: configDef.DisintegrateChance,
		Range:              configDef.Range,
		ProjectileSpeed:    configDef.ProjectileSpeed,
		ProjectileSize:     configDef.ProjectileSize,
		LifeTime:           configDef.Lifetime,
		IsProjectile:       configDef.IsProjectile,
		IsUtility:          configDef.IsUtility,
		VisualEffect:       configDef.VisualEffect,
		StatusIcon:         configDef.StatusIcon,
		StatBonus:          configDef.StatBonus,
		// Effect configuration from YAML
		HealAmount:     configDef.HealAmount,
		VisionBonus:    configDef.VisionBonus,
		TargetSelf:     configDef.TargetSelf,
		Awaken:         configDef.Awaken,
		WaterWalk:      configDef.WaterWalk,
		WaterBreathing: configDef.WaterBreathing,
		Message:        configDef.Message,
	}, nil
}

// CreateSpellItem creates an item from a spell definition
func CreateSpellItem(spellID SpellID) (items.Item, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return items.Item{}, err
	}

	var itemType items.ItemType
	var spellEffect items.SpellEffect

	// Dynamic spell effect assignment from YAML
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
	}, nil
}

// GetSpellIDByName returns dynamic SpellID for a given spell name
func GetSpellIDByName(name string) (SpellID, error) {
	if _, spellKey, exists := config.GetSpellDefinitionByName(name); exists {
		return SpellID(spellKey), nil
	}
	return "", fmt.Errorf("spell '%s' not found in spells.yaml", name)
}

// GetSpellIDsBySchool returns all spell IDs for a given magic school
func GetSpellIDsBySchool(school string) ([]SpellID, error) {
	spellKeys := config.GetSpellsBySchool(school)
	spellIDs := make([]SpellID, 0, len(spellKeys))

	for _, spellKey := range spellKeys {
		spellIDs = append(spellIDs, SpellID(spellKey))
	}

	return spellIDs, nil
}

// ConvertSpellIDToCharacterSpell converts a SpellID to character.Spell format
func ConvertSpellIDToCharacterSpell(spellID SpellID) (interface{}, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return nil, err
	}

	// This will be used to create character.Spell structs
	// We return interface{} to avoid circular imports for now
	return map[string]interface{}{
		"Name":        def.Name,
		"School":      def.School,
		"Level":       def.Level,
		"SpellPoints": def.SpellPointsCost,
		"Description": def.Description,
		"Duration":    def.Duration,
	}, nil
}
