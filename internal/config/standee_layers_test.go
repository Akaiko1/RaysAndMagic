package config

import (
	"math"
	"testing"
)

func TestCrossedStandeeLayerBudget(t *testing.T) {
	c := CrossedStandeeLayersConfig{
		DistanceBands:         []StandeeLayerBand{{0, 16}, {1, 8}, {2, 4}, {3, 2}, {5, 0}},
		ForwardHalfWidthTiles: 1.5, OffAxisMaxDistanceTiles: 1,
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		distance, lateral float64
		want              int
	}{
		{0, 0, 16}, {.999, 0, 16}, {1, 0, 8}, {1.999, 0, 8},
		{2, 0, 4}, {2.999, 0, 4}, {3, 0, 2}, {4.999, 0, 2}, {5, 0, 0}, {30, 0, 0},
		{3, 1.5, 2}, {3, -1.5, 2}, {3, 1.501, 0}, {3, -1.501, 0},
	} {
		if got := c.LayerCount(tc.distance, tc.lateral); got != tc.want {
			t.Errorf("distance=%g lateral=%g: got %d layers, want %d", tc.distance, tc.lateral, got, tc.want)
		}
	}
	// Use a narrower authored corridor so the off-axis radius boundary is
	// geometrically reachable; in a three-lane corridor it lies inside it.
	c.ForwardHalfWidthTiles = .25
	if c.LayerCount(1, .5) != 8 || c.LayerCount(1.001, .5) != 0 {
		t.Fatal("off-axis cutoff must preserve the exact radius boundary")
	}
}

func TestCrossedStandeeLayerValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*CrossedStandeeLayersConfig)
	}{
		{"missing", func(c *CrossedStandeeLayersConfig) { c.DistanceBands = nil }},
		{"no origin", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[0].DistanceTiles = 1 }},
		{"duplicate", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[1].DistanceTiles = 0 }},
		{"nan distance", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[1].DistanceTiles = math.NaN() }},
		{"infinite distance", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[1].DistanceTiles = math.Inf(1) }},
		{"too many", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[0].Layers = MaxStandeeCoreLayers + 1 }},
		{"negative", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[1].Layers = -1 }},
		{"increasing", func(c *CrossedStandeeLayersConfig) { c.DistanceBands[1].Layers = 9 }},
		{"zero width", func(c *CrossedStandeeLayersConfig) { c.ForwardHalfWidthTiles = 0 }},
		{"nan width", func(c *CrossedStandeeLayersConfig) { c.ForwardHalfWidthTiles = math.NaN() }},
		{"negative cutoff", func(c *CrossedStandeeLayersConfig) { c.OffAxisMaxDistanceTiles = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := CrossedStandeeLayersConfig{DistanceBands: []StandeeLayerBand{{0, 8}, {2, 4}}, ForwardHalfWidthTiles: 1, OffAxisMaxDistanceTiles: 1}
			tc.edit(&c)
			if c.Validate() == nil {
				t.Fatal("invalid budget accepted")
			}
		})
	}
}
