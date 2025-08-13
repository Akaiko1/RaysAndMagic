package collision

import (
	"math"
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

func TestCastRay_StartInOpaqueile(t *testing.T) {
	checker := newMockTileChecker(10, 10)
	cs := NewCollisionSystem(checker, 64.0)

	// Set starting tile as opaque
	checker.setOpaque(0, 0, true)

	x1, y1 := 32.0, 32.0 // Center of tile (0,0)
	x2, y2 := 192.0, 32.0

	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected immediate hit when starting in opaque tile")
	} else if hit.Dist != 0 {
		t.Errorf("Expected distance 0 for immediate hit, got %f", hit.Dist)
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

	// Set the tile as opaque and test again
	checker.setOpaque(0, 0, true)
	hit, hasHit := cs.CastRay(x1, y1, x2, y2, true)
	if !hasHit {
		t.Errorf("Expected hit for zero distance ray in opaque tile")
	} else if hit.Dist != 0 {
		t.Errorf("Expected distance 0 for immediate hit in opaque tile, got %f", hit.Dist)
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
