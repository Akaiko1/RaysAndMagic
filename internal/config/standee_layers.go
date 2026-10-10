package config

import (
	"fmt"
	"math"
)

// MaxStandeeCoreLayers is the engine's bounded volume-shader loop capacity.
const MaxStandeeCoreLayers = 16

// StandeeLayerBand takes effect at and beyond DistanceTiles.
type StandeeLayerBand struct {
	DistanceTiles float64 `yaml:"distance_tiles"`
	Layers        int     `yaml:"layers"`
}

// CrossedStandeeLayersConfig budgets interiors of natural crossed tiles.
type CrossedStandeeLayersConfig struct {
	DistanceBands           []StandeeLayerBand `yaml:"distance_bands"`
	ForwardHalfWidthTiles   float64            `yaml:"forward_half_width_tiles"`
	OffAxisMaxDistanceTiles float64            `yaml:"off_axis_max_distance_tiles"`
}

// Validate rejects missing budgets, unordered bands and unsupported counts.
func (c CrossedStandeeLayersConfig) Validate() error {
	const field = "graphics.standee.crossed_standee_layers"
	if len(c.DistanceBands) == 0 || c.DistanceBands[0].DistanceTiles != 0 {
		return fmt.Errorf("%s.distance_bands must start at distance_tiles: 0", field)
	}
	for i, band := range c.DistanceBands {
		if math.IsNaN(band.DistanceTiles) || math.IsInf(band.DistanceTiles, 0) || band.DistanceTiles < 0 ||
			(i > 0 && band.DistanceTiles <= c.DistanceBands[i-1].DistanceTiles) {
			return fmt.Errorf("%s.distance_bands[%d] requires a finite, strictly increasing distance", field, i)
		}
		if band.Layers < 0 || band.Layers > MaxStandeeCoreLayers || (i > 0 && band.Layers > c.DistanceBands[i-1].Layers) {
			return fmt.Errorf("%s.distance_bands[%d].layers must be in [0, %d] and not increase", field, i, MaxStandeeCoreLayers)
		}
	}
	if !(c.ForwardHalfWidthTiles > 0) || math.IsInf(c.ForwardHalfWidthTiles, 0) ||
		!(c.OffAxisMaxDistanceTiles >= 0) || math.IsInf(c.OffAxisMaxDistanceTiles, 0) {
		return fmt.Errorf("%s requires a finite positive forward half-width and nonnegative off-axis distance", field)
	}
	return nil
}

// LayerCount uses radial distance and signed camera-relative lateral distance,
// both in tiles. The forward corridor rotates with the view, not the map grid.
// Authored counts intentionally ignore resolution, FOV and slab thickness;
// only other standees retain projected-thickness sampling. With an off-axis
// cutoff <= the corridor half-width, every outside point already exceeds the
// cutoff. The separate knob matters when authoring a narrower corridor or a
// larger off-axis reach, not for every possible combination of settings.
func (c CrossedStandeeLayersConfig) LayerCount(distanceTiles, lateralTiles float64) int {
	if math.Abs(lateralTiles) > c.ForwardHalfWidthTiles && distanceTiles > c.OffAxisMaxDistanceTiles {
		return 0
	}
	for i := len(c.DistanceBands) - 1; i >= 0; i-- {
		if distanceTiles >= c.DistanceBands[i].DistanceTiles {
			return c.DistanceBands[i].Layers
		}
	}
	return 0
}
