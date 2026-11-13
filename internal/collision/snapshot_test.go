package collision

import (
	"fmt"
	"math/rand"
	"testing"
)

// linearCanMove is the reference implementation the spatial index must match
// exactly: the pre-index full scan over the snapshot's entity map.
func linearCanMove(snap *CollisionSnapshot, movingID string, movingType CollisionType, box *BoundingBox) bool {
	for id, other := range snap.entities {
		if id == movingID || !other.Solid {
			continue
		}
		if shouldIgnoreCollisionTypes(movingType, other.CollisionType) {
			continue
		}
		otherBox := other.Box
		if box.Intersects(&otherBox) {
			return false
		}
	}
	return true
}

// TestSnapshotBucketsMatchLinearScan fuzzes the bucketed entity-passability
// query against the linear reference on randomized entity soups, including
// boxes larger than a bucket, negative coords and exact-touch edges. The
// reused frame snapshot is checked after the population behind it changed:
// entities moved, left and joined since the storage was last filled.
func TestSnapshotBucketsMatchLinearScan(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const tileSize = 64.0

	types := []CollisionType{CollisionTypeMonster, CollisionTypeMonsterEngaged, CollisionTypePlayer, CollisionTypeProjectile}
	sys := NewCollisionSystem(nil, tileSize)
	populate := func(from, to int) {
		for i := from; i < to; i++ {
			w := 16 + rng.Float64()*112 // up to ~2 buckets wide
			h := 16 + rng.Float64()*112
			x := -200 + rng.Float64()*1600
			y := -200 + rng.Float64()*1600
			ct := types[rng.Intn(len(types))]
			solid := rng.Intn(4) != 0
			sys.RegisterEntity(NewEntity(fmt.Sprintf("e%d", i), x, y, w, h, ct, solid))
		}
	}
	populate(0, 150)
	// Exact bucket-boundary touch: two boxes meeting precisely on a tile line.
	sys.RegisterEntity(NewEntity("edgeA", 32, 32, 64, 64, CollisionTypeMonsterEngaged, true))
	sys.RegisterEntity(NewEntity("edgeB", 96, 32, 64, 64, CollisionTypeMonsterEngaged, true))

	check := func(name string, snap *CollisionSnapshot) {
		if snap.buckets == nil {
			t.Fatalf("%s: snapshot with tileSize > 0 must build the spatial index", name)
		}
		fresh := sys.Snapshot()
		for i := 0; i < 3000; i++ {
			movingType := types[rng.Intn(len(types))]
			box := NewBoundingBox(-250+rng.Float64()*1700, -250+rng.Float64()*1700, 8+rng.Float64()*140, 8+rng.Float64()*140)
			movingID := fmt.Sprintf("e%d", rng.Intn(260)) // some IDs don't exist: also valid
			got := snap.canMoveToEntityPosition(movingID, movingType, box)
			want := linearCanMove(snap, movingID, movingType, box)
			if got != want {
				t.Fatalf("%s: bucketed=%v linear=%v for moving=%s type=%v box=%+v", name, got, want, movingID, movingType, *box)
			}
			x, y := -250+rng.Float64()*1700, -250+rng.Float64()*1700
			if got, want := snap.IsMonsterAttackPostReserved(movingID, x, y), fresh.IsMonsterAttackPostReserved(movingID, x, y); got != want {
				t.Fatalf("%s: attack post at (%.0f,%.0f) for %s = %v, a fresh copy says %v", name, x, y, movingID, got, want)
			}
		}
	}
	check("fresh", sys.Snapshot())
	check("frame", sys.FrameSnapshot())

	for i := 0; i < 150; i += 3 {
		sys.UnregisterEntity(fmt.Sprintf("e%d", i))
	}
	for i := 1; i < 150; i += 3 {
		sys.UpdateEntity(fmt.Sprintf("e%d", i), -200+rng.Float64()*1600, -200+rng.Float64()*1600)
	}
	populate(150, 250)
	reused := sys.FrameSnapshot()
	if _, stale := reused.entities["e0"]; stale || len(reused.entities) != len(sys.Snapshot().entities) {
		t.Fatal("the reused snapshot kept an entity that left the system")
	}
	check("frame, reused", reused)
}

// TestSnapshotWithoutTileSizeFallsBackLinear: tileSize 0 snapshots (hand-built
// in tests elsewhere) must keep working without the index.
func TestSnapshotWithoutTileSizeFallsBackLinear(t *testing.T) {
	sys := NewCollisionSystem(nil, 0)
	sys.RegisterEntity(NewEntity("a", 0, 0, 32, 32, CollisionTypeMonsterEngaged, true))
	sys.RegisterEntity(NewEntity("b", 10, 0, 32, 32, CollisionTypeMonsterEngaged, true))
	snap := sys.Snapshot()
	if snap.buckets != nil {
		t.Fatal("tileSize 0 snapshot must not build buckets")
	}
	if snap.canMoveToEntityPosition("a", CollisionTypeMonsterEngaged, NewBoundingBox(10, 0, 32, 32)) {
		t.Fatal("overlap with b must block via the linear fallback")
	}
}
