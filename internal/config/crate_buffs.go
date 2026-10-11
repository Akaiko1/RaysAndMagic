package config

import (
	"fmt"
	"strings"
)

// Crate buffs share the party combat clock and regeneration cadence.
type CrateBuffConfig struct {
	Name            string `yaml:"name"`
	StatusIcon      string `yaml:"status_icon"`
	DurationSeconds int    `yaml:"duration_seconds"`
	HPRegenPct      int    `yaml:"hp_regen_pct,omitempty"`
	ManaRegenPct    int    `yaml:"mana_regen_pct,omitempty"`
}

func validateCrateBuff(key string, b *CrateBuffConfig) error {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.StatusIcon) == "" || b.DurationSeconds <= 0 || b.HPRegenPct < 0 || b.HPRegenPct > 100 || b.ManaRegenPct < 0 || b.ManaRegenPct > 100 || b.HPRegenPct+b.ManaRegenPct == 0 {
		return fmt.Errorf("crate %q: buff needs name, status_icon, positive duration_seconds and regeneration in 0..100 with at least one positive resource", key)
	}
	return nil
}

const cratePrefix = "crate:"

// CrateID is a crate's scoped identity: its status buff and its adventure loot slot.
func CrateID(key string) string { return cratePrefix + key }

func CrateBuff(id string) *CrateBuffConfig {
	key, ok := strings.CutPrefix(id, cratePrefix)
	if !ok {
		return nil
	}
	if c := GetCrateConfig(key); c != nil {
		return c.Buff
	}
	return nil
}

func (b *CrateBuffConfig) EffectLines() []string {
	// Reuse the same wording and stacking explanation as timed draughts.
	d := ItemDefinitionConfig{BuffDurationSeconds: b.DurationSeconds, BuffHPRegenPct: b.HPRegenPct, BuffManaRegenPct: b.ManaRegenPct}
	return d.TimedBuffEffectLines()
}
