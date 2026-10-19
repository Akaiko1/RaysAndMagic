package config

import (
	"fmt"
	"math"
	"slices"
)

// AdventureConfig describes closed, calendar-owned visits and authored terrain.
// All positions are source-map tile coordinates, including in a stitched world.
type AdventureConfig struct {
	ExperienceCap int                  `yaml:"experience_cap,omitempty"`
	ResetDays     int                  `yaml:"reset_days,omitempty"`
	JumpLinks     [][4]int             `yaml:"jump_links,omitempty"` // Authored launch/landing pairs for resource reachability.
	OpeningOwned  bool                 `yaml:"opening_owned,omitempty"`
	Element       string               `yaml:"element,omitempty"`
	Schedule      *AdventureSchedule   `yaml:"schedule,omitempty"`
	Effects       []EnvironmentEffect  `yaml:"effects,omitempty"`
	Controls      []EnvironmentControl `yaml:"controls,omitempty"`
	Boss          *AdventureBoss       `yaml:"boss,omitempty"`
}
type AdventureSchedule struct {
	AlternativeTechnique string      `yaml:"alternative_technique,omitempty"`
	RequiresQuest        string      `yaml:"requires_quest"`
	RequiresSpell        string      `yaml:"requires_spell"`
	Days                 [][2]string `yaml:"days"`
}

// EffectKind is the closed set of chamber environment effects.
type EffectKind string

const (
	EffectOccupation EffectKind = "occupation"
	EffectTrap       EffectKind = "trap"
	EffectTransfer   EffectKind = "transfer"
	EffectLane       EffectKind = "lane"
)

// KindNames is a closed set of typed kinds as plain strings (editor choices,
// messages that list a set).
func KindNames[K ~string](kinds []K) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// EffectKinds lists every effect kind, in the editor's order.
var EffectKinds = []EffectKind{EffectTrap, EffectOccupation, EffectTransfer, EffectLane}

type EnvironmentEffect struct {
	RequiresControl string     `yaml:"requires_control,omitempty"`
	TriggerLane     string     `yaml:"trigger_lane,omitempty"`
	ID              string     `yaml:"id"`
	Kind            EffectKind `yaml:"kind"`
	Name            string     `yaml:"name"`
	Rect            [4]int     `yaml:"rect"` // inclusive x0,y0,x1,y1
	School          string     `yaml:"school,omitempty"`
	SecondarySchool string     `yaml:"secondary_school,omitempty"`
	Damage          int        `yaml:"damage,omitempty"`
	SustainDamage   int        `yaml:"sustain_damage,omitempty"`
	// DamagePercent and SustainPercent author the damage as a percent of the
	// map boss's attack at the visit's level, so it grows with the chamber;
	// each replaces its flat counterpart.
	DamagePercent    int     `yaml:"damage_percent,omitempty"`
	SustainPercent   int     `yaml:"sustain_percent,omitempty"`
	Destination      [2]int  `yaml:"destination,omitempty"`
	WarningSeconds   float64 `yaml:"warning_seconds,omitempty"`
	WarningRounds    int     `yaml:"warning_rounds,omitempty"`
	DurationSeconds  int     `yaml:"duration_seconds,omitempty"` // a lane's lingering field: whole rounds, both modes
	RearmSeconds     float64 `yaml:"rearm_seconds,omitempty"`
	RearmRounds      int     `yaml:"rearm_rounds,omitempty"`
	BossBelowPercent int     `yaml:"boss_below_percent,omitempty"`
}
type EnvironmentControl struct {
	Message string            `yaml:"message,omitempty"`
	ID      string            `yaml:"id"`
	Disable []string          `yaml:"disable,omitempty"`
	Tiles   []EnvironmentTile `yaml:"tiles,omitempty"`
	// ArmorReductionPercent removes that percent of the boss's armor while the
	// switch stays operated (percents of several switches add up).
	ArmorReductionPercent int  `yaml:"armor_reduction_percent,omitempty"`
	StopHealing           bool `yaml:"stop_healing,omitempty"`
}
type EnvironmentTile struct {
	X    int    `yaml:"x"`
	Y    int    `yaml:"y"`
	Tile string `yaml:"tile"`
}
type AdventureBoss struct {
	Phases  []AdventureBossPhase `yaml:"phases,omitempty"`
	Arena   *[4]int              `yaml:"arena,omitempty"`
	Monster string               `yaml:"monster"`
	Lanes   []string             `yaml:"lanes"`
	// CooldownSeconds (whole rounds, at least 2) is the lane cadence: once it
	// is over, the boss's next landed attack also marks the next pattern.
	CooldownSeconds int `yaml:"cooldown_seconds"`
	// HealPercent of max HP is restored every round (its seconds in RT) while
	// the boss is below HealCeilingPercent of max HP.
	HealPercent        int     `yaml:"heal_percent,omitempty"`
	HealCeilingPercent int     `yaml:"heal_ceiling_percent,omitempty"`
	ArmorFloor         int     `yaml:"armor_floor,omitempty"`
	LowHealthRange     float64 `yaml:"low_health_range,omitempty"`
}

