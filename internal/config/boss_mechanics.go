package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// BossMechanics extends map encounters without changing monster archetypes.
// Values are percentages; operated controls subtract percentage points.
type BossMechanic struct {
	ID         string         `yaml:"id"`
	Stat       string         `yaml:"stat"`
	Base       int            `yaml:"base"`
	Floor      int            `yaml:"floor,omitempty"`
	Reductions map[string]int `yaml:"reductions,omitempty"`
}
type BossMechanicsConfig struct {
	Maps map[string][]BossMechanic `yaml:"maps"`
}

var GlobalBossMechanics = &BossMechanicsConfig{Maps: map[string][]BossMechanic{}}

const BossMechanicsPath = "assets/boss_mechanics.yaml"

func ReadBossMechanics(path string) (*BossMechanicsConfig, error) {
	out := &BossMechanicsConfig{Maps: map[string][]BossMechanic{}}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, out.Validate(nil)
}
func (c *BossMechanicsConfig) Validate(maps map[string]*MapConfig) error {
	for key, rules := range c.Maps {
		var a *AdventureConfig
		if maps != nil && len(rules) > 0 {
			m := maps[key]
			if m == nil || m.Adventure == nil || m.Adventure.Boss == nil {
				return fmt.Errorf("boss mechanics %s: configure a map boss first", key)
			}
			a = m.Adventure
		}
		seen := map[string]bool{}
		stats := map[string]bool{}
		for _, r := range rules {
			if r.ID == "" || seen[r.ID] || stats[r.Stat] {
				return fmt.Errorf("boss mechanics %s: duplicate ID or stat", key)
			}
			seen[r.ID], stats[r.Stat] = true, true
			if r.Stat != "critical_chance" && r.Stat != "perfect_dodge" {
				return fmt.Errorf("boss mechanics %s: unknown stat %q", key, r.Stat)
			}
			if r.Base < 0 || r.Base > 100 || r.Floor < 0 || r.Floor > r.Base {
				return fmt.Errorf("boss mechanics %s/%s: require 0 <= floor <= base <= 100", key, r.ID)
			}
			for id, reduction := range r.Reductions {
				if id == "" || reduction < 1 || reduction > 100 || (a != nil && a.Control(id) == nil) {
					return fmt.Errorf("boss mechanics %s/%s: invalid control %q or reduction", key, r.ID, id)
				}
			}
		}
	}
	return nil
}
func (r BossMechanic) Value(controls map[string]bool) int {
	value := r.Base
	for id, reduction := range r.Reductions {
		if controls[id] {
			value -= reduction
		}
	}
	return max(r.Floor, value)
}
