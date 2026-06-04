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
	//   Bodybuilding → +Max HP, Meditation → faster mana regen,
	//   Learning → +% XP gained, ArmsMaster → +weapon damage,
	//   Merchant → better buy/sell prices,
	//   DisarmTrap → PLACEHOLDER incoming-damage reduction (see TODO below).
	SkillBodybuilding
	SkillMeditation
	SkillMerchant
	// SkillRepair: no effect yet. TODO: implement an item-durability system
	// (gear wears with use; Repair restores it / slows wear) for this to matter.
	SkillRepair
	// SkillIdentifyItem: no effect yet. TODO: implement unidentified loot (dropped
	// items start with hidden stats until a party member with Identify inspects).
	SkillIdentifyItem
	// SkillDisarmTrap: currently a placeholder damage reduction. TODO: implement
	// trap special-tiles on maps that a DisarmTrap check defuses.
	SkillDisarmTrap
	SkillLearning
	SkillArmsMaster
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

// WeaponSkillForCategory maps a weapon category string (lowercased) to the
// SkillType that gates wielding/proficiency bonuses. The "blaster" category
// returns (0, false) because it's universally usable — callers handle that
// special case explicitly.
func WeaponSkillForCategory(category string) (SkillType, bool) {
	switch category {
	case "sword":
		return SkillSword, true
	case "dagger", "throwing":
		return SkillDagger, true
	case "axe":
		return SkillAxe, true
	case "spear":
		return SkillSpear, true
	case "bow":
		return SkillBow, true
	case "mace":
		return SkillMace, true
	case "staff":
		return SkillStaff, true
	default:
		return 0, false
	}
}

// ArmorSkillForCategory maps an armor category string (lowercased) to the
// SkillType that gates wearing it. The "cloth" category returns (0, false)
// because it's universally wearable.
func ArmorSkillForCategory(category string) (SkillType, bool) {
	switch category {
	case "leather":
		return SkillLeather, true
	case "chain":
		return SkillChain, true
	case "plate":
		return SkillPlate, true
	case "shield":
		return SkillShield, true
	default:
		return 0, false
	}
}
