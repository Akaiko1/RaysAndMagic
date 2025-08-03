package items

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
	Damage int
	Range  int // In tiles
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

type SpellEffect int

const (
	SpellEffectFireball SpellEffect = iota
	SpellEffectFireBolt
	SpellEffectTorchLight
	SpellEffectLightning
	SpellEffectIceShard
	SpellEffectHealSelf
	SpellEffectHealOther
	SpellEffectPartyBuff
	SpellEffectShield
	SpellEffectBless
	SpellEffectWizardEye
	SpellEffectAwaken
	SpellEffectWalkOnWater
)

// Helper functions to create items
func CreateWeapon(name string, damage, weaponRange int, description string) Item {
	return Item{
		Name:        name,
		Type:        ItemWeapon,
		Damage:      damage,
		Range:       weaponRange,
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
