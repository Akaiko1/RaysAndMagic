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

// PassabilityView answers passability, sight and attack-post questions as the
// live system does after every kind of change, and keeps its storage across
// changes that cannot alter an answer (a body that blocks nobody moving).
func TestPassabilityViewAnswersAsTheLiveSystem(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const tileSize = 64.0
	tiles := newMockTileChecker(24, 24)
	for i := 0; i < 60; i++ {
		tiles.setBlocking(rng.Intn(24), rng.Intn(24), true)
	}
	sys := NewCollisionSystem(tiles, tileSize)
	types := []CollisionType{CollisionTypeMonster, CollisionTypeMonsterEngaged, CollisionTypePlayer, CollisionTypeNPC}
	spot := func() (float64, float64) { return rng.Float64() * 24 * tileSize, rng.Float64() * 24 * tileSize }
	for i := 0; i < 60; i++ {
		x, y := spot()
		sys.RegisterEntity(NewEntity(fmt.Sprintf("e%d", i), x, y, 20+rng.Float64()*60, 20+rng.Float64()*60, types[rng.Intn(len(types))], i%3 == 0))
	}
	x, y := spot()
	sys.RegisterEntity(NewSightBlockingEntity("door", x, y, 64, 64, CollisionTypeNPC, true))
	sys.RegisterEntity(NewEntity("calm", 300, 300, 40, 40, CollisionTypeMonster, false))
	sys.RegisterEntity(NewEntity("post", 500, 500, 40, 40, CollisionTypeMonsterEngaged, false))

	same := func(step string) {
		t.Helper()
		view := sys.PassabilityView()
		for i := 0; i < 1500; i++ {
			id := fmt.Sprintf("e%d", rng.Intn(64)) // some ids left or never came
			x, y := spot()
			if got, want := view.CanMoveToWithTileOverrides(id, x, y, nil, false), sys.CanMoveToWithTileOverrides(id, x, y, nil, false); got != want {
				t.Fatalf("%s: %s to (%.0f,%.0f): the view says %v, the live system %v", step, id, x, y, got, want)
			}
			if got, want := view.IsMonsterAttackPostReserved(id, x, y), sys.IsMonsterAttackPostReserved(id, x, y); got != want {
				t.Fatalf("%s: attack post at (%.0f,%.0f) for %s: the view says %v, the live system %v", step, x, y, id, got, want)
			}
			x2, y2 := spot()
			if got, want := view.CheckLineOfSight(x, y, x2, y2), sys.CheckLineOfSight(x, y, x2, y2); got != want {
				t.Fatalf("%s: sight (%.0f,%.0f)-(%.0f,%.0f): the view says %v, the live system %v", step, x, y, x2, y2, got, want)
			}
		}
	}
	same("as built")
	// e0 is solid; "calm" blocks nobody; "post" marks an attack post.
	for _, step := range []struct {
		name   string
		change func()
		refill bool
	}{
		{"a solid body moves", func() { x, y := spot(); sys.UpdateEntity("e0", x, y) }, true},
		{"a body that blocks nobody moves", func() { x, y := spot(); sys.UpdateEntity("calm", x, y) }, false},
		{"an attack post moves", func() { x, y := spot(); sys.UpdateEntity("post", x, y) }, true},
		{"the sight blocker moves", func() { x, y := spot(); sys.UpdateEntity("door", x, y) }, true},
		{"a type changes", func() { sys.SetEntityCollisionType("e3", CollisionTypePlayer) }, true},
		{"a body stops blocking", func() { sys.SetEntitySolid("e6", false) }, true},
		{"a body starts blocking", func() { sys.SetEntitySolid("e7", true) }, true},
		{"a body keeps its solidity", func() { sys.SetEntitySolid("e7", true) }, false},
		{"bounds close in", func() {
			sys.SetMovementBounds("e2", MovementBounds{Enabled: true, MinX: 200, MinY: 200, MaxX: 900, MaxY: 900})
		}, true},
		{"a body leaves", func() { sys.UnregisterEntity("e9") }, true},
		{"a body joins", func() {
			x, y := spot()
			sys.RegisterEntity(NewEntity("e61", x, y, 70, 70, CollisionTypeNPC, true))
		}, true},
		{"the map changes", func() {
			other := newMockTileChecker(24, 24)
			other.setBlocking(5, 5, true)
			sys.UpdateTileChecker(other)
		}, true},
	} {
		sys.PassabilityView()
		before := sys.passabilityAt
		step.change()
		same(step.name)
		if refilled := sys.passabilityAt != before; refilled != step.refill {
			t.Errorf("%s: view refilled = %v, want %v", step.name, refilled, step.refill)
		}
	}
}
