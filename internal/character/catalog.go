package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
)

// Canonical, single-source catalogs of the playable classes and skills, shared
// by the game and the map-viewer/editor so neither hardcodes (and drifts from)
// the lists. Add a class/skill in one place (the enums + these tables) and every
// consumer picks it up.

// Skill-effect balance constants. These live here (not in the game package) so
// the SAME numbers drive combat, the in-game skill tooltip, AND the map editor -
// one source of truth. (Bodybuilding/Meditation-regen constants live alongside
// CalculateMaxHP in character.go.) The game package re-exports these as aliases
// so existing combat references keep working unchanged.
const (
	// MasteryWeaponTrueDamagePerTier: bonus TRUE damage per weapon-mastery tier
	// (resistance applies; armor/flat/dodge do not). Expert +3 / Master +6 / GM +9.
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
	// LuckToCritDivisor: Luck/this adds to critical chance (percent points), on
	// top of the weapon's base crit_chance.
	LuckToCritDivisor = 4
	// CritDamageMultiplier multiplies final damage on a critical hit (weapon,
	// melee, ranged, and spells alike).
	CritDamageMultiplier = 2
	// DisarmTrapDamageReductionPerTier: flat incoming-damage reduction per tier
	// after armor and resistance. Novice has tier 0, then Expert/Master/GM get
	// 1/2/3 reduction.
	DisarmTrapDamageReductionPerTier = 1
	// Disarm Trap fully AVOIDS a chest/door trap at base + per-tier percent:
	// Novice/Expert/Master/GM -> 40/60/80/100.
	DisarmTrapAvoidBasePct    = 40
	DisarmTrapAvoidPerTierPct = 20
	// TrapperDamagePerTier: bonus damage of damage traps per Trapper tier.
	TrapperDamagePerTier = 5
	// TrapperSecondsPerTier: extra RT seconds of control (stun/root) per Trapper
	// tier - linear (base 2 -> 2/4/6/8). TB turns scale separately and
	// NON-linearly; see TrapperTurnBonus.
	TrapperSecondsPerTier = 2
	// TrapStatScalingDivisor: trap damage gains (Intellect+Accuracy)/this.
	TrapStatScalingDivisor = 3
	// SleightChancePctPerTier: pickpocket chance per Sleight of Hand tier on
	// each melee hit. A successful pick rolls the victim's loot table; a missed
	// loot roll pays consolation gold instead.
	SleightChancePctPerTier = 10
	// SleightGoldHighLevel / SleightGoldLow: consolation gold when the pick
	// succeeds but the loot roll misses - split by SleightHighLevelThreshold.
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
	// Party armor mitigation - a PERCENTAGE model with diminishing returns:
	//   physical% = min(ArmorPhysicalMitigationCap, 100*AC/(AC+ArmorMitigationK))
	//   elemental% = physical% * ArmorElementalMitigationCap / ArmorPhysicalMitigationCap
	// Elemental is the SAME curve scaled down, so it reaches its 33% cap at the
	// exact AC where physical reaches 75% - not capping out far earlier.
	// K sets the curve (AC == K gives 50% pre-cap).
	ArmorMitigationK            = 45
	ArmorPhysicalMitigationCap  = 75
	ArmorElementalMitigationCap = 33
	// TurnBasedTurnSeconds is the real-time equivalent one turn-based round
	// consumes for periodic effects (DoTs, damage zones). Shared with the game's
	// TurnBasedPeriodicEffectSeconds so cards can state ticks per turn.
	TurnBasedTurnSeconds = 3
	// MasterySpellEffectPerLevel: flat bonus per magic-school mastery tier above
	// Novice to spell damage/healing (buff magnitudes stay flat; duration
	// scales via SpellMasteryDurationBonusPct).
	MasterySpellEffectPerLevel = 5
	// SpellMasteryDurationBonusPct: +% spell duration per mastery tier above
	// Novice (100/120/140/160% of the YAML duration).
	SpellMasteryDurationBonusPct = 20
	// SelfMagicGMResistPiercePct: a Grandmaster Body/Mind/Spirit caster's
	// damaging spells ignore this % of the matching resistance. Elemental
	// schools, including Light/Dark, use Elemental Mastery instead.
	SelfMagicGMResistPiercePct = 50
	// MerchantPricePctPerTier: % better buy AND sell prices per the party's best
	// Merchant tier.
	MerchantPricePctPerTier = 5
	// DualWieldingCDReductionPerTier: % off BOTH weapons' cooldown per tier ABOVE
	// Novice (Novice just unlocks the off-hand weapon slot, no reduction yet).
	DualWieldingCDReductionPerTier = 10
	// IronBodyACPerTier: flat Armor Class per Iron Body tier, INCLUDING Novice
	// (tier+1)*this - a Monk has no armor slots, so this is their main AC source
	// besides Endurance scaling.
	IronBodyACPerTier = 10
	// IronBodyGMDodgeBonus: extra Perfect Dodge % at Grandmaster Iron Body.
	IronBodyGMDodgeBonus = 10
	// SpiritualTrainingProcPctPerTier: chance per tier, INCLUDING Novice
	// ((tier+1)*this), that a melee hit also fires the slotted quick-spell for
	// free (0 SP), mirroring the Pixie Card's free Fire Bolt proc.
	SpiritualTrainingProcPctPerTier = 10
	// AnimalBondingSummonMax: maximum living Animal Bonding bears per Druid.
	AnimalBondingSummonMax = 2
	// DoorForceChancePct is the fixed chance of a qualifying Might/Intellect
	// attempt. DoorMaxNonKeyAttempts failed non-key attempts jam the lock.
	DoorForceChancePct    = 20
	DoorMaxNonKeyAttempts = 3
)

