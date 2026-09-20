package config

import (
	"fmt"
	"math"
)

// Fish are short local encounters, not persistent roaming populations.
type FishSpawnConfig struct {
	Species            map[string]string `yaml:"species"`
	RollEveryFrames    int               `yaml:"roll_every_frames"`
	SpawnChancePerTile float64           `yaml:"spawn_chance_per_tile"`
	FlightSeconds      float64           `yaml:"flight_seconds"`
	HeightTiles        float64           `yaml:"height_tiles"`
	RadiusTiles        float64           `yaml:"radius_tiles"`
	BeachChance        float64           `yaml:"beach_chance"`
}

func (f *FishSpawnConfig) Validate() error {
	for _, value := range []float64{f.FlightSeconds, f.HeightTiles, f.RadiusTiles} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("fish timing, height and radius must be finite and positive")
		}
	}
	if f.RollEveryFrames <= 0 || f.RadiusTiles < 2 || f.RadiusTiles > 16 || f.HeightTiles > 3 {
		return fmt.Errorf("invalid fish roll cadence, height or radius bounds")
	}
	if math.IsNaN(f.SpawnChancePerTile) || f.SpawnChancePerTile < 0 || f.SpawnChancePerTile > 1 {
		return fmt.Errorf("fish spawn_chance_per_tile must be in [0,1]")
	}
	if math.IsNaN(f.BeachChance) || f.BeachChance < 0 || f.BeachChance > 1 {
		return fmt.Errorf("fish beach_chance must be in [0,1]")
	}
	if len(f.Species) == 0 {
		return fmt.Errorf("fish needs map-to-monster species")
	}
	for region, species := range f.Species {
		if region == "" || species == "" {
			return fmt.Errorf("fish species needs map and monster keys")
		}
	}
	return nil
}
