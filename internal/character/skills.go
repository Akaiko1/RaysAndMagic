package character

type SkillType int

const (
	// Weapon skills
	SkillSword SkillType = iota
	SkillDagger
	SkillAxe
	SkillSpear
	SkillBow
	SkillMace
	SkillStaff

	// Armor skills
	SkillLeather
	SkillChain
	SkillPlate
	SkillShield

	// Misc skills.
	// Implemented effects (scaled by SkillTier, see combat/character code):
	//   Bodybuilding -> +Max HP, Meditation -> faster mana regen,
	//   Learning -> +% XP gained, ArmsMaster -> +weapon damage,
	//   Merchant -> better buy/sell prices,
	//   DisarmTrap -> chest-trap disarm chance + personal flat damage reduction.
	SkillBodybuilding
	SkillMeditation
	SkillMerchant
	// SkillRepair: no effect yet. TODO: implement an item-durability system
	// (gear wears with use; Repair restores it / slows wear) for this to matter.
	SkillRepair
	// SkillIdentifyItem: no effect yet. TODO: implement unidentified loot (dropped
	// items start with hidden stats until a party member with Identify inspects).
	SkillIdentifyItem
	// SkillDisarmTrap: the party's best active user can disarm chest traps; each
	// trained character also gains personal flat damage reduction.
	SkillDisarmTrap
	SkillLearning
	SkillArmsMaster
	// SkillTrapper: thief trap mastery - +damage per tier for damage traps,
	// +1 turn / +5 sec per tier for duration traps (constants in catalog.go).
	SkillTrapper
	// SkillSleightOfHand: on melee hits, 10%/tier chance to pick the victim's
	// pocket - rolls its loot table, consolation gold on a miss.
	SkillSleightOfHand
	// SkillDualWielding: Novice unlocks a second weapon in the off-hand;
	// Expert+ shaves weapon cooldown (tier*10%, zero at Novice - see catalog.go).
	SkillDualWielding
	// SkillIronBody: flat Armor Class per tier INCLUDING Novice, plus GM dodge -
	// see catalog.go.
	SkillIronBody
	// SkillSpiritualTraining: chance on a melee hit to also fire the
	// slotted quick-spell for free - see catalog.go.
	SkillSpiritualTraining
	// SkillMartialArts: unarmed combat - gates the Monk's fists. Appended last:
	// SkillType persists as a raw int, so new skills go at the end, never mid-block.
	SkillMartialArts
)

// String returns the display name of the skill (Stringer interface).
func (s SkillType) String() string {
	switch s {
	case SkillSword:
		return "Sword"
	case SkillDagger:
		return "Dagger"
	case SkillAxe:
		return "Axe"
	case SkillSpear:
		return "Spear"
	case SkillBow:
		return "Bow"
	case SkillMace:
		return "Mace"
	case SkillStaff:
		return "Staff"
	case SkillMartialArts:
		return "Martial Arts"
	case SkillLeather:
		return "Leather"
	case SkillChain:
		return "Chain"
	case SkillPlate:
		return "Plate"
	case SkillShield:
		return "Shield"
	case SkillBodybuilding:
		return "Bodybuilding"
	case SkillMeditation:
		return "Meditation"
	case SkillMerchant:
		return "Merchant"
	case SkillRepair:
		return "Repair"
	case SkillIdentifyItem:
		return "Identify Item"
	case SkillDisarmTrap:
		return "Disarm Trap"
	case SkillLearning:
		return "Learning"
	case SkillArmsMaster:
		return "Arms Master"
	case SkillTrapper:
		return "Trapper"
	case SkillSleightOfHand:
		return "Sleight of Hand"
	case SkillDualWielding:
		return "Dual Wielding"
	case SkillIronBody:
		return "Iron Body"
	case SkillSpiritualTraining:
		return "Spiritual Training"
	default:
		return "Unknown"
	}
}

type SkillMastery int

const (
	MinSkillLevel = 1
	MaxSkillLevel = 4
)

const (
	MasteryNovice SkillMastery = iota
	MasteryExpert
	MasteryMaster
	MasteryGrandMaster
)