var (
	elementalMasteryPiercePct = [...]int{10, 20, 35, 50}
	animalBondingProcPct      = [...]int{5, 8, 12, 15}
	animalBondingStatPct      = [...]int{40, 60, 80, 100}
	animalBondingHPPct        = [...]int{100, 200, 300, 500}
	sacrificeRedirectPct      = [...]int{10, 20, 30, 50}
	impenetrableDefenseFlat   = [...]int{3, 5, 7, 10}
	lockpickingChancePct      = [...]int{20, 35, 50, 60}
	naturalHealerBonusPct     = [...]int{20, 40, 60, 100}
)

func masteryTableValue(table [4]int, tier int) int {
	if tier < int(MasteryNovice) {
		return 0
	}
	if tier > int(MasteryGrandMaster) {
		tier = int(MasteryGrandMaster)
	}
	return table[tier]
}

func ElementalMasteryPiercePct(tier int) int {
	return masteryTableValue(elementalMasteryPiercePct, tier)
}

func AnimalBondingProcPct(tier int) int {
	return masteryTableValue(animalBondingProcPct, tier)
}

func AnimalBondingStatPct(tier int) int {
	return masteryTableValue(animalBondingStatPct, tier)
}

func AnimalBondingHPPct(tier int) int {
	return masteryTableValue(animalBondingHPPct, tier)
}

func SacrificeRedirectPct(tier int) int {
	return masteryTableValue(sacrificeRedirectPct, tier)
}

func ImpenetrableDefenseReduction(tier int) int {
	return masteryTableValue(impenetrableDefenseFlat, tier)
}

func LockpickingChancePct(tier int) int {
	return masteryTableValue(lockpickingChancePct, tier)
}

func NaturalHealerBonusPct(tier int) int {
	return masteryTableValue(naturalHealerBonusPct, tier)
}

