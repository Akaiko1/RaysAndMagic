package items

import (
	"strings"
)

type EquipSlot int

const (
	SlotMainHand EquipSlot = iota
	SlotOffHand
	SlotArmor
	SlotHelmet
	SlotBoots
	SlotCloak
	SlotGauntlets
	SlotBelt
	SlotAmulet
	SlotRing1
	SlotRing2
	SlotBattleSpell  // Legacy slot for equipped battle spell
	SlotUtilitySpell // Legacy slot for equipped utility spell
	SlotSpell        // New unified spell slot for any spell
)

type Item struct {
	Name        string
	Type        ItemType
	Attributes  map[string]int
	Description string
	// For weapons
	Damage    int
	Range     int    // In tiles
	BonusStat string // "Might", "Accuracy", "Intellect", etc. - which stat provides damage bonus
	// Advanced weapon properties
	BonusStatSecondary string // Secondary scaling stat (e.g., "Intellect")
	DamageType         string // "physical", "fire", "dark", etc.
	MaxProjectiles     int    // Maximum projectiles allowed at once (0 = unlimited)
	// For spells
	SpellSchool string // Will use string instead of character.MagicSchool to avoid cycles
	SpellCost   int
	SpellEffect SpellEffect
}

type ItemType int

const (
	ItemWeapon ItemType = iota
	ItemArmor
	ItemAccessory
	ItemConsumable
	ItemQuest
	ItemBattleSpell  // Offensive spells (Fireball, Lightning, etc.)
	ItemUtilitySpell // Support spells (Heal, Buffs, etc.)
)

// Legacy weapon types removed - use YAML weapon keys instead

// SpellEffect represents dynamic spell effects (replaces hardcoded enum!)
type SpellEffect string

// Dynamic spell effect constants (these map to SpellID from config)
const (
	SpellEffectFireball    SpellEffect = "fireball"
	SpellEffectFireBolt    SpellEffect = "firebolt"
	SpellEffectIceBolt     SpellEffect = "ice_bolt"
	SpellEffectTorchLight  SpellEffect = "torch_light"
	SpellEffectLightning   SpellEffect = "lightning"
	SpellEffectIceShard    SpellEffect = "ice_shard"
	SpellEffectHealSelf    SpellEffect = "heal"
	SpellEffectHealOther   SpellEffect = "heal_other"
	SpellEffectPartyBuff   SpellEffect = "party_buff"
	SpellEffectShield      SpellEffect = "shield"
	SpellEffectBless       SpellEffect = "bless"
	SpellEffectWizardEye   SpellEffect = "wizard_eye"
	SpellEffectAwaken      SpellEffect = "awaken"
	SpellEffectWalkOnWater SpellEffect = "walk_on_water"
)

// SpellEffectToSpellID converts SpellEffect to SpellID (dynamic mapping!)
func SpellEffectToSpellID(effect SpellEffect) string {
	return string(effect) // Direct conversion since they're the same now!
}

// SpellIDToSpellEffect converts SpellID to SpellEffect
func SpellIDToSpellEffect(spellID string) SpellEffect {
	return SpellEffect(spellID) // Direct conversion since they're the same now!
}

// Helper functions to create items
func CreateWeapon(name string, damage, weaponRange int, bonusStat, description string) Item {
	return Item{
		Name:        name,
		Type:        ItemWeapon,
		Damage:      damage,
		Range:       weaponRange,
		BonusStat:   bonusStat,
		Description: description,
		Attributes:  make(map[string]int),
	}
}

func CreateBattleSpell(name string, effect SpellEffect, school string, cost int, description string) Item {
	return Item{
		Name:        name,
		Type:        ItemBattleSpell,
		SpellEffect: effect,
		SpellSchool: school,
		SpellCost:   cost,
		Description: description,
		Attributes:  make(map[string]int),
	}
}

func CreateUtilitySpell(name string, effect SpellEffect, school string, cost int, description string) Item {
	return Item{
		Name:        name,
		Type:        ItemUtilitySpell,
		SpellEffect: effect,
		SpellSchool: school,
		SpellCost:   cost,
		Description: description,
		Attributes:  make(map[string]int),
	}
}

// CreateWeaponFromYAML creates a weapon item from YAML weapon configuration
func CreateWeaponFromYAML(weaponKey string) Item {
	// Import here to avoid circular dependency - access global config
	weaponDef, exists := getWeaponDefinitionFromGlobal(weaponKey)
	if !exists {
		panic("weapon '" + weaponKey + "' not found in weapons.yaml - system misconfigured")
	}

	return Item{
		Name:               weaponDef.Name,
		Type:               ItemWeapon,
		Damage:             weaponDef.Damage,
		Range:              weaponDef.Range,
		BonusStat:          weaponDef.BonusStat,
		BonusStatSecondary: weaponDef.BonusStatSecondary,
		DamageType:         weaponDef.DamageType,
		MaxProjectiles:     weaponDef.MaxProjectiles,
		Description:        weaponDef.Description,
		Attributes:         make(map[string]int),
	}
}

// getWeaponDefinitionFromGlobal accesses weapon definition from global config
func getWeaponDefinitionFromGlobal(weaponKey string) (*WeaponDefinitionFromYAML, bool) {
	// This will access the global weapon config that's loaded in main
	// We'll define a simple struct to avoid circular imports
	return getGlobalWeaponDef(weaponKey)
}

// WeaponDefinitionFromYAML represents weapon data without circular import
type WeaponDefinitionFromYAML struct {
	Name               string
	Description        string
	Category           string
	Damage             int
	Range              int
	BonusStat          string
	BonusStatSecondary string // Secondary scaling stat
	DamageType         string // Damage element type
	MaxProjectiles     int    // Max projectiles at once
	HitBonus           int
	CritChance         int
	Rarity             string
}

// getGlobalWeaponDef accesses the global weapon configuration
func getGlobalWeaponDef(weaponKey string) (*WeaponDefinitionFromYAML, bool) {
	// We'll use an external accessor to avoid circular imports
	return GlobalWeaponAccessor(weaponKey)
}

// GlobalWeaponAccessor is set by the config module to provide weapon access
var GlobalWeaponAccessor func(string) (*WeaponDefinitionFromYAML, bool)

// GetWeaponKeyByName returns the YAML weapon key for a given weapon name

// GetWeaponKeyByName dynamically converts a display name to a YAML weapon key.
// Example: "Steel Axe" -> "steel_axe"
func GetWeaponKeyByName(name string) string {
	key := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	return key
}
