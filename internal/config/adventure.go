package config

import (
	"fmt"
	"math"
)

// AdventureConfig describes closed, calendar-owned visits and authored terrain.
// All positions are source-map tile coordinates, including in a stitched world.
type AdventureConfig struct {
	JumpLinks    [][4]int             `yaml:"jump_links,omitempty"` // Authored launch/landing pairs for resource reachability.
	OpeningOwned bool                 `yaml:"opening_owned,omitempty"`
	Element      string               `yaml:"element,omitempty"`
	Schedule     *AdventureSchedule   `yaml:"schedule,omitempty"`
	Effects      []EnvironmentEffect  `yaml:"effects,omitempty"`
	Controls     []EnvironmentControl `yaml:"controls,omitempty"`
	Boss         *AdventureBoss       `yaml:"boss,omitempty"`
}
type AdventureSchedule struct {
	AlternativeTechnique string      `yaml:"alternative_technique,omitempty"`
	RequiresQuest        string      `yaml:"requires_quest"`
	RequiresSpell        string      `yaml:"requires_spell"`
	Days                 [][2]string `yaml:"days"`
}
type EnvironmentEffect struct {
	RequiresControl  string  `yaml:"requires_control,omitempty"`
	TriggerLane      string  `yaml:"trigger_lane,omitempty"`
	ID               string  `yaml:"id"`
	Kind             string  `yaml:"kind"` // occupation, trap, transfer, or lane
	Name             string  `yaml:"name"`
	Rect             [4]int  `yaml:"rect"` // inclusive x0,y0,x1,y1
	School           string  `yaml:"school,omitempty"`
	SecondarySchool  string  `yaml:"secondary_school,omitempty"`
	Damage           int     `yaml:"damage,omitempty"`
	SustainDamage    int     `yaml:"sustain_damage,omitempty"`
	Destination      [2]int  `yaml:"destination,omitempty"`
	WarningSeconds   float64 `yaml:"warning_seconds,omitempty"`
	WarningRounds    int     `yaml:"warning_rounds,omitempty"`
	DurationSeconds  float64 `yaml:"duration_seconds,omitempty"`
	DurationRounds   int     `yaml:"duration_rounds,omitempty"`
	RearmSeconds     float64 `yaml:"rearm_seconds,omitempty"`
	RearmRounds      int     `yaml:"rearm_rounds,omitempty"`
	BossBelowPercent int     `yaml:"boss_below_percent,omitempty"`
}
type EnvironmentControl struct {
	Message        string            `yaml:"message,omitempty"`
	ID             string            `yaml:"id"`
	Disable        []string          `yaml:"disable,omitempty"`
	Tiles          []EnvironmentTile `yaml:"tiles,omitempty"`
	ArmorReduction int               `yaml:"armor_reduction,omitempty"`
	StopHealing    bool              `yaml:"stop_healing,omitempty"`
}
type EnvironmentTile struct {
	X    int    `yaml:"x"`
	Y    int    `yaml:"y"`
	Tile string `yaml:"tile"`
}
type AdventureBoss struct {
	Phases         []AdventureBossPhase `yaml:"phases,omitempty"`
	Arena          *[4]int              `yaml:"arena,omitempty"`
	Monster        string               `yaml:"monster"`
	Lanes          []string             `yaml:"lanes"`
	EveryActions   int                  `yaml:"every_actions"`
	HealPercent    int                  `yaml:"heal_percent,omitempty"`
	HealCapPercent int                  `yaml:"heal_cap_percent,omitempty"`
	ArmorFloor     int                  `yaml:"armor_floor,omitempty"`
	LowHealthRange float64              `yaml:"low_health_range,omitempty"`
}

// Phases are ordered from high to low HP. Each pattern consumes one special
// action, even when it contains several separately marked lanes.
type AdventureBossPhase struct {
	BelowPercent int        `yaml:"below_percent"`
	Patterns     [][]string `yaml:"patterns"`
	EveryActions int        `yaml:"every_actions,omitempty"`
}