func elementalMagicSchoolNames() string {
	var names []string
	for _, school := range AllMagicSchools {
		if school.IsElemental() {
			names = append(names, school.DisplayName())
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	}
}

// TrapperTurnBonus is the EXTRA TB turns a control trap (stun/root) gains at the
// given Trapper tier, on top of its 1-turn base: Novice/Expert +0, Master +1,
// Grandmaster +2 (so the total reads 1/1/2/3 turns). Deliberately NON-linear,
// unlike the RT-seconds scaling (TrapperSecondsPerTier).
func TrapperTurnBonus(tier int) int {
	if tier <= int(MasteryExpert) {
		return 0
	}
	return tier - int(MasteryExpert)
}

// PlayableClasses is every playable class in canonical (enum) order.
var PlayableClasses = []CharacterClass{
	ClassKnight, ClassPaladin, ClassArcher, ClassCleric, ClassSorcerer, ClassDruid, ClassThief,
	ClassArmsMaster, ClassMonk,
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
	case ClassArmsMaster:
		return "arms_master"
	case ClassMonk:
		return "monk"
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
		return "Holy warrior - axes, swords, chain and shield, with a touch of self magic."
	case ClassArcher:
		return "Bow master with high Accuracy and a little Air magic for utility."
	case ClassCleric:
		return "Healer and master of self magic (Body/Mind/Spirit), scaling with Personality."
	case ClassSorcerer:
		return "Elemental nuker - Fire, Water and Air magic scaling with Intellect."
	case ClassDruid:
		return "Nature hybrid - Water, Mind and Earth magic; staff and wilderness skills."
	case ClassThief:
		return "No magic - a trap book instead: deadly tile traps, daggers and quick fingers."
	case ClassArmsMaster:
		return "Master of every weapon - dual-wields for two independent attacks, expert from level 1."
	case ClassMonk:
		return "Unarmed fighter and self-magic adept - no weapons or armor, fists scale with Might and Speed."
	default:
		return ""
	}
}

// StatDescription is the canonical player-facing explanation of a primary
// stat - quoted by the in-game stat tooltip AND the map editor, built from the
// same balance constants combat uses.
func StatDescription(stat string) string {
	switch strings.ToLower(stat) {
	case "might":
		return "Improves damage for weapons that scale from Might."
	case "intellect":
		return fmt.Sprintf("Drives elemental spell damage (Intellect/%d) and trap damage (+(Int+Acc)/%d); "+
			"adds to max spell points. Improves damage for weapons that scale from Intellect.",
			spells.SpellIntellectDivisor, TrapStatScalingDivisor)
	case "personality":
		return fmt.Sprintf("Drives self-magic (Body/Mind/Spirit) damage (Personality/%d) and ALL healing "+
			"(Personality/%d); adds Personality/%d to max spell points and improves SP regen.",
			spells.SpellIntellectDivisor, spells.HealingPersonalityDivisor, MaxSPPersonalityDivisor)
	case "endurance":
		return "Increases max HP, armor-class scaling on equipped armor, and potion healing."
	case "accuracy":
		return fmt.Sprintf("Improves damage for weapons that scale from Accuracy; "+
			"feeds trap damage (+(Int+Acc)/%d).", TrapStatScalingDivisor)
	case "speed":
		return fmt.Sprintf("Reduces real-time action cooldowns. In turn-based mode the fastest living party member grants party bonus actions (Speed >%d -> +1, >%d -> +2). Improves damage for weapons that scale from Speed.",
			SpeedBonusAction1Threshold, SpeedBonusAction2Threshold)
	case "luck":
		return "Improves critical chance and Perfect Dodge."
	default:
		return ""
	}
}

// WeaponCombatLines lists the game-side combat traits of a weapon that the
// config-level EffectLines can't compute (the category->skill mapping lives
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
		// Show the raw multiplier + how it compares to the baseline weapon
		// (a sword, x1.00) - "+10%" alone read as "vs my current weapon" or
		// "+10% of 1s". The actual cooldown in seconds is shown alongside.
		d := mult - 1.0
		rel := "slower"
		if d < 0 {
			d, rel = -d, "faster"
		}
		out = append(out, fmt.Sprintf("Attack cooldown x%.2f (%d%% %s than standard)", mult, int(d*100+0.5), rel))
	}
	damageType, damageTypeErr := damagecalc.ParseType(def.DamageType)
	if def.Physics != nil && (def.DamageType == "" || (damageTypeErr == nil && damageType == damagecalc.Physical)) {
		out = append(out, fmt.Sprintf("%d%% of shots pierce armor entirely", ArmorPierceRangedChancePct))
	}
	return out
}

