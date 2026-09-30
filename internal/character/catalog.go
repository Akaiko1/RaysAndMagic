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
	// SpellAbsorbChancePctPerTier: Battle Mage Spell Absorption chance per tier
	// (Novice included) - 15/30/45/60% to eat a hostile spell hit.
	SpellAbsorbChancePctPerTier = 15
	// StrongMagicPctPerTier: Battle Mage Strong Magic exchange per tier
	// (Novice included) - 25/50/75/100% of SP cost burned as HP, and the same
	// percent added to the offensive spell's damage.
	StrongMagicPctPerTier = 25
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
	MasterySpellEffectPerLevel = spells.MasterySpellEffectPerLevel
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
	// DarkElfBindingChancePct is shared by the racial hit gate and reference text.
	DarkElfBindingChancePct = 3
	// OverwatchReadySeconds: stationary time before RT reactions become ready.
	OverwatchReadySeconds = 1.0
	// DoorForceChancePct is the fixed chance of a qualifying Might/Intellect
	// attempt. DoorMaxNonKeyAttempts failed non-key attempts jam the lock.
	DoorForceChancePct    = 20
	DoorMaxNonKeyAttempts = 3
)

var (
	overwatchChancePct              = [...]int{20, 30, 40, 50}
	ballisticsSpeedPct              = [...]int{15, 25, 35, 50}
	ballisticsRangeTiles            = [...]int{0, 0, 1, 2}
	ballisticsCritPct               = [...]int{2, 4, 6, 8}
	fieldMedicineRestorePct         = [...]int{15, 25, 40, 60}
	fieldMedicinePoisonReductionPct = [...]int{10, 20, 30, 40}
	designationCritPct              = [...]int{5, 8, 12, 15}
	designationSeconds              = [...]int{6, 9, 12, 15}
	elementalMasteryPiercePct       = [...]int{10, 20, 35, 50}
	animalBondingProcPct            = [...]int{5, 8, 12, 15}
	animalBondingStatPct            = [...]int{40, 60, 80, 100}
	animalBondingHPPct              = [...]int{100, 200, 300, 500}
	sacrificeRedirectPct            = [...]int{10, 20, 30, 50}
	impenetrableDefenseFlat         = [...]int{3, 5, 7, 10}
	lockpickingChancePct            = [...]int{20, 35, 50, 60}
	naturalHealerBonusPct           = [...]int{20, 40, 60, 100}
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

const OverwatchAttackChanceScale = 0.5

func OverwatchChancePct(tier int) int {
	return masteryTableValue(overwatchChancePct, tier)
}

func BallisticsSpeedPct(tier int) int {
	return masteryTableValue(ballisticsSpeedPct, tier)
}

func BallisticsRangeTiles(tier int) int {
	return masteryTableValue(ballisticsRangeTiles, tier)
}

func BallisticsCritPct(tier int) int {
	return masteryTableValue(ballisticsCritPct, tier)
}

func FieldMedicineRestorePct(tier int) int {
	return masteryTableValue(fieldMedicineRestorePct, tier)
}

func FieldMedicinePoisonReductionPct(tier int) int {
	return masteryTableValue(fieldMedicinePoisonReductionPct, tier)
}

func DesignationCritPct(tier int) int {
	return masteryTableValue(designationCritPct, tier)
}

func DesignationSeconds(tier int) int {
	return masteryTableValue(designationSeconds, tier)
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

var orcishFuryDamage = [...]int{3, 5, 7, 10}

func OrcishFuryDamageBonus(tier int) int {
	return masteryTableValue(orcishFuryDamage, tier)
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
	ClassArmsMaster, ClassMonk, ClassBattleMage, ClassSniper, ClassAlchemist, ClassWayfarer,
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
	case ClassBattleMage:
		return "battle_mage"
	case ClassSniper:
		return "sniper"
	case ClassAlchemist:
		return "alchemist"
	case ClassWayfarer:
		return "wayfarer"
	default:
		return "unknown"
	}
}

// Blurb is a one-line class description for UI/tooltips.
func (c CharacterClass) Blurb() string {
	switch c {
	case ClassAlchemist:
		return "Brews supplies, strengthens party potions and throws explosive flasks."
	case ClassWayfarer:
		return "Staff adept - folds space and protects the party with automatic techniques."
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
	case ClassSniper:
		return "Legendary marksman - covers the party from a fixed position and designates priority targets."
	case ClassBattleMage:
		return "Spellblade in plate - drinks hostile magic, and pays in blood to make its own hit harder."
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

// WeaponCooldownMultiplier resolves a weapon's authored override or its
// category multiplier. Combat and both weapon-card builders share this lookup.
func WeaponCooldownMultiplier(def *config.WeaponDefinitionConfig) float64 {
	if def == nil {
		return 1
	}
	mult := def.CooldownMultiplier
	if mult <= 0 {
		if skill, ok := WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
			mult = config.WeaponCooldownMultiplierForSkill(skill.WeaponNoun())
		}
	}
	if mult <= 0 {
		return 1
	}
	return mult
}

// WeaponCombatLines lists combat traits governed by character rules.
func WeaponCombatLines(def *config.WeaponDefinitionConfig) []string {
	if def == nil {
		return nil
	}
	var out []string
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
	base := fmt.Sprintf("Standard spell damage/healing +0/%d/%d/%d; duration +0/%d/%d/%d%%.",
		MasterySpellEffectPerLevel, 2*MasterySpellEffectPerLevel, 3*MasterySpellEffectPerLevel,
		SpellMasteryDurationBonusPct, 2*SpellMasteryDurationBonusPct, 3*SpellMasteryDurationBonusPct)
	if school.IsElemental() {
		return fmt.Sprintf("%s\n\nGrand Master:\nThe standard +%d damage becomes %s true damage.", base, int(MasteryGrandMaster)*MasterySpellEffectPerLevel, school.DisplayName())
	}
	return fmt.Sprintf("%s\n\nGrand Master:\nDamaging spells ignore %d%% of enemy %s Resistance.", base, SelfMagicGMResistPiercePct, school.DisplayName())
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
	SkillCelestialProvidence, SkillOrcishFury, SkillHalflingGuile, SkillDarkElfBinding,
	SkillSpellAbsorption, SkillStrongMagic,
	SkillBallistics, SkillFieldMedicine, SkillDesignateTarget, SkillOverwatch,
	SkillAlchemy, SkillPharmacology, SkillBombThrowing, SkillTranslocation, SkillFlowingStaff, SkillPathfinding,
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

func masteryValues(values [4]int) string {
	return fmt.Sprintf("%d/%d/%d/%d", values[0], values[1], values[2], values[3])
}

func masteryProgression(value func(int) int) string {
	return masteryValues([4]int{value(0), value(1), value(2), value(3)})
}

// Description is the compact skill reference shared by the game and editor.
// Quote mastery effects from combat values; controls and detailed action rules
// belong on the relevant item, spell or technique card.
func (s SkillType) Description() string {
	switch s {
	case SkillAlchemy, SkillPharmacology, SkillBombThrowing, SkillTranslocation, SkillFlowingStaff, SkillPathfinding:
		return rareSkillDescription(s)
	case SkillOverwatch:
		return fmt.Sprintf("While stationary with a clear shot, each tile an enemy advances toward the party in weapon range has a %s%% chance of a free bow/blaster shot. Enemy attacks trigger at half that chance. Ready after %.1fs in RT; moving disables it until next round in TB.", masteryProgression(OverwatchChancePct), OverwatchReadySeconds)
	case SkillBallistics:
		return fmt.Sprintf("Bow/blaster projectile speed +%s%%; range +%s tiles; critical chance +%s%%.", masteryProgression(BallisticsSpeedPct), masteryProgression(BallisticsRangeTiles), masteryProgression(BallisticsCritPct))
	case SkillFieldMedicine:
		return fmt.Sprintf("Consumable HP/SP recovery +%s%%; poison duration -%s%%. Revival unchanged.", masteryProgression(FieldMedicineRestorePct), masteryProgression(FieldMedicinePoisonReductionPct))
	case SkillDesignateTarget:
		return fmt.Sprintf("Ranged hits mark a target for %ss: party weapon crit +%s percentage points. One mark per user; strongest applies while the marker can act.", masteryProgression(DesignationSeconds), masteryProgression(DesignationCritPct))
	case SkillSword, SkillDagger, SkillAxe, SkillSpear, SkillBow, SkillMace, SkillStaff, SkillBlaster, SkillMartialArts:
		lead := fmt.Sprintf("Allows wielding %ss.", weaponNoun(s))
		if s == SkillMartialArts {
			lead = "Unarmed combat proficiency."
		} else if WeaponCategorySkillOptional(weaponNoun(s)) {
			lead = "Can be fired untrained."
		}
		return fmt.Sprintf("%s +0/%d/%d/%d true damage (resistance applies).\n\nGrand Master:\n+%d%% crit; attacks ignore Perfect Dodge.", lead, MasteryWeaponTrueDamagePerTier, 2*MasteryWeaponTrueDamagePerTier, 3*MasteryWeaponTrueDamagePerTier, WeaponGMCritBonus)
	case SkillLeather, SkillChain, SkillPlate:
		return fmt.Sprintf("Allows wearing %s armor. +0/%d/%d/%d AC per equipped piece.\n\nGrand Master:\n+%d%% Perfect Dodge, once per armor type.", weaponNoun(s), MasteryArmorACPerLevel, 2*MasteryArmorACPerLevel, 3*MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillShield:
		return fmt.Sprintf("Allows using a shield. Shield AC +0/%d/%d/%d.\n\nGrand Master:\n+%d%% Perfect Dodge with a shield equipped.", MasteryArmorACPerLevel, 2*MasteryArmorACPerLevel, 3*MasteryArmorACPerLevel, ArmorGMDodgeBonus)
	case SkillBodybuilding:
		return fmt.Sprintf("Max HP +0/%d/%d/%d.\n\nGrand Master:\nAlso +%d%% of base max HP.", BodybuildingHPPerTier, 2*BodybuildingHPPerTier, 3*BodybuildingHPPerTier, BodybuildingGMMaxHPPct)
	case SkillMeditation:
		return fmt.Sprintf("SP regen +0/%d/%d/%d per tick.\n\nGrand Master:\nSpells, traps and techniques cost %d%% less SP.", MeditationRegenPerTier, 2*MeditationRegenPerTier, 3*MeditationRegenPerTier, MeditationGMSpellCostReductionPct)
	case SkillLearning:
		return fmt.Sprintf("Personal XP +0/%d/%d/%d%%.\n\nGrand Master:\nA living teacher grants +%d%% XP to all rosters; does not stack.", LearningXPPctPerTier, 2*LearningXPPctPerTier, 3*LearningXPPctPerTier, LearningGMPartyXPPct)
	case SkillArmsMaster:
		return fmt.Sprintf("Normal weapon damage +0/%d/%d/%d.\n\nGrand Master:\n+%d%% weapon crit.", ArmsMasterDamagePerTier, 2*ArmsMasterDamagePerTier, 3*ArmsMasterDamagePerTier, ArmsMasterGMCritBonus)
	case SkillMerchant:
		return fmt.Sprintf("Gold-shop buy prices -0/%d/%d/%d%%; sell prices +0/%d/%d/%d%%. Best active Merchant only.", MerchantPricePctPerTier, 2*MerchantPricePctPerTier, 3*MerchantPricePctPerTier, MerchantPricePctPerTier, 2*MerchantPricePctPerTier, 3*MerchantPricePctPerTier)
	case SkillDisarmTrap:
		return fmt.Sprintf("Chest disarm: %d/%d/%d/%d%% (best active user). Personal hit damage -0/%d/%d/%d after defenses; true damage and DoTs bypass it.", DisarmTrapAvoidBasePct, DisarmTrapAvoidBasePct+DisarmTrapAvoidPerTierPct, DisarmTrapAvoidBasePct+2*DisarmTrapAvoidPerTierPct, DisarmTrapAvoidBasePct+3*DisarmTrapAvoidPerTierPct, DisarmTrapDamageReductionPerTier, 2*DisarmTrapDamageReductionPerTier, 3*DisarmTrapDamageReductionPerTier)
	case SkillTrapper:
		return fmt.Sprintf("Trap damage +0/%d/%d/%d + floor((Intellect + Accuracy)/%d). Control duration: +0/%d/%d/%ds RT; +%s turns TB.", TrapperDamagePerTier, 2*TrapperDamagePerTier, 3*TrapperDamagePerTier, TrapStatScalingDivisor, TrapperSecondsPerTier, 2*TrapperSecondsPerTier, 3*TrapperSecondsPerTier, masteryProgression(TrapperTurnBonus))
	case SkillSleightOfHand:
		return fmt.Sprintf("Melee hits: %d/%d/%d/%d%% pickpocket chance. One successful theft per surviving enemy; yields loot or gold.", SleightChancePctPerTier, 2*SleightChancePctPerTier, 3*SleightChancePctPerTier, 4*SleightChancePctPerTier)
	case SkillRepair, SkillIdentifyItem:
		return "No effect yet."
	case SkillDualWielding:
		return fmt.Sprintf("Allows an off-hand weapon. RT weapon cooldown -0/%d/%d/%d%%; separate hand recovery. TB: an off-hand weapon grants at least 2 actions per round.", DualWieldingCDReductionPerTier, 2*DualWieldingCDReductionPerTier, 3*DualWieldingCDReductionPerTier)
	case SkillIronBody:
		return fmt.Sprintf("Armor Class +%d/%d/%d/%d.\n\nGrand Master:\n+%d%% Perfect Dodge.", IronBodyACPerTier, 2*IronBodyACPerTier, 3*IronBodyACPerTier, 4*IronBodyACPerTier, IronBodyGMDodgeBonus)
	case SkillSpiritualTraining:
		return fmt.Sprintf("Melee swings: %d/%d/%d/%d%% chance to cast the slotted offensive spell for free (no SP or extra action), even on a miss.", SpiritualTrainingProcPctPerTier, 2*SpiritualTrainingProcPctPerTier, 3*SpiritualTrainingProcPctPerTier, 4*SpiritualTrainingProcPctPerTier)
	case SkillElementalMastery:
		return fmt.Sprintf("%s spells ignore %s%% of matching resistance.", elementalMagicSchoolNames(), masteryProgression(ElementalMasteryPiercePct))
	case SkillAnimalBonding:
		return fmt.Sprintf("Weapon attacks/spells: %s%% summon chance, up to %d living bears. Bear HP: %s%% of hero max HP; AC/damage: %s%% of hero stats.", masteryProgression(AnimalBondingProcPct), AnimalBondingSummonMax, masteryProgression(AnimalBondingHPPct), masteryProgression(AnimalBondingStatPct))
	case SkillSacrifice:
		return fmt.Sprintf("Redirects %s%% of ally hit damage after defenses. Strongest living active protector only; no second mitigation or DoTs.", masteryProgression(SacrificeRedirectPct))
	case SkillImpenetrableDefense:
		return fmt.Sprintf("Hit damage -%s after defenses. Does not reduce true damage or DoTs.", masteryProgression(ImpenetrableDefenseReduction))
	case SkillLockpicking:
		return fmt.Sprintf("Locked-door success: %s%%. After %d failed non-key attempts the lock jams; a key still works.", masteryProgression(LockpickingChancePct), DoorMaxNonKeyAttempts)
	case SkillNaturalHealer:
		return fmt.Sprintf("Spell HP healing +%s%%.", masteryProgression(NaturalHealerBonusPct))
	case SkillCelestialProvidence:
		return "Living active Celestial: random Master-tier party buff at dawn/dusk, lasting until the next phase. Does not stack."
	case SkillOrcishFury:
		return fmt.Sprintf("Normal weapon damage +%s.", masteryProgression(OrcishFuryDamageBonus))
	case SkillHalflingGuile:
		return "Halves this hero's weight when enemies choose a random party target."
	case SkillDarkElfBinding:
		return fmt.Sprintf("Weapon/spell hits, including fields: %d%% chance to turn the target into an ally for this map. Replaces damage; excludes undead, formless, bosses and invulnerable targets.", DarkElfBindingChancePct)
	case SkillSpellAbsorption:
		return fmt.Sprintf("%s%% chance to absorb a hostile spell hit. Restores its damage before defenses as both HP and SP instead.", masteryProgression(SpellAbsorbChancePct))
	case SkillStrongMagic:
		return fmt.Sprintf("Offensive spell damage +%s%%. HP cost: %s%% of paid SP, capped to leave 1 HP.", masteryProgression(StrongMagicPct), masteryProgression(StrongMagicPct))
	default:
		return ""
	}
}

// SpellAbsorbChancePct is the Spell Absorption trigger chance at the given
// tier (0-based, Novice included like Sleight of Hand): 15/30/45/60%.
func SpellAbsorbChancePct(tier int) int {
	return (tier + 1) * SpellAbsorbChancePctPerTier
}

// StrongMagicPct is the Strong Magic exchange rate at the given tier: the
// percent of the spell's SP cost burned as HP, and the percent added to the
// spell's damage - 25/50/75/100%.
func StrongMagicPct(tier int) int {
	return (tier + 1) * StrongMagicPctPerTier
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
