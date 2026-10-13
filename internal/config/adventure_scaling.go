package config

import (
	"fmt"
	"math"
	"slices"
)

// AdventureScalingConfig is the auto-level of every map an adventure schedule
// opens: a visit sets a level and every monster moves from its authored level
// to it along these curves, keeping its archetype ratio.
type AdventureScalingConfig struct {
	VisitLevelBonus int                `yaml:"visit_level_bonus"` // visit level = party average + bonus
	ArmorExponent   float64            `yaml:"armor_exponent"`    // AC x (level / own level)^exponent
	Regular         AdventureSideCurve `yaml:"regular"`
	Boss            AdventureSideCurve `yaml:"boss"`
}

// AdventureSideCurve is what a monster authored on its curve has at a level.
type AdventureSideCurve struct {
	HP  LevelCurve `yaml:"hp"`
	Hit LevelCurve `yaml:"hit"` // the average of damage_min and damage_max
	// ExperienceLevelPercent is a kill's XP as a percent of the XP a hero of
	// the party's level (visit level - bonus) needs for the next level.
	ExperienceLevelPercent int `yaml:"experience_level_percent"`
}

// LevelCurve grows exponentially piece by piece, then by FlatPerLevel past
// the last piece: exp(base + sum of slope x levels in each piece) + flat.
type LevelCurve struct {
	Base         float64      `yaml:"base"`
	Growth       []CurvePiece `yaml:"growth"`
	FlatPerLevel float64      `yaml:"flat_per_level"`
}

// CurvePiece is exponential growth by Slope per level up to ToLevel.
type CurvePiece struct {
	ToLevel int     `yaml:"to_level"`
	Slope   float64 `yaml:"slope"`
}

func (c LevelCurve) At(level int) float64 {
	exponent, from := c.Base, 0
	for _, p := range c.Growth {
		exponent += p.Slope * float64(max(0, min(level, p.ToLevel)-from))
		from = p.ToLevel
	}
	return math.Exp(exponent) + c.FlatPerLevel*float64(max(0, level-from))
}

// FlatFrom is the level past which the curve grows by FlatPerLevel.
func (c LevelCurve) FlatFrom() int {
	if len(c.Growth) == 0 {
		return 0
	}
	return c.Growth[len(c.Growth)-1].ToLevel
}

// Side is the curve set of a boss or a regular monster.
func (s AdventureScalingConfig) Side(boss bool) AdventureSideCurve {
	if boss {
		return s.Boss
	}
	return s.Regular
}

// adventureCurveMaxLevel is how far Validate evaluates every curve.
const adventureCurveMaxLevel = 200

func (s AdventureScalingConfig) Validate() error {
	if s.VisitLevelBonus < 0 || !finite(s.ArmorExponent) || s.ArmorExponent < 0 {
		return fmt.Errorf("adventure_scaling: visit_level_bonus and armor_exponent must be >= 0")
	}
	for _, side := range []struct {
		name string
		c    AdventureSideCurve
	}{{"regular", s.Regular}, {"boss", s.Boss}} {
		if side.c.ExperienceLevelPercent < 1 {
			return fmt.Errorf("adventure_scaling.%s: experience_level_percent must be >= 1", side.name)
		}
		for _, curve := range []struct {
			name string
			c    LevelCurve
		}{{"hp", side.c.HP}, {"hit", side.c.Hit}} {
			if err := curve.c.validate(); err != nil {
				return fmt.Errorf("adventure_scaling.%s.%s: %w", side.name, curve.name, err)
			}
		}
	}
	return nil
}

// validate keeps a curve rising: pieces in level order, no negative growth,
// some growth somewhere, and a finite positive value at every level it can be
// asked for.
func (c LevelCurve) validate() error {
	rising, from := c.FlatPerLevel > 0, 0
	for _, p := range c.Growth {
		if p.ToLevel <= from || !finite(p.Slope) || p.Slope < 0 {
			return fmt.Errorf("growth pieces need rising to_level and slope >= 0")
		}
		rising, from = rising || p.Slope > 0, p.ToLevel
	}
	if !finite(c.Base) || !finite(c.FlatPerLevel) || c.FlatPerLevel < 0 || !rising {
		return fmt.Errorf("needs a finite base, a positive slope or flat_per_level, and flat_per_level >= 0")
	}
	previous := 0.0
	for level := 1; level <= adventureCurveMaxLevel; level++ {
		v := c.At(level)
		if !finite(v) || v <= 0 || v < previous {
			return fmt.Errorf("curve(L%d) = %g: must stay finite, positive and rising to L%d", level, v, adventureCurveMaxLevel)
		}
		previous = v
	}
	return nil
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Destinations are the maps the schedule opens; every visit auto-levels them.
func (s *AdventureSchedule) Destinations() []string {
	var out []string
	for _, day := range s.Days {
		for _, key := range day {
			if !slices.Contains(out, key) {
				out = append(out, key)
			}
		}
	}
	return out
}
