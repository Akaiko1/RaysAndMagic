package config

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"iter"
	"os"
	"slices"
	"strings"
	"ugataima/internal/items"
)

// Techniques are class actions, independent of magic schools and spell learning.
// Key is the content identity; Kind picks the engine behavior.
type TechniqueDefinition struct {
	Key             string  `yaml:"key"`
	Kind            string  `yaml:"kind"`
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
	// ReuseSeconds is the party's shared spatial lock after a step
	// (TB: also once per turn).
	ReuseSeconds int  `yaml:"reuse_seconds"`
	Automatic    bool `yaml:"automatic"`
}
type TechniqueConfig struct {
	Techniques []TechniqueDefinition `yaml:"techniques"`
}

// Technique kinds: the closed set of behaviors the engine implements.
const (
	TechniqueFoldStep   = "fold_step"   // forward step that leaves a return anchor
	TechniqueReturnStep = "return_step" // back to the fold anchor
	TechniqueDodge      = "dodge"       // timed party dodge buff
	TechniqueHaste      = "haste"       // timed recovery cut and TB bonus actions
	TechniquePurify     = "purify"      // cleanses afflictions, root and slow
)

type techniqueKindRule struct {
	// step moves the party: no action cost, no cooldown, a shared reuse lock.
	step                                 bool
	rangeTiles, duration, power, tbPower bool // tier ladders the kind reads
	// autocastRank orders automatic techniques (lower first); 0 never autocasts.
	autocastRank int
}

// AutocastTechniqueKinds lists the technique kinds that may cast themselves;
// the game owns one trigger for each.
func AutocastTechniqueKinds() []string {
	var out []string
	for kind, rule := range techniqueKindRules {
		if rule.autocastRank > 0 {
			out = append(out, kind)
		}
	}
	slices.Sort(out)
	return out
}

var techniqueKindRules = map[string]techniqueKindRule{
	TechniqueFoldStep:   {step: true, rangeTiles: true, duration: true},
	TechniqueReturnStep: {step: true},
	TechniquePurify:     {autocastRank: 1},
	TechniqueDodge:      {duration: true, power: true, autocastRank: 2},
	TechniqueHaste:      {duration: true, power: true, tbPower: true, autocastRank: 3},
}

// IsStep reports a spatial step (Fold Step, Return Step).
func (d *TechniqueDefinition) IsStep() bool { return techniqueKindRules[d.Kind].step }

var GlobalTechniques *TechniqueConfig

func LoadTechniques(filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var c TechniqueConfig
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err = dec.Decode(&c); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, d := range c.Techniques {
		if err := validateTechnique(&d, seen); err != nil {
			return err
		}
	}
	GlobalTechniques = &c
	return nil
}

// validateTechnique checks one entry against its kind; seen tracks keys and
// step kinds, which input addresses by kind and must stay unique.
func validateTechnique(d *TechniqueDefinition, seen map[string]bool) error {
	rule, ok := techniqueKindRules[d.Kind]
	if d.Key == "" || seen[d.Key] || d.Name == "" || d.Icon == "" || d.Level < 1 || d.CooldownSeconds < 0 {
		return fmt.Errorf("invalid technique %q", d.Key)
	}
	seen[d.Key] = true
	if !ok {
		return fmt.Errorf("technique %q has unknown kind %q", d.Key, d.Kind)
	}
	if rule.step {
		if seen["kind:"+d.Kind] {
			return fmt.Errorf("technique %q: only one %s technique may exist", d.Key, d.Kind)
		}
		seen["kind:"+d.Kind] = true
	}
	if d.Automatic && rule.autocastRank == 0 {
		return fmt.Errorf("technique %q: kind %s cannot autocast", d.Key, d.Kind)
	}
	if d.Automatic != (strings.TrimSpace(d.Trigger) != "") {
		return fmt.Errorf("technique %q: trigger text is required on automatic techniques and unsupported elsewhere", d.Key)
	}
	// Only steps have a shared spatial reuse lock, and they have no cooldown.
	if d.ReuseSeconds < 0 || rule.step != (d.ReuseSeconds > 0) || rule.step && d.CooldownSeconds > 0 {
		return fmt.Errorf("technique %q: reuse_seconds is required on steps, cooldown_seconds unsupported on them", d.Key)
	}
	if rule.rangeTiles != (d.MinRange >= 1) || d.MinRange < 0 {
		return fmt.Errorf("technique %q has invalid minimum range", d.Key)
	}
	for _, ladder := range []struct {
		name   string
		used   bool
		values [4]int
	}{{"duration_seconds", rule.duration, d.Duration}, {"power", rule.power, d.Power}, {"tb_power", rule.tbPower, d.TBPower}, {"range_tiles", rule.rangeTiles, d.Range}} {
		if !ladder.used && ladder.values != [4]int{} {
			return fmt.Errorf("technique %q: kind %s does not use %s", d.Key, d.Kind, ladder.name)
		}
	}
	for i := range d.SPCost {
		if d.SPCost[i] < 1 || rule.duration != (d.Duration[i] > 0) || rule.power != (d.Power[i] > 0) || rule.tbPower != (d.TBPower[i] > 0) || d.Range[i] < 0 {
			return fmt.Errorf("technique %q has invalid tier values", d.Key)
		}
		if d.Duration[i] > 0 {
			if err := ValidateEffectSeconds(fmt.Sprintf("technique %q duration_seconds[%d]", d.Key, i), d.Duration[i]); err != nil {
				return err
			}
		}
		if rule.rangeTiles && d.Range[i] < d.MinRange {
			return fmt.Errorf("technique %q has range below minimum", d.Key)
		}
	}
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

// TechniqueOfKind returns the catalog's technique of a step kind, which
// validation keeps unique; nil when the catalog has none.
func TechniqueOfKind(kind string) *TechniqueDefinition {
	if GlobalTechniques != nil {
		for i := range GlobalTechniques.Techniques {
			if GlobalTechniques.Techniques[i].Kind == kind {
				return &GlobalTechniques.Techniques[i]
			}
		}
	}
	return nil
}

var maxAutocastRank = func() int {
	n := 0
	for _, r := range techniqueKindRules {
		n = max(n, r.autocastRank)
	}
	return n
}()

// AutomaticTechniques yields the autocast-capable techniques in trigger order:
// kind rank, then catalog order.
func AutomaticTechniques() iter.Seq[*TechniqueDefinition] {
	return func(yield func(*TechniqueDefinition) bool) {
		if GlobalTechniques == nil {
			return
		}
		for rank := 1; rank <= maxAutocastRank; rank++ {
			for i := range GlobalTechniques.Techniques {
				d := &GlobalTechniques.Techniques[i]
				if d.Automatic && techniqueKindRules[d.Kind].autocastRank == rank && !yield(d) {
					return
				}
			}
		}
	}
}

const techniquePrefix = "technique:"

// TechniqueID is a technique's scoped identity in the timed-buff registry,
// which spells, draughts and crates share.
func TechniqueID(key string) string { return techniquePrefix + key }

// TechniqueFromID resolves a TechniqueID; nil for any other source.
func TechniqueFromID(id string) *TechniqueDefinition {
	key, ok := strings.CutPrefix(id, techniquePrefix)
	if !ok {
		return nil
	}
	return Technique(key)
}

func TechniqueItem(key string) (items.Item, bool) {
	d := Technique(key)
	if d == nil {
		return items.Item{}, false
	}
	return items.Item{Name: d.Name, Type: items.ItemTechnique, Technique: key, Description: d.Description}, true
}
func TierValue(values [4]int, tier int) int { return values[max(0, min(3, tier))] }
