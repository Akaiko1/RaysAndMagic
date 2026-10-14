package monster

import (
	"fmt"
	"reflect"
	"strings"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
)

// Abilities are a monster's authored combat abilities: the monsters.yaml
// `abilities:` map, one entry per kind. A kind left out stays zero, which is
// absent. The loader decodes strictly, so an unknown kind or field fails the
// load. abilityRows is the one list behind validation, the ability lines and
// AbilityKinds: adding an ability means a field here, a row there and its
// combat consumer.
type Abilities struct {
	Telegraph    *TelegraphedAttack  `yaml:"telegraph,omitempty"`
	RootParty    TimedStatusAbility  `yaml:"root_party,omitempty"`
	RearBlink    RearBlinkAbility    `yaml:"rear_blink,omitempty"`
	TrapVolley   TrapVolleyAbility   `yaml:"trap_volley,omitempty"`
	Pounce       PounceAbility       `yaml:"pounce,omitempty"`
	PhaseShift   PhaseShiftAbility   `yaml:"phase_shift,omitempty"`
	Poison       DoTAbility          `yaml:"poison,omitempty"`
	Ignite       DoTAbility          `yaml:"ignite,omitempty"`
	Stun         TimedStatusAbility  `yaml:"stun,omitempty"`
	Dispel       ChanceAbility       `yaml:"dispel,omitempty"`
	MeltArmor    ChanceAbility       `yaml:"melt_armor,omitempty"`
	Fireburst    FireburstAbility    `yaml:"fireburst,omitempty"`
	DragonBreath DragonBreathAbility `yaml:"dragon_breath,omitempty"`
	PiercingShot PiercingShotAbility `yaml:"piercing_shot,omitempty"`
	AllyHeal     AllyHealAbility     `yaml:"ally_heal,omitempty"`
	Inferno      InfernoAbility      `yaml:"inferno,omitempty"`
	Teleport     TeleportAbility     `yaml:"teleport,omitempty"`
	Summon       SummonAbility       `yaml:"summon,omitempty"`
	Enrage       EnrageAbility       `yaml:"enrage,omitempty"`
}

// ChanceAbility is a bare on-hit roll (dispel, melt_armor).
type ChanceAbility struct {
	Chance float64 `yaml:"chance"`
}

// DoTAbility is an on-hit damage-over-time rider (poison, ignite).
type DoTAbility struct {
	Chance          float64 `yaml:"chance"`
	DurationSeconds int     `yaml:"duration_seconds"`
}

// TimedStatusAbility is a control rider (stun, root_party); its seconds are
// whole rounds, the same span in both modes.
type TimedStatusAbility struct {
	Chance  float64 `yaml:"chance"`
	Seconds int     `yaml:"seconds"`
}

// RearBlinkAbility: a ranged attacker blinks behind the party and strikes.
type RearBlinkAbility struct {
	Chance     float64 `yaml:"chance"`
	RangeTiles float64 `yaml:"range_tiles"`
}

// TrapVolleyAbility: the boss sows fire traps around itself on a cadence.
type TrapVolleyAbility struct {
	Count           int     `yaml:"count"`
	RadiusTiles     float64 `yaml:"radius_tiles"`
	IntervalSeconds float64 `yaml:"interval_seconds"` // RT cadence
	IntervalTurns   int     `yaml:"interval_turns"`   // TB cadence
	Damage          int     `yaml:"damage"`
}

// PounceAbility: from within range (beyond melee) the monster leaps to melee
// and attacks; the cooldown throttles repeats.
type PounceAbility struct {
	RangeTiles      float64 `yaml:"range_tiles"`
	CooldownSeconds float64 `yaml:"cooldown_seconds"`
}

// PhaseShiftAbility: a walker blinks up to RangeTiles straight along a row or
// column across a chasm or water, onto the first walkable tile beyond it,
// when it has no walking route to the party or the detour is far longer.
// Walls, doors and exits block the blink. Planning it is expensive
// (phase_shift.go): author it for monsters of small dungeon maps.
type PhaseShiftAbility struct {
	RangeTiles      int `yaml:"range_tiles"`
	CooldownSeconds int `yaml:"cooldown_seconds"` // whole rounds, the same span in both modes
}

// FireburstAbility: a fire blast instead of the normal attack.
type FireburstAbility struct {
	Chance    float64 `yaml:"chance"`
	DamageMin int     `yaml:"damage_min"`
	DamageMax int     `yaml:"damage_max"`
}

// DragonBreathAbility: the attack hits every living party member in a school.
type DragonBreathAbility struct {
	Chance     float64 `yaml:"chance"`
	DamageType string  `yaml:"damage_type"`
}

