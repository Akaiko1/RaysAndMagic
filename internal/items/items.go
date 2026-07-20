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
	// InstanceID identifies this item across save/load and the cross-save stash.
	// Stackable items retain it as the primary lineage ID for compatibility with
	// older saves; Lineages carries the complete provenance after stacks merge.
	// 0 means "untracked" (a pre-InstanceID save/stash item): it never triggers
	// stash reconciliation and is stamped lazily on load.
	InstanceID uint64 `json:"instance_id,omitempty"`
	// Quantity is the stack size for stackable items (consumables, trinkets).
	// 0 means 1 (items saved before stacking existed) - always read through
	// Count().
	Quantity int `json:"quantity,omitempty"`
	// Lineages records the source units that make up a merged stack. It is
	// omitted for the common single-lineage case, which older saves represent by
	// InstanceID alone. Keeping the components lets the shared stash remove only
	// its own units from a stale save even after those units were merged with
	// another stack.
	Lineages []StackLineage `json:"stack_lineages,omitempty"`
	// For armor
	ArmorCategory string
	// Set is the armor-set key (items.yaml item_sets) this piece belongs to.
	Set string `json:"set,omitempty"`
	// For spells
	SpellSchool string // Will use string instead of character.MagicSchoolID to avoid cycles
	SpellCost   int
	SpellEffect SpellEffect
}

// StackLineage identifies quantity fungible units that came from the same
// pre-stash stack. ID 0 is deliberately not persisted as a component: zero-ID
// legacy items are untracked until the normal load migration stamps them.
type StackLineage struct {
	ID       uint64 `json:"id"`
	Quantity int    `json:"quantity"`
}

// Count returns the stack size, treating the zero value as a single item so
// pre-stacking saves and freshly created items need no migration.
func (it Item) Count() int {
	if it.Quantity < 1 {
		return 1
	}
	return it.Quantity
}

// Stackable reports whether copies of this item merge into one inventory
// stack. Only fungible pocket goods stack: gear carries per-instance identity
// (equip state), quest items are unique, cards live in the collection.
func (it Item) Stackable() bool {
	return it.Type == ItemConsumable || it.Type == ItemTrinket
}

// StackLineageParts returns a normalized copy of the stack's provenance. Old
// single-lineage saves derive one component from InstanceID, so callers never
// need a separate legacy branch.
func (it Item) StackLineageParts() []StackLineage {
	if !it.Stackable() || it.InstanceID == 0 {
		return nil
	}
	return normalizeStackLineages(it.Lineages, it.InstanceID, it.Count())
}

// MergeStack adds another fungible stack using the one provenance-preserving
// merge path. SameStack remains the product-identity check; lineage does not
// prevent stack merging in the UI.
func (it *Item) MergeStack(other Item) bool {
	if it == nil || !SameStack(*it, other) {
		return false
	}
	// A pre-ID stack can arrive from an old in-memory fixture or an extension
	// path before the normal load migration. Stamp it at the mutation boundary
	// so no untracked units are folded into another lineage.
	EnsureInstanceID(it)
	EnsureInstanceID(&other)
	parts := append(it.StackLineageParts(), other.StackLineageParts()...)
	it.Quantity = it.Count() + other.Count()
	it.setStackLineageParts(parts)
	return true
}

// ConsumeStackUnits removes units from a stack while preserving provenance for
// the units that remain. Whole-entry removal stays the owning container's job.
func (it *Item) ConsumeStackUnits(quantity int) bool {
	if it == nil || !it.Stackable() || quantity < 1 || quantity >= it.Count() {
		return false
	}
	parts := it.StackLineageParts()
	if len(parts) == 0 {
		it.Quantity = it.Count() - quantity
		return true
	}
	remainingToConsume := quantity
	kept := make([]StackLineage, 0, len(parts))
	for _, part := range parts {
		consumed := part.Quantity
		if consumed > remainingToConsume {
			consumed = remainingToConsume
		}
		part.Quantity -= consumed
		remainingToConsume -= consumed
		if part.Quantity > 0 {
			kept = append(kept, part)
		}
	}
	it.Quantity = it.Count() - quantity
	it.setStackLineageParts(kept)
	return true
}

// SplitOff removes quantity units from a stack and returns them as a separate
// item. The moved fragment keeps its source lineage and only the residual units
// from a split component are rekeyed. This is the direction used when a party
// stack is partially deposited into the shared stash.
func (it *Item) SplitOff(quantity int) (Item, bool) {
	return it.splitOff(quantity, true)
}

// SplitOffForStashWithdrawal is the inverse partial-transfer direction. The
// stash keeps the old lineage for units it still owns; only the withdrawn piece
// of a split component is rekeyed. That prevents the live party save from
// looking like it still owns the same units as the chest on a later reload.
func (it *Item) SplitOffForStashWithdrawal(quantity int) (Item, bool) {
	return it.splitOff(quantity, false)
}