// MagicMasteryDescription explains only the selected school's rules. Keeping
// the school in the contract prevents one tooltip from leaking unrelated
// elemental/self-magic policies into every school row.
func MagicMasteryDescription(school MagicSchoolID) string {
	name := school.DisplayName()
	base := fmt.Sprintf(
		"%s Magic: spells with a duration last +%d%% per mastery tier above Novice. "+
			"Projectiles, damage zones, and healing spells gain +%d damage or healing per tier above Novice.",
		name, SpellMasteryDurationBonusPct, MasterySpellEffectPerLevel)
	if school.IsElemental() {
		gmBonus := int(MasteryGrandMaster) * MasterySpellEffectPerLevel
		return fmt.Sprintf(
			"%s At Grandmaster, that +%d damage becomes %s true damage.",
			base, gmBonus, name)
	}
	return fmt.Sprintf(
		"%s At Grandmaster, damaging spells ignore %d%% of enemy %s Resistance.",
		base, SelfMagicGMResistPiercePct, name)
}

// AllSkills is every skill in canonical (enum) order.
var AllSkills = []SkillType{
	SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff, SkillMartialArts,
	SkillLeather, SkillChain, SkillPlate, SkillShield,
	SkillBodybuilding, SkillMeditation, SkillMerchant, SkillRepair,
	SkillIdentifyItem, SkillDisarmTrap, SkillLearning, SkillArmsMaster,
	SkillTrapper, SkillSleightOfHand,
	SkillDualWielding, SkillIronBody, SkillSpiritualTraining,
	SkillBlaster, SkillElementalMastery, SkillAnimalBonding, SkillSacrifice,
	SkillImpenetrableDefense, SkillLockpicking, SkillNaturalHealer,
}

// Category groups a skill for display: "Weapon", "Armor", or "Misc".
func (s SkillType) Category() string {
	switch {
	case s.IsWeaponSkill():
		return "Weapon"
	case s.IsArmorSkill():
		return "Armor"
	default:
		return "Misc"
	}
}

