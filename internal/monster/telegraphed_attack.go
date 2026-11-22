package monster

import (
	"fmt"
	"math"
	"slices"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/status"
)

// TelegraphedAttack defines one visible, avoidable special action. Its warning
// and cooldown are timers authored in RT seconds and TB rounds; its effect
// (a sustained field, an ally shield, the slow or root it lands) lasts
// DurationSeconds, whole rounds, the same span in both modes. Runtime keeps
// them on DualClocks (TelegraphState).
type TelegraphedAttack struct {
	Name   string          `yaml:"name"`
	Kind   TelegraphKind   `yaml:"kind"`
	School damagecalc.Type `yaml:"school"`
	Shape  TelegraphShape  `yaml:"shape"`
	Range  int             `yaml:"range"`
	// Timer exception: cooldown and warning are authored RT/TB pairs, not the
	// 1 round = 3 s conversion; the warning is a reaction window.
	CooldownSeconds float64 `yaml:"cooldown_seconds"`
	CooldownRounds  int     `yaml:"cooldown_rounds"`
	WarningSeconds  float64 `yaml:"warning_seconds"`
	WarningRounds   int     `yaml:"warning_rounds"`
	DurationSeconds int     `yaml:"duration_seconds,omitempty"` // whole rounds, both modes: no exception
	DamagePercent   int     `yaml:"damage_percent"`
	SustainPercent  int     `yaml:"sustain_percent,omitempty"` // a lingering field's damage per second, percent of the attack
	Riders          Riders  `yaml:"riders,omitempty"`          // required on a damaging telegraph
	BelowHPPercent  int     `yaml:"below_hp_percent,omitempty"`
}

// TelegraphKind is what a telegraph does on release.
type TelegraphKind string

const (
	TelegraphStrike  TelegraphKind = "strike"
	TelegraphCharge  TelegraphKind = "charge"  // rushes along its lane, then strikes
	TelegraphSlow    TelegraphKind = "slow"    // a landed hit slows
	TelegraphRoot    TelegraphKind = "root"    // a landed hit roots
	TelegraphShield  TelegraphKind = "shield"  // shields an ally instead of striking
	TelegraphPush    TelegraphKind = "push"    // a landed hit shoves the party along its direction
	TelegraphRetreat TelegraphKind = "retreat" // strikes, then steps back
)

var telegraphKinds = []TelegraphKind{TelegraphStrike, TelegraphCharge, TelegraphSlow, TelegraphRoot, TelegraphShield, TelegraphPush, TelegraphRetreat}

// Hinders reports a kind whose landed hit slows or roots.
func (k TelegraphKind) Hinders() bool { return k == TelegraphSlow || k == TelegraphRoot }

// TelegraphShape is the footprint of marked tiles.
type TelegraphShape string

const (
	TelegraphLane   TelegraphShape = "lane"
	TelegraphBurst  TelegraphShape = "burst"
	TelegraphCone   TelegraphShape = "cone"
	TelegraphTarget TelegraphShape = "target"
)

var telegraphShapes = []TelegraphShape{TelegraphLane, TelegraphBurst, TelegraphCone, TelegraphTarget}

// telegraphSchools are the elements a telegraph may carry; silence stops
// every telegraph for that reason.
var telegraphSchools = []damagecalc.Type{damagecalc.Fire, damagecalc.Water, damagecalc.Earth, damagecalc.Air}

// TelegraphShieldAbsorption is the flat per-hit soak granted to an ally by a
// shield telegraph. Preview descriptions and combat use the same staged stats.
func TelegraphShieldAbsorption(damageMin, damageMax int) int {
	return max(1, (damageMin+damageMax)/4)
}

func (a *TelegraphedAttack) validate() error {
	if a == nil {
		return nil
	}
	if !slices.Contains(telegraphKinds, a.Kind) {
		return fmt.Errorf("unknown telegraphed attack kind %q", a.Kind)
	}
	if !slices.Contains(telegraphShapes, a.Shape) {
		return fmt.Errorf("unknown telegraphed attack shape %q", a.Shape)
	}
	if !slices.Contains(telegraphSchools, a.School) {
		return fmt.Errorf("invalid telegraphed school %q", a.School)
	}
	if a.Name == "" || a.Range < 1 || a.Range > 8 || a.CooldownRounds < 1 || a.WarningRounds < 1 || a.DamagePercent < 0 || a.DamagePercent > MaxAttackPercent || a.BelowHPPercent < 0 || a.BelowHPPercent > 100 || a.SustainPercent < 0 || a.SustainPercent > MaxAttackPercent {
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
	if (a.DamagePercent > 0) != a.Riders.Authored() {
		return fmt.Errorf("a damaging telegraph needs riders: true or false (a shield has none)")
	}
	if (a.Kind == TelegraphShield || a.Kind.Hinders() || a.SustainPercent > 0) && a.DurationSeconds <= 0 {
		return fmt.Errorf("shield, slow, root and sustained fields need duration_seconds")
	}
	if a.DurationSeconds > 0 {
		check := config.ValidateEffectSeconds
		if a.Kind == TelegraphSlow {
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
	school := a.School.String()
	var out []EffectLine
	add := func(text string) { out = append(out, EffectLine{Text: text, School: school}) }
	add(uitext.Text("monster.telegraph", a.Name, a.Kind, a.Shape, a.Range))
	add(uitext.Text("monster.telegraph_timing", config.NumberText(a.WarningSeconds), a.WarningRounds, config.NumberText(a.CooldownSeconds), a.CooldownRounds))
	if a.Kind == TelegraphShield {
		add(uitext.Text("monster.telegraph_shield", TelegraphShieldAbsorption(d.DamageMin, d.DamageMax)))
	} else if a.DamagePercent > 0 {
		add(uitext.Text("monster.telegraph_damage", a.DamagePercent))
	}
	if a.DurationSeconds > 0 {
		text := uitext.Text("monster.telegraph_duration", config.DurationText(a.DurationSeconds))
		if a.Kind != TelegraphShield && a.SustainPercent > 0 {
			text += uitext.Text("monster.telegraph_sustain", a.SustainPercent)
		}
		add(text)
	}
	if a.BelowHPPercent > 0 {
		out = append(out, EffectLine{Text: uitext.Text("monster.telegraph_below_hp", a.BelowHPPercent)})
	}
	return out
}
