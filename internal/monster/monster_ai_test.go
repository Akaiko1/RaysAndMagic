package monster

import (
	"math"
	"testing"
)

// MockCollisionChecker implements CollisionChecker for testing
type MockCollisionChecker struct {
	blockedTiles map[[2]int]bool // Map of blocked tile coordinates
	tileSize     float64
	checkCount   int    // Count how many times CanMoveTo was called
	lastX, lastY float64 // Last position checked
}

func NewMockCollisionChecker(tileSize float64) *MockCollisionChecker {
	return &MockCollisionChecker{
		blockedTiles: make(map[[2]int]bool),
		tileSize:     tileSize,
	}
}

func (m *MockCollisionChecker) BlockTile(x, y int) {
	m.blockedTiles[[2]int{x, y}] = true
}

func (m *MockCollisionChecker) CanMoveTo(entityID string, x, y float64) bool {
	m.checkCount++
	m.lastX = x
	m.lastY = y

	// Check all corners of a 48x48 box centered at (x, y)
	halfSize := 24.0
	corners := [][2]float64{
		{x - halfSize, y - halfSize},
		{x + halfSize, y - halfSize},
		{x - halfSize, y + halfSize},
		{x + halfSize, y + halfSize},
	}

	for _, corner := range corners {
		tileX := int(math.Floor(corner[0] / m.tileSize))
		tileY := int(math.Floor(corner[1] / m.tileSize))
		if m.blockedTiles[[2]int{tileX, tileY}] {
			return false
		}
	}
	return true
}

func (m *MockCollisionChecker) CheckLineOfSight(x1, y1, x2, y2 float64) bool {
	return true // Always clear for these tests
}

// TestMonsterGridMovementBasic tests that a monster can move in open terrain
func TestMonsterGridMovementBasic(t *testing.T) {
	// Create a monster at tile center (32, 32) - center of tile (0, 0)
	m := &Monster3D{
		X:     32.0,
		Y:     32.0,
		Speed: 1.5,
	}

	checker := NewMockCollisionChecker(64.0)

	// Target is at tile (2, 0) center = (160, 32)
	targetX, targetY := 160.0, 32.0

	initialX, initialY := m.X, m.Y

	// Run one update cycle
	m.moveGridBased(checker, "test_monster", targetX, targetY)

	// Monster should have moved East (positive X)
	if m.X <= initialX {
		t.Errorf("Monster didn't move East. Initial X: %f, Final X: %f", initialX, m.X)
	}
	if m.Y != initialY {
		t.Errorf("Monster moved in Y unexpectedly. Initial Y: %f, Final Y: %f", initialY, m.Y)
	}

	t.Logf("Monster moved from (%f, %f) to (%f, %f)", initialX, initialY, m.X, m.Y)
	t.Logf("Collision checks made: %d", checker.checkCount)
}

// TestMonsterAtTileCenter tests monster at exact tile center can move
func TestMonsterAtTileCenter(t *testing.T) {
	// Monster exactly at tile center of tile (1, 1) = (96, 96)
	m := &Monster3D{
		X:     96.0,
		Y:     96.0,
		Speed: 1.5,
	}

	checker := NewMockCollisionChecker(64.0)

	// Target is at tile (3, 1) center = (224, 96)
	targetX, targetY := 224.0, 96.0

	initialX := m.X

	// Run one update cycle
	m.moveGridBased(checker, "test_monster", targetX, targetY)

	if m.X <= initialX {
		t.Errorf("Monster at tile center didn't move. Initial X: %f, Final X: %f", initialX, m.X)
	}

	t.Logf("Monster moved from %f to %f (delta: %f)", initialX, m.X, m.X-initialX)
}

// TestMonsterBlockedByTile tests that monster can't move into blocked tile
func TestMonsterBlockedByTile(t *testing.T) {
	// Monster at tile (1, 1) center = (96, 96)
	m := &Monster3D{
		X:     96.0,
		Y:     96.0,
		Speed: 1.5,
	}

	checker := NewMockCollisionChecker(64.0)
	// Block all adjacent tiles around (1, 1)
	checker.BlockTile(2, 1) // East
	checker.BlockTile(0, 1) // West
	checker.BlockTile(1, 2) // South
	checker.BlockTile(1, 0) // North

	// Target is at tile (3, 1) center = (224, 96)
	targetX, targetY := 224.0, 96.0

	initialX, initialY := m.X, m.Y

	// Run multiple update cycles - monster should not move since all directions blocked
	for i := 0; i < 10; i++ {
		m.moveGridBased(checker, "test_monster", targetX, targetY)
	}

	if m.X != initialX || m.Y != initialY {
		t.Errorf("Monster moved when blocked. Initial: (%f, %f), Final: (%f, %f)",
			initialX, initialY, m.X, m.Y)
	} else {
		t.Logf("Monster correctly stayed in place when blocked")
	}
}

