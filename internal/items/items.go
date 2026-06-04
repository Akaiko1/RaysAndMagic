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
	SlotSpell // Unified spell slot for any spell
)

type Item struct {
	Name        string
	Type        ItemType
	Attributes  map[string]int
	Description string
	Rarity      string
	// For armor
	ArmorCategory string
	// For spells
	SpellSchool string // Will use string instead of character.MagicSchoolID to avoid cycles
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
	ItemTrinket      // Collectible cards/curios — non-equippable, discardable, sellable.
)

// String returns the display name of the item type (Stringer interface).
func (t ItemType) String() string {
	switch t {
	case ItemWeapon:
		return "Weapon"
	case ItemArmor:
		return "Armor"
	case ItemAccessory:
		return "Accessory"
	case ItemConsumable:
		return "Consumable"
	case ItemQuest:
		return "Quest Item"
	case ItemBattleSpell:
		return "Battle Spell"
	case ItemUtilitySpell:
		return "Utility Spell"
	case ItemTrinket:
		return "Trinket"
	default:
		return "Unknown"
	}
}

// SpellEffect stores the spell identifier on spell items.
type SpellEffect string

// Spell effect constants mirror spell IDs from config.
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

// Helper functions to create items
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
		Name:        weaponDef.Name,
		Type:        ItemWeapon,
		Description: weaponDef.Description,
		Rarity:      weaponDef.Rarity,
		Attributes:  make(map[string]int),
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

// WeaponDefinitionFromYAML is the minimal mirror of config.WeaponDefinitionConfig
// needed by packages that can't import config (items, character). All combat
// math reads from config.WeaponDefinitionConfig directly; this struct only
// exposes the fields needed to construct a base items.Item and to enforce
// class skill checks.
type WeaponDefinitionFromYAML struct {
	Name        string
	Description string
	Category    string
	Rarity      string
	Value       int
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
	Rarity      string
	// Optional numeric stats
	ArmorClassBase            int
	EnduranceScalingDivisor   int
	IntellectScalingDivisor   int
	PersonalityScalingDivisor int
	BonusIntellect            int
	BonusPersonality          int
	BonusEndurance            int
	BonusAccuracy             int
	BonusSpeed                int
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
	PromotesLich              bool
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
	case "trinket":
		t = ItemTrinket
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
	if def.BonusIntellect != 0 {
		attrs["bonus_intellect"] = def.BonusIntellect
	}
	if def.BonusPersonality != 0 {
		attrs["bonus_personality"] = def.BonusPersonality
	}
	if def.BonusEndurance != 0 {
		attrs["bonus_endurance"] = def.BonusEndurance
	}
	if def.BonusAccuracy != 0 {
		attrs["bonus_accuracy"] = def.BonusAccuracy
	}
	if def.BonusSpeed != 0 {
		attrs["bonus_speed"] = def.BonusSpeed
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
	if def.PromotesLich {
		attrs["promotes_lich"] = 1
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
		Rarity:        def.Rarity,
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