// Phases are ordered from high to low HP. Each pattern is one lane action,
// even when it marks several lanes; lane actions ride the boss's landed
// attack for free.
type AdventureBossPhase struct {
	BelowPercent    int        `yaml:"below_percent"`
	Patterns        [][]string `yaml:"patterns"`
	CooldownSeconds int        `yaml:"cooldown_seconds,omitempty"` // 0 keeps the boss's
}

// phase is the lane patterns and their cooldown at an HP.
func (b *AdventureBoss) phase(hp, maxHP int) ([][]string, int) {
	cooldown := b.CooldownSeconds
	patterns := make([][]string, len(b.Lanes))
	for i, lane := range b.Lanes {
		patterns[i] = []string{lane}
	}
	for _, p := range b.Phases {
		if hp*100 <= maxHP*p.BelowPercent {
			patterns = p.Patterns
			if p.CooldownSeconds > 0 {
				cooldown = p.CooldownSeconds
			}
		}
	}
	return patterns, cooldown
}

// Pattern is the phase's next lane pattern after fired earlier ones, in turn.
func (b *AdventureBoss) Pattern(hp, maxHP, fired int) []string {
	patterns, _ := b.phase(hp, maxHP)
	if len(patterns) == 0 {
		return nil
	}
	return patterns[max(0, fired)%len(patterns)]
}

// LaneCooldown is the phase's lane cooldown at an HP, in seconds.
func (b *AdventureBoss) LaneCooldown(hp, maxHP int) int {
	_, cooldown := b.phase(hp, maxHP)
	return cooldown
}