func (it *Item) splitOff(quantity int, movedKeepsSplitLineage bool) (Item, bool) {
	if it == nil || !it.Stackable() || quantity < 1 || quantity >= it.Count() {
		return Item{}, false
	}
	EnsureInstanceID(it)
	fragment := *it
	parts := it.StackLineageParts()
	if len(parts) == 0 {
		fragment.Quantity = quantity
		it.Quantity = it.Count() - quantity
		if movedKeepsSplitLineage {
			it.InstanceID = NewInstanceID()
		} else {
			fragment.InstanceID = NewInstanceID()
		}
		return fragment, true
	}

	remainingToMove := quantity
	moved := make([]StackLineage, 0, len(parts))
	kept := make([]StackLineage, 0, len(parts))
	for _, part := range parts {
		if remainingToMove == 0 {
			kept = append(kept, part)
			continue
		}
		movedQuantity := part.Quantity
		if movedQuantity > remainingToMove {
			movedQuantity = remainingToMove
		}
		remainingToMove -= movedQuantity
		movedPart := StackLineage{ID: part.ID, Quantity: movedQuantity}
		leftQuantity := part.Quantity - movedQuantity
		if leftQuantity > 0 {
			leftPart := StackLineage{ID: part.ID, Quantity: leftQuantity}
			if movedKeepsSplitLineage {
				leftPart.ID = NewInstanceID()
			} else {
				movedPart.ID = NewInstanceID()
			}
			kept = append(kept, leftPart)
		}
		moved = append(moved, movedPart)
	}

	fragment.Quantity = quantity
	fragment.setStackLineageParts(moved)
	it.Quantity = it.Count() - quantity
	it.setStackLineageParts(kept)
	return fragment, true
}

// StripStashOwnedUnits removes only the lineage units currently owned by the
// shared stash. claims is consumed globally by the caller, which matters when
// a stale save carries the same original lineage in more than one location.
// It returns whether the item survives and whether a surviving split component
// had to be rekeyed for idempotent future loads.
func (it *Item) StripStashOwnedUnits(claims map[uint64]int) (keep, rekeyed bool) {
	if it == nil || it.Name == "" || it.InstanceID == 0 {
		return true, false
	}
	if !it.Stackable() {
		if claims[it.InstanceID] < 1 {
			return true, false
		}
		claims[it.InstanceID]--
		return false, false
	}
	parts := it.StackLineageParts()
	if len(parts) == 0 {
		return true, false
	}
	kept := make([]StackLineage, 0, len(parts))
	remaining := 0
	for _, part := range parts {
		claimed := claims[part.ID]
		removed := part.Quantity
		if removed > claimed {
			removed = claimed
		}
		claims[part.ID] -= removed
		part.Quantity -= removed
		if part.Quantity == 0 {
			continue
		}
		if removed > 0 {
			part.ID = NewInstanceID()
			rekeyed = true
		}
		remaining += part.Quantity
		kept = append(kept, part)
	}
	if remaining == 0 {
		return false, false
	}
	it.Quantity = remaining
	it.setStackLineageParts(kept)
	return true, rekeyed
}

func (it *Item) setStackLineageParts(parts []StackLineage) {
	if it == nil || !it.Stackable() {
		return
	}
	parts = normalizeStackLineages(parts, it.InstanceID, it.Count())
	if len(parts) == 0 {
		it.Lineages = nil
		return
	}
	it.InstanceID = parts[0].ID
	if len(parts) == 1 && parts[0].Quantity == it.Count() {
		it.Lineages = nil
		return
	}
	it.Lineages = parts
}

func normalizeStackLineages(parts []StackLineage, fallbackID uint64, count int) []StackLineage {
	if count < 1 {
		return nil
	}
	remaining := count
	normalized := make([]StackLineage, 0, len(parts)+1)
	indexByID := make(map[uint64]int, len(parts)+1)
	add := func(id uint64, quantity int) {
		if id == 0 || quantity < 1 || remaining == 0 {
			return
		}
		if quantity > remaining {
			quantity = remaining
		}
		if i, ok := indexByID[id]; ok {
			normalized[i].Quantity += quantity
		} else {
			indexByID[id] = len(normalized)
			normalized = append(normalized, StackLineage{ID: id, Quantity: quantity})
		}
		remaining -= quantity
	}
	for _, part := range parts {
		add(part.ID, part.Quantity)
	}
	// A malformed or older stack with missing component data remains owned by its
	// legacy primary ID rather than silently dropping units from deduplication.
	if fallbackID != 0 {
		add(fallbackID, remaining)
	}
	return normalized
}

// SameStack is THE stack-identity rule: both stackable and the same item
// definition (display name + type; item display names are validated unique at
// load). Every merge site (bag, load-time fold, quick slots) must use this -
// hand-rolled copies drift.
func SameStack(a, b Item) bool {
	return a.Stackable() && b.Stackable() && a.Name == b.Name && a.Type == b.Type
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
		Set:         weaponDef.Set,
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
	Set         string // equipment-set key this weapon piece belongs to
	// EquipPersonalityMin: any character with this much effective Personality
	// may wield the weapon without its category skill (Lanista's Scepter).
	EquipPersonalityMin int
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
	DoorKey                   int
	MasterKey                 int
	OpensMap                  bool
	PromotesLich              bool
	Discardable               bool
	Set                       string // armor-set key this piece belongs to
	PartyArmorBonus           int    // flat AC to every OTHER party member while equipped
	ManaBase                  int    // consumable: SP restored (+ Personality/divisor)
	ManaPersonalityDivisor    int
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
	if def.ManaBase != 0 {
		attrs["mana_base"] = def.ManaBase
	}
	if def.ManaPersonalityDivisor != 0 {
		attrs["mana_personality_divisor"] = def.ManaPersonalityDivisor
	}
	if def.PartyArmorBonus != 0 {
		attrs["party_armor_bonus"] = def.PartyArmorBonus
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
	if def.DoorKey != 0 {
		attrs["door_key"] = def.DoorKey
	}
	if def.MasterKey != 0 {
		attrs["master_key"] = def.MasterKey
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
		Set:           def.Set,
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
