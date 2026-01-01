package items

import (
	"fmt"
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
	// For armor
	ArmorCategory string
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

// CreateWeaponFromYAML creates a weapon item from YAML weapon configuration.
// Returns an error if the weapon is not found in weapons.yaml.
func CreateWeaponFromYAML(weaponKey string) Item {
	item, err := TryCreateWeaponFromYAML(weaponKey)
	if err != nil {
		// Panic for backwards compatibility with initialization code that expects this to succeed
		panic("weapon '" + weaponKey + "' not found in weapons.yaml - system misconfigured")
	}
	return item
}

// TryCreateWeaponFromYAML creates a weapon item from YAML weapon configuration.
// Returns an error if the weapon is not found.
func TryCreateWeaponFromYAML(weaponKey string) (Item, error) {
	weaponDef, exists := getWeaponDefinitionFromGlobal(weaponKey)
	if !exists {
		return Item{}, fmt.Errorf("weapon '%s' not found in weapons.yaml", weaponKey)
	}

	it := Item{
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
	if weaponDef.Value > 0 {
		it.Attributes["value"] = weaponDef.Value
	}
	return it, nil
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
	Value              int
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

// ------- Non-weapon items from YAML -------

// ItemDefinitionFromYAML represents simple item data from YAML
type ItemDefinitionFromYAML struct {
	Name        string
	Description string
	Flavor      string
	Type        string // "armor", "accessory", "consumable", "quest"
	ArmorType   string
	// Optional numeric stats
	ArmorClassBase            int
	EnduranceScalingDivisor   int
	IntellectScalingDivisor   int
	PersonalityScalingDivisor int
	BonusLuck                 int
	HealBase                  int
	HealEnduranceDivisor      int
	SummonDistanceTiles       int
	EquipSlot                 string
	BonusMight                int
	Value                     int
	Revive                    bool
	FullHeal                  bool
	OpensMap                  bool
}

// GlobalItemAccessor is set by a bridge to provide item access without circular imports
var GlobalItemAccessor func(string) (*ItemDefinitionFromYAML, bool)

// CreateItemFromYAML creates a non-weapon, non-spell item from YAML item definition.
// Panics if item is not found - use TryCreateItemFromYAML for error handling.
func CreateItemFromYAML(itemKey string) Item {
	item, err := TryCreateItemFromYAML(itemKey)
	if err != nil {
		panic(err.Error())
	}
	return item
}

// TryCreateItemFromYAML creates a non-weapon, non-spell item from YAML item definition.
// Returns an error if the item is not found or has an unknown type.
func TryCreateItemFromYAML(itemKey string) (Item, error) {
	if GlobalItemAccessor == nil {
		return Item{}, fmt.Errorf("item accessor not configured - call bridge.SetupItemBridge()")
	}
	def, ok := GlobalItemAccessor(itemKey)
	if !ok || def == nil {
		return Item{}, fmt.Errorf("item '%s' not found in items.yaml", itemKey)
	}

	// Map type string to ItemType
	var t ItemType
	switch def.Type {
	case "armor":
		t = ItemArmor
	case "accessory":
		t = ItemAccessory
	case "consumable":
		t = ItemConsumable
	case "quest":
		t = ItemQuest
	default:
		return Item{}, fmt.Errorf("unknown item type for '%s': %s", itemKey, def.Type)
	}

	// Populate attributes from definition
	attrs := make(map[string]int)
	if def.ArmorClassBase != 0 {
		attrs["armor_class_base"] = def.ArmorClassBase
	}
	if def.EnduranceScalingDivisor != 0 {
		attrs["endurance_scaling_divisor"] = def.EnduranceScalingDivisor
	}
	if def.IntellectScalingDivisor != 0 {
		attrs["intellect_scaling_divisor"] = def.IntellectScalingDivisor
	}
	if def.PersonalityScalingDivisor != 0 {
		attrs["personality_scaling_divisor"] = def.PersonalityScalingDivisor
	}
	if def.BonusLuck != 0 {
		attrs["bonus_luck"] = def.BonusLuck
	}
	if def.HealBase != 0 {
		attrs["heal_base"] = def.HealBase
	}
	if def.HealEnduranceDivisor != 0 {
		attrs["heal_endurance_divisor"] = def.HealEnduranceDivisor
	}
	if def.SummonDistanceTiles != 0 {
		attrs["summon_distance_tiles"] = def.SummonDistanceTiles
	}
	// Map equip_slot string to EquipSlot constant and store in attributes
	if def.EquipSlot != "" {
		slotCode := mapEquipSlotStringToCode(def.EquipSlot)
		attrs["equip_slot"] = int(slotCode)
	}
	if def.BonusMight != 0 {
		attrs["bonus_might"] = def.BonusMight
	}
	if def.Value != 0 {
		attrs["value"] = def.Value
	}
	if def.OpensMap {
		attrs["opens_map"] = 1
	}
	if def.Revive {
		attrs["revive"] = 1
	}
	if def.FullHeal {
		attrs["full_heal"] = 1
	}

	// Prefer flavor text if provided, else use description
	desc := def.Description
	if def.Flavor != "" {
		desc = def.Flavor
	}

	return Item{
		Name:          def.Name,
		Type:          t,
		Description:   desc,
		Attributes:    attrs,
		ArmorCategory: def.ArmorType,
	}, nil
}

// mapEquipSlotStringToCode converts equip_slot string to EquipSlot constant
func mapEquipSlotStringToCode(slotStr string) EquipSlot {
	switch slotStr {
	case "armor":
		return SlotArmor
	case "helmet":
		return SlotHelmet
	case "boots":
		return SlotBoots
	case "cloak":
		return SlotCloak
	case "gauntlets":
		return SlotGauntlets
	case "belt":
		return SlotBelt
	case "amulet":
		return SlotAmulet
	case "ring":
		return SlotRing1 // Default to first ring slot
	case "offhand":
		return SlotOffHand
	default:
		return SlotArmor // Default fallback for armor-type items
	}
}