// PhaseLanes lists every lane the boss fires at an HP, in pattern order.
func (b *AdventureBoss) PhaseLanes(hp, maxHP int) []string {
	patterns, _ := b.phase(hp, maxHP)
	var out []string
	for _, pattern := range patterns {
		for _, id := range pattern {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func (s *AdventureSchedule) Destination(day int, night bool) string {
	if s == nil || len(s.Days) == 0 || day < 1 {
		return ""
	}
	phase := 0
	if night {
		phase = 1
	}
	return s.Days[(day-1)%len(s.Days)][phase]
}

// Damaging reports an effect that deals damage, flat or as a percent of the
// boss hit.
func (e EnvironmentEffect) Damaging() bool {
	return e.Damage > 0 || e.SustainDamage > 0 || e.DamagePercent > 0 || e.SustainPercent > 0
}

func (e EnvironmentEffect) Contains(x, y int) bool {
	return x >= e.Rect[0] && y >= e.Rect[1] && x <= e.Rect[2] && y <= e.Rect[3]
}
func (a *AdventureConfig) Effect(id string) *EnvironmentEffect {
	if a != nil {
		for i := range a.Effects {
			if a.Effects[i].ID == id {
				return &a.Effects[i]
			}
		}
	}
	return nil
}
func (a *AdventureConfig) Control(id string) *EnvironmentControl {
	if a != nil {
		for i := range a.Controls {
			if a.Controls[i].ID == id {
				return &a.Controls[i]
			}
		}
	}
	return nil
}
func (c *MapConfigs) ValidateAdventures() error {
	validSchool := func(s string) bool {
		return s == "physical" || s == "fire" || s == "water" || s == "earth" || s == "air"
	}
	for key, m := range c.Maps {
		a := m.Adventure
		if a == nil {
			continue
		}
		fail := func(s string) error { return fmt.Errorf("map %q adventure: %s", key, s) }
		if a.ExperienceCap < 0 || a.ResetDays < 0 || ((a.ExperienceCap > 0 || a.ResetDays > 0) && !a.OpeningOwned) {
			return fail("experience_cap and reset_days require opening ownership and nonnegative values")
		}
		if a.ResetDays > 0 && a.Boss == nil {
			return fail("reset_days requires a boss to start the cooldown")
		}
		if a.OpeningOwned && (m.RespawnDays != 0 || !validSchool(a.Element) || a.Element == "physical") {
			return fail("opening ownership requires an elemental school and excludes respawn_days")
		}
		if s := a.Schedule; s != nil {
			if len(s.Days) != 7 || s.RequiresQuest == "" || s.RequiresSpell == "" {
				return fail("schedule requires seven days, an access quest and a spell")
			}
			if s.AlternativeTechnique != "" {
				// A programming error, not content: the catalog must load first.
				if GlobalTechniques == nil {
					return fmt.Errorf("map configs name techniques: load assets/techniques.yaml before validating them")
				}
				if d := Technique(s.AlternativeTechnique); d == nil || d.Kind != TechniqueFoldStep {
					return fail("schedule alternative_technique must name a fold_step technique")
				}
			}
			for _, day := range s.Days {
				for _, dest := range day {
					d, ok := c.Maps[dest]
					if !ok || dest == key || d.Adventure == nil || !d.Adventure.OpeningOwned {
						return fail("schedule destination is not an opening-owned map: " + dest)
					}
				}
			}
		}
		if len(a.JumpLinks) > 0 && !a.OpeningOwned {
			return fail("jump_links only work on opening-owned maps")
		}
		for _, link := range a.JumpLinks {
			dx, dy := math.Abs(float64(link[2]-link[0])), math.Abs(float64(link[3]-link[1]))
			if link[0] < 0 || link[1] < 0 || link[2] < 0 || link[3] < 0 || !((dx == 2 && dy == 0) || (dx == 0 && dy == 2)) {
				return fail("invalid Jump link")
			}
		}
		ids := map[string]bool{}
		for _, e := range a.Effects {
			if e.ID == "" || e.Name == "" || ids[e.ID] || e.Rect[0] < 0 || e.Rect[1] < 0 || e.Rect[2] < e.Rect[0] || e.Rect[3] < e.Rect[1] {
				return fail("invalid or duplicate effect " + e.ID)
			}
			ids[e.ID] = true
			if !slices.Contains(EffectKinds, e.Kind) {
				return fail("unknown effect kind " + string(e.Kind))
			}
			if e.Kind != EffectTransfer && ((e.Damage > 0) == (e.DamagePercent > 0) || !validSchool(e.School)) {
				return fail("damage needs exactly one of damage or damage_percent, and a known school")
			}
			if e.Damage < 0 || e.DamagePercent < 0 || e.SustainDamage < 0 || e.SustainPercent < 0 || (e.SustainDamage > 0 && e.SustainPercent > 0) {
				return fail("invalid damage on " + e.ID)
			}
			if (e.DamagePercent > 0 || e.SustainPercent > 0) && a.Boss == nil {
				return fail("percent damage on " + e.ID + " needs a map boss")
			}
			if e.Kind != EffectLane && (e.SustainDamage > 0 || e.SustainPercent > 0) {
				return fail("lingering damage on " + e.ID + " is for lanes only")
			}
			if e.Kind == EffectTransfer && (e.Damage > 0 || e.DamagePercent > 0) {
				return fail("transfer " + e.ID + " deals no damage")
			}
			if e.SecondarySchool != "" && !validSchool(e.SecondarySchool) {
				return fail("unknown secondary school")
			}
			if e.Kind == EffectTransfer && (e.Destination[0] < 0 || e.Destination[1] < 0 || e.Contains(e.Destination[0], e.Destination[1])) {
				return fail("invalid receiver")
			}
			if e.Kind == EffectLane && (e.WarningSeconds <= 0 || e.WarningRounds < 1) {
				return fail("lanes require RT and TB warning")
			}
			if e.Kind == EffectLane {
				if err := ValidateEffectSeconds("lane "+e.ID+" duration_seconds", e.DurationSeconds); err != nil {
					return fail(err.Error())
				}
			} else if e.DurationSeconds != 0 {
				return fail("duration_seconds is for lanes only")
			}
			if e.Kind == EffectTrap && (e.RearmSeconds <= 0 || e.RearmRounds < 1) {
				return fail("traps require RT and TB rearm")
			}
			for _, n := range []float64{e.WarningSeconds, e.RearmSeconds} {
				if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
					return fail("invalid duration")
				}
			}
			if e.BossBelowPercent < 0 || e.BossBelowPercent > 100 {
				return fail("invalid HP threshold")
			}
			// An HP-gated effect charges through its warning; a zero clock never
			// counts down, so the effect would wait forever.
			if e.BossBelowPercent > 0 && (a.Boss == nil || e.WarningSeconds <= 0 || e.WarningRounds < 1) {
				return fail("boss HP threshold on " + e.ID + " requires a boss and RT and TB warning")
			}
		}
		controls := map[string]bool{}
		for _, v := range a.Controls {
			if v.ID == "" || controls[v.ID] || v.ArmorReductionPercent < 0 || v.ArmorReductionPercent > 100 {
				return fail("invalid control")
			}
			controls[v.ID] = true
			for _, id := range v.Disable {
				if !ids[id] {
					return fail("control names unknown effect " + id)
				}
			}
			for _, t := range v.Tiles {
				if t.X < 0 || t.Y < 0 || t.Tile == "" {
					return fail("invalid controlled tile")
				}
			}
		}
		for _, e := range a.Effects {
			if e.RequiresControl != "" && !controls[e.RequiresControl] {
				return fail("unknown required control")
			}
			if e.TriggerLane != "" {
				lane := a.Effect(e.TriggerLane)
				if e.Kind != EffectTransfer || lane == nil || lane.Kind != EffectLane {
					return fail("transfer requires a known trigger lane")
				}
			}
		}
		if b := a.Boss; b != nil {
			lastThreshold := 101
			for _, phase := range b.Phases {
				if phase.BelowPercent < 1 || phase.BelowPercent >= lastThreshold || len(phase.Patterns) == 0 ||
					(phase.CooldownSeconds != 0 && ValidateEffectSeconds("boss phase cooldown_seconds", phase.CooldownSeconds) != nil) {
					return fail("invalid boss phase order, patterns or cooldown (whole rounds)")
				}
				lastThreshold = phase.BelowPercent
				for _, pattern := range phase.Patterns {
					seen := map[string]bool{}
					if len(pattern) == 0 {
						return fail("empty boss pattern")
					}
					for _, id := range pattern {
						e := a.Effect(id)
						if e == nil || e.Kind != EffectLane || seen[id] {
							return fail("invalid or duplicate boss phase lane")
						}
						seen[id] = true
					}
				}
			}
			if b.Arena != nil && (!a.OpeningOwned || b.Arena[0] < 0 || b.Arena[1] < 0 || b.Arena[2] < b.Arena[0] || b.Arena[3] < b.Arena[1]) {
				return fail("boss arena requires an opening-owned map and a valid rectangle")
			}
			if b.Monster == "" || b.ArmorFloor < 0 {
				return fail("invalid boss policy")
			}
			if ValidateEffectSeconds("boss cooldown_seconds", b.CooldownSeconds) != nil || CombatRounds(b.CooldownSeconds) < 2 {
				return fail("boss cooldown_seconds must be whole rounds, at least 2")
			}
			if b.HealPercent < 0 || b.HealPercent > 100 || b.HealCeilingPercent < 0 || b.HealCeilingPercent > 100 || (b.HealPercent > 0 && b.HealCeilingPercent == 0) {
				return fail("regeneration needs heal_percent 1..100 per round and heal_ceiling_percent 1..100")
			}
			if b.LowHealthRange < 0 || math.IsNaN(b.LowHealthRange) || math.IsInf(b.LowHealthRange, 0) {
				return fail("invalid boss range")
			}
			for _, id := range b.Lanes {
				e := a.Effect(id)
				if e == nil || e.Kind != EffectLane {
					return fail("boss names invalid lane")
				}
			}
		}
	}
	return nil
}
