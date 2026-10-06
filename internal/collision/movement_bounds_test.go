package collision

import "testing"

func TestMovementBoundsLiveAndSnapshot(t *testing.T) {
	sys := NewCollisionSystem(newMockTileChecker(20, 20), 64)
	bound := MovementBounds{Enabled: true, MinX: 128, MinY: 128, MaxX: 512, MaxY: 512}
	entity := NewEntity("boss", 256, 256, 32, 32, CollisionTypeMonster, false).WithMovementBounds(bound)
	sys.RegisterEntity(entity)
	sys.RegisterEntity(NewEntity("free", 256, 256, 32, 32, CollisionTypeMonster, false))
	snapshot := sys.Snapshot()
	for name, check := range map[string]func(string, float64, float64) bool{
		"live": sys.CanMoveTo, "snapshot": snapshot.CanMoveTo,
		"live flight": func(id string, x, y float64) bool {
			return sys.CanMoveToWithTileOverrides(id, x, y, []string{"wall"}, true)
		},
		"snapshot flight": func(id string, x, y float64) bool {
			return snapshot.CanMoveToWithTileOverrides(id, x, y, []string{"wall"}, true)
		},
		"live terrain": func(id string, x, y float64) bool { return sys.CanOccupyTilesWithTileOverrides(id, x, y, nil, false) },
		"snapshot terrain": func(id string, x, y float64) bool {
			return snapshot.CanOccupyTilesWithTileOverrides(id, x, y, nil, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, c := range []struct {
				x, y float64
				want bool
			}{{144, 256, true}, {496, 256, true}, {256, 144, true}, {256, 496, true}, {143, 256, false}, {497, 256, false}, {256, 143, false}, {256, 497, false}} {
				if got := check("boss", c.x, c.y); got != c.want {
					t.Errorf("position %v,%v=%v want %v", c.x, c.y, got, c.want)
				}
				if !check("free", c.x, c.y) {
					t.Error("unconstrained actor acquired arena bounds")
				}
			}
		})
	}
	// Later live changes cannot race with or alter a worker's frozen view.
	entity.WithMovementBounds(MovementBounds{})
	if !sys.CanMoveTo("boss", 100, 256) || snapshot.CanMoveTo("boss", 100, 256) {
		t.Fatal("snapshot shared mutable movement bounds")
	}
}
