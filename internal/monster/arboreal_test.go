package monster

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
	"ugataima/internal/config"
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

func TestArborealSaveCoordinateFormats(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       ArborealState
	}{
		{"legacy", `{"phase":"jumping","FromX":1,"FromY":2,"ToX":3,"ToY":4,"GroundX":5,"GroundY":6}`, ArborealState{Phase: "jumping", FromX: 1, FromY: 2, ToX: 3, ToY: 4, GroundX: 5, GroundY: 6}},
		{"current", `{"phase":"perched","from_x":1,"from_y":2,"to_x":3,"to_y":4,"ground_x":5,"ground_y":6}`, ArborealState{Phase: "perched", FromX: 1, FromY: 2, ToX: 3, ToY: 4, GroundX: 5, GroundY: 6}},
		{"mixed_zero_wins", `{"FromX":9,"from_x":0,"FromY":2,"to_x":3}`, ArborealState{FromY: 2, ToX: 3}},
		{"empty", `{}`, ArborealState{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s ArborealState
			if err := json.Unmarshal([]byte(tc.data), &s); err != nil {
				t.Fatal(err)
			}
			if s != tc.want {
				t.Fatalf("decoded=%+v, want %+v", s, tc.want)
			}
			encoded, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			for _, old := range []string{"FromX", "FromY", "ToX", "ToY", "GroundX", "GroundY"} {
				if bytes.Contains(encoded, []byte(old)) {
					t.Fatalf("legacy key written: %s", encoded)
				}
			}
			if tc.name == "empty" && string(encoded) != "{}" {
				t.Fatalf("zero coordinates not omitted: %s", encoded)
			}
			var restored ArborealState
			if err := json.Unmarshal(encoded, &restored); err != nil || restored != s {
				t.Fatalf("round trip: %+v %v", restored, err)
			}
		})
	}
}

type treeSearchChecker struct {
	*MockCollisionChecker
	blocked  bool
	landings int
}

func (c *treeSearchChecker) CanOccupyTilesWithTileOverrides(_ string, x, y float64, overrides []string, _ bool) bool {
	if x == 96 && y == 32 {
		return len(overrides) > 0
	}
	return !(c.blocked && y == 32 && x >= 64 && x < 96)
}
func (c *treeSearchChecker) CanMoveToWithTileOverrides(_ string, _, _ float64, _ []string, _ bool) bool {
	c.landings++
	return true
}

func TestArborealRejectsBlockedRouteBeforeLandingSearch(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		c := &treeSearchChecker{MockCollisionChecker: NewMockCollisionChecker(64), blocked: blocked}
		m := &Monster3D{ID: "lemur", X: 32, Y: 32, Speed: 1, HitPoints: 1, config: &config.Config{World: config.WorldConfig{TileSize: 64}}, Arboreal: &ArborealConfig{TreeTiles: []string{"tree"}, HeightTiles: 1, JumpRangeTiles: 3, ClimbSeconds: 1, RestSeconds: 1}}
		m.updateArboreal(c, 0, 0, false)
		if blocked && c.landings != 0 {
			t.Fatalf("blocked candidate searched %d landing cells", c.landings)
		}
		if !blocked && (c.landings == 0 || m.Arbor.Phase != "climbing") {
			t.Fatal("clear candidate did not begin climbing")
		}
	}
}
