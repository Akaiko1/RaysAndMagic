package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
	"ugataima/internal/items"
)

type FlaskDefinition struct {
	Sprite          string  `yaml:"sprite"`
	Element         string  `yaml:"element"`
	Damage          [4]int  `yaml:"damage"`
	PoisonSeconds   [4]int  `yaml:"poison_seconds"`
	BurnSeconds     int     `yaml:"burn_seconds"`
	RangeTiles      int     `yaml:"range_tiles"`
	RadiusTiles     int     `yaml:"radius_tiles"`
	CooldownSeconds float64 `yaml:"cooldown_seconds"`
}
type AlchemyAlternative struct {
	Items []string `yaml:"items"`
	Count int      `yaml:"count"`
}
type AlchemyIngredient struct {
	Label        string               `yaml:"label"`
	Alternatives []AlchemyAlternative `yaml:"alternatives"`
}
type AlchemyRecipe struct {
	Key         string              `yaml:"key"`
	Output      string              `yaml:"output"`
	Family      string              `yaml:"family"`
	Ingredients []AlchemyIngredient `yaml:"ingredients"`
}
type AlchemyConfig struct {
	Recipes []AlchemyRecipe `yaml:"recipes"`
}

var GlobalAlchemy *AlchemyConfig

type HarvestPopulation struct {
	Map     string         `yaml:"map"`
	Key     string         `yaml:"key"`
	Count   int            `yaml:"count"`
	Yield   int            `yaml:"yield"`
	Weights map[string]int `yaml:"weights"`
}
type HarvestPlacement struct {
	TerrainTypes []string `yaml:"terrain_types"`
	ExcludeTiles []string `yaml:"exclude_tiles"`
}
type AlchemySpawnConfig struct {
	Placement   HarvestPlacement    `yaml:"placement"`
	Respawn     string              `yaml:"respawn"`
	Populations []HarvestPopulation `yaml:"populations"`
}

var GlobalAlchemySpawns *AlchemySpawnConfig

func LoadAlchemyConfig(filename string) (*AlchemyConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var c AlchemyConfig
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if err = validateAlchemyConfig(&c); err != nil {
		return nil, err
	}
	GlobalAlchemy = &c
	return &c, nil
}
func validateAlchemyConfig(c *AlchemyConfig) error {
	seen := map[string]bool{}
	if len(c.Recipes) == 0 {
		return fmt.Errorf("alchemy: no recipes")
	}
	for _, r := range c.Recipes {
		if r.Key == "" || seen[r.Key] {
			return fmt.Errorf("alchemy: empty or duplicate recipe %q", r.Key)
		}
		seen[r.Key] = true
		d, _ := GetItemDefinition(r.Output)
		if d == nil || !d.CraftedOnly || d.Value != 1 || d.Type != "consumable" {
			return fmt.Errorf("recipe %q: output must be a value-1 crafted consumable", r.Key)
		}
		if r.Family != "common" && r.Family != "protective" && r.Family != "revival" {
			return fmt.Errorf("recipe %q: unknown family", r.Key)
		}
		if len(r.Ingredients) == 0 {
			return fmt.Errorf("recipe %q: no ingredients", r.Key)
		}
		for _, g := range r.Ingredients {
			if g.Label == "" || len(g.Alternatives) == 0 {
				return fmt.Errorf("recipe %q: empty ingredient group", r.Key)
			}
			for _, a := range g.Alternatives {
				if a.Count < 1 || a.Count > 999 || len(a.Items) == 0 {
					return fmt.Errorf("recipe %q: invalid ingredient quantity", r.Key)
				}
				keys := map[string]bool{}
				for _, key := range a.Items {
					d, _ := GetItemDefinition(key)
					if keys[key] || d == nil || d.Type != "trinket" || d.CraftedOnly || d.DoorKey != 0 {
						return fmt.Errorf("recipe %q: invalid ingredient %q", r.Key, key)
					}
					keys[key] = true
				}
			}
		}
	}
	return nil
}
func AlchemyRecipeByKey(key string) *AlchemyRecipe {
	if GlobalAlchemy != nil {
		for i := range GlobalAlchemy.Recipes {
			if GlobalAlchemy.Recipes[i].Key == key {
				return &GlobalAlchemy.Recipes[i]
			}
		}
	}
	return nil
}
func LoadAlchemySpawns(filename string) (*AlchemySpawnConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var c AlchemySpawnConfig
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Respawn != "dawn" || len(c.Placement.TerrainTypes) == 0 {
		return nil, fmt.Errorf("harvest: require dawn respawn and terrain_types")
	}
	seen := map[string]bool{}
	for _, p := range c.Populations {
		id := p.Map + ":" + p.Key
		if p.Map == "" || p.Key == "" || seen[id] || p.Count < 1 || p.Count > 64 || p.Yield < 1 || p.Yield > 99 || len(p.Weights) == 0 {
			return nil, fmt.Errorf("harvest: invalid population %q", id)
		}
		seen[id] = true
		for key, w := range p.Weights {
			d, _ := GetItemDefinition(key)
			if w < 1 || d == nil || d.Type != "trinket" || d.HarvestSprite == "" {
				return nil, fmt.Errorf("harvest %q: invalid reagent %q", id, key)
			}
		}
	}
	GlobalAlchemySpawns = &c
	return &c, nil
}

