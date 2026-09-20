package config

import (
	"math"
	"testing"
)

func TestFishSpawnValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*FishSpawnConfig)
		valid bool
	}{
		{"valid", func(*FishSpawnConfig) {}, true},
		{"zero cadence", func(f *FishSpawnConfig) { f.RollEveryFrames = 0 }, false},
		{"negative cadence", func(f *FishSpawnConfig) { f.RollEveryFrames = -1 }, false},
		{"overlapping flights", func(f *FishSpawnConfig) { f.FlightSeconds = 30 }, true},
		{"disabled rolls", func(f *FishSpawnConfig) { f.SpawnChancePerTile = 0 }, true},
		{"certain rolls", func(f *FishSpawnConfig) { f.SpawnChancePerTile = 1 }, true},
		{"nan spawn chance", func(f *FishSpawnConfig) { f.SpawnChancePerTile = math.NaN() }, false},
		{"negative spawn chance", func(f *FishSpawnConfig) { f.SpawnChancePerTile = -.1 }, false},
		{"excessive spawn chance", func(f *FishSpawnConfig) { f.SpawnChancePerTile = 1.1 }, false},
		{"nonfinite duration", func(f *FishSpawnConfig) { f.FlightSeconds = math.Inf(1) }, false},
		{"nan radius", func(f *FishSpawnConfig) { f.RadiusTiles = math.NaN() }, false},
		{"unbounded radius", func(f *FishSpawnConfig) { f.RadiusTiles = 1000 }, false},
		{"negative chance", func(f *FishSpawnConfig) { f.BeachChance = -.01 }, false},
		{"nan chance", func(f *FishSpawnConfig) { f.BeachChance = math.NaN() }, false},
		{"missing species", func(f *FishSpawnConfig) { f.Species = nil }, false},
		{"empty key", func(f *FishSpawnConfig) { f.Species = map[string]string{"forest": ""} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := FishSpawnConfig{Species: map[string]string{"forest": "common_carp"}, RollEveryFrames: 120, SpawnChancePerTile: .0135, FlightSeconds: 1.8, HeightTiles: .7, RadiusTiles: 7, BeachChance: .06}
			tc.edit(&f)
			if err := f.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