// TestTileCenterCalculation verifies tile center calculation
func TestTileCenterCalculation(t *testing.T) {
	testCases := []struct {
		name           string
		posX, posY     float64
		expectedCenterX, expectedCenterY float64
	}{
		{"At center (0,0)", 32.0, 32.0, 32.0, 32.0},
		{"At center (1,1)", 96.0, 96.0, 96.0, 96.0},
		{"Near edge (0,0)", 10.0, 10.0, 32.0, 32.0},
		{"Near edge (0,0) other side", 50.0, 50.0, 32.0, 32.0},
		{"At corner boundary", 64.0, 64.0, 96.0, 96.0}, // 64.0 should be tile (1,1)
		{"At spawned position", 32.0, 32.0, 32.0, 32.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentTileX := math.Floor(tc.posX/tileSize)*tileSize + tileSize/2
			currentTileY := math.Floor(tc.posY/tileSize)*tileSize + tileSize/2

			if currentTileX != tc.expectedCenterX || currentTileY != tc.expectedCenterY {
				t.Errorf("Position (%f, %f) -> Center (%f, %f), expected (%f, %f)",
					tc.posX, tc.posY, currentTileX, currentTileY,
					tc.expectedCenterX, tc.expectedCenterY)
			}
		})
	}
}

// TestTryMoveCardinal tests the tryMoveCardinal function directly
func TestTryMoveCardinal(t *testing.T) {
	m := &Monster3D{
		X:     32.0, // Tile (0,0) center
		Y:     32.0,
		Speed: 1.5,
	}

	checker := NewMockCollisionChecker(64.0)

	// Try to move East (1, 0)
	success := m.tryMoveCardinal(checker, "test", 1, 0)

	if !success {
		t.Errorf("tryMoveCardinal failed when it should succeed")
		t.Logf("Collision checks: %d, Last check at: (%f, %f)",
			checker.checkCount, checker.lastX, checker.lastY)
	}

	t.Logf("After tryMoveCardinal East: Position = (%f, %f)", m.X, m.Y)
	t.Logf("Expected intermediate: X should be > 32, Y should be 32")
}

// TestMonsterShakingScenario simulates the actual shaking bug
func TestMonsterShakingScenario(t *testing.T) {
	// Monster spawned at tile center
	m := &Monster3D{
		X:     32.0,
		Y:     32.0,
		Speed: 1.8, // Bear speed
	}

	checker := NewMockCollisionChecker(64.0)

	// Player is somewhere to the East
	playerX, playerY := 320.0, 32.0

	// Run multiple update cycles to see if monster makes progress
	positions := make([][2]float64, 0)
	positions = append(positions, [2]float64{m.X, m.Y})

	for i := 0; i < 100; i++ {
		m.moveGridBased(checker, "test", playerX, playerY)
		positions = append(positions, [2]float64{m.X, m.Y})
	}

	// Check if monster made progress
	finalX := positions[len(positions)-1][0]
	initialX := positions[0][0]

	if finalX <= initialX {
		t.Errorf("Monster didn't make progress after 100 updates. Initial X: %f, Final X: %f",
			initialX, finalX)
	}

	// Check for oscillation (going back and forth)
	oscillations := 0
	for i := 2; i < len(positions); i++ {
		dx1 := positions[i][0] - positions[i-1][0]
		dx2 := positions[i-1][0] - positions[i-2][0]
		if dx1*dx2 < 0 { // Direction changed
			oscillations++
		}
	}

	if oscillations > 10 {
		t.Errorf("Monster is oscillating! %d direction changes in 100 updates", oscillations)
	}

	t.Logf("Final position: (%f, %f), Progress: %f tiles",
		finalX, positions[len(positions)-1][1], (finalX-initialX)/64.0)
}