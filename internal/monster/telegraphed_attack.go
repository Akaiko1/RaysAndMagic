package monster

import (
	"fmt"
	"math"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/status"
)

// TelegraphedAttack defines one visible, avoidable special action. Its warning
// and cooldown are timers authored in RT seconds and TB rounds; its effect
// (a sustained field, an ally shield, the slow or root it lands) lasts
// DurationSeconds, whole rounds, the same span in both modes. Runtime keeps
// them on DualClocks (TelegraphState).
type TelegraphedAttack struct {
	Name            string  `yaml:"name"`
	Kind            string  `yaml:"kind"`
	School          string  `yaml:"school"`
	Shape           string  `yaml:"shape"`
	Range           int     `yaml:"range"`
	CooldownSeconds float64 `yaml:"cooldown_seconds"`
	CooldownRounds  int     `yaml:"cooldown_rounds"`
	WarningSeconds  float64 `yaml:"warning_seconds"`
	WarningRounds   int     `yaml:"warning_rounds"`
	DurationSeconds int     `yaml:"duration_seconds,omitempty"`
	DamagePercent   int     `yaml:"damage_percent"`
	SustainDamage   int     `yaml:"sustain_damage,omitempty"`
	BelowHPPercent  int     `yaml:"below_hp_percent,omitempty"`
}

// TelegraphShieldAbsorption is the flat per-hit soak granted to an ally by a
// shield telegraph. Preview descriptions and combat use the same staged stats.
func TelegraphShieldAbsorption(damageMin, damageMax int) int {
	return max(1, (damageMin+damageMax)/4)
}

func (a *TelegraphedAttack) validate() error {
	if a == nil {
		return nil
	}
	switch a.Kind {
	case "strike", "charge", "slow", "root", "shield", "push", "retreat":
	default:
		return fmt.Errorf("unknown telegraphed attack kind %q", a.Kind)
	}
	switch a.Shape {
	case "lane", "burst", "cone", "target":
	default:
		return fmt.Errorf("unknown telegraphed attack shape %q", a.Shape)
	}
	switch a.School {
	case "fire", "water", "earth", "air":
	default:
		return fmt.Errorf("invalid telegraphed school %q", a.School)
	}
	if a.Name == "" || a.Range < 1 || a.Range > 8 || a.CooldownRounds < 1 || a.WarningRounds < 1 || a.DamagePercent < 0 || a.DamagePercent > 200 || a.BelowHPPercent < 0 || a.BelowHPPercent > 100 || a.SustainDamage < 0 {
		return fmt.Errorf("invalid telegraphed attack bounds")
	}
	for _, v := range []float64{a.CooldownSeconds, a.WarningSeconds} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return fmt.Errorf("telegraphed timing must be finite and positive")
		}
	}
	if a.DurationSeconds < 0 {
		return fmt.Errorf("invalid telegraphed duration")
	}
	if (a.Kind == "shield" || a.Kind == "slow" || a.Kind == "root" || a.SustainDamage > 0) && a.DurationSeconds <= 0 {
		return fmt.Errorf("shield, slow, root and sustained fields need duration_seconds")
	}
	if a.DurationSeconds > 0 {
		check := config.ValidateEffectSeconds
		if a.Kind == "slow" {
			check = config.ValidateSlowSeconds
		}
		if err := check("telegraph duration_seconds", a.DurationSeconds); err != nil {
			return err
		}
	}
	return nil
}

// TelegraphState retains the exact announced tiles across saving and mode
// changes. TargetID empty means the party; it never retargets on release.
type TelegraphState struct {
	Carry     float64          `json:"carry,omitempty"`
	Cooldown  status.DualClock `json:"cooldown_clock,omitzero"`
	Warning   status.DualClock `json:"warning_clock,omitzero"`
	Remaining status.DualClock `json:"remaining_clock,omitzero"` // the lingering field
	Tiles     [][2]int         `json:"tiles,omitempty"`
	Origin    [2]int           `json:"origin,omitempty"`
	Direction [2]int           `json:"direction,omitempty"`
	TargetID  string           `json:"target_id,omitempty"`
	Released  bool             `json:"released,omitempty"`
	// Legacy saves: the part of each span left.
	LegacyCooldown  float64 `json:"cooldown,omitempty"`
	LegacyWarning   float64 `json:"warning,omitempty"`
	LegacyRemaining float64 `json:"remaining,omitempty"`
}

func (s TelegraphState) Clone() TelegraphState { s.Tiles = append([][2]int(nil), s.Tiles...); return s }

// CooldownClock arms the authored cooldown: RT seconds beside TB rounds.
func (a *TelegraphedAttack) CooldownClock(tps int) status.DualClock {
	return status.PairClock(a.CooldownSeconds, a.CooldownRounds, tps, 1)
}

// WarningClock arms the authored warning: RT seconds beside TB rounds.
func (a *TelegraphedAttack) WarningClock(tps int) status.DualClock {
	return status.PairClock(a.WarningSeconds, a.WarningRounds, tps, 1)
}

// MigrateLegacy reads an old save's spans left into the clocks.
func (s *TelegraphState) MigrateLegacy(a *TelegraphedAttack, tps int) {
	if a != nil {
		if s.LegacyCooldown > 0 {
			s.Cooldown = status.PairClock(a.CooldownSeconds, a.CooldownRounds, tps, s.LegacyCooldown)
		}
		if s.LegacyWarning > 0 {
			s.Warning = status.PairClock(a.WarningSeconds, a.WarningRounds, tps, s.LegacyWarning)
		}
		if s.LegacyRemaining > 0 {
			s.Remaining = status.PairClock(float64(a.DurationSeconds), config.CombatRounds(a.DurationSeconds), tps, s.LegacyRemaining)
		}
	}
	s.LegacyCooldown, s.LegacyWarning, s.LegacyRemaining = 0, 0, 0
}

// lines describes the telegraph as an ability; d supplies the attack that a
// shield variant converts into absorption.
func (a *TelegraphedAttack) lines(d MonsterDefinition) []EffectLine {
	school := normalizeEffectSchool(a.School)
	var out []EffectLine
	add := func(text string) { out = append(out, EffectLine{Text: text, School: school}) }
	add(uitext.Text("monster.telegraph", a.Name, a.Kind, a.Shape, a.Range))
	add(uitext.Text("monster.telegraph_timing", config.NumberText(a.WarningSeconds), a.WarningRounds, config.NumberText(a.CooldownSeconds), a.CooldownRounds))
	if a.Kind == "shield" {
		add(uitext.Text("monster.telegraph_shield", TelegraphShieldAbsorption(d.DamageMin, d.DamageMax)))
	} else if a.DamagePercent > 0 {
		add(uitext.Text("monster.telegraph_damage", a.DamagePercent))
	}
	if a.DurationSeconds > 0 {
		text := uitext.Text("monster.telegraph_duration", config.DurationText(a.DurationSeconds))
		if a.Kind != "shield" && a.SustainDamage > 0 {
			text += uitext.Text("monster.telegraph_sustain", a.SustainDamage)
		}
		add(text)
	}
	if a.BelowHPPercent > 0 {
		out = append(out, EffectLine{Text: uitext.Text("monster.telegraph_below_hp", a.BelowHPPercent)})
	}
	return out
}
