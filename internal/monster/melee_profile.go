package monster

import (
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
)

// MeleeProfile is the shared ordinary-monster rule, independent of delivery.
// A ranged capability does not exclude an adjacent melee swing.
type MeleeProfile struct {
	School          string
	ElementalSchool string
	ElementalAttack config.ElementalAttackConfig
}

func meleeProfile(champion, inert bool, rules config.ElementalAttackConfig, school string) MeleeProfile {
	if champion || inert {
		return MeleeProfile{}
	}
	if parsed, err := damagecalc.ParseType(school); err == nil && parsed != damagecalc.Physical {
		school = parsed.String()
	} else {
		school = ""
	}
	return MeleeProfile{School: damagecalc.Physical.String(), ElementalSchool: school, ElementalAttack: rules}
}

func (m *Monster3D) MeleeProfile(rules config.ElementalAttackConfig, school string) MeleeProfile {
	if m == nil {
		return MeleeProfile{}
	}
	if m.Disposition != "" {
		school = ""
	}
	return meleeProfile(m.IsChampion(), m.WarlordIdol, rules, school)
}

func (d MonsterDefinition) MeleeProfile(rules config.ElementalAttackConfig, school string) MeleeProfile {
	if d.Disposition != "" {
		school = ""
	}
	return meleeProfile(d.Champion != "", d.WarlordIdol, rules, school)
}

// Resolve rolls at most once, before target defenses, and preserves one hit.
// Excluded profiles and missing biome context consume no randomness.
func (p MeleeProfile) Resolve(parts damagecalc.Parts, roll func() float64) (damagecalc.Parts, string, bool) {
	if p.School != "" && p.ElementalSchool != "" && p.ElementalAttack.Chance > 0 && roll() < p.ElementalAttack.Chance {
		return p.ElementalAttack.Scale(parts), p.ElementalSchool, true
	}
	return parts, p.School, false
}

type CombatEffectContext struct {
	ElementalAttack config.ElementalAttackConfig
	ElementalSchool string
}
