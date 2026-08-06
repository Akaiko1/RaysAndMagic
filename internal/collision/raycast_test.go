package collision

import (
	"math"
	"sync"
	"testing"
)

// mockTileChecker implements TileChecker for testing
type mockTileChecker struct {
	width, height int
	blockingTiles map[int]map[int]bool
	opaqueTiles   map[int]map[int]bool
}

func newMockTileChecker(width, height int) *mockTileChecker {
	return &mockTileChecker{
		width:         width,
		height:        height,
		blockingTiles: make(map[int]map[int]bool),
		opaqueTiles:   make(map[int]map[int]bool),
	}
}

func (m *mockTileChecker) IsTileBlocking(tileX, tileY int) bool {
	if row, ok := m.blockingTiles[tileY]; ok {
		return row[tileX]
	}
	return false
}

func (m *mockTileChecker) IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string, flying bool) bool {
	return m.IsTileBlocking(tileX, tileY)
}

func (m *mockTileChecker) IsTileOpaque(tileX, tileY int) bool {
	if row, ok := m.opaqueTiles[tileY]; ok {
		return row[tileX]
	}
	return false
}

func (m *mockTileChecker) GetWorldBounds() (width, height int) {
	return m.width, m.height
}

func (m *mockTileChecker) setBlocking(tileX, tileY int, blocking bool) {
	if m.blockingTiles[tileY] == nil {
		m.blockingTiles[tileY] = make(map[int]bool)
	}
	m.blockingTiles[tileY][tileX] = blocking
}

func (m *mockTileChecker) setOpaque(tileX, tileY int, opaque bool) {
	if m.opaqueTiles[tileY] == nil {
		m.opaqueTiles[tileY] = make(map[int]bool)
	}
	m.opaqueTiles[tileY][tileX] = opaque
}

func TestCastRay_HorizontalLine(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Test horizontal line from (0,0) to (320,0) - should cross 5 tiles
	x1, y1 := 0.0, 32.0
	x2, y2 := 320.0, 32.0

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for clear horizontal line")
	}

	// Add an opaque tile at position (3, 0) - tile coordinates
	checker.setOpaque(3, 0, true)

	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit for horizontal line with opaque tile")
	} else if hit.TileX != 3 || hit.TileY != 0 {
		t.Errorf("Expected hit at tile (3, 0), got (%d, %d)", hit.TileX, hit.TileY)
	}
}

func TestCastRay_VerticalLine(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Test vertical line from (32,0) to (32,320) - should cross 5 tiles
	x1, y1 := 32.0, 0.0
	x2, y2 := 32.0, 320.0

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for clear vertical line")
	}

	// Add an opaque tile at position (0, 3) - tile coordinates
	checker.setOpaque(0, 3, true)

	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit for vertical line with opaque tile")
	} else if hit.TileX != 0 || hit.TileY != 3 {
		t.Errorf("Expected hit at tile (0, 3), got (%d, %d)", hit.TileX, hit.TileY)
	}
}

func TestCastRay_DiagonalLine(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Test diagonal line from (0,0) to (192,192) - 45 degree angle
	x1, y1 := 0.0, 0.0
	x2, y2 := 192.0, 192.0

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for clear diagonal line")
	}

	// Add an opaque tile at position (1, 1) - should be crossed by diagonal
	checker.setOpaque(1, 1, true)

	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit for diagonal line with opaque tile")
	} else if hit.TileX != 1 || hit.TileY != 1 {
		t.Errorf("Expected hit at tile (1, 1), got (%d, %d)", hit.TileX, hit.TileY)
	}
}

func TestCastRay_OutOfBounds(t *testing.T) {
	checker := newMockTileChecker(5, 5)
	cs := NewCollisionSystem(checker, 64.0)

	// Test ray that goes out of bounds
	x1, y1 := 32.0, 32.0
	x2, y2 := 500.0, 32.0 // Goes beyond world boundary

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit when ray goes out of bounds")
	}
}

