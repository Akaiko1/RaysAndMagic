package spells

import "ugataima/internal/stats"

type DamageKind int

const (
	DamageNone DamageKind = iota
	DamageProjectile
	DamageZone
	DamageNova
)

type DamageFormula struct {
	stats.Formula
	Kind           DamageKind
	CostMultiplier int
}

// DamageFormula selects the complete source formula, independently of delivery
// code or UI. Control and utility spells cannot acquire damage from their cost.
func (d SpellDefinition) DamageFormula() DamageFormula {
	if d.DealsNoDamage {
		return DamageFormula{}
	}
	f := DamageFormula{}
	switch {
	case d.PartyAoeRadiusTiles > 0 || d.MapWide:
		f.Kind = DamageNova
		f.Base = d.SpellPointsCost * SpellDamagePerSP
		f.MasteryPerTier = d.MasteryDamagePerTier
	case d.ZoneRadiusTiles > 0:
		f.Kind = DamageZone
		f.Base = d.ZoneTickDamage
		f.Terms = []stats.ScalingTerm{{Stat: "Intellect", Divisor: SpellIntellectDivisor}}
		f.MasteryPerTier = MasterySpellEffectPerLevel
	case d.IsProjectile:
		f.Kind = DamageProjectile
		f.CostMultiplier = max(1, d.DamageCostMultiplier)
		f.Base = d.SpellPointsCost * SpellDamagePerSP * f.CostMultiplier
		primary := "Intellect"
		if SchoolScalesWithPersonality(d.School) {
			primary = "Personality"
		}
		f.Terms = []stats.ScalingTerm{{Stat: primary, Divisor: SpellIntellectDivisor}}
		if d.ScalesWithPersonality && primary != "Personality" {
			f.Terms = append(f.Terms, stats.ScalingTerm{Stat: "Personality", Divisor: SpellIntellectDivisor})
		}
		f.MasteryPerTier = MasterySpellEffectPerLevel
	default:
		return f
	}
	if len(d.DamageByMastery) == 4 {
		f.Formula = stats.Formula{MasteryLadder: d.DamageByMastery}
		f.CostMultiplier = 0
	}
	return f
}

func (d SpellDefinition) HealingFormula() stats.Formula {
	if d.HealAmount <= 0 {
		return stats.Formula{}
	}
	return stats.Formula{
		Base:           d.HealAmount,
		Terms:          []stats.ScalingTerm{{Stat: "Personality", Divisor: HealingPersonalityDivisor}},
		MasteryPerTier: MasterySpellEffectPerLevel,
	}
}
