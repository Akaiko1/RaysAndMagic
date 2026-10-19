package config

import (
	"fmt"
	"math"
	"strconv"

	uitext "ugataima/assets/text"
)

// StatScaledDamage is a flat amount plus one effective stat divided:
// damage + scaling_stat/stat_divisor. Without a divisor it is flat. Every
// weapon rider resolves and prints it the same way.
type StatScaledDamage struct {
	Damage      int    `yaml:"damage"`
	ScalingStat string `yaml:"scaling_stat,omitempty"`
	StatDivisor int    `yaml:"stat_divisor,omitempty"`
}

// Text is the formula as cards print it: "12 + Speed/4", or "12" when flat.
func (s StatScaledDamage) Text() string {
	if s.StatDivisor <= 0 {
		return strconv.Itoa(s.Damage)
	}
	return fmt.Sprintf("%d + %s/%d", s.Damage, s.ScalingStat, s.StatDivisor)
}

// ElementalAbilityKind is the closed set of elemental weapon abilities.
type ElementalAbilityKind string

const (
	AbilityPressure  ElementalAbilityKind = "pressure"
	AbilityBackwash  ElementalAbilityKind = "backwash"
	AbilityAnchor    ElementalAbilityKind = "anchor"
	AbilityCrosswind ElementalAbilityKind = "crosswind"
)

// ElementalWeaponAbility authors a direct-hit signature. Marks are shared by
// the party; secondary damage never triggers another weapon rider.
type ElementalWeaponAbility struct {
	StatScaledDamage `yaml:",inline"`
	Kind             ElementalAbilityKind `yaml:"kind"`
	DurationSeconds  int                  `yaml:"duration_seconds"`
	Hits             int                  `yaml:"hits,omitempty"`
	RangeTiles       float64              `yaml:"range_tiles,omitempty"`
	MaxStacks        int                  `yaml:"max_stacks,omitempty"`
	HitsPerTarget    int                  `yaml:"hits_per_target,omitempty"`
}

