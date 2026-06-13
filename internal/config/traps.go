package config

import (
	"fmt"
	"os"
	"sort"

	"ugataima/internal/items"

	"gopkg.in/yaml.v3"
)

// TrapDefinitionConfig is one trap from assets/traps.yaml. Damage traps carry
// damage_base (+optional AoE); control traps carry stun or root durations in
// BOTH turn (TB) and second (RT) units — pairs are validated at load.
type TrapDefinitionConfig struct {
	Name            string  `yaml:"name"`
	Description     string  `yaml:"description"`
	Icon            string  `yaml:"icon"`
	Level           int     `yaml:"level"`            // owner level required
	SPCost          int     `yaml:"sp_cost"`          // spell points to place
	CooldownSeconds float64 `yaml:"cooldown_seconds"` // RT cadence after placing
	LifetimeSeconds int     `yaml:"lifetime_seconds"` // armed trap despawns after this
	Element         string  `yaml:"element"`          // particle colour/shape family
	DamageBase      int     `yaml:"damage_base,omitempty"`
	AoeRadiusTiles  float64 `yaml:"aoe_radius_tiles,omitempty"`
	StunTurns       int     `yaml:"stun_turns,omitempty"`
	StunSeconds     int     `yaml:"stun_seconds,omitempty"`
	RootTurns       int     `yaml:"root_turns,omitempty"`
	RootSeconds     int     `yaml:"root_seconds,omitempty"`
	BorderColor     [3]int  `yaml:"border_color"` // armed-tile edge glow
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
		// Duration pairs must be authored in both TB and RT units.
		if (t.StunTurns > 0) != (t.StunSeconds > 0) {
			return fmt.Errorf("trap %q: stun_turns and stun_seconds must be set together", key)
		}
		if (t.RootTurns > 0) != (t.RootSeconds > 0) {
			return fmt.Errorf("trap %q: root_turns and root_seconds must be set together", key)
		}
		if t.AoeRadiusTiles > 0 && t.DamageBase <= 0 {
			return fmt.Errorf("trap %q: aoe_radius_tiles requires damage_base", key)
		}
		if t.DamageBase <= 0 && t.StunTurns <= 0 && t.RootTurns <= 0 {
			return fmt.Errorf("trap %q: defines no effect (damage, stun or root)", key)
		}
	}
	return nil
}

// TrapItem builds the quick-slot item form of a trap — the ONE constructor
// (class kits, the trap book and tests all use it). It lives in the SAME
// Equipment[SlotSpell] slot quick spells use, so the HUD, RT capability
// checks and save round-trip work unchanged.
func TrapItem(key string) (items.Item, bool) {
	def, ok := GetTrapDefinition(key)
	if !ok {
		return items.Item{}, false
	}
	return items.Item{
		Name:        def.Name,
		Type:        items.ItemTrap,
		Description: def.Description,
		SpellCost:   def.SPCost,
		SpellEffect: items.SpellEffect(key),
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

// TrapKeysOrdered returns all trap keys sorted by level (ties by key) — the
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

// EffectLines returns the character-INDEPENDENT mechanic lines of a trap —
// the shared SSoT for the in-game trap book tooltip and the map-editor card
// (same contract as spells.SpellDefinition.EffectLines). Caster-scaled
// numbers (actual damage/duration) are added by each consumer.
func (t *TrapDefinitionConfig) EffectLines() []string {
	var out []string
	if t.DamageBase > 0 {
		out = append(out, fmt.Sprintf("Base damage %d (%s), scales with Intellect & Accuracy", t.DamageBase, t.Element))
	}
	if t.AoeRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Hits everything within %.1f tiles", t.AoeRadiusTiles))
	}
	if t.StunTurns > 0 {
		out = append(out, fmt.Sprintf("Stuns: %d turns / %d sec (+Trapper mastery)", t.StunTurns, t.StunSeconds))
	}
	if t.RootTurns > 0 {
		out = append(out, fmt.Sprintf("Pins in place (no stun): %d turns / %d sec (+Trapper mastery)", t.RootTurns, t.RootSeconds))
	}
	out = append(out, "Triggers when a monster steps on it")
	return out
}
