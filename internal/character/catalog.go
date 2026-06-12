package character

import (
	"fmt"

	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"
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
	// TrapperDamagePerTier: bonus damage of damage traps per Trapper tier.
	TrapperDamagePerTier = 5
	// TrapperTurnsPerTier / TrapperSecondsPerTier: extra duration of control
	// traps (stun/root) per Trapper tier, in TB turns / RT seconds.
	TrapperTurnsPerTier   = 1
	TrapperSecondsPerTier = 5
	// TrapStatScalingDivisor: trap damage gains (Intellect+Accuracy)/this.
	TrapStatScalingDivisor = 3
	// SleightChancePctPerTier: pickpocket chance per Sleight of Hand tier on
	// each melee hit. A successful pick rolls the victim's loot table; a missed
	// loot roll pays consolation gold instead.
	SleightChancePctPerTier = 10
	// SleightGoldHighLevel / SleightGoldLow: consolation gold when the pick
	// succeeds but the loot roll misses — split by SleightHighLevelThreshold.
	SleightGoldHighLevel      = 35
	SleightGoldLow            = 5
	SleightHighLevelThreshold = 5
	// WeaponPrimaryStatDivisor: a weapon's bonus_stat adds stat/this to damage.
	WeaponPrimaryStatDivisor = 3
	// WeaponSecondaryStatDivisor: bonus_stat_secondary adds stat/this.
	WeaponSecondaryStatDivisor = 4
	// ArmorPierceRangedChancePct: a ranged physical hit has this % chance to
	// ignore the target's armor entirely.
	ArmorPierceRangedChancePct = 33
	// ArmorPhysicalReductionDivisor: physical damage is reduced by AC/this.
	ArmorPhysicalReductionDivisor = 2
	// MasterySpellEffectPerLevel: flat bonus per magic-school mastery tier above
	// Novice to spell damage/healing (buff magnitudes stay flat; duration
	// scales via SpellMasteryDurationBonusPct).
	MasterySpellEffectPerLevel = 5
	// SpellMasteryDurationBonusPct: +% spell duration per mastery tier above
	// Novice (100/120/140/160% of the YAML duration).
	SpellMasteryDurationBonusPct = 20
	// MagicGMResistPiercePct: a Grandmaster's spells ignore this % of the
	// target's resistance to the school's damage type.
	MagicGMResistPiercePct = 50
	// MerchantPricePctPerTier: % better buy AND sell prices per the party's best
	// Merchant tier.
	MerchantPricePctPerTier = 5
)

// PlayableClasses is every playable class in canonical (enum) order.
var PlayableClasses = []CharacterClass{
	ClassKnight, ClassPaladin, ClassArcher, ClassCleric, ClassSorcerer, ClassDruid, ClassThief,
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
	case ClassThief:
		return "thief"
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
	case ClassThief:
		return "No magic — a trap book instead: deadly tile traps, daggers and quick fingers."
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

// StatDescription is the canonical player-facing explanation of a primary
// stat — quoted by the in-game stat tooltip AND the map editor, built from the
// same balance constants combat uses.
func StatDescription(stat string) string {
	switch strings.ToLower(stat) {
	case "might":
		return fmt.Sprintf("Adds Might/%d to damage of weapons that scale with Might.", WeaponPrimaryStatDivisor)
	case "intellect":
		return fmt.Sprintf("Drives elemental spell damage (Intellect/%d) and trap damage (+(Int+Acc)/%d); "+
			"adds to max spell points and to weapons that scale with Intellect.",
			spells.SpellIntellectDivisor, TrapStatScalingDivisor)
	case "personality":
		return fmt.Sprintf("Drives self-magic (Body/Mind/Spirit) damage (Personality/%d) and ALL healing "+
			"(Personality/%d); adds to max spell points and SP regen.",
			spells.SpellIntellectDivisor, spells.HealingPersonalityDivisor)
	case "endurance":
		return "Increases max HP, armor-class scaling on equipped armor, and potion healing."
	case "accuracy":
		return fmt.Sprintf("Adds Accuracy/%d to damage of weapons that scale with Accuracy; "+
			"feeds trap damage (+(Int+Acc)/%d).", WeaponPrimaryStatDivisor, TrapStatScalingDivisor)
	case "speed":
		return fmt.Sprintf("Reduces real-time action cooldowns. In turn-based mode grants extra actions per turn (Speed >%d → 2 actions, >%d → 3).",
			SpeedActionSlot2Threshold, SpeedActionSlot3Threshold)
	case "luck":
		return "Improves critical chance and Perfect Dodge."
	default:
		return ""
	}
}

// WeaponCombatLines lists the game-side combat traits of a weapon that the
// config-level EffectLines can't compute (the category→skill mapping lives
// here): the effective attack-speed multiplier (per-weapon override OR the
// category multiplier from weapons.yaml) and the ranged armor-pierce chance.
// Shared by the in-game weapon tooltip and the map-editor card.
func WeaponCombatLines(def *config.WeaponDefinitionConfig) []string {
	if def == nil {
		return nil
	}
	var out []string
	mult := def.CooldownMultiplier
	if mult <= 0 {
		if skill, ok := WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
			mult = config.WeaponCooldownMultiplierForSkill(skill.WeaponNoun())
		}
	}
	if mult > 0 && mult != 1.0 {
		out = append(out, fmt.Sprintf("Attack cooldown %+.0f%%", (mult-1.0)*100))
	}
	if def.Physics != nil && (def.DamageType == "" || def.DamageType == "physical") {
		out = append(out, fmt.Sprintf("%d%% of shots pierce armor entirely", ArmorPierceRangedChancePct))
	}
	return out
}

// MagicMasteryDescription explains what a magic school's mastery grants — one
// text for the in-game tooltip and the map editor.
func MagicMasteryDescription() string {
	return fmt.Sprintf(
		"Magic Mastery: +%d%% spell duration and +%d damage/healing per mastery tier above Novice. "+
			"Grandmaster: ignores %d%% of enemy resistance with spells (except Inferno).",
		SpellMasteryDurationBonusPct, MasterySpellEffectPerLevel, MagicGMResistPiercePct)
}

// AllSkills is every skill in canonical (enum) order.
var AllSkills = []SkillType{
	SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff,
	SkillLeather, SkillChain, SkillPlate, SkillShield,
	SkillBodybuilding, SkillMeditation, SkillMerchant, SkillRepair,
	SkillIdentifyItem, SkillDisarmTrap, SkillLearning, SkillArmsMaster,
	SkillTrapper, SkillSleightOfHand,
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
			"Grandmaster: −%d%% spell point cost on all spells and traps.",
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
	case SkillTrapper:
		return fmt.Sprintf("Trapper: traps deal +%d damage per mastery level; control traps "+
			"last +%d turn / +%d sec per level. Trap damage scales with Intellect and Accuracy.",
			TrapperDamagePerTier, TrapperTurnsPerTier, TrapperSecondsPerTier)
	case SkillSleightOfHand:
		return fmt.Sprintf("Sleight of Hand: %d-%d%% chance (by mastery, Novice included) to pick a pocket "+
			"on each melee hit — rolls the victim's loot; a missed loot roll pays %d gold (level %d+ foes) or %d gold.",
			SleightChancePctPerTier, 4*SleightChancePctPerTier,
			SleightGoldHighLevel, SleightHighLevelThreshold+1, SleightGoldLow)
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
