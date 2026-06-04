package game

// Balance constants are the single source of truth shared by combat formulas
// and tooltip text. Touching a number here updates BOTH the gameplay
// calculation and the description shown to the player — that's the whole
// point. Do not introduce duplicate literals in combat code or UI strings;
// reference these constants instead.

// Mastery progression bonuses applied per mastery level.
// Mastery values: Novice=0, Expert=1, Master=2, Grandmaster=3.
const (
	// MasteryWeaponTrueDamagePerTier is bonus TRUE damage added per weapon-mastery
	// tier for the weapon's category skill. True damage bypasses the target's
	// armor class AND lands even through a Perfect Dodge (only the dodged portion
	// is the normal damage). Expert +3 / Master +6 / Grandmaster +9.
	MasteryWeaponTrueDamagePerTier = 3

	// WeaponGMCritBonus is the extra crit chance (percent points) a Grandmaster
	// gets with their mastered weapon. GM also makes the whole strike ignore the
	// target's Perfect Dodge (see weaponMasteryStrike).
	WeaponGMCritBonus = 7

	// MasteryArmorACPerLevel is the bonus armor class added per armor-mastery
	// level for the armor's category skill.
	MasteryArmorACPerLevel = 2

	// ArmorGMDodgeBonus is the extra Perfect Dodge chance (percent points) granted
	// per Grandmaster-mastered armor type the character is wearing at least one
	// piece of (e.g. GM Plate + plate worn → +5; also GM Shield + shield → +10).
	ArmorGMDodgeBonus = 5

	// MagicGMResistPiercePct: a Grandmaster's spells ignore this percent of the
	// target's resistance to that school's damage type.
	MagicGMResistPiercePct = 50

	// MasterySpellEffectPerLevel is the bonus added to a spell's effect
	// (damage, healing, duration in seconds, stat bonus) per magic-school
	// mastery tier ABOVE Novice. Applied as `skill.Mastery × this` where
	// Mastery is 0/1/2/3 for Novice/Expert/Master/Grandmaster — so a Novice
	// caster gets +0 here. The duration calculation also multiplies by
	// SpellSchoolLevelDurationBonus, which uses `skill.Level()` (1..4), so
	// Novice still gets a +10% duration bump; this asymmetry between
	// damage (no Novice bonus) and duration (Novice bonus) is intentional.
	MasterySpellEffectPerLevel = 5

	// Misc-skill effects, scaled by mastery tier (Novice=0 → bonus from Expert up),
	// mirroring the weapon/armor/magic mastery pattern.

	// LearningXPPctPerTier: +% experience this character gains, per Learning tier.
	LearningXPPctPerTier = 10
	// LearningGMPartyXPPct: a Grandmaster "teacher" grants this extra % XP to the
	// WHOLE party (every member), on top of their own per-tier bonus.
	LearningGMPartyXPPct = 5
	// ArmsMasterDamagePerTier: bonus damage with ANY weapon, per ArmsMaster tier
	// (stacks on top of the weapon's own category mastery).
	ArmsMasterDamagePerTier = 2
	// ArmsMasterGMCritBonus: a Grandmaster Arms Master gets this extra crit chance
	// (percent points) with ANY weapon.
	ArmsMasterGMCritBonus = 5
	// DisarmTrapDamageReductionPerTier: flat incoming-damage reduction per
	// DisarmTrap tier. PLACEHOLDER — see TODO at usage; the real feature is
	// disarming trap tiles, not a damage shield.
	DisarmTrapDamageReductionPerTier = 1
	// MerchantPricePctPerTier: % better buy AND sell prices, per the party's best
	// Merchant tier (sell +this%, buy -this%).
	MerchantPricePctPerTier = 5
	// MeditationGMSpellCostReductionPct: a Grandmaster meditator pays this percent
	// less spell points for every spell.
	MeditationGMSpellCostReductionPct = 25
)

// Stat-to-damage scaling. A weapon's `bonus_stat` field selects which stat
// scales its damage; the value is divided by these divisors.
const (
	// WeaponPrimaryStatDivisor: primary stat bonus = stat / divisor.
	WeaponPrimaryStatDivisor = 3

	// WeaponSecondaryStatDivisor: weaker secondary scaling for weapons that
	// list a `bonus_stat_secondary`.
	WeaponSecondaryStatDivisor = 4
)