// PiercingShotAbility: an armor-piercing shot at several party members.
type PiercingShotAbility struct {
	Chance  float64 `yaml:"chance"`
	Targets int     `yaml:"targets"`
}

// AllyHealAbility: heals itself or a nearby ally instead of attacking.
type AllyHealAbility struct {
	Chance      float64 `yaml:"chance"`
	Amount      int     `yaml:"amount"`
	RadiusTiles float64 `yaml:"radius_tiles"`
}

// InfernoAbility: a boss's party-wide fire nova.
type InfernoAbility struct {
	Chance     float64 `yaml:"chance"`      // per action
	Damage     int     `yaml:"damage"`      // fire damage, pre-mitigation
	RangeTiles float64 `yaml:"range_tiles"` // the nova's reach; melee reach can still replace a hit with it
}

// TeleportAbility: at or below AtHP the boss may blink to a random tile.
type TeleportAbility struct {
	AtHP   int     `yaml:"at_hp"`
	Chance float64 `yaml:"chance"` // per action
}

// SummonAbility: on its action the boss may rally adds.
type SummonAbility struct {
	Chance          float64  `yaml:"chance"`           // per action
	FirstGuaranteed bool     `yaml:"first_guaranteed"` // the first summon ignores Chance; refills use it
	Monsters        []string `yaml:"monsters"`         // monster keys to pick from
	Count           int      `yaml:"count"`            // adds per summon (default 1)
	Max             int      `yaml:"max"`              // cap on live summons (0 = uncapped)
}

// EnrageAbility: at or below AtHP the boss hits harder and/or faster.
type EnrageAbility struct {
	AtHP         int     `yaml:"at_hp"`
	DamageMult   float64 `yaml:"damage_mult"`
	CooldownMult float64 `yaml:"cooldown_mult"`
}

// abilityRow is one ability kind: whether this monster authors it, whether it
// needs boss: true, its load problems and its description lines.
type abilityRow struct {
	kind     string
	present  bool
	bossOnly bool
	problems func() []string
	lines    func() []EffectLine
}

