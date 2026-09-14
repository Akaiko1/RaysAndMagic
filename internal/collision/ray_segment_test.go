package collision

import "testing"

func TestRayStopsAtSameCellEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name           string
		x1, y1, x2, y2 float64
	}{
		{"west_edge", 10, 0, 0, 0},
		{"east_wall_beyond_target", 1, 32, 63, 32},
		{"north_edge", 32, 10, 32, 0},
		{"diagonal", 1, 1, 63, 63},
		{"zero", 32, 32, 32, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := newMockTileChecker(2, 2)
			checker.setOpaque(1, 0, true)
			checker.setOpaque(0, 1, true)
			checker.setOpaque(1, 1, true)
			cs := NewCollisionSystem(checker, 64)
			for name, los := range map[string]func(float64, float64, float64, float64) bool{"live": cs.CheckLineOfSight, "snapshot": cs.Snapshot().CheckLineOfSight} {
				if !los(tc.x1, tc.y1, tc.x2, tc.y2) {
					t.Fatalf("%s ray inspected terrain beyond its endpoint", name)
				}
			}
		})
	}
}