// TestCastRay_SightSeesOutOfOpaqueStartTile guards the flying-ranged-mob fix: an
// observer perched on an opaque tile (a flying mob on a solid-but-transparent
// sprite, e.g. boulder/canopy) must still have line of sight OUT of its own
// tile. The start tile's opacity is ignored for sight; opacity DOWNSTREAM still
// blocks. Movement, by contrast, is still blocked by an impassable start tile.
func TestCastRay_SightSeesOutOfOpaqueStartTile(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Observer stands on an opaque tile, clear path to the target.
	checker.setOpaque(0, 0, true)

	x1, y1 := 32.0, 32.0 // Center of tile (0,0)
	x2, y2 := 192.0, 32.0

	if _, hasHit := cs.CastRay(x1, y1, x2, y2, true); hasHit {
		t.Errorf("Expected clear sight out of an opaque start tile, got a hit")
	}

	// An opaque tile further along the ray still blocks sight.
	checker.setOpaque(2, 0, true)
	if hit, hasHit := cs.CastRay(x1, y1, x2, y2, true); !hasHit {
		t.Errorf("Expected downstream opaque tile to block sight")
	} else if hit.TileX != 2 || hit.TileY != 0 {
		t.Errorf("Expected block at tile (2,0), got (%d,%d)", hit.TileX, hit.TileY)
	}

	// Movement is still blocked when the start tile is impassable.
	checker.setBlocking(0, 0, true)
	if hit, hasHit := cs.CastRay(x1, y1, x2, y2, false); !hasHit || hit.Dist != 0 {
		t.Errorf("Expected immediate movement hit on impassable start tile")
	}
}

func TestCastRay_ZeroDistance(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Test ray with zero distance (same start and end point)
	x1, y1 := 32.0, 32.0
	x2, y2 := 32.0, 32.0

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for zero distance ray in clear tile")
	}

	// A sight ray sees its own tile even when opaque (observer sees out of it).
	checker.setOpaque(0, 0, true)
	if _, hasHit := cs.CastRay(x1, y1, x2, y2, true); hasHit {
		t.Errorf("Expected no sight hit for zero distance ray in own (opaque) tile")
	}

	// Movement, however, is blocked by an impassable tile at zero distance.
	checker.setBlocking(0, 0, true)
	if hit, hasHit := cs.CastRay(x1, y1, x2, y2, false); !hasHit {
		t.Errorf("Expected movement hit for zero distance ray in blocking tile")
	} else if hit.Dist != 0 {
		t.Errorf("Expected distance 0 for immediate hit in blocking tile, got %f", hit.Dist)
	}
}

func TestCastRay_BlockingVsOpaque(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Set up a tile that blocks movement but not sight
	checker.setBlocking(2, 0, true)
	checker.setOpaque(2, 0, false)

	x1, y1 := 32.0, 32.0
	x2, y2 := 192.0, 32.0

	// Test with sightOnly=true (should NOT hit the blocking-but-transparent tile)
	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for sight ray through blocking-but-transparent tile")
	}

	// Test with sightOnly=false (should hit the blocking tile)
	hit, hasHit := cs.CastRay(x1, y1, x2, y2, false)
	if !hasHit {
		t.Errorf("Expected hit for movement ray through blocking tile")
	} else if hit.TileX != 2 || hit.TileY != 0 {
		t.Errorf("Expected hit at tile (2, 0), got (%d, %d)", hit.TileX, hit.TileY)
	}
}

func TestCastRay_DistanceCalculation(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Place an opaque tile at (2, 0)
	checker.setOpaque(2, 0, true)

	x1, y1 := 32.0, 32.0  // Center of tile (0,0)
	x2, y2 := 320.0, 32.0 // Should hit the opaque tile before reaching end

	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit for ray with opaque tile")
	}

	// Calculate expected distance (should be reasonably close to 2 tiles distance)
	expectedDist := 128.0 // 2 tiles * 64 pixels/tile
	tolerance := 50.0     // Allow more tolerance for DDA algorithm approximation

	if math.Abs(hit.Dist-expectedDist) > tolerance {
		t.Errorf("Expected distance ~%f, got %f (tolerance %f)", expectedDist, hit.Dist, tolerance)
	}

	// Ensure distance is positive and reasonable
	if hit.Dist < 0 || hit.Dist > 200 {
		t.Errorf("Distance %f is unreasonable for this test case", hit.Dist)
	}
}

func TestCheckLineOfSight_Integration(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	x1, y1 := 32.0, 32.0
	x2, y2 := 192.0, 32.0

	// Test clear line of sight
	hasLOS := cs.CheckLineOfSight(x1, y1, x2, y2)
	if !hasLOS {
		t.Errorf("Expected clear line of sight")
	}

	// Add opaque tile and test again
	checker.setOpaque(1, 0, true)
	hasLOS = cs.CheckLineOfSight(x1, y1, x2, y2)
	if hasLOS {
		t.Errorf("Expected blocked line of sight")
	}
}

