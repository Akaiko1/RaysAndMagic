package collision

import (
	"fmt"
	"testing"
)

func TestSightRayReciprocalCornersAndLongSegments(t *testing.T) {
	for _, length := range []int{1, 12} {
		for _, sx := range []int{-1, 1} {
			for _, sy := range []int{-1, 1} {
				for _, obstacle := range []string{"clear", "x corner", "y corner", "door", "late wall"} {
					t.Run(fmt.Sprintf("length=%d/%d,%d/%s", length, sx, sy, obstacle), func(t *testing.T) {
						tiles := newMockTileChecker(40, 40)
						ax, ay := 20, 20
						bx, by := ax+sx*length, ay+sy*length
						cs := NewCollisionSystem(tiles, 64)
						switch obstacle {
						case "x corner":
							tiles.setOpaque(ax+sx, ay, true)
						case "y corner":
							tiles.setOpaque(ax, ay+sy, true)
						case "door":
							cs.RegisterEntity(NewSightBlockingEntity("door", float64(ax+sx)*64+32, float64(ay)*64+32, 56, 56, CollisionTypeNPC, true))
						case "late wall":
							tiles.setOpaque(bx, by, true)
						}
						snap := cs.Snapshot()
						for _, reverse := range []bool{false, true} {
							if obstacle == "late wall" && reverse {
								continue
							} // Sight deliberately sees out of its own tile.
							x1, y1, x2, y2 := float64(ax)*64+32, float64(ay)*64+32, float64(bx)*64+32, float64(by)*64+32
							if reverse {
								x1, x2 = x2, x1
								y1, y2 = y2, y1
							}
							want := obstacle == "clear"
							if cs.CheckLineOfSight(x1, y1, x2, y2) != want || snap.CheckLineOfSight(x1, y1, x2, y2) != want {
								t.Fatalf("reverse=%v: live and snapshot must report clear=%v", reverse, want)
							}
						}
					})
				}
			}
		}
	}
}

func TestAttackEndpointsLiveAndSnapshot(t *testing.T) {
	for _, endpoint := range []string{"source", "target"} {
		for _, dynamic := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dynamic=%v", endpoint, dynamic), func(t *testing.T) {
				tiles := newMockTileChecker(10, 10)
				cs := NewCollisionSystem(tiles, 64)
				x1, y1, x2, y2 := 2.5*64, 2.5*64, 4.5*64, 2.5*64
				x, y := x1, y1
				if endpoint == "target" {
					x, y = x2, y2
				}
				if dynamic {
					cs.RegisterEntity(NewSightBlockingEntity("door", x, y, 56, 56, CollisionTypeNPC, true))
				} else {
					tiles.setOpaque(int(x/64), int(y/64), true)
				}
				for _, checker := range []SightChecker{cs, cs.Snapshot()} {
					if CanAttackFrom(checker, x, y) || AttackLineClear(checker, x1, y1, x2, y2) || AttackLineClear(checker, x2, y2, x1, y1) {
						t.Fatal("blocked endpoint allowed an attack")
					}
				}
			})
		}
	}
}

// Door writes and live attack-origin reads share sightMu. Immutable snapshots
// keep the same endpoint contract without needing a lock.
func TestAttackOriginConcurrentDoorMovement(t *testing.T) {
	cs := NewCollisionSystem(newMockTileChecker(10, 10), 64)
	cs.RegisterEntity(NewSightBlockingEntity("door", 160, 160, 56, 56, CollisionTypeNPC, true))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			cs.UpdateEntity("door", float64(2+i%2)*64+32, 160)
		}
	}()
	for i := 0; i < 2000; i++ {
		cs.CanAttackFrom(160, 160)
		AttackLineClear(cs, 96, 160, 288, 160)
	}
	<-done
}

func BenchmarkAttackVisibility(b *testing.B) {
	cs := NewCollisionSystem(newMockTileChecker(40, 40), 64)
	for _, live := range []bool{true, false} {
		var checker SightChecker = cs
		if !live {
			checker = cs.Snapshot()
		}
		for _, batched := range []bool{false, true} {
			b.Run(fmt.Sprintf("live=%v/batched=%v", live, batched), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if batched {
						AttackLineClear(checker, 160, 160, 2080, 2080)
					} else {
						_ = CanAttackFrom(checker, 160, 160) && CanAttackFrom(checker, 2080, 2080) && checker.CheckLineOfSight(160, 160, 2080, 2080) && checker.CheckLineOfSight(2080, 2080, 160, 160)
					}
				}
			})
		}
	}
}
