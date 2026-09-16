package config

import (
	"fmt"
	"strings"
)

// ItemTooltipUsageConfig contains wording only. Item properties determine which
// messages apply, and per-item tooltip_usage remains an explicit override.
type ItemTooltipUsageConfig struct {
	Card                   []string `yaml:"card"`
	Trinket                string   `yaml:"trinket"`
	ActivateInventory      string   `yaml:"activate_inventory"`
	ConsumedOnUse          string   `yaml:"consumed_on_use"`
	ConsumedAfterPromotion string   `yaml:"consumed_after_promotion"`
	CannotSell             string   `yaml:"cannot_sell"`
	CannotDrop             string   `yaml:"cannot_drop"`
}

func (c *ItemTooltipUsageConfig) validate() error {
	for _, field := range []struct {
		key   string
		lines []string
	}{
		{"card", c.Card},
		{"trinket", []string{c.Trinket}},
		{"activate_inventory", []string{c.ActivateInventory}},
		{"consumed_on_use", []string{c.ConsumedOnUse}},
		{"consumed_after_promotion", []string{c.ConsumedAfterPromotion}},
		{"cannot_sell", []string{c.CannotSell}},
		{"cannot_drop", []string{c.CannotDrop}},
	} {
		if len(field.lines) == 0 {
			return fmt.Errorf("tooltip_usage_defaults.%s: at least one line is required", field.key)
		}
		for _, line := range field.lines {
			if strings.TrimSpace(line) == "" {
				return fmt.Errorf("tooltip_usage_defaults.%s: text must not be blank", field.key)
			}
			for _, r := range line {
				if r > 127 {
					return fmt.Errorf("tooltip_usage_defaults.%s: text must be ASCII", field.key)
				}
			}
		}
	}
	return nil
}