// Defense and progression.
const (
	// ArmorPhysicalReductionDivisor: physical damage reduction = AC / divisor.
	// Quoted in armor tooltips, applied in ApplyArmorDamageReduction.
	ArmorPhysicalReductionDivisor = 2

	// StatPointsPerLevel is granted on each level-up. Mentioned in the
	// level-up combat message and applied in checkLevelUp.
	StatPointsPerLevel = 5

	// XPRequiredPerLevel multiplied by current level gives the XP needed for
	// the next level: required = currentLevel * XPRequiredPerLevel.
	XPRequiredPerLevel = 100

	// LevelUpChoiceInterval: a class-progression choice is offered every Nth
	// level (3, 6, 9, 12, ...).
	LevelUpChoiceInterval = 3

	// MinLevelUpOptions: a level-up choice always presents at least this many
	// options. Levels with fewer (or zero) explicit options in level_up.yaml are
	// padded with random upgrades of skills the character already owns.
	MinLevelUpOptions = 4

	// LuckToCritDivisor: Luck/divisor adds to a character's critical chance
	// (in percent points), on top of the weapon's base crit_chance.
	LuckToCritDivisor = 4

	// LuckToDodgeDivisor: Luck/divisor sets the perfect-dodge chance in
	// percent points.
	LuckToDodgeDivisor = 5

	// CritDamageMultiplier multiplies final damage on a critical hit.
	// Applied identically to weapon swings, melee, and ranged.
	CritDamageMultiplier = 2

	// ArmorPierceRangedChancePct: a ranged hit has this percent chance to
	// bypass armor entirely (treated as armor=0 for that strike).
	ArmorPierceRangedChancePct = 33

	// SpellSchoolLevelDurationBonus: per skill LEVEL of the spell's school,
	// duration is scaled by (1 + level * bonus). 0.1 → +10% per level.
	// Note level here is `skill.Level()` (1..4 for Novice..Grandmaster), so
	// Novice already enjoys +10%. Damage scaling, by contrast, uses Mastery
	// (0..3) and so deliberately gives Novice no damage bonus. See
	// MasterySpellEffectPerLevel for the full rationale.
	SpellSchoolLevelDurationBonus = 0.1
)

// Combat reach distances in tiles. Multiplied by tile size at call time.
const (
	// TurnBasedVisionRangeTiles is how far a monster's "I saw the party"
	// trigger reaches when starting / entering turn-based mode.
	TurnBasedVisionRangeTiles = 6.0

	// PackAggroRadiusTiles: when a monster is hit, same-name neighbors
	// within this radius become aggressive too.
	PackAggroRadiusTiles = 8.0

	// TurnBasedSpRegenEveryNRounds: how many full party rounds must pass in
	// turn-based mode between SP regeneration ticks. Each tick adds
	// CalculateManaRegenAmount SP to every able-bodied member.
	TurnBasedSpRegenEveryNRounds = 3

	// TorchLightRadiusTiles: the lit-area radius granted by the torch_light
	// utility spell. Tuning this changes how far the player can see in dark
	// biomes.
	TorchLightRadiusTiles = 4.0
)

// Speed-stat → action cooldown curve. Cooldown in frames is a linear function
// of the character's effective Speed stat, clamped to [Min, Max] frames.
// The formula `frames = Intercept - Slope * Speed` was originally fit through
// two anchor points: Speed=5 ⇒ ~60 frames, Speed=50 ⇒ ~30 frames. Adjusting
// these knobs changes how much Speed matters for action cadence in realtime
// combat.
const (
	AttackCooldownIntercept  = 63.333333 // frames at Speed=0 (before clamp)
	AttackCooldownSpeedSlope = 2.0 / 3.0 // frames lost per +1 Speed
	AttackCooldownMinFrames  = 15        // floor: ~0.125s at 120 TPS
	AttackCooldownMaxFrames  = 90        // ceiling: ~0.75s at 120 TPS
)

// Sprite animation timing.
const (
	// SpriteFrameStride is the number of game frames each animation frame is
	// held for in horizontal sprite sheets (~0.3s at 60 FPS).
	SpriteFrameStride = 18

	// SpriteSheetFrameCount is the expected number of frames in a horizontal
	// sprite sheet (sheet width = frame height × SpriteSheetFrameCount).
	SpriteSheetFrameCount = 4
)
