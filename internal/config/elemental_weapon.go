package config

import (
	"fmt"
	"math"
)

// ElementalWeaponAbility authors a direct-hit signature. Marks are shared by
// the party; secondary damage never triggers another weapon rider.
type ElementalWeaponAbility struct {
	Kind            string  `yaml:"kind"`
	Damage          int     `yaml:"damage"`
	DurationSeconds int     `yaml:"duration_seconds"`
	Hits            int     `yaml:"hits,omitempty"`
	RangeTiles      float64 `yaml:"range_tiles,omitempty"`
	ScalingStat     string  `yaml:"scaling_stat,omitempty"`
	StatDivisor     int     `yaml:"stat_divisor,omitempty"`
	MaxStacks       int     `yaml:"max_stacks,omitempty"`
	HitsPerTarget   int     `yaml:"hits_per_target,omitempty"`
}

func (a *ElementalWeaponAbility) validate(w *WeaponDefinitionConfig) error {
	if a == nil {
		return nil
	}
	school := map[string]string{"pressure": "fire", "backwash": "water", "anchor": "earth", "crosswind": "air"}[a.Kind]
	if school == "" || w.DamageType != school {
		return fmt.Errorf("elemental ability %q requires its matching damage school", a.Kind)
	}
	if a.Damage <= 0 || math.IsNaN(a.RangeTiles) || math.IsInf(a.RangeTiles, 0) {
		return fmt.Errorf("elemental ability requires positive damage and finite range")
	}
	if a.Kind == "backwash" {
		if a.DurationSeconds != 0 || a.HitsPerTarget < 1 || a.HitsPerTarget > 10 {
			return fmt.Errorf("backwash requires hits_per_target (1..10) and no mark duration")
		}
	} else if a.DurationSeconds < 1 || a.DurationSeconds > 60 || a.HitsPerTarget != 0 {
		return fmt.Errorf("marks require 1..60 second duration and no hits_per_target")
	}
	counted := a.Kind == "pressure" || a.Kind == "crosswind"
	if (counted && (a.Hits < 2 || a.Hits > 10)) || (!counted && a.Hits != 0) {
		return fmt.Errorf("only pressure and crosswind author hits (2..10)")
	}
	if (a.Kind == "anchor" && (a.MaxStacks < 1 || a.MaxStacks > 10)) || (a.Kind != "anchor" && a.MaxStacks != 0) {
		return fmt.Errorf("only anchor authors max_stacks (1..10)")
	}
	if a.Kind == "backwash" || a.Kind == "crosswind" {
		if a.RangeTiles <= 0 || a.RangeTiles > 6 {
			return fmt.Errorf("backwash and crosswind require range_tiles in (0,6]")
		}
	} else if a.RangeTiles != 0 {
		return fmt.Errorf("only backwash and crosswind author range_tiles")
	}
	scalingStat := map[string]string{"pressure": "Speed", "backwash": "Intellect"}[a.Kind]
	if a.ScalingStat != scalingStat || (scalingStat != "" && a.StatDivisor <= 0) || (scalingStat == "" && a.StatDivisor != 0) {
		return fmt.Errorf("%s requires scaling_stat %q and a positive divisor only when scaling", a.Kind, scalingStat)
	}
	if a.Kind == "backwash" {
		if w.Physics == nil || w.ProjectileSchool != school {
			return fmt.Errorf("backwash requires a water projectile weapon")
		}
	} else if w.Melee == nil || w.Physics != nil {
		return fmt.Errorf("%s requires a melee weapon", a.Kind)
	}
	return nil
}

func (a *ElementalWeaponAbility) Lines() []string {
	if a == nil {
		return nil
	}
	duration := weaponStatusDurationLabel(a.DurationSeconds)
	var effect string
	switch a.Kind {
	case "pressure":
		effect = fmt.Sprintf("Pressure Vent: every %d hits on one target release %d + %s/%d extra fire damage.", a.Hits, a.Damage, a.ScalingStat, a.StatDivisor)
	case "backwash":
		return []string{fmt.Sprintf("Backwash: %d + %s/%d water damage; a charge jumps between foes less than %g tiles apart with clear sight, up to %d hits per foe.", a.Damage, a.ScalingStat, a.StatDivisor, a.RangeTiles, a.HitsPerTarget)}
	case "anchor":
		effect = fmt.Sprintf("Stored Load: each hit adds a stack, then deals %d extra earth damage per stack on that target (maximum %d stacks).", a.Damage, a.MaxStacks)
	case "crosswind":
		effect = fmt.Sprintf("Transfer: every %d hits on one target deal %d extra air damage and push it up to %g tiles away, stopping at obstacles.", a.Hits, a.Damage, a.RangeTiles)
	}
	return []string{effect, "Marks shared by party; refreshed on hit: " + duration + "."}
}