// String returns the display name of the mastery level (Stringer interface).
func (m SkillMastery) String() string {
	switch m {
	case MasteryNovice:
		return "Novice"
	case MasteryExpert:
		return "Expert"
	case MasteryMaster:
		return "Master"
	case MasteryGrandMaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

// masteryFromKey resolves a lowercase config.yaml mastery key (class kit
// skill_start_mastery) to its SkillMastery. Returns false for an unknown key.
// MasteryFromKey resolves a YAML mastery key (novice/expert/master/grandmaster).
func MasteryFromKey(key string) (SkillMastery, bool) { return masteryFromKey(key) }

func masteryFromKey(key string) (SkillMastery, bool) {
	switch key {
	case "novice":
		return MasteryNovice, true
	case "expert":
		return MasteryExpert, true
	case "master":
		return MasteryMaster, true
	case "grandmaster":
		return MasteryGrandMaster, true
	default:
		return 0, false
	}
}

type Skill struct {
	Mastery SkillMastery
}

func MasteryForLevel(level int) SkillMastery {
	if level < MinSkillLevel {
		return MasteryNovice
	}
	if level > MaxSkillLevel {
		return MasteryGrandMaster
	}
	return SkillMastery(level - 1)
}

func (s *Skill) Level() int {
	return int(s.Mastery) + 1
}

func (s *Skill) IncreaseMastery() bool {
	if s == nil || s.Mastery >= MasteryGrandMaster {
		return false
	}
	s.Mastery++
	return true
}

// skillTypeByKey maps snake_case skill keys (config.yaml class kits) to skill
// types. Weapon/armor category strings coincide with these keys; the category
// lookups below gate on the SkillType const-block ranges.
var skillTypeByKey = map[string]SkillType{
	"sword":              SkillSword,
	"dagger":             SkillDagger,
	"axe":                SkillAxe,
	"spear":              SkillSpear,
	"bow":                SkillBow,
	"mace":               SkillMace,
	"staff":              SkillStaff,
	"martial_arts":       SkillMartialArts,
	"leather":            SkillLeather,
	"chain":              SkillChain,
	"plate":              SkillPlate,
	"shield":             SkillShield,
	"bodybuilding":       SkillBodybuilding,
	"meditation":         SkillMeditation,
	"merchant":           SkillMerchant,
	"repair":             SkillRepair,
	"identify_item":      SkillIdentifyItem,
	"disarm_trap":        SkillDisarmTrap,
	"learning":           SkillLearning,
	"arms_master":        SkillArmsMaster,
	"trapper":            SkillTrapper,
	"sleight_of_hand":    SkillSleightOfHand,
	"dual_wielding":      SkillDualWielding,
	"iron_body":          SkillIronBody,
	"spiritual_training": SkillSpiritualTraining,
}

// SkillTypeFromKey resolves a snake_case config key (config.yaml class kits)
// to its SkillType. Returns false for an unknown key.
func SkillTypeFromKey(key string) (SkillType, bool) {
	t, ok := skillTypeByKey[key]
	return t, ok
}

// weaponSkills / armorSkills are the ONE canonical membership sets for "which
// SkillTypes are weapon- / armor-proficiency skills". Every "is this a weapon
// (or armor) skill" decision reads these instead of re-listing the skills or
// range-checking their enum values: a numeric range silently drops any skill
// appended past its end, which is exactly where new skills MUST go for
// save-compat (that is why SkillMartialArts already sits outside the old
// Sword..Staff range). Whether a given site ALSO counts Martial Arts stays an
// explicit decision there (e.g. HasAnyWeaponSkill excludes it), not a side
// effect of which idiom - list vs range - the site happened to use.
var weaponSkills = map[SkillType]bool{
	SkillSword: true, SkillDagger: true, SkillAxe: true, SkillSpear: true,
	SkillBow: true, SkillMace: true, SkillStaff: true, SkillMartialArts: true,
}

var armorSkills = map[SkillType]bool{
	SkillLeather: true, SkillChain: true, SkillPlate: true, SkillShield: true,
}

// IsWeaponSkill reports whether s is a weapon-proficiency skill (INCLUDES
// Martial Arts). IsArmorSkill is the armor-proficiency analogue.
func (s SkillType) IsWeaponSkill() bool { return weaponSkills[s] }
func (s SkillType) IsArmorSkill() bool  { return armorSkills[s] }

// WeaponSkillForCategory maps a weapon category string (lowercased) to the
// SkillType that gates wielding/proficiency bonuses. The "blaster" category
// returns (0, false) because it's universally usable - callers handle that
// special case explicitly.
func WeaponSkillForCategory(category string) (SkillType, bool) {
	if category == "throwing" {
		return SkillDagger, true // throwing weapons use the dagger skill
	}
	t, ok := skillTypeByKey[category]
	if !ok || !t.IsWeaponSkill() {
		return 0, false
	}
	return t, true
}

// ArmorSkillForCategory maps an armor category string (lowercased) to the
// SkillType that gates wearing it. The "cloth" category returns (0, false)
// because it's universally wearable.
func ArmorSkillForCategory(category string) (SkillType, bool) {
	t, ok := skillTypeByKey[category]
	if !ok || !t.IsArmorSkill() {
		return 0, false
	}
	return t, true
}
