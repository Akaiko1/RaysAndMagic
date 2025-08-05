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
	Damage    int
	Range     int    // In tiles
	BonusStat string // "Might", "Accuracy", "Intellect", etc. - which stat provides damage bonus
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

// WeaponType represents specific individual weapons (like SpellType)
type WeaponType int

const (
	WeaponTypeIronSword WeaponType = iota
	WeaponTypeMagicDagger
	WeaponTypeHolyMace
	WeaponTypeHuntingBow
	WeaponTypeOakStaff
	WeaponTypeSilverSword
	WeaponTypeIronSpear
	WeaponTypeGoldSword   // Now we can add multiple swords
	WeaponTypeSteelAxe    // And proper axe
	WeaponTypeElvenBow    // Multiple bows
	WeaponTypeBattleStaff // Multiple staves
)

// WeaponDefinition represents the complete definition of a weapon
type WeaponDefinition struct {
	Type        WeaponType
	Name        string
	Description string
	Category    string // "melee", "ranged", "magic"
	Damage      int
	Range       int    // In tiles
	BonusStat   string // "Might", "Accuracy", "Intellect"
	HitBonus    int    // Base accuracy bonus
	CritChance  int    // Critical hit chance percentage
	Rarity      string // "common", "uncommon", "rare", "legendary"
}

// SpellEffect represents dynamic spell effects (replaces hardcoded enum!)
type SpellEffect string

// Dynamic spell effect constants (these map to SpellID from config)
const (
	SpellEffectFireball    SpellEffect = "fireball"
	SpellEffectFireBolt    SpellEffect = "firebolt"
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

// GetWeaponDefinition returns the definition for a given weapon type
func GetWeaponDefinition(weaponType WeaponType) WeaponDefinition {
	definitions := map[WeaponType]WeaponDefinition{
		WeaponTypeIronSword: {
			Type:        WeaponTypeIronSword,
			Name:        "Iron Sword",
			Description: "A sturdy iron sword favored by knights",
			Category:    "sword",
			Damage:      8,
			Range:       2,
			BonusStat:   "Might",
			HitBonus:    5,
			CritChance:  10,
			Rarity:      "common",
		},
		WeaponTypeMagicDagger: {
			Type:        WeaponTypeMagicDagger,
			Name:        "Magic Dagger",
			Description: "A light dagger enchanted for spellcasters",
			Category:    "dagger",
			Damage:      4,
			Range:       2,
			BonusStat:   "Accuracy",
			HitBonus:    10,
			CritChance:  15,
			Rarity:      "uncommon",
		},
		WeaponTypeHolyMace: {
			Type:        WeaponTypeHolyMace,
			Name:        "Holy Mace",
			Description: "A blessed mace that channels divine power",
			Category:    "mace",
			Damage:      6,
			Range:       2,
			BonusStat:   "Might",
			HitBonus:    5,
			CritChance:  8,
			Rarity:      "uncommon",
		},
		WeaponTypeHuntingBow: {
			Type:        WeaponTypeHuntingBow,
			Name:        "Hunting Bow",
			Description: "A well-crafted bow for precise ranged combat",
			Category:    "bow",
			Damage:      6,
			Range:       8,
			BonusStat:   "Accuracy",
			HitBonus:    15,
			CritChance:  12,
			Rarity:      "common",
		},
		WeaponTypeOakStaff: {
			Type:        WeaponTypeOakStaff,
			Name:        "Oak Staff",
			Description: "A staff carved from ancient oak, perfect for magic users",
			Category:    "staff",
			Damage:      5,
			Range:       2,
			BonusStat:   "Intellect",
			HitBonus:    8,
			CritChance:  5,
			Rarity:      "common",
		},
		WeaponTypeSilverSword: {
			Type:        WeaponTypeSilverSword,
			Name:        "Silver Sword",
			Description: "A blessed silver sword with enhanced power",
			Category:    "sword",
			Damage:      10,
			Range:       2,
			BonusStat:   "Might",
			HitBonus:    8,
			CritChance:  12,
			Rarity:      "rare",
		},
		WeaponTypeIronSpear: {
			Type:        WeaponTypeIronSpear,
			Name:        "Iron Spear",
			Description: "A long-reaching iron spear",
			Category:    "spear",
			Damage:      7,
			Range:       3,
			BonusStat:   "Might",
			HitBonus:    6,
			CritChance:  9,
			Rarity:      "common",
		},
		WeaponTypeGoldSword: {
			Type:        WeaponTypeGoldSword,
			Name:        "Gold Sword",
			Description: "A magnificent sword forged from pure gold",
			Category:    "sword",
			Damage:      12,
			Range:       2,
			BonusStat:   "Might",
			HitBonus:    10,
			CritChance:  15,
			Rarity:      "rare",
		},
		WeaponTypeSteelAxe: {
			Type:        WeaponTypeSteelAxe,
			Name:        "Steel Axe",
			Description: "A heavy steel axe that cleaves through armor",
			Category:    "axe",
			Damage:      11,
			Range:       2,
			BonusStat:   "Might",
			HitBonus:    3,
			CritChance:  18,
			Rarity:      "uncommon",
		},
		WeaponTypeElvenBow: {
			Type:        WeaponTypeElvenBow,
			Name:        "Elven Bow",
			Description: "An elegant elven bow with superior accuracy",
			Category:    "bow",
			Damage:      8,
			Range:       10,
			BonusStat:   "Accuracy",
			HitBonus:    20,
			CritChance:  14,
			Rarity:      "rare",
		},
		WeaponTypeBattleStaff: {
			Type:        WeaponTypeBattleStaff,
			Name:        "Battle Staff",
			Description: "A reinforced staff designed for combat mages",
			Category:    "staff",
			Damage:      7,
			Range:       2,
			BonusStat:   "Intellect",
			HitBonus:    10,
			CritChance:  8,
			Rarity:      "uncommon",
		},
	}

	return definitions[weaponType]
}

// CreateWeaponFromDefinition creates a weapon item from a weapon definition
func CreateWeaponFromDefinition(weaponType WeaponType) Item {
	def := GetWeaponDefinition(weaponType)
	return Item{
		Name:        def.Name,
		Type:        ItemWeapon,
		Damage:      def.Damage,
		Range:       def.Range,
		BonusStat:   def.BonusStat,
		Description: def.Description,
		Attributes:  make(map[string]int),
	}
}

// GetWeaponTypeByName returns the weapon type for a given weapon name
func GetWeaponTypeByName(name string) WeaponType {
	nameMap := map[string]WeaponType{
		"Iron Sword":   WeaponTypeIronSword,
		"Magic Dagger": WeaponTypeMagicDagger,
		"Holy Mace":    WeaponTypeHolyMace,
		"Hunting Bow":  WeaponTypeHuntingBow,
		"Oak Staff":    WeaponTypeOakStaff,
		"Silver Sword": WeaponTypeSilverSword,
		"Iron Spear":   WeaponTypeIronSpear,
		"Gold Sword":   WeaponTypeGoldSword,
		"Steel Axe":    WeaponTypeSteelAxe,
		"Elven Bow":    WeaponTypeElvenBow,
		"Battle Staff": WeaponTypeBattleStaff,
	}

	if weaponType, exists := nameMap[name]; exists {
		return weaponType
	}

	return WeaponTypeIronSword // Default fallback
}
