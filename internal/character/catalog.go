package character

import (
	"fmt"
	"strings"
)

// Canonical, single-source catalogs of the playable classes and skills, shared
// by the game and the map-viewer/editor so neither hardcodes (and drifts from)
// the lists. Add a class/skill in one place (the enums + these tables) and every
// consumer picks it up.

// Skill-effect balance constants. These live here (not in the game package) so
// the SAME numbers drive combat, the in-game skill tooltip, AND the map editor —
// one source of truth. (Bodybuilding/Meditation-regen constants live alongside
// CalculateMaxHP in character.go.) The game package re-exports these as aliases
// so existing combat references keep working unchanged.
const (
	// MasteryWeaponTrueDamagePerTier: bonus TRUE damage per weapon-mastery tier
	// (ignores armor, lands through dodges). Expert +3 / Master +6 / GM +9.
	MasteryWeaponTrueDamagePerTier = 3
	// WeaponGMCritBonus: extra crit % a Grandmaster gets with their mastered weapon.
	WeaponGMCritBonus = 7
	// MasteryArmorACPerLevel: bonus armor class per armor-mastery level.
	MasteryArmorACPerLevel = 2
	// ArmorGMDodgeBonus: extra Perfect-Dodge % per GM-mastered armor type worn.
	ArmorGMDodgeBonus = 5
	// MeditationGMSpellCostReductionPct: a GM meditator pays this % less SP per spell.
	MeditationGMSpellCostReductionPct = 25
	// LearningXPPctPerTier: +% experience this character gains per Learning tier.
	LearningXPPctPerTier = 10
	// LearningGMPartyXPPct: a GM "teacher" grants this extra % XP to the whole party.
	LearningGMPartyXPPct = 5
	// ArmsMasterDamagePerTier: bonus damage with ANY weapon per ArmsMaster tier.
	ArmsMasterDamagePerTier = 2
	// ArmsMasterGMCritBonus: extra crit % a GM Arms Master gets with ANY weapon.
	ArmsMasterGMCritBonus = 5
	// DisarmTrapDamageReductionPerTier: flat incoming-damage reduction per tier
	// (PLACEHOLDER until trap tiles exist).
	DisarmTrapDamageReductionPerTier = 1
	// MerchantPricePctPerTier: % better buy AND sell prices per the party's best
	// Merchant tier.
	MerchantPricePctPerTier = 5
)

// PlayableClasses is every playable class in canonical (enum) order.
var PlayableClasses = []CharacterClass{
	ClassKnight, ClassPaladin, ClassArcher, ClassCleric, ClassSorcerer, ClassDruid,
}

// Key returns the lowercase class key (knight/paladin/...).
func (c CharacterClass) Key() string {
	switch c {
	case ClassKnight:
		return "knight"
	case ClassPaladin:
		return "paladin"
	case ClassArcher:
		return "archer"
	case ClassCleric:
		return "cleric"
	case ClassSorcerer:
		return "sorcerer"
	case ClassDruid:
		return "druid"
	default:
		return "unknown"
	}
}

// Blurb is a one-line class description for UI/tooltips.
func (c CharacterClass) Blurb() string {
	switch c {
	case ClassKnight:
		return "Front-line fighter. No magic; the highest HP and weapon focus."
	case ClassPaladin:
		return "Holy warrior — axes, swords, chain and shield, with a touch of self magic."
	case ClassArcher:
		return "Bow master with high Accuracy and a little Air magic for utility."
	case ClassCleric:
		return "Healer and master of self magic (Body/Mind/Spirit), scaling with Personality."
	case ClassSorcerer:
		return "Elemental nuker — Fire, Water and Air magic scaling with Intellect."
	case ClassDruid:
		return "Nature hybrid — Water, Mind and Earth magic; staff and wilderness skills."
	default:
		return ""
	}
}

// DefaultCharacterName returns the canonical hero name for a class (used to pick
// its portrait art), falling back to the class key. Sourced from the default
// rosters so it never drifts from the shipped portraits.
func DefaultCharacterName(c CharacterClass) string {
	key := c.Key()
	for _, e := range defaultStartingParty {
		if e.Class == key {
			return e.Name
		}
	}
	for _, e := range defaultCaptives {
		if e.Class == key {
			return e.Name
		}
	}
	return key
}

