package monster

import (
	"fmt"
	"testing"
)

type targetTileChecker struct{ *MockCollisionChecker }

func (c targetTileChecker) IsMonsterAttackPostReserved(_ string, x, y float64) bool {
	return worldToTile(x) != 6 || worldToTile(y) != 5
}

func TestPursuitNeverRoutesThroughAttackTarget(t *testing.T) {
	for _, path := range []string{"rt", "tb", "tb_goals", "cached_rt", "rt_overlap"} {
		for _, ranged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ranged=%v", path, ranged), func(t *testing.T) {
				checker := targetTileChecker{NewMockCollisionChecker(defaultTileSize)}
				x, y := tileToWorldCenter(4, 5)
				tx, ty := tileToWorldCenter(5, 5)
				m := &Monster3D{ID: "pursuer", X: x, Y: y, HitPoints: 10, MaxHitPoints: 10, Speed: 2, AttackRadius: defaultTileSize, AlertRadius: 4 * defaultTileSize, State: StatePursuing, IsEngagingPlayer: true}
				if ranged {
					m.ProjectileSpell = "firebolt"
					m.RangedAttackRange = defaultTileSize
				}
				if path == "rt_overlap" {
					m.X, m.Y = tx, ty
				}
				switch path {
				case "tb", "tb_goals":
					var nx, ny int
					var ok bool
					if path == "tb" {
						nx, ny, ok = m.NextPathStepTile(checker, tx, ty)
					} else {
						nx, ny, ok = m.NextPathStepTileToAny(checker, []TileCoord{{X: 6, Y: 5}}, &TileCoord{X: 5, Y: 5})
					}
					if !ok {
						t.Fatal("legal detour was not found")
					}
					if nx == 5 && ny == 5 {
						t.Fatal("A* entered the occupied target tile")
					}
				default:
					if path == "cached_rt" {
						// A route cached before the target moved here must not bypass the guard.
						m.PathTiles = []TileCoord{{X: 4, Y: 5}, {X: 5, Y: 5}, {X: 6, Y: 5}}
						m.PathIndex = 1
						m.PathTargetTileX, m.PathTargetTileY = 5, 5
					}
					leftTarget := path != "rt_overlap"
					for tick := 0; tick < 240; tick++ {
						m.StateTimer++
						m.followPathToTarget(checker, tx, ty)
						inside := worldToTile(m.X) == 5 && worldToTile(m.Y) == 5
						if !inside {
							leftTarget = true
						}
						if inside && leftTarget {
							t.Fatalf("interpolation entered target tile at tick %d", tick)
						}
					}
					if !leftTarget {
						t.Fatal("overlapped actor could not leave target tile")
					}
				}
			})
		}
	}
}