// Description is the player-facing explanation of a skill, built from the SAME
// balance constants the combat code uses - so the in-game tooltip, combat, and
// the map editor can never drift. Mastery tiers: Novice 0 / Expert 1 / Master 2
// / Grandmaster 3 (bonuses scale per tier above Novice unless noted).
func (s SkillType) Description() string {
	switch s {
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff, SkillBlaster:
		// A skill-optional category (blaster) needs no training to fire; its
		// skill only pays the mastery bonuses.
		lead := fmt.Sprintf("Proficiency to wield %ss.", weaponNoun(s))
		if WeaponCategorySkillOptional(weaponNoun(s)) {
			lead = fmt.Sprintf("Anyone can fire a %s untrained - this skill only makes it better.", weaponNoun(s))
		}
		return fmt.Sprintf("%s Weapon Mastery: +%d true damage per tier above Novice "+
			"(resistance applies; ignores armor/flat reduction and lands through dodges). Grandmaster: +%d%% crit with this weapon and "+
			"strikes ignore Perfect Dodge.",
			lead, MasteryWeaponTrueDamagePerTier, WeaponGMCritBonus)
	case SkillMartialArts:
		return fmt.Sprintf("Proficiency fighting unarmed. Weapon Mastery: +%d true damage per tier above Novice "+
			"(resistance applies; ignores armor/flat reduction and lands through dodges). Grandmaster: +%d%% crit unarmed and "+
			"strikes ignore Perfect Dodge.",
			MasteryWeaponTrueDamagePerTier, WeaponGMCritBonus)
	case SkillLeather, SkillChain, SkillPlate:
		return fmt.Sprintf("Required to wear %s armor. Armor Mastery: +%d base AC per tier above Novice. "+
			"Grandmaster: +%d%% Perfect Dodge while wearing this armor type.",
			weaponNoun(s), MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillShield:
		return fmt.Sprintf("Required to use a shield (off-hand). Armor Mastery: +%d base AC per tier above Novice. "+
			"Grandmaster: +%d%% Perfect Dodge while a shield is equipped.",
			MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillBodybuilding:
		return fmt.Sprintf("Bodybuilding: +%d max HP per tier above Novice. Grandmaster: +%d%% max HP.",
			BodybuildingHPPerTier, BodybuildingGMMaxHPPct)
	case SkillMeditation:
		return fmt.Sprintf("Meditation: +%d spell points per regen tick per tier above Novice. "+
			"Grandmaster: -%d%% spell point cost on all spells and traps.",
			MeditationRegenPerTier, MeditationGMSpellCostReductionPct)
	case SkillLearning:
		return fmt.Sprintf("Learning: +%d%% experience gained per tier above Novice. "+
			"Grandmaster: +%d%% experience to the whole party.",
			LearningXPPctPerTier, LearningGMPartyXPPct)
	case SkillArmsMaster:
		return fmt.Sprintf("Arms Master: +%d damage with any weapon per tier above Novice (stacks with the weapon's "+
			"own mastery). Grandmaster: +%d%% crit with any weapon.",
			ArmsMasterDamagePerTier, ArmsMasterGMCritBonus)
	case SkillMerchant:
		return fmt.Sprintf("Merchant: %d%% better buy/sell prices per tier above Novice "+
			"(the party's best Merchant applies).", MerchantPricePctPerTier)
	case SkillDisarmTrap:
		return fmt.Sprintf("Disarm Trap: the party's best active user disarms chest traps with %d/%d/%d/%d%% chance at Novice/Expert/Master/Grandmaster. "+
			"Each trained character also reduces normal hit damage by 0/%d/%d/%d after armor and resistance; true damage and damage over time bypass it.",
			DisarmTrapAvoidBasePct,
			DisarmTrapAvoidBasePct+DisarmTrapAvoidPerTierPct,
			DisarmTrapAvoidBasePct+2*DisarmTrapAvoidPerTierPct,
			DisarmTrapAvoidBasePct+3*DisarmTrapAvoidPerTierPct,
			DisarmTrapDamageReductionPerTier,
			2*DisarmTrapDamageReductionPerTier,
			3*DisarmTrapDamageReductionPerTier)
	case SkillTrapper:
		return fmt.Sprintf("Trapper: traps deal +%d damage per tier above Novice; control traps "+
			"last +%d RT sec per tier above Novice and gain up to +%d TB turns at Grandmaster. "+
			"Trap damage scales with Intellect and Accuracy.",
			TrapperDamagePerTier, TrapperSecondsPerTier, TrapperTurnBonus(int(MasteryGrandMaster)))
	case SkillSleightOfHand:
		return fmt.Sprintf("Sleight of Hand: %d-%d%% chance (by mastery, Novice included) to pick a pocket "+
			"on each melee hit - rolls the victim's loot; a missed loot roll pays %d gold (level %d+ foes) or %d gold.",
			SleightChancePctPerTier, 4*SleightChancePctPerTier,
			SleightGoldHighLevel, SleightHighLevelThreshold+1, SleightGoldLow)
	case SkillRepair:
		return "Repair: no effect yet (planned: equipment durability)."
	case SkillIdentifyItem:
		return "Identify Item: no effect yet (planned: reveal unidentified loot)."
	case SkillDualWielding:
		return fmt.Sprintf("Dual Wielding: Novice unlocks a second weapon in the off-hand, each with its "+
			"own cooldown. From Expert on, -%d%% cooldown on both weapons per mastery tier above Novice.",
			DualWieldingCDReductionPerTier)
	case SkillIronBody:
		return fmt.Sprintf("Iron Body: +%d Armor Class per mastery tier, Novice included. Grandmaster: +%d%% Perfect Dodge.",
			IronBodyACPerTier, IronBodyGMDodgeBonus)
	case SkillSpiritualTraining:
		return fmt.Sprintf("Spiritual Training: %d-%d%% chance (by mastery, Novice included) that a melee "+
			"attack also casts the slotted offensive quick-spell for free (no spell points spent).",
			SpiritualTrainingProcPctPerTier, 4*SpiritualTrainingProcPctPerTier)
	case SkillElementalMastery:
		return fmt.Sprintf("Elemental Mastery: %s spells ignore %d/%d/%d/%d%% of matching enemy resistance at Novice/Expert/Master/Grandmaster.",
			elementalMagicSchoolNames(),
			ElementalMasteryPiercePct(0), ElementalMasteryPiercePct(1),
			ElementalMasteryPiercePct(2), ElementalMasteryPiercePct(3))
	case SkillAnimalBonding:
		return fmt.Sprintf("Animal Bonding: each successful attack or spell cast has a %d/%d/%d/%d%% chance to summon one allied bear. "+
			"A Druid can have up to %d living bears at once. The bear has %d/%d/%d/%d%% of the Druid's maximum HP and copies %d/%d/%d/%d%% of their current Armor Class and attack damage.",
			AnimalBondingProcPct(0), AnimalBondingProcPct(1), AnimalBondingProcPct(2), AnimalBondingProcPct(3),
			AnimalBondingSummonMax,
			AnimalBondingHPPct(0), AnimalBondingHPPct(1), AnimalBondingHPPct(2), AnimalBondingHPPct(3),
			AnimalBondingStatPct(0), AnimalBondingStatPct(1), AnimalBondingStatPct(2), AnimalBondingStatPct(3))
	case SkillSacrifice:
		return fmt.Sprintf("Sacrifice: redirects %d/%d/%d/%d%% of mitigated hit damage from another party member to this Paladin. "+
			"If several living Paladins have Sacrifice, only the strongest applies.",
			SacrificeRedirectPct(0), SacrificeRedirectPct(1), SacrificeRedirectPct(2), SacrificeRedirectPct(3))
	case SkillImpenetrableDefense:
		return fmt.Sprintf("Impenetrable Defense: reduces normal damage taken by %d/%d/%d/%d after armor and resistance. True damage and damage over time bypass it.",
			ImpenetrableDefenseReduction(0), ImpenetrableDefenseReduction(1),
			ImpenetrableDefenseReduction(2), ImpenetrableDefenseReduction(3))
	case SkillLockpicking:
		return fmt.Sprintf("Lockpicking: %d/%d/%d/%d%% chance to open a locked door. Three failed non-key attempts jam the lock; a key still opens it.",
			LockpickingChancePct(0), LockpickingChancePct(1), LockpickingChancePct(2), LockpickingChancePct(3))
	case SkillNaturalHealer:
		return fmt.Sprintf("Natural Healer: healing spells restore %d/%d/%d/%d%% more HP at Novice/Expert/Master/Grandmaster.",
			NaturalHealerBonusPct(0), NaturalHealerBonusPct(1),
			NaturalHealerBonusPct(2), NaturalHealerBonusPct(3))
	default:
		return ""
	}
}

// WeaponNoun is the exported canonical lowercase noun for a weapon skill
// ("sword", "dagger", ...) - the single key other packages use to look up
// per-weapon-type tuning (e.g. attack-cooldown multipliers in weapons.yaml).
func (s SkillType) WeaponNoun() string { return weaponNoun(s) }

// weaponNoun returns the lowercase noun for a weapon/armor skill ("sword",
// "leather", ...) used in skill descriptions.
func weaponNoun(s SkillType) string {
	// Derive from String() (lowercased) so the noun can never drift from the
	// canonical name - load-bearing: WeaponNoun() keys the cooldown table.
	switch s {
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff, SkillBlaster,
		SkillLeather, SkillChain, SkillPlate:
		return strings.ToLower(s.String())
	default:
		return ""
	}
}
