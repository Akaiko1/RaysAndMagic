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
	DisintegrateChance float64
	AoeRadiusTiles     float64 // 0 = single-target; >0 = splash radius in tiles
	ProjectileSize     int
	IsProjectile       bool
	IsUtility          bool
	StatusIcon         string
	StatBonus          int // Stat bonus for buff spells like Bless
	// Damage-formula modifiers (default behaviour when zero/false)
	DamageCostMultiplier  int  // base = cost × SpellDamagePerSP × this (default 1)
	ScalesWithPersonality bool // also add Personality/divisor to spell damage
	// AoE-stun effect (Darkness): >0 radius stuns all monsters in range, no damage
	StunRadiusTiles     float64
	StunDurationSeconds int
	StunDurationTurns   int
	DealsNoDamage       bool // zero direct damage (Disintegrate: only the instakill roll matters)
	// Party combat buffs (duration seconds)
	ResistBuffPct           int // Day of the Gods: % incoming damage reduction
	OutgoingDamageBonus     int // Hour of Power: flat outgoing damage bonus
	IncomingDamageReduction int // Hour of Power: flat incoming damage reduction
	Charm                   bool // bind_undead: charm an undead target
	CharmDurationSeconds    int  // charm duration (RT seconds)
	Revive                  bool // resurrect: restore a fallen ally (incl. eradicated)
	FullHeal                bool // resurrect: restore to maximum HP
	// Effect configuration
	HealAmount     int     // For healing spells
	VisionBonus    float64 // For vision enhancement spells
	TargetSelf     bool    // Whether spell targets self or others
	Awaken         bool    // For awaken spell
	WaterWalk      bool    // For water walking spell
	WaterBreathing bool    // For water breathing spell
	Message        string  // Effect message to display
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
		DisintegrateChance: configDef.DisintegrateChance,
		AoeRadiusTiles:     configDef.AoeRadiusTiles,
		ProjectileSize:     configDef.ProjectileSize,
		IsProjectile:       configDef.IsProjectile,
		IsUtility:          configDef.IsUtility,
		StatusIcon:         configDef.StatusIcon,
		StatBonus:          configDef.StatBonus,
		DamageCostMultiplier:  configDef.DamageCostMultiplier,
		ScalesWithPersonality: configDef.ScalesWithPersonality,
		StunRadiusTiles:       configDef.StunRadiusTiles,
		StunDurationSeconds:   configDef.StunDurationSeconds,
		StunDurationTurns:     configDef.StunDurationTurns,
		DealsNoDamage:         configDef.DealsNoDamage,
		ResistBuffPct:           configDef.ResistBuffPct,
		OutgoingDamageBonus:     configDef.OutgoingDamageBonus,
		IncomingDamageReduction: configDef.IncomingDamageReduction,
		Charm:                   configDef.Charm,
		CharmDurationSeconds:    configDef.CharmDurationSeconds,
		Revive:                  configDef.Revive,
		FullHeal:                configDef.FullHeal,
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

	itemType := items.ItemBattleSpell
	if def.IsUtility {
		itemType = items.ItemUtilitySpell
	}

	return items.Item{
		Name:        def.Name,
		Type:        itemType,
		Description: def.Description,
		SpellSchool: def.School,
		SpellCost:   def.SpellPointsCost,
		SpellEffect: items.SpellEffect(spellID),
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

