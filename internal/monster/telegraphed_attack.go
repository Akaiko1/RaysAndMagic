package monster

import (
	"fmt"
	"math"
)

// TelegraphedAttack defines one visible, avoidable special action. Durations
// specify equivalent RT seconds and TB rounds; runtime stores spent fractions.
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
	DurationSeconds float64 `yaml:"duration_seconds,omitempty"`
	DurationRounds  int     `yaml:"duration_rounds,omitempty"`
	DamagePercent   int     `yaml:"damage_percent"`
	SustainDamage   int     `yaml:"sustain_damage,omitempty"`
	BelowHPPercent  int     `yaml:"below_hp_percent,omitempty"`
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
	if math.IsNaN(a.DurationSeconds) || math.IsInf(a.DurationSeconds, 0) || a.DurationSeconds < 0 || a.DurationRounds < 0 || (a.DurationSeconds > 0) != (a.DurationRounds > 0) {
		return fmt.Errorf("invalid telegraphed duration")
	}
	if (a.Kind == "shield" || a.SustainDamage > 0) && a.DurationSeconds <= 0 {
		return fmt.Errorf("shield and sustained fields need durations")
	}
	return nil
}

// TelegraphState retains the exact announced tiles across saving and mode
// changes. TargetID empty means the party; it never retargets on release.
type TelegraphState struct {
	Carry     float64  `json:"carry,omitempty"`
	Cooldown  float64  `json:"cooldown,omitempty"`
	Warning   float64  `json:"warning,omitempty"`
	Remaining float64  `json:"remaining,omitempty"`
	Tiles     [][2]int `json:"tiles,omitempty"`
	Origin    [2]int   `json:"origin,omitempty"`
	Direction [2]int   `json:"direction,omitempty"`
	TargetID  string   `json:"target_id,omitempty"`
	Released  bool     `json:"released,omitempty"`
}

func (s TelegraphState) Clone() TelegraphState { s.Tiles = append([][2]int(nil), s.Tiles...); return s }
