package monster

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	uitext "ugataima/assets/text"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"

	"gopkg.in/yaml.v3"
)

// Abilities are a monster's authored combat abilities: the monsters.yaml
// `abilities:` map, one entry per kind. A kind left out stays zero, which is
// absent. The loader decodes strictly, so an unknown kind or field fails the
// load. abilityRows is the one list behind validation, the ability lines and
// AbilityKinds: adding an ability means a field here, a row there and its
// combat consumer.
type Abilities struct {
	Telegraph    *TelegraphedAttack `yaml:"telegraph,omitempty"`
	RootParty    TimedStatusAbility `yaml:"root_party,omitempty"`
	RearBlink    RearBlinkAbility   `yaml:"rear_blink,omitempty"`
	TrapVolley   TrapVolleyAbility  `yaml:"trap_volley,omitempty"`
	Pounce       PounceAbility      `yaml:"pounce,omitempty"`
	PhaseShift   PhaseShiftAbility  `yaml:"phase_shift,omitempty"`
	Poison       DoTAbility         `yaml:"poison,omitempty"`
	Ignite       DoTAbility         `yaml:"ignite,omitempty"`
	Stun         TimedStatusAbility `yaml:"stun,omitempty"`
	Dispel       ChanceAbility      `yaml:"dispel,omitempty"`
	MeltArmor    ChanceAbility      `yaml:"melt_armor,omitempty"`
	PartyAttacks []PartyAttack      `yaml:"party_attacks,omitempty"`
	AllyHeal     AllyHealAbility    `yaml:"ally_heal,omitempty"`
	Teleport     TeleportAbility    `yaml:"teleport,omitempty"`
	Summon       SummonAbility      `yaml:"summon,omitempty"`
	Push         PushAbility        `yaml:"push,omitempty"`
	Enrage       EnrageAbility      `yaml:"enrage,omitempty"`
}

// Clone is a copy that shares no slice or pointer with a.
func (a Abilities) Clone() Abilities {
	a.Summon.Monsters = slices.Clone(a.Summon.Monsters)
	a.PartyAttacks = slices.Clone(a.PartyAttacks)
	if a.Telegraph != nil {
		t := *a.Telegraph
		a.Telegraph = &t
	}
	return a
}

// MaxAttackPercent caps an ability's damage authored as a percent of its
// monster's attack (a telegraph, a party attack).
const MaxAttackPercent = 200

// Riders is an area ability's authored `riders: true|false`: whether the
// monster's on-hit riders (poison, ignite, stun, dispel, melt armor) ride
// each member it strikes. Required, so no area attack carries them by an
// unwritten rule.
type Riders struct{ Set, On bool }

// CarryRiders is an authored Riders value.
func CarryRiders(on bool) Riders { return Riders{Set: true, On: on} }

func (r *Riders) UnmarshalYAML(node *yaml.Node) error {
	var on bool
	if err := node.Decode(&on); err != nil {
		return fmt.Errorf("riders must be true or false: %w", err)
	}
	*r = CarryRiders(on)
	return nil
}

func (r Riders) MarshalYAML() (any, error) { return r.On, nil }

// Carried reports riders: true.
func (r Riders) Carried() bool { return r.On }

// Authored reports the field was written.
func (r Riders) Authored() bool { return r.Set }

