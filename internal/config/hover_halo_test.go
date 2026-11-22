package config

import (
	"math"
	"strings"
	"testing"
)

func TestHoverHaloSettings(t *testing.T) {
	valid := DefaultHoverHaloConfig()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	hidden := valid
	hidden.Strength = 0
	if err := hidden.Validate(); err != nil {
		t.Fatalf("strength 0 must be the way to hide the halo: %v", err)
	}
	tests := []struct {
		key    string
		change func(*HoverHaloConfig)
	}{
		{"color", func(c *HoverHaloConfig) { c.Color[1] = 256 }},
		{"color", func(c *HoverHaloConfig) { c.Color[2] = -1 }},
		{"width_px", func(c *HoverHaloConfig) { c.WidthPx = 0 }},
		{"width_px", func(c *HoverHaloConfig) { c.WidthPx = HoverHaloMaxWidthPx + 1 }},
		{"width_px", func(c *HoverHaloConfig) { c.WidthPx = math.NaN() }},
		{"strength", func(c *HoverHaloConfig) { c.Strength = -0.1 }},
		{"strength", func(c *HoverHaloConfig) { c.Strength = 1.5 }},
		{"strength", func(c *HoverHaloConfig) { c.Strength = math.NaN() }},
	}
	for _, tt := range tests {
		c := valid
		tt.change(&c)
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "graphics.monster.hover_halo."+tt.key) {
			t.Fatalf("%+v: error %v, want one naming %s", c, err, tt.key)
		}
	}
}
