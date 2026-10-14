package config

import (
	"fmt"
	"os"
	"sort"

	"ugataima/internal/items"

	"gopkg.in/yaml.v3"
)

// TrapDefinitionConfig is one trap from assets/traps.yaml. Damage traps carry
// damage_base (+optional AoE); control traps carry a stun or root duration in
// seconds (whole rounds), which Trapper mastery extends by whole rounds.
type TrapDefinitionConfig struct {
	Name            string  `yaml:"name"`
	Description     string  `yaml:"description"`
	Icon            string  `yaml:"icon"`
	Level           int     `yaml:"level"`              // owner level required
	SPCost          int     `yaml:"sp_cost"`            // spell points to place
	CooldownSeconds float64 `yaml:"cooldown_seconds"`   // RT cooldown after placing
	LifetimeSeconds int     `yaml:"lifetime_seconds"`   // armed trap despawns after this
	Element         string  `yaml:"element"`            // gameplay damage school
	ArmedFx         string  `yaml:"armed_fx,omitempty"` // shader model and activation style
	DamageBase      int     `yaml:"damage_base,omitempty"`
	AoeRadiusTiles  float64 `yaml:"aoe_radius_tiles,omitempty"`
	StunSeconds     int     `yaml:"stun_seconds,omitempty"` // whole rounds, before the Trapper bonus
	RootSeconds     int     `yaml:"root_seconds,omitempty"` // whole rounds, before the Trapper bonus
	BorderColor     [3]int  `yaml:"border_color"`           // model accent / energy colour
}

// TrapSystemConfig is the full traps.yaml document.
type TrapSystemConfig struct {
	Traps map[string]*TrapDefinitionConfig `yaml:"traps"`
}

// Trap placement tuning shared by gameplay and the editor cards.
const (
	// TrapPlaceRangeTiles is how far ahead (facing direction) a trap can be thrown.
	TrapPlaceRangeTiles = 3
	// MaxTrapsPerOwner is how many armed traps one character may have on a map.
	MaxTrapsPerOwner = 3
)

// GlobalTrapConfig is loaded once at startup (LoadTrapConfig).
var GlobalTrapConfig *TrapSystemConfig

// LoadTrapConfig reads and validates assets/traps.yaml.
func LoadTrapConfig(filename string) (*TrapSystemConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read trap config: %w", err)
	}
	var cfg TrapSystemConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse trap config: %w", err)
	}
	if err := validateTrapConfig(&cfg); err != nil {
		return nil, err
	}
	GlobalTrapConfig = &cfg
	return &cfg, nil
}

// MustLoadTrapConfig follows the Must* startup convention of the other loaders.
func MustLoadTrapConfig(filename string) *TrapSystemConfig {
	cfg, err := LoadTrapConfig(filename)
	if err != nil {
		panic(err)
	}
	return cfg
}

func validateTrapConfig(cfg *TrapSystemConfig) error {
	if len(cfg.Traps) == 0 {
		return fmt.Errorf("trap config defines no traps")
	}
	for key, t := range cfg.Traps {
		if t == nil || t.Name == "" {
			return fmt.Errorf("trap %q: missing name", key)
		}
		if t.Icon == "" {
			return fmt.Errorf("trap %q: missing icon", key)
		}
		if t.Element == "" {
			return fmt.Errorf("trap %q: missing element", key)
		}
		element, err := canonicalDamageSchool(t.Element)
		if err != nil {
			return fmt.Errorf("trap %q: unsupported element %q", key, t.Element)
		}
		t.Element = element
		if t.Level <= 0 {
			return fmt.Errorf("trap %q: level must be positive", key)
		}
		if t.SPCost <= 0 {
			return fmt.Errorf("trap %q: sp_cost must be positive", key)
		}
		if t.CooldownSeconds <= 0 {
			return fmt.Errorf("trap %q: cooldown_seconds must be positive", key)
		}
		if t.LifetimeSeconds <= 0 {
			return fmt.Errorf("trap %q: lifetime_seconds must be positive", key)
		}
		if err := validateEffectFields(
			effectField{fmt.Sprintf("trap %q: stun_seconds", key), t.StunSeconds},
			effectField{fmt.Sprintf("trap %q: root_seconds", key), t.RootSeconds},
		); err != nil {
			return err
		}
		if t.AoeRadiusTiles > 0 && t.DamageBase <= 0 {
			return fmt.Errorf("trap %q: aoe_radius_tiles requires damage_base", key)
		}
		if t.DamageBase <= 0 && t.StunSeconds <= 0 && t.RootSeconds <= 0 {
			return fmt.Errorf("trap %q: defines no effect (damage, stun or root)", key)
		}
	}
	return nil
}

// TrapItem builds the quick-slot item form of a trap - the ONE constructor
// (class kits, the trap book and tests all use it). It shares the
// Equipment[SlotQuickAction] slot with spells, techniques and flasks; its cost
// and cooldown always come from traps.yaml.
func TrapItem(key string) (items.Item, bool) {
	def, ok := GetTrapDefinition(key)
	if !ok {
		return items.Item{}, false
	}
	return items.Item{
		Name:        def.Name,
		Type:        items.ItemTrap,
		Description: def.Description,
		Trap:        key,
		Attributes:  make(map[string]int),
	}, true
}

// GetTrapDefinition resolves a trap by its YAML key.
func GetTrapDefinition(key string) (*TrapDefinitionConfig, bool) {
	if GlobalTrapConfig == nil {
		return nil, false
	}
	t, ok := GlobalTrapConfig.Traps[key]
	return t, ok
}

// TrapKeysOrdered returns all trap keys sorted by level (ties by key) - the
// canonical trap-book ordering.
func TrapKeysOrdered() []string {
	if GlobalTrapConfig == nil {
		return nil
	}
	keys := make([]string, 0, len(GlobalTrapConfig.Traps))
	for k := range GlobalTrapConfig.Traps {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := GlobalTrapConfig.Traps[keys[i]], GlobalTrapConfig.Traps[keys[j]]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return keys[i] < keys[j]
	})
	return keys
}