func (b *AdventureBoss) Pattern(hp, maxHP, actions int) []string {
	every := b.EveryActions
	patterns := make([][]string, len(b.Lanes))
	for i, lane := range b.Lanes {
		patterns[i] = []string{lane}
	}
	for _, p := range b.Phases {
		if hp*100 <= maxHP*p.BelowPercent {
			patterns = p.Patterns
			if p.EveryActions > 0 {
				every = p.EveryActions
			}
		}
	}
	if every < 1 || actions < 1 || actions%every != 0 || len(patterns) == 0 {
		return nil
	}
	return patterns[(actions/every-1)%len(patterns)]
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
		if a.OpeningOwned && (m.RespawnDays != 0 || !validSchool(a.Element) || a.Element == "physical") {
			return fail("opening ownership requires an elemental school and excludes respawn_days")
		}
		if s := a.Schedule; s != nil {
			if len(s.Days) != 7 || s.RequiresQuest == "" || s.RequiresSpell == "" {
				return fail("schedule requires seven days, an access quest and a spell")
			}
			if s.AlternativeTechnique != "" && s.AlternativeTechnique != "fold_step" {
				return fail("schedule alternative_technique must be fold_step")
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
			if e.Kind != "occupation" && e.Kind != "trap" && e.Kind != "transfer" && e.Kind != "lane" {
				return fail("unknown effect kind " + e.Kind)
			}
			if e.Kind != "transfer" && (e.Damage <= 0 || !validSchool(e.School)) {
				return fail("damage needs a positive amount and known school")
			}
			if e.SecondarySchool != "" && !validSchool(e.SecondarySchool) {
				return fail("unknown secondary school")
			}
			if e.Kind == "transfer" && (e.Destination[0] < 0 || e.Destination[1] < 0 || e.Contains(e.Destination[0], e.Destination[1])) {
				return fail("invalid receiver")
			}
			if e.Kind == "lane" && (e.WarningSeconds <= 0 || e.WarningRounds < 1 || e.DurationSeconds <= 0 || e.DurationRounds < 1) {
				return fail("lanes require RT and TB warning/duration")
			}
			if e.Kind == "trap" && (e.RearmSeconds <= 0 || e.RearmRounds < 1) {
				return fail("traps require RT and TB rearm")
			}
			for _, n := range []float64{e.WarningSeconds, e.DurationSeconds, e.RearmSeconds} {
				if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
					return fail("invalid duration")
				}
			}
			if e.BossBelowPercent < 0 || e.BossBelowPercent > 100 {
				return fail("invalid HP threshold")
			}
		}
		controls := map[string]bool{}
		for _, v := range a.Controls {
			if v.ID == "" || controls[v.ID] || v.ArmorReduction < 0 {
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
				if e.Kind != "transfer" || lane == nil || lane.Kind != "lane" {
					return fail("transfer requires a known trigger lane")
				}
			}
		}
		if b := a.Boss; b != nil {
			lastThreshold := 101
			for _, phase := range b.Phases {
				if phase.BelowPercent < 1 || phase.BelowPercent >= lastThreshold || len(phase.Patterns) == 0 || phase.EveryActions < 0 {
					return fail("invalid boss phase order, patterns or cadence")
				}
				lastThreshold = phase.BelowPercent
				for _, pattern := range phase.Patterns {
					seen := map[string]bool{}
					if len(pattern) == 0 {
						return fail("empty boss pattern")
					}
					for _, id := range pattern {
						e := a.Effect(id)
						if e == nil || e.Kind != "lane" || seen[id] {
							return fail("invalid or duplicate boss phase lane")
						}
						seen[id] = true
					}
				}
			}
			if b.Arena != nil && (!a.OpeningOwned || b.Arena[0] < 0 || b.Arena[1] < 0 || b.Arena[2] < b.Arena[0] || b.Arena[3] < b.Arena[1]) {
				return fail("boss arena requires an opening-owned map and a valid rectangle")
			}
			if b.Monster == "" || b.EveryActions < 2 || b.HealPercent < 0 || b.HealCapPercent < 0 || b.HealCapPercent > 100 {
				return fail("invalid boss policy")
			}
			if b.LowHealthRange < 0 || math.IsNaN(b.LowHealthRange) || math.IsInf(b.LowHealthRange, 0) {
				return fail("invalid boss range")
			}
			for _, id := range b.Lanes {
				e := a.Effect(id)
				if e == nil || e.Kind != "lane" {
					return fail("boss names invalid lane")
				}
			}
		}
	}
	return nil
}
