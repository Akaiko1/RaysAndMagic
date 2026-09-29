package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// PotionPreferences are player policy, separate from authored timing and campaign saves.
type PotionPreferences struct {
	Health PotionResourcePreferences `json:"health"`
	Mana   PotionResourcePreferences `json:"mana"`
}
type PotionResourcePreferences struct {
	ThresholdPct int             `json:"threshold_percent"`
	Allowed      map[string]bool `json:"allowed_types"`
}
type PotionChoice struct {
	Key        string
	Definition *ItemDefinitionConfig
}

func IsAutomaticPotion(def *ItemDefinitionConfig, mana bool) bool {
	if def == nil || def.Type != "consumable" || def.Revive || def.SummonDistanceTiles > 0 || def.Flask != nil {
		return false
	}
	if mana {
		return def.ManaBase > 0
	}
	return def.HealBase > 0
}

// Brewed copies share the policy of their authored original potion family.
func PotionFamily(def *ItemDefinitionConfig) string {
	if def == nil {
		return ""
	}
	if def.BrewedFrom != "" {
		return def.BrewedFrom
	}
	_, key, _ := GetItemDefinitionByName(def.Name)
	return key
}
func AutomaticPotionChoices(mana bool) []PotionChoice {
	var result []PotionChoice
	if GlobalItems == nil {
		return result
	}
	for key, def := range GlobalItems.Items {
		if def.BrewedFrom == "" && IsAutomaticPotion(def, mana) {
			result = append(result, PotionChoice{key, def})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Definition.Name < result[j].Definition.Name })
	return result
}
func (p *PotionPreferences) Resource(mana bool) *PotionResourcePreferences {
	if mana {
		return &p.Mana
	}
	return &p.Health
}
func (c *Config) AutoPotionThreshold(mana bool) int {
	if c == nil {
		return 0
	}
	if c.PlayerPotions == nil {
		return c.Characters.AutoDrink.ThresholdPct
	}
	return c.PlayerPotions.Resource(mana).ThresholdPct
}
func (c *Config) AllowsAutoPotion(def *ItemDefinitionConfig, mana bool) bool {
	if c == nil || !IsAutomaticPotion(def, mana) {
		return false
	}
	if c.PlayerPotions == nil {
		return true
	}
	return c.PlayerPotions.Resource(mana).Allowed[PotionFamily(def)]
}
func (c *Config) DefaultPotionPreferences() *PotionPreferences {
	p := &PotionPreferences{}
	for _, mana := range []bool{false, true} {
		r := p.Resource(mana)
		r.ThresholdPct = c.Characters.AutoDrink.ThresholdPct
		r.Allowed = map[string]bool{}
		for _, choice := range AutomaticPotionChoices(mana) {
			r.Allowed[choice.Key] = true
		}
	}
	return p
}
func (c *Config) EnsurePotionPreferences() *PotionPreferences {
	if c.PlayerPotions == nil {
		c.PlayerPotions = c.DefaultPotionPreferences()
	}
	return c.PlayerPotions
}

// Missing fields inherit defaults, while an explicitly empty allowed-types map
// means none. Unknown saved keys never authorize a new item family implicitly.
func (c *Config) LoadPotionPreferences(path string) error {
	defaults := c.DefaultPotionPreferences()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c.PlayerPotions = defaults
		return nil
	}
	if err != nil {
		return err
	}
	type savedResource struct {
		ThresholdPct *int            `json:"threshold_percent"`
		Allowed      map[string]bool `json:"allowed_types"`
	}
	var saved struct {
		Health savedResource `json:"health"`
		Mana   savedResource `json:"mana"`
	}
	if err = json.Unmarshal(raw, &saved); err != nil {
		return fmt.Errorf("potion preferences: %w", err)
	}
	for _, mana := range []bool{false, true} {
		source := saved.Health
		if mana {
			source = saved.Mana
		}
		target := defaults.Resource(mana)
		if source.ThresholdPct != nil {
			target.ThresholdPct = max(0, min(100, *source.ThresholdPct))
		}
		if source.Allowed != nil {
			target.Allowed = map[string]bool{}
			for _, choice := range AutomaticPotionChoices(mana) {
				target.Allowed[choice.Key] = source.Allowed[choice.Key]
			}
		}
	}
	c.PlayerPotions = defaults
	return nil
}