func (a *ElementalWeaponAbility) validate(w *WeaponDefinitionConfig) error {
	if a == nil {
		return nil
	}
	school := map[ElementalAbilityKind]string{AbilityPressure: "fire", AbilityBackwash: "water", AbilityAnchor: "earth", AbilityCrosswind: "air"}[a.Kind]
	if school == "" || w.DamageType != school {
		return fmt.Errorf("elemental ability %q requires its matching damage school", a.Kind)
	}
	if a.Damage <= 0 || math.IsNaN(a.RangeTiles) || math.IsInf(a.RangeTiles, 0) {
		return fmt.Errorf("elemental ability requires positive damage and finite range")
	}
	if a.Kind == AbilityBackwash {
		if a.DurationSeconds != 0 || a.HitsPerTarget < 1 || a.HitsPerTarget > 10 {
			return fmt.Errorf("backwash requires hits_per_target (1..10) and no mark duration")
		}
	} else if a.DurationSeconds > 60 || a.HitsPerTarget != 0 {
		return fmt.Errorf("marks require up to a 60 second duration and no hits_per_target")
	} else if err := ValidateEffectSeconds("mark duration_seconds", a.DurationSeconds); err != nil {
		return err
	}
	counted := a.Kind == AbilityPressure || a.Kind == AbilityCrosswind
	if (counted && (a.Hits < 2 || a.Hits > 10)) || (!counted && a.Hits != 0) {
		return fmt.Errorf("only pressure and crosswind author hits (2..10)")
	}
	if (a.Kind == AbilityAnchor && (a.MaxStacks < 1 || a.MaxStacks > 10)) || (a.Kind != AbilityAnchor && a.MaxStacks != 0) {
		return fmt.Errorf("only anchor authors max_stacks (1..10)")
	}
	if a.Kind == AbilityBackwash || a.Kind == AbilityCrosswind {
		if a.RangeTiles <= 0 || a.RangeTiles > 6 {
			return fmt.Errorf("backwash and crosswind require range_tiles in (0,6]")
		}
	} else if a.RangeTiles != 0 {
		return fmt.Errorf("only backwash and crosswind author range_tiles")
	}
	scalingStat := map[ElementalAbilityKind]string{AbilityPressure: "Speed", AbilityBackwash: "Intellect"}[a.Kind]
	if a.ScalingStat != scalingStat || (scalingStat != "" && a.StatDivisor <= 0) || (scalingStat == "" && a.StatDivisor != 0) {
		return fmt.Errorf("%s requires scaling_stat %q and a positive divisor only when scaling", a.Kind, scalingStat)
	}
	if a.Kind == AbilityBackwash {
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
	var effect string
	switch a.Kind {
	case AbilityPressure:
		effect = uitext.Text("weapon.pressure_vent", a.Hits, a.Text())
	case AbilityBackwash:
		return []string{uitext.Text("weapon.backwash", a.Text(), NumberText(a.RangeTiles), a.HitsPerTarget)}
	case AbilityAnchor:
		effect = uitext.Text("weapon.stored_load", a.Damage, a.MaxStacks)
	case AbilityCrosswind:
		effect = uitext.Text("weapon.transfer", a.Hits, a.Damage, NumberText(a.RangeTiles))
	}
	return []string{effect, uitext.Text("weapon.marks_shared", DurationText(a.DurationSeconds))}
}

// SignatureKind is the closed set of weapon signatures.
type SignatureKind string

const (
	SignatureSolarFlare    SignatureKind = "solar_flare"
	SignatureRuneDischarge SignatureKind = "rune_discharge"
	SignatureReturning     SignatureKind = "returning"
	SignatureSkirmish      SignatureKind = "skirmish"
	SignatureWindUp        SignatureKind = "wind_up"
)

// WeaponSignatureConfig is a weapon's signature rider. Solar Flare and Rune
// Discharge deal StatScaledDamage in School.
type WeaponSignatureConfig struct {
	StatScaledDamage      `yaml:",inline"`
	Kind                  SignatureKind `yaml:"kind"`
	School                string        `yaml:"school,omitempty"`
	SPCost                int           `yaml:"sp_cost,omitempty"`
	ChargeLimit           int           `yaml:"charge_limit,omitempty"`
	ChargeMoveTiles       float64       `yaml:"charge_move_tiles,omitempty"` // a charge per this many tiles the party moves, any way
	ChargeOnOffensiveCast bool          `yaml:"charge_on_offensive_cast,omitempty"`
	LaneBehindTiles       float64       `yaml:"lane_behind_tiles,omitempty"`
	LaneWidthTiles        float64       `yaml:"lane_width_tiles,omitempty"`
	BurnSeconds           int           `yaml:"burn_seconds,omitempty"`
	ReturnDamagePercent   int           `yaml:"return_damage_percent,omitempty"`
}

// HasSignature reports a weapon whose signature is kind.
func (w *WeaponDefinitionConfig) HasSignature(kind SignatureKind) bool {
	return w != nil && w.Signature != nil && w.Signature.Kind == kind
}

// ChargesByMovement reports a weapon whose signature stores charges earned by
// party movement (Solar Flare, Skirmish).
func (w *WeaponDefinitionConfig) ChargesByMovement() bool {
	return w.HasSignature(SignatureSolarFlare) || w.HasSignature(SignatureSkirmish)
}

// StoresCharges reports a weapon whose signature keeps charges on the item:
// the movement-charged ones and Wind-up, which winds a spring per round.
func (w *WeaponDefinitionConfig) StoresCharges() bool {
	return w.ChargesByMovement() || w.HasSignature(SignatureWindUp)
}

// TileCountText prints a tile distance with its noun: "1 tile", "2 tiles".
func TileCountText(tiles float64) string {
	if tiles == 1 {
		return "1 tile"
	}
	return NumberText(tiles) + " tiles"
}

func positive(value float64) bool { return value > 0 && finite(value) }

// NumberText prints a number without padding decimals (2, 0.5, 1.25).
func NumberText(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func (s *WeaponSignatureConfig) validate(d *WeaponDefinitionConfig) error {
	if s == nil {
		return nil
	}
	unused := *s
	unused.Kind = ""
	switch s.Kind {
	case SignatureSolarFlare, SignatureRuneDischarge:
		if s.Damage <= 0 || !validWeaponBonusStats[s.ScalingStat] || s.StatDivisor <= 0 {
			return fmt.Errorf("signature requires positive damage/stat_divisor and a valid scaling_stat")
		}
		school, err := canonicalDamageSchool(s.School)
		if err != nil {
			return err
		}
		s.School = school
		unused.StatScaledDamage, unused.School = StatScaledDamage{}, ""
		if s.Kind == SignatureSolarFlare {
			if d.Category != "staff" || !d.IsRanged() || s.ChargeLimit <= 0 || !positive(s.ChargeMoveTiles) || !positive(s.LaneBehindTiles) || !positive(s.LaneWidthTiles) {
				return fmt.Errorf("solar_flare requires a ranged staff, charge limit, movement and lane geometry")
			}
			if err := ValidateEffectSeconds("solar_flare burn_seconds", s.BurnSeconds); err != nil {
				return err
			}
			unused.ChargeLimit, unused.ChargeMoveTiles, unused.LaneBehindTiles, unused.LaneWidthTiles, unused.BurnSeconds = 0, 0, 0, 0, 0
			unused.ChargeOnOffensiveCast = false
		} else if d.IsRanged() || s.SPCost <= 0 {
			return fmt.Errorf("rune_discharge requires a melee weapon and positive sp_cost")
		} else {
			unused.SPCost = 0
		}
	case SignatureReturning:
		if d.Category != "dagger" || !d.IsRanged() || !validPercent(s.ReturnDamagePercent) {
			return fmt.Errorf("returning requires a ranged dagger and return_damage_percent 1..100")
		}
		unused.ReturnDamagePercent = 0
	case SignatureSkirmish:
		if !d.IsRanged() || s.ChargeLimit <= 0 || !positive(s.ChargeMoveTiles) {
			return fmt.Errorf("skirmish requires a ranged weapon, charge limit and movement")
		}
		unused.ChargeLimit, unused.ChargeMoveTiles = 0, 0
	case SignatureWindUp:
		if !d.IsRanged() || s.ChargeLimit <= 0 {
			return fmt.Errorf("wind_up requires a ranged weapon and a charge limit")
		}
		unused.ChargeLimit = 0
	default:
		return fmt.Errorf("unknown weapon signature kind %q", s.Kind)
	}
	if unused != (WeaponSignatureConfig{}) {
		return fmt.Errorf("signature %q has fields belonging to a different kind", s.Kind)
	}
	return nil
}

// FlareLine is what a stored Solar Flare charge releases.
func (s *WeaponSignatureConfig) FlareLine() string {
	return uitext.Text("weapon.solar_flare", s.Text(), s.School, NumberText(s.LaneWidthTiles), NumberText(s.LaneBehindTiles), DurationText(s.BurnSeconds))
}

// StoredLine is a weapon card's count of the charges its item holds.
func (s *WeaponSignatureConfig) StoredLine(charges int) string {
	switch s.Kind {
	case SignatureSolarFlare:
		return uitext.Text("weapon.solar_stored", charges, s.ChargeLimit)
	case SignatureSkirmish:
		return uitext.Text("weapon.skirmish_stored", charges, s.ChargeLimit)
	case SignatureWindUp:
		return uitext.Text("weapon.wind_up_stored", charges, s.ChargeLimit)
	}
	return ""
}

func (s *WeaponSignatureConfig) Lines() []string {
	if s == nil {
		return nil
	}
	switch s.Kind {
	case SignatureSolarFlare:
		lines := []string{uitext.Text("weapon.solar_charge", uitext.Text("weapon.solar_flare_name"), TileCountText(s.ChargeMoveTiles), s.ChargeLimit)}
		if s.ChargeOnOffensiveCast {
			lines = append(lines, uitext.Text("weapon.solar_charge_spell"))
		}
		return append(lines, s.FlareLine())
	case SignatureRuneDischarge:
		return []string{uitext.Text("weapon.rune_discharge", uitext.Text("weapon.rune_discharge_name"), s.SPCost, s.Text(), s.School)}
	case SignatureReturning:
		return []string{uitext.Text("weapon.returning_gale", s.ReturnDamagePercent)}
	case SignatureSkirmish:
		return []string{uitext.Text("weapon.skirmish", uitext.Text("weapon.skirmish_name"), TileCountText(s.ChargeMoveTiles), s.ChargeLimit)}
	case SignatureWindUp:
		return []string{uitext.Text("weapon.wind_up", uitext.Text("weapon.wind_up_name"), s.ChargeLimit)}
	}
	return nil
}
