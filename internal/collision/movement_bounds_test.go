package collision

import "testing"

func TestMovementBoundsUseFloorCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float64
		want bool
	}{
		{"interior", 32, 32, true}, {"west_touch", 8, 32, true}, {"north_touch", 32, 8, true},
		{"west_fraction", 7.99, 32, false}, {"north_fraction", 32, 7.99, false},
		{"west_outside", -32, 32, false}, {"north_outside", 32, -32, false},
		{"east_inside", 119.99, 32, true}, {"south_inside", 32, 119.99, true},
		{"east_edge", 120, 32, false}, {"south_edge", 32, 120, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := NewCollisionSystem(newMockTileChecker(2, 2), 64)
			cs.RegisterEntity(NewEntity("actor", 32, 32, 16, 16, CollisionTypeMonster, false))
			snap := cs.Snapshot()
			gotDebug, _ := cs.DebugCanMoveTo("actor", tc.x, tc.y)
			for name, got := range map[string]bool{
				"ordinary":           cs.CanMoveTo("actor", tc.x, tc.y),
				"habitat":            cs.CanMoveToWithHabitat("actor", tc.x, tc.y, nil, false),
				"flying":             cs.CanMoveToWithHabitat("actor", tc.x, tc.y, nil, true),
				"occupancy":          cs.CanOccupyTilesWithHabitat("actor", tc.x, tc.y, nil, false),
				"debug":              gotDebug,
				"snapshot":           snap.CanMoveTo("actor", tc.x, tc.y),
				"snapshot_habitat":   snap.CanMoveToWithHabitat("actor", tc.x, tc.y, nil, false),
				"snapshot_occupancy": snap.CanOccupyTilesWithHabitat("actor", tc.x, tc.y, nil, false),
			} {
				if got != tc.want {
					t.Errorf("%s accepted=%v, want %v", name, got, tc.want)
				}
			}
		})
	}
}
