package monster

import (
	"math"
	"testing"
)

func TestArborealConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*MonsterDefinition)
		bad  bool
	}{
		{"valid", func(*MonsterDefinition) {}, false},
		{"no_trees", func(d *MonsterDefinition) { d.Arboreal.TreeTiles = nil }, true},
		{"zero_time", func(d *MonsterDefinition) { d.Arboreal.JumpSeconds = 0 }, true},
		{"unbounded_search", func(d *MonsterDefinition) { d.Arboreal.JumpRangeTiles = 100 }, true},
		{"nan_height", func(d *MonsterDefinition) { d.Arboreal.HeightTiles = math.NaN() }, true},
		{"hostile", func(d *MonsterDefinition) { d.Disposition = "" }, true},
		{"flying", func(d *MonsterDefinition) { d.Flying = true }, true},
		{"ground_overrides", func(d *MonsterDefinition) { d.WalkableTileOverrides = []string{"wall"} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := MonsterDefinition{SizeClass: "small", Disposition: "wildlife", Arboreal: &ArborealConfig{
				TreeTiles: []string{"tree"}, HeightTiles: 1, JumpRangeTiles: 3, ClimbSeconds: 1, JumpSeconds: 1, RestSeconds: 2,
			}}
			tc.edit(&d)
			err := validateMonsterConfiguration(&MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{"lemur": d}})
			if (err != nil) != tc.bad {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestArborealCoordinateMapping(t *testing.T) {
	original := ArborealState{Phase: "jumping", Height: 1.4, Progress: .3, FromX: 10, FromY: 20, ToX: 30, ToY: 40, GroundX: 50, GroundY: 60}
	placed := original.MapPositions(func(x, y float64) (float64, float64) { return 1000 - y, x + 500 })
	if placed.FromX != 980 || placed.ToX != 960 || placed.GroundX != 940 {
		t.Fatal("an anchor was not transformed")
	}
	local := placed.MapPositions(func(x, y float64) (float64, float64) { return y - 500, 1000 - x })
	if local != original {
		t.Fatalf("open-world transform lost state: %+v", local)
	}
}
