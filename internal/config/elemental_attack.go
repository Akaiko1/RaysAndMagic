package config

import (
	"fmt"
	"math"

	damagecalc "ugataima/internal/damage"
)

// ElementalAttackConfig is shared by monster combat and attack descriptions.
// A zero chance disables the proc, including in minimal test configurations.
type ElementalAttackConfig struct {
	Chance           float64 `yaml:"chance"`
	DamageMultiplier float64 `yaml:"damage_multiplier"`
}

type MonsterCombatConfig struct {
	ElementalAttack ElementalAttackConfig `yaml:"elemental_attack"`
}

type ElementalAttackFXConfig struct {
	DurationSeconds float64 `yaml:"duration_seconds"`
	RadiusTiles     float64 `yaml:"radius_tiles"`
	ParticleCount   int     `yaml:"particle_count"`
	MaxActive       int     `yaml:"max_active"`
}

func (c ElementalAttackConfig) Validate() error {
	if math.IsNaN(c.Chance) || math.IsInf(c.Chance, 0) || c.Chance < 0 || c.Chance > 1 {
		return fmt.Errorf("monster_combat.elemental_attack.chance must be in [0,1]")
	}
	if math.IsNaN(c.DamageMultiplier) || math.IsInf(c.DamageMultiplier, 0) || c.DamageMultiplier < 0 || (c.Chance > 0 && c.DamageMultiplier <= 0) {
		return fmt.Errorf("monster_combat.elemental_attack.damage_multiplier must be finite and positive when enabled")
	}
	return nil
}

func (c ElementalAttackConfig) Scale(parts damagecalc.Parts) damagecalc.Parts {
	return damagecalc.Parts{Normal: int(float64(parts.Normal) * c.DamageMultiplier), True: int(float64(parts.True) * c.DamageMultiplier)}
}

func (c ElementalAttackFXConfig) Validate() error {
	if math.IsNaN(c.DurationSeconds) || math.IsInf(c.DurationSeconds, 0) || c.DurationSeconds <= 0 ||
		math.IsNaN(c.RadiusTiles) || math.IsInf(c.RadiusTiles, 0) || c.RadiusTiles <= 0 || c.ParticleCount <= 0 || c.MaxActive <= 0 {
		return fmt.Errorf("graphics.elemental_attack requires finite positive duration_seconds, radius_tiles, particle_count and max_active")
	}
	return nil
}

func (c MapConfigs) ValidateElementalSchools(required bool) error {
	for name, biome := range c.Biomes {
		if biome.ElementalAttackSchool == "" && !required {
			continue
		}
		school, err := damagecalc.ParseType(biome.ElementalAttackSchool)
		if err != nil || school == damagecalc.Physical {
			return fmt.Errorf("biome %q requires a non-physical elemental_attack_school", name)
		}
	}
	return nil
}
