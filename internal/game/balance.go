package game

// Balance constants are the single source of truth shared by combat formulas
// and tooltip text. Touching a number here updates BOTH the gameplay
// calculation and the description shown to the player — that's the whole
// point. Do not introduce duplicate literals in combat code or UI strings;
// reference these constants instead.

// Mastery progression bonuses applied per mastery level.
// Mastery values: Novice=0, Expert=1, Master=2, Grandmaster=3.
const (
	// MasteryWeaponDamagePerLevel is the bonus melee/ranged damage added
	// per weapon-mastery level for the weapon's category skill.
	MasteryWeaponDamagePerLevel = 2

	// MasteryArmorACPerLevel is the bonus armor class added per armor-mastery
	// level for the armor's category skill.
	MasteryArmorACPerLevel = 1

	// MasterySpellEffectPerLevel is the bonus added to a spell's effect
	// (damage, healing, duration in seconds, stat bonus) per magic-school
	// mastery level.
	MasterySpellEffectPerLevel = 5
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

	// AutoMasteryCastsPerLevel: every N casts of a spell school auto-bumps
	// its mastery one tier (capped at Grandmaster).
	AutoMasteryCastsPerLevel = 30

	// LuckToCritDivisor: Luck/divisor adds to a character's critical chance
	// (in percent points), on top of the weapon's base crit_chance.
	LuckToCritDivisor = 4

	// LuckToDodgeDivisor: Luck/divisor sets the perfect-dodge chance in
	// percent points.
	LuckToDodgeDivisor = 5
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
