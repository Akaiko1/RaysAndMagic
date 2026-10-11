package config

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// ControlSaveConfig defines a target's saving throw against authored spell controls.
// The curve is for a Grandmaster caster against zero school resistance.
type ControlSaveConfig struct {
	Mechanics             []string           `yaml:"mechanics"`
	DiminishingMechanics  []string           `yaml:"diminishing_mechanics"`
	Curve                 []ControlSavePoint `yaml:"curve"`
	MinPercent            float64            `yaml:"min_percent"`
	MaxPercent            float64            `yaml:"max_percent"`
	MasteryPenaltyPercent float64            `yaml:"mastery_penalty_percent"`
	ResistanceWeight      float64            `yaml:"resistance_weight"`
	ResistedMessage       string             `yaml:"resisted_message"`
	Summary               string             `yaml:"summary"`
	Details               string             `yaml:"details"`
	DiminishingSummary    string             `yaml:"diminishing_summary"`
}

type ControlSavePoint struct {
	LevelDifference int     `yaml:"level_difference"`
	Percent         float64 `yaml:"percent"`
}

func (c ControlSaveConfig) Applies(kind string) bool { return slices.Contains(c.Mechanics, kind) }

func (c ControlSaveConfig) Diminishes(kind string) bool {
	return slices.Contains(c.DiminishingMechanics, kind)
}

func (c ControlSaveConfig) Chance(levelDifference, masteryDeficit, resistance int) float64 {
	chance := c.Curve[0].Percent
	for i := 1; i < len(c.Curve); i++ {
		a, b := c.Curve[i-1], c.Curve[i]
		if levelDifference >= b.LevelDifference {
			chance = b.Percent
			continue
		}
		if levelDifference > a.LevelDifference {
			chance = a.Percent + (b.Percent-a.Percent)*float64(levelDifference-a.LevelDifference)/float64(b.LevelDifference-a.LevelDifference)
		}
		break
	}
	chance += float64(masteryDeficit)*c.MasteryPenaltyPercent + float64(resistance)*c.ResistanceWeight
	return min(c.MaxPercent, max(c.MinPercent, chance))
}

func (c ControlSaveConfig) validate() error {
	fail := func(field string) error {
		return fmt.Errorf("status_effects.control_saves: invalid or missing %s", field)
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(c.MinPercent) || !finite(c.MaxPercent) || c.MinPercent < 0 || c.MaxPercent <= c.MinPercent || c.MaxPercent > 100 {
		return fail("min_percent/max_percent")
	}
	if !finite(c.MasteryPenaltyPercent) || c.MasteryPenaltyPercent < 0 {
		return fail("mastery_penalty_percent")
	}
	if !finite(c.ResistanceWeight) || c.ResistanceWeight < 0 {
		return fail("resistance_weight")
	}
	if len(c.Curve) < 2 {
		return fail("curve")
	}
	for i, p := range c.Curve {
		if !finite(p.Percent) || p.Percent < c.MinPercent || p.Percent > c.MaxPercent || i > 0 && (p.LevelDifference <= c.Curve[i-1].LevelDifference || p.Percent < c.Curve[i-1].Percent) {
			return fail("curve (levels must increase; chances must not decrease)")
		}
	}
	if len(c.Mechanics) == 0 {
		return fail("mechanics")
	}
	for i, kind := range c.Mechanics {
		switch kind {
		case "turn_undead", "mass_fear", "feeblemind":
		default:
			return fail("mechanics: " + kind)
		}
		if slices.Contains(c.Mechanics[:i], kind) {
			return fail("duplicate mechanic: " + kind)
		}
	}
	for i, kind := range c.DiminishingMechanics {
		if !c.Applies(kind) || (kind != "turn_undead" && kind != "mass_fear") || slices.Contains(c.DiminishingMechanics[:i], kind) {
			return fail("diminishing_mechanics: " + kind)
		}
	}
	if c.ResistedMessage == "" || c.Summary == "" || c.Details == "" || len(c.DiminishingMechanics) > 0 && c.DiminishingSummary == "" {
		return fail("resisted_message/summary/details/diminishing_summary")
	}
	format := strings.ReplaceAll(c.ResistedMessage, "%%", "")
	if strings.Count(format, "%s") != 2 || strings.Contains(strings.ReplaceAll(format, "%s", ""), "%") {
		return fail("resisted_message must contain exactly two %s placeholders")
	}
	return nil
}
