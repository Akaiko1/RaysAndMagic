package config

import (
	"math"
	"strings"
	"testing"
)

func TestWalkablePropFadeSettings(t *testing.T) {
	valid := DefaultWalkablePropFadeConfig()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		distance, want float64
	}{
		{valid.FromTiles + 1, 1},
		{valid.FromTiles, 1},
		{(valid.FromTiles + valid.ToTiles) / 2, 0.5},
		{valid.ToTiles, 0},
		{0, 0},
	} {
		if got := valid.Opacity(tt.distance); math.Abs(got-tt.want) > 1e-9 {
			t.Fatalf("opacity at %v tiles = %v, want %v", tt.distance, got, tt.want)
		}
	}
	tests := []struct {
		key    string
		change func(*WalkablePropFadeConfig)
	}{
		{"to_tiles", func(c *WalkablePropFadeConfig) { c.ToTiles = 0 }},
		{"to_tiles", func(c *WalkablePropFadeConfig) { c.ToTiles = c.FromTiles }},
		{"to_tiles", func(c *WalkablePropFadeConfig) { c.ToTiles = math.NaN() }},
		{"from_tiles", func(c *WalkablePropFadeConfig) { c.FromTiles = WalkablePropFadeMaxTiles + 1 }},
	}
	for _, tt := range tests {
		c := valid
		tt.change(&c)
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "graphics.walkable_prop_fade."+tt.key) {
			t.Fatalf("%+v: error %v, want one naming %s", c, err, tt.key)
		}
	}
}