// resolveBrewedItems expands only effect fields. Names, prices, identity and
// presentation remain authored on each brewed product; chained sources are invalid.
func resolveBrewedItems(c *ItemSystemConfig) error {
	for key, d := range c.Items {
		if d == nil || d.BrewedFrom == "" {
			continue
		}
		source := c.Items[d.BrewedFrom]
		if source == nil || source.BrewedFrom != "" || source.CraftedOnly || source.Type != "consumable" || source.SummonDistanceTiles > 0 || source.Flask != nil ||
			(source.HealBase <= 0 && source.ManaBase <= 0 && !source.Revive && !source.HasTimedBuff()) {
			return fmt.Errorf("item %q: invalid brewed_from %q", key, d.BrewedFrom)
		}
		if d.Name == "" || d.Name == source.Name || d.Value != 1 || d.Type != "consumable" {
			return fmt.Errorf("item %q: brewed product needs a unique name, consumable type and value 1", key)
		}
		d.CraftedOnly = true
		d.HealBase = source.HealBase
		d.HealEnduranceDivisor = source.HealEnduranceDivisor
		d.ManaBase = source.ManaBase
		d.ManaPersonalityDivisor = source.ManaPersonalityDivisor
		d.Revive = source.Revive
		d.FullHeal = source.FullHeal
		d.CurePoison = source.CurePoison
		d.ResistBuffSchool = source.ResistBuffSchool
		d.ResistBuffSchoolPct = source.ResistBuffSchoolPct
		d.BuffArmorClass = source.BuffArmorClass
		d.BuffDodgePct = source.BuffDodgePct
		d.BuffDurationSeconds = source.BuffDurationSeconds
		d.StatusIcon = source.StatusIcon
	}
	return nil
}
func validateCraftedItem(key string, d *ItemDefinitionConfig) error {
	if d.CraftedOnly && (d.Type != "consumable" || d.Value != 1) {
		return fmt.Errorf("item %q: crafted products require consumable type and value 1", key)
	}
	if f := d.Flask; f != nil {
		element, err := canonicalDamageSchool(f.Element)
		if err != nil {
			return err
		}
		f.Element = element
		if f.Sprite == "" || !d.CraftedOnly || f.RangeTiles < 1 || f.RadiusTiles < 1 || f.CooldownSeconds <= 0 || f.BurnSeconds < 0 || d.HealBase > 0 || d.ManaBase > 0 || d.Revive || d.SummonDistanceTiles > 0 || d.HasTimedBuff() {
			return fmt.Errorf("item %q: invalid throwable flask", key)
		}
		for i := range f.Damage {
			if f.Damage[i] < 1 || f.PoisonSeconds[i] < 0 {
				return fmt.Errorf("item %q: invalid flask mastery values", key)
			}
		}
	}
	return nil
}

// ValidateOrdinaryItemGrant guards every authored source other than recipes and
// class supplies. Transfer/sale of already-owned products is deliberately legal.
func ValidateOrdinaryItemGrant(key string) error {
	if d, _ := GetItemDefinition(key); d != nil && d.CraftedOnly {
		return fmt.Errorf("crafted-only item %q cannot be granted by this source", key)
	}
	if d, _ := GetItemDefinition(key); d != nil && d.HarvestSprite != "" {
		return fmt.Errorf("harvest-only item %q cannot be granted by this source", key)
	}
	return nil
}

// Explicit monster herb drops do not open herbs to ordinary catalog grants.
func ValidateMonsterHarvestGrant(key string) error {
	d, ok := GetItemDefinition(key)
	if !ok || d == nil || d.HarvestSprite == "" || d.CraftedOnly {
		return fmt.Errorf("monster harvest drop %q must name a gathering ingredient", key)
	}
	return nil
}

func FlaskItem(key string) (items.Item, bool) {
	d, _ := GetItemDefinition(key)
	if d == nil || d.Flask == nil {
		return items.Item{}, false
	}
	return items.Item{Name: d.Name, Type: items.ItemThrowable, SpellEffect: items.SpellEffect(key), Description: d.Description}, true
}
func FlaskKeys() []string {
	var keys []string
	if GlobalItems != nil {
		for key, d := range GlobalItems.Items {
			if d != nil && d.Flask != nil {
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	return keys
}
