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

// DisplayEquipSlots is the canonical render order of wearable equipment slots
// (excludes the spell slot). UI panels iterate this so adding a slot surfaces it
// everywhere without re-encoding the order per screen.
var DisplayEquipSlots = []EquipSlot{
	SlotMainHand, SlotOffHand, SlotArmor, SlotHelmet, SlotBoots,
	SlotCloak, SlotGauntlets, SlotBelt, SlotAmulet, SlotRing1, SlotRing2,
}

// DisplayName is the player-facing slot label (item cards, tooltips).
func (s EquipSlot) DisplayName() string {
	switch s {
	case SlotMainHand:
		return "Main Hand"
	case SlotOffHand:
		return "Off-Hand"
	case SlotArmor:
		return "Armor"
	case SlotHelmet:
		return "Helmet"
	case SlotBoots:
		return "Boots"
	case SlotCloak:
		return "Cloak"
	case SlotGauntlets:
		return "Gauntlets"
	case SlotBelt:
		return "Belt"
	case SlotAmulet:
		return "Amulet"
	case SlotRing1, SlotRing2:
		return "Ring"
	case SlotSpell:
		return "Quick Slot"
	}
	return "Gear"
}

type Item struct {
	Name        string
	Type        ItemType
	Attributes  map[string]int
	Description string
	Rarity      string
	// InstanceID uniquely identifies THIS physical item across save/load and the
	// cross-save stash. Assigned once at creation (the factories) and carried for
	// life. 0 means "untracked" (a pre-InstanceID save/stash item): it never
	// triggers stash reconciliation and is stamped lazily on load. Lets the stash
	// recognise "the copy I already hold" and strip it from a reloaded bag,
	// closing the save-scum dupe.
	InstanceID uint64 `json:"instance_id,omitempty"`
	// For armor
	ArmorCategory string
	// For spells
	SpellSchool string // Will use string instead of character.MagicSchoolID to avoid cycles
	SpellCost   int
	SpellEffect SpellEffect
}

// PreferredSlot resolves where this item equips from its equip_slot attribute,
// falling back to the given slot when unset. Single source for the
// equip_slot->slot mapping (used by EquipItem and the class-kit loader).
func (it Item) PreferredSlot(fallback EquipSlot) EquipSlot {
	if code, ok := it.Attributes["equip_slot"]; ok {
		return EquipSlot(code)
	}
	return fallback
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
	ItemTrinket      // Collectible curios (gems, trophies) - non-equippable, discardable, sellable.
	// ItemTrap and ItemCard are APPENDED at the end and must never be reordered:
	// saves serialize Type as an int, so inserting mid-enum re-types every item
	// after the insertion point (trinkets were briefly read back as traps).
	ItemTrap // Thief traps (quick-slot devices armed with Space/F)
	ItemCard // Collectible monster cards (party-wide collection via the card collector)
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
	case ItemTrap:
		return "Trap"
	case ItemTrinket:
		return "Trinket"
	case ItemCard:
		return "Card"
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

	desc := weaponDef.Description
	if weaponDef.Flavor != "" {
		desc = weaponDef.Flavor
	}

	it := Item{
		Name:        weaponDef.Name,
		Type:        ItemWeapon,
		Description: desc,
		Rarity:      weaponDef.Rarity,
		Attributes:  make(map[string]int),
		InstanceID:  NewInstanceID(),
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
	Flavor      string
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

// GlobalWeaponKeyByName is set by the config bridge to resolve a weapon's display
// name to its YAML key via the real name index - handles flavor names the naive
// transform below can't (e.g. "Kage-kunai, the Twin Shadows" -> "kage_kunai",
// "Kanabo" -> "kanabo"). Unset in isolated tests, where the fallback applies.
var GlobalWeaponKeyByName func(string) (string, bool)

// GetWeaponKeyByName returns the YAML weapon key for a display name. Prefers the
// exact config name index (punctuation/flavor-name safe); falls back to a
// lower+underscore transform only when the bridge is unset or the name is unknown.
// The fallback is why a weapon whose display name isn't its key-with-spaces (a
// comma or an "of the ...") was previously unequippable - now resolved.
func GetWeaponKeyByName(name string) string {
	if GlobalWeaponKeyByName != nil {
		if key, ok := GlobalWeaponKeyByName(name); ok {
			return key
		}
	}
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
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
	Resistances               map[string]int // per-school % damage resist (school->percent)
	Value                     int
	Revive                    bool
	FullHeal                  bool
	CurePoison                bool
	OpensMap                  bool
	PromotesLich              bool
	Discardable               bool
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
	case "card":
		t = ItemCard
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
	for school, pct := range def.Resistances {
		if pct != 0 {
			attrs["resist_"+strings.ToLower(strings.TrimSpace(school))] = pct
		}
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
	if def.Discardable {
		attrs["discardable"] = 1
	}
	if def.Revive {
		attrs["revive"] = 1
	}
	if def.FullHeal {
		attrs["full_heal"] = 1
	}
	if def.CurePoison {
		attrs["cure_poison"] = 1
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
		InstanceID:    NewInstanceID(),
	}, nil
}

// equipSlotByName is the ONE equip_slot name -> slot mapping. Config validation
// (validateItemConfig) checks names against it too, so a YAML typo fails at
// load instead of silently routing to the armor slot.
var equipSlotByName = map[string]EquipSlot{
	"armor":     SlotArmor,
	"helmet":    SlotHelmet,
	"boots":     SlotBoots,
	"cloak":     SlotCloak,
	"gauntlets": SlotGauntlets,
	"belt":      SlotBelt,
	"amulet":    SlotAmulet,
	"ring":      SlotRing1, // default to first ring slot
	"offhand":   SlotOffHand,
}

// EquipSlotFromName resolves an equip_slot name; ok=false for unknown names.
func EquipSlotFromName(name string) (EquipSlot, bool) {
	slot, ok := equipSlotByName[name]
	return slot, ok
}

// mapEquipSlotStringToCode converts equip_slot string to EquipSlot constant
func mapEquipSlotStringToCode(slotStr string) EquipSlot {
	if slot, ok := equipSlotByName[slotStr]; ok {
		return slot
	}
	return SlotArmor // unreachable for YAML items: load-time validation rejects unknown names
}