// AllSkills is every skill in canonical (enum) order.
var AllSkills = []SkillType{
	SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff,
	SkillLeather, SkillChain, SkillPlate, SkillShield,
	SkillBodybuilding, SkillMeditation, SkillMerchant, SkillRepair,
	SkillIdentifyItem, SkillDisarmTrap, SkillLearning, SkillArmsMaster,
}

// Category groups a skill for display: "Weapon", "Armor", or "Misc".
func (s SkillType) Category() string {
	switch s {
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff:
		return "Weapon"
	case SkillLeather, SkillChain, SkillPlate, SkillShield:
		return "Armor"
	default:
		return "Misc"
	}
}

// Description is the player-facing explanation of a skill, built from the SAME
// balance constants the combat code uses — so the in-game tooltip, combat, and
// the map editor can never drift. Mastery tiers: Novice 0 / Expert 1 / Master 2
// / Grandmaster 3 (bonuses scale per tier above Novice unless noted).
func (s SkillType) Description() string {
	switch s {
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff:
		return fmt.Sprintf("Proficiency to wield %ss. Weapon Mastery: +%d true damage per level "+
			"(ignores armor, lands through dodges). Grandmaster: +%d%% crit with this weapon and "+
			"strikes ignore Perfect Dodge.",
			weaponNoun(s), MasteryWeaponTrueDamagePerTier, WeaponGMCritBonus)
	case SkillLeather, SkillChain, SkillPlate:
		return fmt.Sprintf("Required to wear %s armor. Armor Mastery: +%d base AC per level. "+
			"Grandmaster: +%d%% Perfect Dodge while wearing this armor type.",
			weaponNoun(s), MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillShield:
		return fmt.Sprintf("Required to use a shield (off-hand). Armor Mastery: +%d base AC per level. "+
			"Grandmaster: +%d%% Perfect Dodge while a shield is equipped.",
			MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillBodybuilding:
		return fmt.Sprintf("Bodybuilding: +%d max HP per level. Grandmaster: +%d%% max HP.",
			BodybuildingHPPerTier, BodybuildingGMMaxHPPct)
	case SkillMeditation:
		return fmt.Sprintf("Meditation: +%d spell points per regen tick per level (faster mana recovery). "+
			"Grandmaster: −%d%% spell point cost on all spells.",
			MeditationRegenPerTier, MeditationGMSpellCostReductionPct)
	case SkillLearning:
		return fmt.Sprintf("Learning: +%d%% experience gained per level. "+
			"Grandmaster: +%d%% experience to the whole party.",
			LearningXPPctPerTier, LearningGMPartyXPPct)
	case SkillArmsMaster:
		return fmt.Sprintf("Arms Master: +%d damage with any weapon per level (stacks with the weapon's "+
			"own mastery). Grandmaster: +%d%% crit with any weapon.",
			ArmsMasterDamagePerTier, ArmsMasterGMCritBonus)
	case SkillMerchant:
		return fmt.Sprintf("Merchant: %d%% better buy/sell prices per mastery level "+
			"(the party's best Merchant applies).", MerchantPricePctPerTier)
	case SkillDisarmTrap:
		return fmt.Sprintf("Disarm Trap: −%d incoming damage per mastery level "+
			"(placeholder until trap tiles are added).", DisarmTrapDamageReductionPerTier)
	case SkillRepair:
		return "Repair: no effect yet (planned: equipment durability)."
	case SkillIdentifyItem:
		return "Identify Item: no effect yet (planned: reveal unidentified loot)."
	default:
		return ""
	}
}

// WeaponNoun is the exported canonical lowercase noun for a weapon skill
// ("sword", "dagger", …) — the single key other packages use to look up
// per-weapon-type tuning (e.g. attack-cooldown multipliers in weapons.yaml).
func (s SkillType) WeaponNoun() string { return weaponNoun(s) }

// weaponNoun returns the lowercase noun for a weapon/armor skill ("sword",
// "leather", …) used in skill descriptions.
func weaponNoun(s SkillType) string {
	// Derive from String() (lowercased) so the noun can never drift from the
	// canonical name — load-bearing: WeaponNoun() keys the cooldown table.
	switch s {
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff,
		SkillLeather, SkillChain, SkillPlate:
		return strings.ToLower(s.String())
	default:
		return ""
	}
}
