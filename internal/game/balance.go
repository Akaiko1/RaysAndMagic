package game

import (
	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// Balance constants are the single source of truth shared by combat formulas
// and tooltip text. Touching a number here updates BOTH the gameplay
// calculation and the description shown to the player - that's the whole
// point. Do not introduce duplicate literals in combat code or UI strings;
// reference these constants instead.

// Skill-effect constants now live in the character package (so the map editor
// shares them too - see character/catalog.go); these are thin re-exports so the
// existing combat references keep compiling unchanged. The VALUE is defined once.
const (
	MasteryWeaponTrueDamagePerTier    = character.MasteryWeaponTrueDamagePerTier
	WeaponGMCritBonus                 = character.WeaponGMCritBonus
	MasteryArmorACPerLevel            = character.MasteryArmorACPerLevel
	ArmorGMDodgeBonus                 = character.ArmorGMDodgeBonus
	LearningXPPctPerTier              = character.LearningXPPctPerTier
	LearningGMPartyXPPct              = character.LearningGMPartyXPPct
	ArmsMasterDamagePerTier           = character.ArmsMasterDamagePerTier
	ArmsMasterGMCritBonus             = character.ArmsMasterGMCritBonus
	DisarmTrapDamageReductionPerTier  = character.DisarmTrapDamageReductionPerTier
	MerchantPricePctPerTier           = character.MerchantPricePctPerTier
	MeditationGMSpellCostReductionPct = character.MeditationGMSpellCostReductionPct
)

// Game-only mastery constants (not needed by the editor) stay here.
const (
	// Canonical values live in character/catalog.go (shared with tooltips and
	// the map editor); these are package-local aliases.
	MagicGMResistPiercePct     = character.MagicGMResistPiercePct
	MasterySpellEffectPerLevel = character.MasterySpellEffectPerLevel
)

// Stat-to-damage scaling. A weapon's `bonus_stat` field selects which stat
// scales its damage; the value is divided by these divisors.
const (
	// WeaponPrimaryStatDivisor: primary stat bonus = stat / divisor.
	WeaponPrimaryStatDivisor = character.WeaponPrimaryStatDivisor

	// WeaponSecondaryStatDivisor: weaker secondary scaling for weapons that
	// list a `bonus_stat_secondary`.
	WeaponSecondaryStatDivisor = character.WeaponSecondaryStatDivisor
)

// Defense and progression.
const (
	// Armor mitigation (percentage, diminishing returns) - shared by party AND
	// monster armor via armorMitigationPctFromAC. Canonical values in
	// character/catalog.go. Physical caps at 75%; elemental is the same curve
	// scaled to reach its 33% cap at the same AC.
	ArmorMitigationK            = character.ArmorMitigationK
	ArmorPhysicalMitigationCap  = character.ArmorPhysicalMitigationCap
	ArmorElementalMitigationCap = character.ArmorElementalMitigationCap

	// MaxStatValue is the cap a character's base stat can reach (the stat +button
	// and AUTO distribution both stop here). A mechanics constant, kept out of the
	// UI files that happen to render it.
	MaxStatValue = 99

	// StatPointsPerLevel is granted on each level-up. Mentioned in the
	// level-up combat message and applied in checkLevelUp.
	StatPointsPerLevel = 5

	// XPRequiredPerLevel multiplied by current level gives the LINEAR branch of
	// the XP needed for the next level (see xpStepCost). It alone defines the
	// curve through L12 - the early game is exactly the classic 100 x L.
	XPRequiredPerLevel = 100

	// XPQuadPerLevel is the quadratic branch coefficient of xpStepCost; it
	// takes over from L13 (8 x 13^2 > 100 x 13). Monster XP grows roughly
	// quadratically with a zone's level (~2.2 x L^2: bandit L5 = 55, dust
	// slime L22 = 1000, grandfather clock L30 = 2400), so a purely linear
	// level cost makes kills-per-level FALL as ~1/L - post-20 farming used to
	// run away (~29 level-appropriate kills per level at L5, only ~5 at L30).
	// The L^2 branch holds the pace FLAT at ~15 zone-level kills per level in
	// a 4-hero party (8 / (2.2/4)), L13 through L50.
	// Tuning: 6 -> ~11 kills/level, 10 -> ~18. Totals to REACH a level:
	// L13=7800, L20=22360, L30=71040, L50=326000. Pinned by
	// TestXPStepCostCurve; the pacing note in config.yaml (characters:)
	// mirrors this - keep both in sync.
	XPQuadPerLevel = 8

	// LevelUpChoiceInterval: a class-progression choice is offered every Nth
	// level (3, 6, 9, 12, ...).
	LevelUpChoiceInterval = 3

	// MinLevelUpOptions: a level-up choice always presents at least this many
	// options. Levels with fewer (or zero) explicit options in level_up.yaml are
	// padded with random upgrades of skills the character already owns.
	MinLevelUpOptions = 4

	// LuckToCritDivisor lives in character/catalog.go (cards quote it too).
	LuckToCritDivisor = character.LuckToCritDivisor

	// LuckToDodgeDivisor: Luck/divisor sets the perfect-dodge chance in
	// percent points.
	LuckToDodgeDivisor = 5

	// CritDamageMultiplier lives in character/catalog.go (cards quote it too).
	CritDamageMultiplier = character.CritDamageMultiplier

	// ArmorPierceRangedChancePct lives in character/catalog.go (tooltip SSoT).
	ArmorPierceRangedChancePct = character.ArmorPierceRangedChancePct

	// SpellMasteryDurationBonusPct lives in character/catalog.go (tooltip SSoT).
	SpellMasteryDurationBonusPct = character.SpellMasteryDurationBonusPct
)

// Combat reach distances in tiles. Multiplied by tile size at call time.
const (
	// TurnBasedInputCooldownSeconds throttles party move/rotate repeats in
	// turn-based mode (frames are derived from TPS at the call site).
	TurnBasedInputCooldownSeconds = 0.15

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

	// TurnBasedExtraMonsterActionDelaySeconds: visual pause between the normal
	// monster action pass and the anti-kite extra pass.
	TurnBasedExtraMonsterActionDelaySeconds = 0.18

	// Camping (the Camp button in the inventory tab): costs CampFoodCost food
	// and is refused while any living monster is within CampEnemyRadiusTiles.
	CampFoodCost         = 1
	CampEnemyRadiusTiles = 5.0
)

// Speed-stat -> action cooldown curve. Cooldown in frames is a linear function
// of the character's effective Speed stat, clamped to [Min, Max] frames.
// The formula `frames = Intercept - Slope * Speed` reaches the minimum cooldown
// at AttackCooldownCapSpeed. Adjusting these knobs changes how much Speed
// matters for action cadence in realtime combat.
const (
	AttackCooldownIntercept  = 63.333333 // frames at Speed=0 (before clamp)
	AttackCooldownCapSpeed   = 150.0     // Speed where the curve first reaches MinFrames
	AttackCooldownMinFrames  = 15        // floor: ~0.125s at 120 TPS
	AttackCooldownMaxFrames  = 90        // ceiling: ~0.75s at 120 TPS
	AttackCooldownSpeedSlope = (AttackCooldownIntercept - AttackCooldownMinFrames) /
		AttackCooldownCapSpeed // frames lost per +1 Speed
)

// Real-time per-character cooldown model. In RT every character has their OWN
// cooldown timer (frames); after acting they go gray and selection auto-advances
// to the next ready member, so holding an attack key fires the party in turn
// instead of one member machine-gunning. TB is unaffected (it uses action slots).
const (
	// RTBaseCooldownMult doubles the Speed-curve cooldown for weapon attacks in
	// real time (the "x2 base" balance pass). Speed still scales the curve, so
	// faster characters keep their cadence advantage proportionally.
	RTBaseCooldownMult = 2.0
	// RTCooldownMinFrames / RTCooldownMaxFrames are a safety clamp on the final
	// per-character cooldown (weapon OR spell) so a stray multiplier can't
	// produce a silly value. It is NOT the design range: weapons land ~0.15-1.8s
	// and spells are authored 0.8-5s (x1.35 for slow casters => up to ~6.75s), so
	// the cap is deliberately generous. 12 ~ 0.1s, 900 = 7.5s at 120 TPS.
	RTCooldownMinFrames = 12
	RTCooldownMaxFrames = 900

	// Spell cooldowns are authored in seconds per spell (spells.yaml
	// `cooldown_seconds`); see SpellCooldownDefaultSecondsForLevel for the
	// fallback when a spell omits it. The authored seconds are the cooldown at
	// the reference Speed below; Speed scales it via spellCooldownSpeedFactor.
	SpellCooldownSpeedRefSpeed  = 25   // Speed at which a spell's authored seconds apply as-is
	SpellCooldownSpeedFactorMin = 0.5  // fastest characters: x0.5 (never below half)
	SpellCooldownSpeedFactorMax = 1.35 // slowest characters: x1.35
)

// Per-weapon-TYPE attack-cooldown multipliers are DATA, not code: they live in
// weapons.yaml under `weapon_cooldown_multipliers`, keyed by the canonical
// weapon-skill noun (sword/dagger/axe/spear/bow/mace/staff). A weapon resolves
// to its skill via character.WeaponSkillForCategory (so "throwing" -> dagger),
// and a weapon may override its type with `cooldown_multiplier` (legendaries).
// Read through config.WeaponCooldownMultiplierForSkill.

// Monster target-selection rules (who in the party gets hit). The party is a
// single blob, so this is damage distribution, not positioning. MELEE = random
// living member (both modes). RANGED single-target = the TANK (party slot 0) in
// real time; in turn-based it's the tank most of the time but sometimes a
// back-liner. AoE always hits everyone. The "tank" is the fixed FRONT SLOT
// (index 0), not the highest-Endurance member.
const (
	// RangedOffTankChance: in turn-based, a single-target ranged/projectile hit
	// lands on a random NON-tank living member this often; otherwise on the tank.
	RangedOffTankChance = 0.30
)

// BoundAllySeekTiles is how far a bound undead (bind_undead) hunts for an enemy
// to walk toward - deliberately wider than a typical alert radius so it actively
// seeks across the room rather than only engaging foes already on its doorstep.
const BoundAllySeekTiles = 10.0

// MonsterHitFlashFrames is how long a monster flashes red when hit. The shared
// config `damage_blink_frames` (3) is far too brief to see; this dedicated value
// (~0.1s at 120 TPS) makes the reaction read clearly.
const MonsterHitFlashFrames = 12

// MonsterAttackAnimFrames: how long a striking monster plays its movement
// cycle (a readable lunge) - without it attackers froze on the rest pose.
const MonsterAttackAnimFrames = 18

// volleySpacingFrac: tiles between successive darts of a volley (party bows and
// monster/champion projectiles trail their darts by the same stream spacing).
const volleySpacingFrac = 0.45

// MonsterHitShakeAmplitudeFrac is the peak left-right sprite jitter on hit, as a
// fraction of the sprite's on-screen size. Driven by the same HitTintFrames timer
// as the red flash and decaying with it, it makes a struck monster shudder in
// place - replacing the old positional knockback (it stays put, just rattles).
const MonsterHitShakeAmplitudeFrac = 0.0333

// MonsterHitShakeMaxRefPx caps the on-screen sprite size used to scale the hit
// shudder. The amplitude is a fraction of sprite size, so without a cap a giant
// sprite point-blank (e.g. a size-12 troll) jolts enormously every frame - in
// standee mode that whips it across wall/tree occluders (and past the camera
// plane), reading as furious blinking. Beyond this size the shudder stops growing.
const MonsterHitShakeMaxRefPx = 300.0

// Stun diminishing returns: each successive stun on the SAME target lands for a
// smaller fraction of its duration - 100% -> 50% -> 25% -> 0% (immune) - so no
// target (boss included) can be perma-stun-locked. The chain resets once the
// target has been stun-free for the window below (TB turns / RT seconds). The
// chain length is mode-agnostic; the reset window is tracked per mode so a
// TB<->RT switch mid-fight is conservative (never speeds up the reset).
var StunDRFactorsPct = []int{100, 50, 25, 0}

const (
	StunDRResetTurns   = 4 // TB: stun-free turns that clear the DR chain
	StunDRResetSeconds = 8 // RT: stun-free seconds that clear the DR chain
)

// SmartHealWoundedPct is the HP fraction below which the Space "smart attack"
// treats an ally as wounded and auto-heals them (with a slotted heal) instead
// of attacking. 0.6 = heal anyone at or below 60% HP; healthier party -> attack.
const SmartHealWoundedPct = 0.6

// SpellCooldownDefaultSecondsForLevel lives in the spells package (the editor
// quotes the same default); this alias keeps game-side call sites unchanged.
var SpellCooldownDefaultSecondsForLevel = spells.SpellCooldownDefaultSecondsForLevel

// Sprite animation timing.
const (
	// SpriteFrameStride is the number of game frames each animation frame is
	// held for in horizontal sprite sheets (~0.3s at 60 FPS).
	SpriteFrameStride = 18

	// SpriteSheetFrameCount is the expected number of frames in a horizontal
	// sprite sheet (sheet width = frame height x SpriteSheetFrameCount).
	SpriteSheetFrameCount = 4
)
