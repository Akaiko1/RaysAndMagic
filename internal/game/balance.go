package game

import "ugataima/internal/character"

// Balance constants are the single source of truth shared by combat formulas
// and tooltip text. Touching a number here updates BOTH the gameplay
// calculation and the description shown to the player — that's the whole
// point. Do not introduce duplicate literals in combat code or UI strings;
// reference these constants instead.

// Skill-effect constants now live in the character package (so the map editor
// shares them too — see character/catalog.go); these are thin re-exports so the
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

// Real-time per-character cooldown model. In RT every character has their OWN
// cooldown timer (frames); after acting they go gray and selection auto-advances
// to the next ready member, so holding an attack key fires the party in turn
// instead of one member machine-gunning. TB is unaffected (it uses action slots).
const (
	// RTBaseCooldownMult doubles the Speed-curve cooldown for weapon attacks in
	// real time (the "×2 base" balance pass). Speed still scales the curve, so
	// faster characters keep their cadence advantage proportionally.
	RTBaseCooldownMult = 2.0
	// RTCooldownMinFrames / RTCooldownMaxFrames are a safety clamp on the final
	// per-character cooldown (weapon OR spell) so a stray multiplier can't
	// produce a silly value. It is NOT the design range: weapons land ~0.15–1.8s
	// and spells are authored 0.8–5s (×1.35 for slow casters ⇒ up to ~6.75s), so
	// the cap is deliberately generous. 12 ≈ 0.1s, 900 = 7.5s at 120 TPS.
	RTCooldownMinFrames = 12
	RTCooldownMaxFrames = 900

	// Spell cooldowns are authored in seconds per spell (spells.yaml
	// `cooldown_seconds`); see SpellCooldownDefaultSecondsForLevel for the
	// fallback when a spell omits it. The authored seconds are the cooldown at
	// the reference Speed below; Speed scales it via spellCooldownSpeedFactor.
	SpellCooldownSpeedRefSpeed = 25 // Speed at which a spell's authored seconds apply as-is
	SpellCooldownSpeedFactorMin = 0.5  // fastest characters: ×0.5 (never below half)
	SpellCooldownSpeedFactorMax = 1.35 // slowest characters: ×1.35
)

// Per-weapon-TYPE attack-cooldown multipliers are DATA, not code: they live in
// weapons.yaml under `weapon_cooldown_multipliers`, keyed by the canonical
// weapon-skill noun (sword/dagger/axe/spear/bow/mace/staff). A weapon resolves
// to its skill via character.WeaponSkillForCategory (so "throwing" → dagger),
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

// MonsterHitFlashFrames is how long a monster flashes red when hit. The shared
// config `damage_blink_frames` (3) is far too brief to see; this dedicated value
// (~0.1s at 120 TPS) makes the reaction read clearly.
const MonsterHitFlashFrames = 12

// MonsterHitShakeAmplitudeFrac is the peak left-right sprite jitter on hit, as a
// fraction of the sprite's on-screen size. Driven by the same HitTintFrames timer
// as the red flash and decaying with it, it makes a struck monster shudder in
// place — replacing the old positional knockback (it stays put, just rattles).
const MonsterHitShakeAmplitudeFrac = 0.0333

// SmartHealWoundedPct is the HP fraction below which the Space "smart attack"
// treats an ally as wounded and auto-heals them (with a slotted heal) instead
// of attacking. 0.9 = heal anyone at or below 90% HP; full-HP party → attack.
const SmartHealWoundedPct = 0.9

// SpellCooldownDefaultSecondsForLevel is the fallback spell cooldown (seconds)
// when a spell omits `cooldown_seconds` in spells.yaml: 0.8s at L1 rising 0.1s
// per level. Authored per-spell values (see spells.yaml) override this.
func SpellCooldownDefaultSecondsForLevel(level int) float64 {
	if level < 1 {
		level = 1
	}
	return 0.8 + 0.1*float64(level-1)
}

// Sprite animation timing.
const (
	// SpriteFrameStride is the number of game frames each animation frame is
	// held for in horizontal sprite sheets (~0.3s at 60 FPS).
	SpriteFrameStride = 18

	// SpriteSheetFrameCount is the expected number of frames in a horizontal
	// sprite sheet (sheet width = frame height × SpriteSheetFrameCount).
	SpriteSheetFrameCount = 4
)
