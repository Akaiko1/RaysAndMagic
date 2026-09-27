package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"ugataima/internal/items"
)

// Techniques are class actions, independent of magic schools and spell learning.
type TechniqueDefinition struct {
	Key             string  `yaml:"key"`
	Name            string  `yaml:"name"`
	Description     string  `yaml:"description"`
	Trigger         string  `yaml:"trigger"`
	Icon            string  `yaml:"icon"`
	Level           int     `yaml:"level"`
	SPCost          [4]int  `yaml:"sp_cost"`
	Duration        [4]int  `yaml:"duration_seconds"`
	Power           [4]int  `yaml:"power"`
	TBPower         [4]int  `yaml:"tb_power"`
	Range           [4]int  `yaml:"range_tiles"`
	MinRange        int     `yaml:"min_range_tiles"`
	CooldownSeconds float64 `yaml:"cooldown_seconds"`
	Automatic       bool    `yaml:"automatic"`
	FreeStep        bool    `yaml:"free_step"`
}
type TechniqueConfig struct {
	Techniques []TechniqueDefinition `yaml:"techniques"`
}

var GlobalTechniques *TechniqueConfig

func LoadTechniques(filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var c TechniqueConfig
	if err = yaml.Unmarshal(b, &c); err != nil {
		return err
	}
	seen := map[string]bool{}
	required := map[string]bool{"fold_step": true, "phase_veil": true, "purify": true, "quickening": true, "return_step": true}
	for _, d := range c.Techniques {
		if !required[d.Key] || seen[d.Key] || d.Name == "" || d.Icon == "" || d.Level < 1 || d.CooldownSeconds < 0 {
			return fmt.Errorf("invalid technique %q", d.Key)
		}
		seen[d.Key] = true
		if d.FreeStep != (d.Key == "fold_step" || d.Key == "return_step") || d.Automatic == d.FreeStep {
			return fmt.Errorf("technique %q has incompatible controls", d.Key)
		}
		if d.MinRange < 0 || d.Key == "fold_step" && d.MinRange < 1 {
			return fmt.Errorf("technique %q has invalid minimum range", d.Key)
		}
		for i := range d.SPCost {
			if d.SPCost[i] < 1 || d.Duration[i] < 0 || d.Power[i] < 0 || d.TBPower[i] < 0 || d.Range[i] < 0 {
				return fmt.Errorf("technique %q has invalid tier values", d.Key)
			}
			if d.Key == "fold_step" && d.Range[i] < d.MinRange {
				return fmt.Errorf("technique %q has range below minimum", d.Key)
			}
		}
	}
	if len(seen) != len(required) {
		return fmt.Errorf("techniques: incomplete catalog")
	}
	GlobalTechniques = &c
	return nil
}
func Technique(key string) *TechniqueDefinition {
	if GlobalTechniques != nil {
		for i := range GlobalTechniques.Techniques {
			if GlobalTechniques.Techniques[i].Key == key {
				return &GlobalTechniques.Techniques[i]
			}
		}
	}
	return nil
}
func TechniqueItem(key string) (items.Item, bool) {
	d := Technique(key)
	if d == nil {
		return items.Item{}, false
	}
	return items.Item{Name: d.Name, Type: items.ItemTechnique, SpellEffect: items.SpellEffect(key), Description: d.Description}, true
}
func TierValue(values [4]int, tier int) int { return values[max(0, min(3, tier))] }
