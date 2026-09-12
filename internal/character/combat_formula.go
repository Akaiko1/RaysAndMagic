package character

import (
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/stats"
)

// EffectiveCombatStats takes one equipment/buff snapshot for a calculation.
func EffectiveCombatStats(c *MMCharacter) stats.StatBonuses {
	if c == nil {
		return stats.StatBonuses{}
	}
	m, i, p, e, a, s, l := c.GetEffectiveStats()
	return stats.StatBonuses{Might: m, Intellect: i, Personality: p, Endurance: e, Accuracy: a, Speed: s, Luck: l}
}

func SpellMasteryTier(c *MMCharacter, def spells.SpellDefinition) int {
	if c != nil {
		if skill := c.SpellMasterySkill(def); skill != nil {
			return int(skill.Mastery)
		}
	}
	return 0
}

func SpellDamageBreakdown(def spells.SpellDefinition, c *MMCharacter) stats.Breakdown {
	return def.DamageFormula().Evaluate(EffectiveCombatStats(c), SpellMasteryTier(c, def))
}

type HealingBreakdown struct {
	stats.Breakdown
	HealerPercent int
}

func SpellHealingBreakdown(def spells.SpellDefinition, c *MMCharacter) HealingBreakdown {
	out := HealingBreakdown{Breakdown: def.HealingFormula().Evaluate(EffectiveCombatStats(c), SpellMasteryTier(c, def))}
	if def.HealAmount > 0 && c != nil && c.HasSkill(SkillNaturalHealer) {
		out.HealerPercent = NaturalHealerBonusPct(c.SkillTier(SkillNaturalHealer))
		out.Total = out.Total * (100 + out.HealerPercent) / 100
	}
	return out
}

func WeaponDamageFormula(def *config.WeaponDefinitionConfig) stats.Formula {
	if def == nil {
		return stats.Formula{}
	}
	primary := def.BonusStat
	if primary == "" {
		primary = "Might"
	}
	f := stats.Formula{Base: def.Damage, Terms: []stats.ScalingTerm{{Stat: primary, Divisor: WeaponPrimaryStatDivisor}}}
	if def.BonusStatSecondary != "" {
		f.Terms = append(f.Terms, stats.ScalingTerm{Stat: def.BonusStatSecondary, Divisor: WeaponSecondaryStatDivisor})
	}
	return f
}

type WeaponBreakdown struct {
	stats.Breakdown
	ArmsMaster int
	OrcishFury int
}

func WeaponDamageBreakdown(def *config.WeaponDefinitionConfig, c *MMCharacter) WeaponBreakdown {
	out := WeaponBreakdown{Breakdown: WeaponDamageFormula(def).Evaluate(EffectiveCombatStats(c), 0)}
	if def != nil && c != nil {
		out.ArmsMaster = c.ArmsMasterTier() * ArmsMasterDamagePerTier
		if c.HasSkill(SkillOrcishFury) {
			out.OrcishFury = OrcishFuryDamageBonus(c.SkillTier(SkillOrcishFury))
		}
		out.Total += out.ArmsMaster + out.OrcishFury
	}
	return out
}

// DurationBreakdown preserves authored seconds and the applied mastery bonus.
type DurationBreakdown struct {
	Base       int
	MasteryPct int
	Seconds    int
}

func SpellDurationBreakdown(def spells.SpellDefinition, c *MMCharacter) DurationBreakdown {
	if def.Duration <= 0 {
		return DurationBreakdown{}
	}
	out := DurationBreakdown{Base: def.Duration, MasteryPct: SpellMasteryTier(c, def) * SpellMasteryDurationBonusPct}
	out.Seconds = out.Base * (100 + out.MasteryPct) / 100
	return out
}

// WeaponStrikeCount and WeaponStrikeDamage describe one melee action. Ranged
// volleys retain full damage per projectile; their count is authored separately.
func WeaponStrikeCount(def *config.WeaponDefinitionConfig) int {
	if def != nil && def.Range <= 3 && def.DoubleStrike {
		return 2
	}
	return 1
}

func WeaponStrikeDamage(def *config.WeaponDefinitionConfig, normal int) int {
	strikes := WeaponStrikeCount(def)
	return (normal + strikes - 1) / strikes
}
