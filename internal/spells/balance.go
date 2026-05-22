package spells

// Balance constants for spell formulas. Single source of truth shared by the
// damage/healing calculators here in the spells package. Game-side combat
// constants live in internal/game/balance.go (the spells package can't
// import game without a cycle, so balance numbers split along that boundary).

const (
	// SpellDamagePerSP: base damage of an offensive spell = SpellPointsCost
	// multiplied by this constant. Caster Intellect adds on top.
	SpellDamagePerSP = 3

	// SpellIntellectDivisor: Intellect/divisor adds to a spell's damage.
	SpellIntellectDivisor = 2

	// HealingPersonalityDivisor: Personality/divisor adds to a healing
	// spell's restored HP, applied on top of the spell's base heal amount.
	HealingPersonalityDivisor = 2
)