func TestCheckLineOfSight_OnlyExplicitEntitiesBlockSight(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)
	x1, y1 := 32.0, 32.0
	x2, y2 := 224.0, 32.0

	ordinaryNPC := NewEntity("npc", 96, 32, 56, 56, CollisionTypeNPC, true)
	cs.RegisterEntity(ordinaryNPC)
	if !cs.CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("ordinary solid entity unexpectedly blocked line of sight")
	}
	cs.UnregisterEntity(ordinaryNPC.ID)

	door := NewSightBlockingEntity("door", 96, 32, 56, 56, CollisionTypeNPC, true)
	cs.RegisterEntity(door)
	closedSnapshot := cs.Snapshot()
	if cs.CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("explicit sight blocker did not block live line of sight")
	}
	if closedSnapshot.CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("explicit sight blocker did not block snapshot line of sight")
	}

	cs.UnregisterEntity(door.ID)
	if !cs.CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("removed sight blocker still blocked live line of sight")
	}
	if closedSnapshot.CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("frozen snapshot changed after live blocker removal")
	}
	if !cs.Snapshot().CheckLineOfSight(x1, y1, x2, y2) {
		t.Fatal("new snapshot retained a removed sight blocker")
	}
}

func TestSightBlockerIndexTracksMoveReplaceAndOverlap(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)
	leftX, y := 32.0, 32.0
	rightX := 224.0

	first := NewSightBlockingEntity("first", 96, y, 56, 56, CollisionTypeNPC, true)
	second := NewSightBlockingEntity("second", 96, y, 56, 56, CollisionTypeNPC, true)
	cs.RegisterEntity(first)
	cs.RegisterEntity(second)
	cs.UnregisterEntity(first.ID)
	if cs.CheckLineOfSight(leftX, y, rightX, y) {
		t.Fatal("removing one overlapping blocker revealed the remaining blocker")
	}

	cs.UpdateEntity(second.ID, 96, 160)
	if !cs.CheckLineOfSight(leftX, y, rightX, y) {
		t.Fatal("moving a blocker left its previous tile occluded")
	}
	if cs.CheckLineOfSight(32, 160, 224, 160) {
		t.Fatal("moving a blocker did not occlude its new tile")
	}

	// Re-registering an ID replaces its old indexed state without leaking a
	// count at the previous position.
	replacement := NewSightBlockingEntity(second.ID, 160, y, 56, 56, CollisionTypeNPC, true)
	cs.RegisterEntity(replacement)
	if !cs.CheckLineOfSight(32, 160, 224, 160) {
		t.Fatal("replacing a blocker left its old tile occluded")
	}
	if cs.CheckLineOfSight(leftX, y, rightX, y) {
		t.Fatal("replacement blocker did not occlude its new tile")
	}
}

func TestTileCoordFloorsNegativeWorldCoordinates(t *testing.T) {
	const invTileSize = 1.0 / 64.0
	tests := []struct {
		world float64
		want  int
	}{
		{world: -64.1, want: -2},
		{world: -64, want: -1},
		{world: -0.1, want: -1},
		{world: 0, want: 0},
		{world: 63.9, want: 0},
		{world: 64, want: 1},
	}
	for _, tt := range tests {
		if got := tileCoord(tt.world, invTileSize); got != tt.want {
			t.Errorf("tileCoord(%v) = %d, want %d", tt.world, got, tt.want)
		}
	}
}

func TestRegisterEntityRejectsNil(t *testing.T) {
	cs := NewCollisionSystem(newMockTileChecker(4, 4), 64)
	defer func() {
		if recover() == nil {
			t.Fatal("RegisterEntity(nil) did not fail fast")
		}
	}()
	cs.RegisterEntity(nil)
}

func TestSightBlockerMoveAndRayCanRunConcurrently(t *testing.T) {
	checker := newMockTileChecker(8, 8)
	cs := NewCollisionSystem(checker, 64)
	blocker := NewSightBlockingEntity("door", 96, 96, 56, 56, CollisionTypeNPC, true)
	cs.RegisterEntity(blocker)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cs.UpdateEntity(blocker.ID, 96+float64((i%2)*64), 96)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cs.CastRay(32, 96, 256, 96, true)
		}
	}()
	wg.Wait()
}

func TestCastRay_EdgeCases(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Test ray along tile boundary (edge of tile)
	x1, y1 := 0.0, 0.0   // Corner of tile
	x2, y2 := 128.0, 0.0 // Along top edge

	_, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for ray along tile edge")
	}

	// Test near-vertical ray (should handle properly)
	x1, y1 = 32.0, 32.0
	x2, y2 = 33.0, 320.0 // Almost vertical but slightly angled

	_, hasHit = cs.CastRay(x1, y1, x2, y2, true)
	if hasHit {
		t.Errorf("Expected no hit for near-vertical ray in clear area")
	}
}