// abilityRows lists every ability kind in display order.
func (d MonsterDefinition) abilityRows() []abilityRow {
	a := d.Abilities
	line := func(text string) []EffectLine { return []EffectLine{{Text: text}} }
	schoolLine := func(school, text string) []EffectLine {
		return []EffectLine{{Text: text, School: normalizeEffectSchool(school)}}
	}
	need := func(ok bool, problem string) []string {
		if ok {
			return nil
		}
		return []string{problem}
	}
	chance := func(kind string, c float64) []string {
		return need(c > 0 && c <= 1, kind+" needs a chance in (0,1]")
	}
	duration := func(kind string, seconds int) []string {
		if err := config.ValidateEffectSeconds(kind+" seconds", seconds); err != nil {
			return []string{err.Error()}
		}
		return nil
	}
	// An ability is authored when any of its fields differs from zero.
	set := func(ability any) bool { return !reflect.ValueOf(ability).IsZero() }
	return []abilityRow{
		{
			kind: "telegraph", present: a.Telegraph != nil,
			problems: func() []string {
				if err := a.Telegraph.validate(); err != nil {
					return []string{err.Error()}
				}
				return nil
			},
			lines: func() []EffectLine { return a.Telegraph.lines(d) },
		},
		{
			kind: "root_party", present: set(a.RootParty),
			problems: func() []string {
				return append(chance("root_party", a.RootParty.Chance), duration("root_party", a.RootParty.Seconds)...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Party root: %.0f%% per attack, %s", a.RootParty.Chance*100, config.DurationText(a.RootParty.Seconds)))
			},
		},
		{
			kind: "rear_blink", present: set(a.RearBlink),
			problems: func() []string {
				return append(chance("rear_blink", a.RearBlink.Chance),
					need(a.RearBlink.RangeTiles >= 2 && a.RearBlink.RangeTiles <= d.RangedAttackRange && (d.ProjectileSpell != "" || d.ProjectileWeapon != ""),
						"rear_blink needs a ranged attack and range_tiles from 2 up to its range")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Rear blink and strike: %.0f%% per attack, %.0f tiles", a.RearBlink.Chance*100, a.RearBlink.RangeTiles))
			},
		},
		{
			kind: "trap_volley", bossOnly: true,
			present: set(a.TrapVolley),
			problems: func() []string {
				t := a.TrapVolley
				return need(t.Count > 0 && t.RadiusTiles > 0 && t.Damage > 0 && t.IntervalSeconds > 0 && t.IntervalTurns > 0,
					"trap_volley needs positive count, radius_tiles, damage, interval_seconds and interval_turns")
			},
			lines: func() []EffectLine {
				t := a.TrapVolley
				return schoolLine(damagecalc.Fire.String(), fmt.Sprintf("Trap field: sows %d fire traps (%.0f dmg) within %.0f tiles every %.0fs / %d turns",
					t.Count, float64(t.Damage), t.RadiusTiles, t.IntervalSeconds, t.IntervalTurns))
			},
		},
		{
			kind: "pounce", present: set(a.Pounce),
			problems: func() []string { return need(a.Pounce.RangeTiles > 0, "pounce needs range_tiles") },
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Pounce: %.1f tiles every %.0fs", a.Pounce.RangeTiles, a.Pounce.CooldownSeconds))
			},
		},
		{
			kind: "phase_shift", present: set(a.PhaseShift),
			problems: func() []string {
				problems := need(a.PhaseShift.RangeTiles >= 2 && a.PhaseShift.RangeTiles <= MaxPhaseShiftRangeTiles,
					fmt.Sprintf("phase_shift needs range_tiles from 2 to %d", MaxPhaseShiftRangeTiles))
				problems = append(problems, need(!d.Flying, "phase_shift is for walkers: a flying monster already crosses gaps")...)
				return append(problems, duration("phase_shift cooldown", a.PhaseShift.CooldownSeconds)...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Phase Shift: crosses chasms and water, %d tiles, every %s", a.PhaseShift.RangeTiles, config.DurationText(a.PhaseShift.CooldownSeconds)))
			},
		},
		{
			kind: "poison", present: set(a.Poison),
			problems: func() []string {
				return append(chance("poison", a.Poison.Chance), duration("poison", a.Poison.DurationSeconds)...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Poison: %.0f%% for %s", a.Poison.Chance*100, config.DurationText(a.Poison.DurationSeconds)))
			},
		},
		{
			kind: "ignite", present: set(a.Ignite),
			problems: func() []string {
				return append(chance("ignite", a.Ignite.Chance), duration("ignite", a.Ignite.DurationSeconds)...)
			},
			lines: func() []EffectLine {
				return schoolLine(damagecalc.Fire.String(), fmt.Sprintf("Ignite: %.0f%% for %s", a.Ignite.Chance*100, config.DurationText(a.Ignite.DurationSeconds)))
			},
		},
		{
			kind: "stun", present: set(a.Stun),
			problems: func() []string {
				return append(chance("stun", a.Stun.Chance), duration("stun", a.Stun.Seconds)...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Stun: %.0f%% for %s", a.Stun.Chance*100, config.DurationText(a.Stun.Seconds)))
			},
		},
		{
			kind: "dispel", present: set(a.Dispel),
			problems: func() []string { return chance("dispel", a.Dispel.Chance) },
			lines:    func() []EffectLine { return line(fmt.Sprintf("Dispel buff: %.0f%%", a.Dispel.Chance*100)) },
		},
		{
			kind: "melt_armor", present: set(a.MeltArmor),
			problems: func() []string { return chance("melt_armor", a.MeltArmor.Chance) },
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Melt armor: %.0f%% on hit: armor reduces physical damage by at most %d%% for %s",
					a.MeltArmor.Chance*100, config.ArmorElementalMitigationCap, config.DurationText(config.ArmorMeltSeconds())))
			},
		},
		{
			kind: "fireburst", present: set(a.Fireburst),
			problems: func() []string {
				return append(chance("fireburst", a.Fireburst.Chance),
					need(a.Fireburst.DamageMin > 0 && a.Fireburst.DamageMax >= a.Fireburst.DamageMin, "fireburst needs damage_min/damage_max (max >= min > 0)")...)
			},
			lines: func() []EffectLine {
				return schoolLine(damagecalc.Fire.String(), fmt.Sprintf("Fireburst: %.0f%% for %d-%d to whole party", a.Fireburst.Chance*100, a.Fireburst.DamageMin, a.Fireburst.DamageMax))
			},
		},
		{
			kind: "dragon_breath", present: set(a.DragonBreath),
			problems: func() []string {
				problems := chance("dragon_breath", a.DragonBreath.Chance)
				if _, err := ParseDamageType(strings.ToLower(strings.TrimSpace(a.DragonBreath.DamageType))); err != nil {
					problems = append(problems, fmt.Sprintf("dragon_breath needs a known damage_type, got %q", a.DragonBreath.DamageType))
				}
				return problems
			},
			lines: func() []EffectLine {
				school := normalizeEffectSchool(a.DragonBreath.DamageType)
				return schoolLine(school, fmt.Sprintf("Dragon Breath: %.0f%% %s attack to whole party", a.DragonBreath.Chance*100, school))
			},
		},
		{
			kind: "piercing_shot", present: set(a.PiercingShot),
			problems: func() []string {
				return append(chance("piercing_shot", a.PiercingShot.Chance), need(a.PiercingShot.Targets > 0, "piercing_shot needs targets")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Piercing shot: %.0f%% (%d targets)", a.PiercingShot.Chance*100, a.PiercingShot.Targets))
			},
		},
		{
			kind: "ally_heal", present: set(a.AllyHeal),
			problems: func() []string {
				return append(chance("ally_heal", a.AllyHeal.Chance),
					need(a.AllyHeal.Amount > 0 && a.AllyHeal.RadiusTiles > 0, "ally_heal needs amount and radius_tiles")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Heals allies: %.0f%% for %d (%.0f tiles)", a.AllyHeal.Chance*100, a.AllyHeal.Amount, a.AllyHeal.RadiusTiles))
			},
		},
		{
			// A nova with no authored reach would silently become map-wide in TB
			// (it used to): the radius is explicit content, like every range.
			kind: "inferno", bossOnly: true, present: set(a.Inferno),
			problems: func() []string {
				return append(chance("inferno", a.Inferno.Chance),
					need(a.Inferno.Damage > 0 && a.Inferno.RangeTiles > 0, "inferno needs damage and range_tiles")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Inferno nova: %.0f%% for %d fire damage (%.1f tiles)", a.Inferno.Chance*100, a.Inferno.Damage, a.Inferno.RangeTiles))
			},
		},
		{
			kind: "teleport", bossOnly: true, present: set(a.Teleport),
			problems: func() []string {
				return append(chance("teleport", a.Teleport.Chance), need(a.Teleport.AtHP > 0, "teleport needs at_hp")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Blinks at/below %d HP (%.0f%%)", a.Teleport.AtHP, a.Teleport.Chance*100))
			},
		},
		{
			kind: "summon", bossOnly: true,
			present: set(a.Summon),
			problems: func() []string {
				// first_guaranteed with chance 0 is a one-shot summon.
				rate := need(a.Summon.Chance >= 0 && a.Summon.Chance <= 1 && (a.Summon.Chance > 0 || a.Summon.FirstGuaranteed),
					"summon needs a chance in (0,1], or first_guaranteed for a one-shot summon")
				return append(rate, need(len(a.Summon.Monsters) > 0, "summon needs monsters")...)
			},
			lines: func() []EffectLine { return line(a.Summon.line()) },
		},
		{
			kind: "enrage", bossOnly: true, present: set(a.Enrage),
			problems: func() []string {
				return append(need(a.Enrage.AtHP > 0, "enrage needs at_hp"),
					need(a.Enrage.DamageMult > 0 || a.Enrage.CooldownMult > 0, "enrage needs damage_mult or cooldown_mult")...)
			},
			lines: func() []EffectLine {
				return line(fmt.Sprintf("Enrages at/below %d HP: dmg x%.1f, cd x%.1f", a.Enrage.AtHP, effectiveMultiplier(a.Enrage.DamageMult), effectiveMultiplier(a.Enrage.CooldownMult)))
			},
		},
	}
}

func (s SummonAbility) line() string {
	n := s.Count
	if n == 0 {
		n = 1
	}
	capText := "no live summon cap"
	if s.Max > 0 {
		capText = fmt.Sprintf("max %d alive", s.Max)
	}
	if s.FirstGuaranteed {
		return fmt.Sprintf("Summons %dx {%s}: first guaranteed, then %.0f%% (%s)", n, strings.Join(s.Monsters, ", "), s.Chance*100, capText)
	}
	return fmt.Sprintf("Summons %dx {%s}: %.0f%% (%s)", n, strings.Join(s.Monsters, ", "), s.Chance*100, capText)
}

// AbilityKinds lists the YAML kinds of the abilities this monster authors, in
// display order.
func (d MonsterDefinition) AbilityKinds() []string {
	var kinds []string
	for _, row := range d.abilityRows() {
		if row.present {
			kinds = append(kinds, row.kind)
		}
	}
	return kinds
}

// abilityProblems is every load problem of the authored abilities.
func (d MonsterDefinition) abilityProblems() []string {
	var out []string
	for _, row := range d.abilityRows() {
		if !row.present {
			continue
		}
		if row.bossOnly && !d.Boss {
			out = append(out, row.kind+" is a boss ability and needs boss: true")
		}
		out = append(out, row.problems()...)
	}
	return out
}

// abilityLines describes the authored abilities, in display order.
func (d MonsterDefinition) abilityLines() []EffectLine {
	var out []EffectLine
	for _, row := range d.abilityRows() {
		if row.present {
			out = append(out, row.lines()...)
		}
	}
	return out
}
