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
// boxes larger than a bucket, negative coords and exact-touch edges.
func TestSnapshotBucketsMatchLinearScan(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const tileSize = 64.0

	types := []CollisionType{CollisionTypeMonster, CollisionTypeMonsterEngaged, CollisionTypePlayer, CollisionTypeProjectile}
	sys := NewCollisionSystem(nil, tileSize)
	for i := 0; i < 150; i++ {
		w := 16 + rng.Float64()*112 // up to ~2 buckets wide
		h := 16 + rng.Float64()*112
		x := -200 + rng.Float64()*1600
		y := -200 + rng.Float64()*1600
		ct := types[rng.Intn(len(types))]
		solid := rng.Intn(4) != 0
		sys.RegisterEntity(NewEntity(fmt.Sprintf("e%d", i), x, y, w, h, ct, solid))
	}
	// Exact bucket-boundary touch: two boxes meeting precisely on a tile line.
	sys.RegisterEntity(NewEntity("edgeA", 32, 32, 64, 64, CollisionTypeMonsterEngaged, true))
	sys.RegisterEntity(NewEntity("edgeB", 96, 32, 64, 64, CollisionTypeMonsterEngaged, true))

	snap := sys.Snapshot()
	if snap.buckets == nil {
		t.Fatal("snapshot with tileSize > 0 must build the spatial index")
	}

	for i := 0; i < 3000; i++ {
		movingType := types[rng.Intn(len(types))]
		box := NewBoundingBox(-250+rng.Float64()*1700, -250+rng.Float64()*1700, 8+rng.Float64()*140, 8+rng.Float64()*140)
		movingID := fmt.Sprintf("e%d", rng.Intn(160)) // some IDs don't exist: also valid
		got := snap.canMoveToEntityPosition(movingID, movingType, box)
		want := linearCanMove(snap, movingID, movingType, box)
		if got != want {
			t.Fatalf("bucketed=%v linear=%v for moving=%s type=%v box=%+v", got, want, movingID, movingType, *box)
		}
	}
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
