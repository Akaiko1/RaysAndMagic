package config

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Ecology is campaign content. Runtime populations and deliveries live in saves.
var GlobalEcology *EcologyConfig

type EcologyConfig struct {
	Fish          *FishSpawnConfig     `yaml:"fish,omitempty"`
	TurnStepSpeed float64              `yaml:"turn_step_speed"`
	Populations   []WildlifePopulation `yaml:"populations"`
	Caravan       CaravanConfig        `yaml:"caravan"`
}
type WildlifePopulation struct {
	Map     string `yaml:"map"`
	Monster string `yaml:"monster"`
	Count   int    `yaml:"count"`
	Phase   string `yaml:"phase"`
}
type RoutePoint struct {
	Map string `yaml:"map" json:"map"`
	X   int    `yaml:"x" json:"x"`
	Y   int    `yaml:"y" json:"y"`
	// Retire an interior waypoint without changing checkpoint indices in saves.
	Skip bool `yaml:"skip,omitempty" json:"skip,omitempty"`
}
type CaravanRoute struct {
	ID     string       `yaml:"id"`
	Points []RoutePoint `yaml:"points"`
}
type CaravanConfig struct {
	AttackAlertCooldownSeconds int            `yaml:"attack_alert_cooldown_seconds"`
	Monster                    string         `yaml:"monster"`
	UnlockQuest                string         `yaml:"unlock_quest"`
	Merchant                   string         `yaml:"merchant"`
	StopSeconds                int            `yaml:"stop_seconds"`
	RewardUnits                int            `yaml:"reward_units"`
	StockSlots                 int            `yaml:"stock_slots"`
	Routes                     []CaravanRoute `yaml:"routes"`
}

func LoadEcology(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c EcologyConfig
	if err = decodeEcology(data, &c); err != nil {
		return err
	}
	if err = c.Validate(); err != nil {
		return err
	}
	GlobalEcology = &c
	return nil
}
func (c *EcologyConfig) Validate() error {
	if c.Fish != nil {
		if err := c.Fish.Validate(); err != nil {
			return err
		}
	}
	if c.TurnStepSpeed <= 0 {
		return fmt.Errorf("turn_step_speed must be positive")
	}
	seen := map[string]bool{}
	for _, p := range c.Populations {
		k := p.Map + ":" + p.Monster
		if p.Map == "" || p.Monster == "" || p.Count <= 0 || (p.Phase != "day" && p.Phase != "night") || seen[k] {
			return fmt.Errorf("invalid wildlife population %q", k)
		}
		seen[k] = true
	}
	v := c.Caravan
	if v.Monster == "" || v.UnlockQuest == "" || v.Merchant == "" || v.AttackAlertCooldownSeconds <= 0 || v.StopSeconds < 0 || v.RewardUnits < 1 || v.StockSlots < 1 || len(v.Routes) < 1 {
		return fmt.Errorf("invalid caravan configuration")
	}
	seen = map[string]bool{}
	for _, r := range v.Routes {
		if r.ID == "" || seen[r.ID] || len(r.Points) < 2 {
			return fmt.Errorf("invalid caravan route %q", r.ID)
		}
		seen[r.ID] = true
		for i, p := range r.Points {
			if p.Map == "" || p.X < 0 || p.Y < 0 {
				return fmt.Errorf("invalid point in route %q", r.ID)
			}
			if p.Skip && (i == 0 || i == len(r.Points)-1 || r.Points[i-1].Map != p.Map || r.Points[i+1].Map != p.Map) {
				return fmt.Errorf("cannot skip endpoint or map crossing in route %q", r.ID)
			}
		}
	}
	return nil
}

// CaravanTradePool derives one entry per item, irrespective of how many mobs
// drop it. Functional keys are trinkets too, but are not trade goods.
func CaravanTradePool() []string {
	if GlobalLoots == nil {
		return nil
	}
	found := map[string]bool{}
	add := func(entries []LootEntry) {
		for _, e := range entries {
			if e.Type != "item" || e.Chance <= 0 {
				continue
			}
			d, _ := GetItemDefinition(e.Key)
			if d != nil && d.Type == "trinket" && d.Rarity != "legendary" && d.Value > 0 && d.DoorKey == 0 {
				found[e.Key] = true
			}
		}
	}
	for _, entries := range GlobalLoots.Loots {
		add(entries)
	}
	add(GlobalLoots.BossLoot)
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func decodeEcology(data []byte, c *EcologyConfig) error {
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	return d.Decode(c)
}