// ScaleDamageBand is an authored damage band moved by ratio: at least 1, the
// top never below the bottom.
func ScaleDamageBand(lo, hi int, ratio float64) (int, int) {
	lo = max(1, int(math.Round(float64(lo)*ratio)))
	return lo, max(lo, int(math.Round(float64(hi)*ratio)))
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
	Count       int     `yaml:"count"`
	RadiusTiles float64 `yaml:"radius_tiles"`
	// Timer exception: the interval is an authored RT/TB pair, not the
	// 1 round = 3 s conversion.
	IntervalSeconds float64 `yaml:"interval_seconds"` // RT cadence
	IntervalTurns   int     `yaml:"interval_turns"`   // TB cadence
	DamagePercent   int     `yaml:"damage_percent"`   // each trap's hit, percent of the attack
	Riders          Riders  `yaml:"riders"`
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

// AllyHealAbility: heals itself or a nearby ally instead of attacking.
type AllyHealAbility struct {
	Chance      float64 `yaml:"chance"`
	Amount      int     `yaml:"amount"`
	RadiusTiles float64 `yaml:"radius_tiles"`
}

// TeleportAbility: at or below AtHP the boss may blink to a random tile.
type TeleportAbility struct {
	AtHP   int     `yaml:"at_hp"`
	Chance float64 `yaml:"chance"` // per action
}

// SummonAbility: the boss rallies adds, on a roll (chance, first_guaranteed)
// at its action or on a timer (cooldown_seconds).
type SummonAbility struct {
	Chance          float64  `yaml:"chance"`           // per action
	FirstGuaranteed bool     `yaml:"first_guaranteed"` // the first summon ignores Chance; refills use it
	CooldownSeconds int      `yaml:"cooldown_seconds"` // a sure summon each time this timer runs out, in place of the roll
	Monsters        []string `yaml:"monsters"`         // monster keys to pick from
	Count           int      `yaml:"count"`            // adds per summon (default 1)
	Max             int      `yaml:"max"`              // cap on live summons (0 = uncapped)
}

// PushAbility: once its cooldown is over, a chamber boss's next landed attack
// also shoves the party one tile toward the nearest lane of its current
// phase. It is free: the boss still attacks.
type PushAbility struct {
	CooldownSeconds int `yaml:"cooldown_seconds"` // whole rounds, the same span in both modes
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
				return line(uitext.Text("monster.ability.root_party", a.RootParty.Chance*100, config.DurationText(a.RootParty.Seconds)))
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
				return line(uitext.Text("monster.ability.rear_blink", a.RearBlink.Chance*100, a.RearBlink.RangeTiles))
			},
		},
		{
			kind: "trap_volley", bossOnly: true,
			present: set(a.TrapVolley),
			problems: func() []string {
				t := a.TrapVolley
				return need(t.Count > 0 && t.RadiusTiles > 0 && t.DamagePercent > 0 && t.DamagePercent <= MaxAttackPercent && t.IntervalSeconds > 0 && t.IntervalTurns > 0 && t.Riders.Authored(),
					"trap_volley needs positive count, radius_tiles, interval_seconds, interval_turns, damage_percent in 1..200 and riders: true or false")
			},
			lines: func() []EffectLine {
				t := a.TrapVolley
				return schoolLine(damagecalc.Fire.String(), uitext.Text("monster.ability.trap_volley",
					t.Count, t.DamagePercent, t.RadiusTiles, t.IntervalSeconds, t.IntervalTurns))
			},
		},
		{
			kind: "pounce", present: set(a.Pounce),
			problems: func() []string {
				return need(a.Pounce.RangeTiles > 0 && a.Pounce.CooldownSeconds > 0, "pounce needs range_tiles and cooldown_seconds")
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.pounce", a.Pounce.RangeTiles, a.Pounce.CooldownSeconds))
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
				return line(uitext.Text("monster.ability.phase_shift", a.PhaseShift.RangeTiles, config.DurationText(a.PhaseShift.CooldownSeconds)))
			},
		},
		{
			kind: "poison", present: set(a.Poison),
			problems: func() []string {
				return append(chance("poison", a.Poison.Chance), duration("poison", a.Poison.DurationSeconds)...)
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.poison", a.Poison.Chance*100, config.DurationText(a.Poison.DurationSeconds)))
			},
		},
		{
			kind: "ignite", present: set(a.Ignite),
			problems: func() []string {
				return append(chance("ignite", a.Ignite.Chance), duration("ignite", a.Ignite.DurationSeconds)...)
			},
			lines: func() []EffectLine {
				return schoolLine(damagecalc.Fire.String(), uitext.Text("monster.ability.ignite", a.Ignite.Chance*100, config.DurationText(a.Ignite.DurationSeconds)))
			},
		},
		{
			kind: "stun", present: set(a.Stun),
			problems: func() []string {
				return append(chance("stun", a.Stun.Chance), duration("stun", a.Stun.Seconds)...)
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.stun", a.Stun.Chance*100, config.DurationText(a.Stun.Seconds)))
			},
		},
		{
			kind: "dispel", present: set(a.Dispel),
			problems: func() []string { return chance("dispel", a.Dispel.Chance) },
			lines:    func() []EffectLine { return line(uitext.Text("monster.ability.dispel", a.Dispel.Chance*100)) },
		},
		{
			kind: "melt_armor", present: set(a.MeltArmor),
			problems: func() []string { return chance("melt_armor", a.MeltArmor.Chance) },
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.melt_armor",
					a.MeltArmor.Chance*100, config.ArmorElementalMitigationCap, config.DurationText(config.ArmorMeltSeconds())))
			},
		},
		{
			// Each entry is checked on its own; a nova needs boss: true.
			kind: "party_attacks", present: len(a.PartyAttacks) > 0,
			problems: func() []string {
				var out []string
				seen := map[string]bool{}
				for _, attack := range a.PartyAttacks {
					if seen[attack.Key] {
						out = append(out, fmt.Sprintf("party_attacks lists %q twice", attack.Key))
					}
					seen[attack.Key] = true
					out = append(out, attack.problems(d.Boss)...)
				}
				return out
			},
			lines: func() []EffectLine {
				out := make([]EffectLine, 0, len(a.PartyAttacks))
				for _, attack := range a.PartyAttacks {
					out = append(out, attack.line())
				}
				return out
			},
		},
		{
			kind: "ally_heal", present: set(a.AllyHeal),
			problems: func() []string {
				return append(chance("ally_heal", a.AllyHeal.Chance),
					need(a.AllyHeal.Amount > 0 && a.AllyHeal.RadiusTiles > 0, "ally_heal needs amount and radius_tiles")...)
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.ally_heal", a.AllyHeal.Chance*100, a.AllyHeal.Amount, a.AllyHeal.RadiusTiles))
			},
		},
		{
			kind: "teleport", bossOnly: true, present: set(a.Teleport),
			problems: func() []string {
				return append(chance("teleport", a.Teleport.Chance), need(a.Teleport.AtHP > 0, "teleport needs at_hp")...)
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.teleport", a.Teleport.AtHP, a.Teleport.Chance*100))
			},
		},
		{
			kind: "summon", bossOnly: true,
			present: set(a.Summon),
			problems: func() []string {
				s := a.Summon
				// first_guaranteed with chance 0 is a one-shot summon.
				rate := need(s.Chance >= 0 && s.Chance <= 1 && (s.Chance > 0 || s.FirstGuaranteed),
					"summon needs a chance in (0,1], or first_guaranteed for a one-shot summon")
				if s.CooldownSeconds != 0 {
					rate = append(need(s.Chance == 0 && !s.FirstGuaranteed, "summon cooldown_seconds replaces chance and first_guaranteed"),
						duration("summon cooldown", s.CooldownSeconds)...)
				}
				return append(rate, need(len(s.Monsters) > 0, "summon needs monsters")...)
			},
			lines: func() []EffectLine { return line(a.Summon.line()) },
		},
		{
			// Lanes come from the boss's chamber: the map load checks it has some.
			kind: "push", bossOnly: true, present: set(a.Push),
			problems: func() []string { return duration("push cooldown", a.Push.CooldownSeconds) },
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.push", config.DurationText(a.Push.CooldownSeconds)))
			},
		},
		{
			kind: "enrage", bossOnly: true, present: set(a.Enrage),
			problems: func() []string {
				return append(need(a.Enrage.AtHP > 0, "enrage needs at_hp"),
					need(a.Enrage.DamageMult > 0 || a.Enrage.CooldownMult > 0, "enrage needs damage_mult or cooldown_mult")...)
			},
			lines: func() []EffectLine {
				return line(uitext.Text("monster.ability.enrage", a.Enrage.AtHP, effectiveMultiplier(a.Enrage.DamageMult), effectiveMultiplier(a.Enrage.CooldownMult)))
			},
		},
	}
}

func (s SummonAbility) line() string {
	n := s.Count
	if n == 0 {
		n = 1
	}
	capText := uitext.Text("monster.ability.summon_no_cap")
	if s.Max > 0 {
		capText = uitext.Text("monster.ability.summon_cap", s.Max)
	}
	adds := strings.Join(s.Monsters, ", ")
	if s.CooldownSeconds > 0 {
		return uitext.Text("monster.ability.summon_timer", n, adds, config.DurationText(s.CooldownSeconds), capText)
	}
	if s.FirstGuaranteed {
		return uitext.Text("monster.ability.summon_first", n, adds, s.Chance*100, capText)
	}
	return uitext.Text("monster.ability.summon_chance", n, adds, s.Chance*100, capText)
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
