package monster

import (
	"math"
	"testing"

	"ugataima/internal/config"
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

	// Run fewer update cycles - monster should not move in first few attempts
	for i := 0; i < 5; i++ {
		m.moveGridBased(checker, "test_monster", targetX, targetY)
	}

	// After 5 attempts with all directions blocked, monster should still be in same position
	// (stuck counter will be 5, but unstuck mechanism only triggers at 10)
	if m.X != initialX || m.Y != initialY {
		t.Errorf("Monster moved when blocked. Initial: (%f, %f), Final: (%f, %f)",
			initialX, initialY, m.X, m.Y)
	} else {
		t.Logf("Monster correctly stayed in place when blocked for 5 frames")
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

// TestMonsterMovementNoShake tests that monsters don't shake during various movement scenarios
func TestMonsterMovementNoShake(t *testing.T) {
	testCases := []struct {
		name         string
		startX       float64
		startY       float64
		targetX      float64
		targetY      float64
		speed        float64
		maxOscillate int // Max allowed direction changes
	}{
		{
			name:         "Straight East Movement",
			startX:       32.0,
			startY:       32.0,
			targetX:      320.0,
			targetY:      32.0,
			speed:        1.8,
			maxOscillate: 0, // Should never change direction
		},
		{
			name:         "Straight South Movement",
			startX:       32.0,
			startY:       32.0,
			targetX:      32.0,
			targetY:      320.0,
			speed:        1.8,
			maxOscillate: 0, // Should never change direction
		},
		{
			name:         "Diagonal Movement (NE)",
			startX:       32.0,
			startY:       320.0,
			targetX:      320.0,
			targetY:      32.0,
			speed:        1.8,
			maxOscillate: 10, // Some direction changes expected for stair-step, but not excessive
		},
		{
			name:         "Diagonal Movement (SE)",
			startX:       32.0,
			startY:       32.0,
			targetX:      320.0,
			targetY:      320.0,
			speed:        1.8,
			maxOscillate: 10, // Some direction changes expected for stair-step, but not excessive
		},
		{
			name:         "Nearly Equal Deltas (Shake-Prone)",
			startX:       32.0,
			startY:       32.0,
			targetX:      192.0,
			targetY:      200.0,
			speed:        1.5,
			maxOscillate: 10, // Should use direction memory to prevent shaking
		},
		{
			name:         "Fast Monster Diagonal",
			startX:       64.0,
			startY:       64.0,
			targetX:      320.0,
			targetY:      320.0,
			speed:        2.5,
			maxOscillate: 15, // Faster = more direction changes, but still reasonable
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create monster at starting position
			m := &Monster3D{
				X:     tc.startX,
				Y:     tc.startY,
				Speed: tc.speed,
			}

			checker := NewMockCollisionChecker(64.0)

			// Track positions and directions
			positions := make([][2]float64, 0)
			directions := make([]float64, 0)
			positions = append(positions, [2]float64{m.X, m.Y})

			// Run 100 movement updates
			for i := 0; i < 100; i++ {
				m.moveGridBased(checker, "test", tc.targetX, tc.targetY)
				positions = append(positions, [2]float64{m.X, m.Y})
				directions = append(directions, m.Direction)

				// Stop if reached target
				dx := tc.targetX - m.X
				dy := tc.targetY - m.Y
				if math.Sqrt(dx*dx+dy*dy) < m.Speed {
					break
				}
			}

			// Count oscillations (direction changes)
			oscillations := 0
			for i := 2; i < len(positions); i++ {
				// Check if movement direction changed
				dx1 := positions[i][0] - positions[i-1][0]
				dy1 := positions[i][1] - positions[i-1][1]
				dx2 := positions[i-1][0] - positions[i-2][0]
				dy2 := positions[i-1][1] - positions[i-2][1]

				// Determine primary axis for each step
				axis1 := "none"
				axis2 := "none"
				if math.Abs(dx1) > 0.1 {
					axis1 = "horizontal"
				} else if math.Abs(dy1) > 0.1 {
					axis1 = "vertical"
				}
				if math.Abs(dx2) > 0.1 {
					axis2 = "horizontal"
				} else if math.Abs(dy2) > 0.1 {
					axis2 = "vertical"
				}

				// Count axis changes as oscillations
				if axis1 != axis2 && axis1 != "none" && axis2 != "none" {
					oscillations++
				}
			}

			// Check if oscillations are within acceptable range
			if oscillations > tc.maxOscillate {
				t.Errorf("Excessive oscillation detected! %d direction changes (max allowed: %d)",
					oscillations, tc.maxOscillate)

				// Log movement pattern for debugging
				t.Logf("Movement pattern (first 20 steps):")
				for i := 0; i < len(positions) && i < 20; i++ {
					t.Logf("  Step %d: (%.1f, %.1f)", i, positions[i][0], positions[i][1])
				}
			}

			// Calculate total progress
			initialDist := math.Sqrt(math.Pow(tc.targetX-tc.startX, 2) + math.Pow(tc.targetY-tc.startY, 2))
			finalPos := positions[len(positions)-1]
			finalDist := math.Sqrt(math.Pow(tc.targetX-finalPos[0], 2) + math.Pow(tc.targetY-finalPos[1], 2))
			progress := initialDist - finalDist

			// Verify monster made forward progress (at least 25% of distance)
			if progress < initialDist*0.25 {
				t.Errorf("Monster made insufficient progress. Initial distance: %.1f, Final distance: %.1f, Progress: %.1f (%.1f%%)",
					initialDist, finalDist, progress, (progress/initialDist)*100)
			}

			t.Logf("✓ Oscillations: %d/%d, Progress: %.1f/%.1f pixels",
				oscillations, tc.maxOscillate, progress, initialDist)
		})
	}
}

// TestMonsterPursuitNoShake tests that monsters don't shake when pursuing and attacking the player
func TestMonsterPursuitNoShake(t *testing.T) {
	testCases := []struct {
		name         string
		startX       float64
		startY       float64
		playerX      float64
		playerY      float64
		speed        float64
		attackRadius float64
		description  string
	}{
		{
			name:         "Straight Pursuit East",
			startX:       32.0,
			startY:       32.0,
			playerX:      320.0,
			playerY:      32.0,
			speed:        1.8,
			attackRadius: 64.0,
			description:  "Monster should move straight toward player without shaking",
		},
		{
			name:         "Diagonal Pursuit",
			startX:       32.0,
			startY:       32.0,
			playerX:      256.0,
			playerY:      256.0,
			speed:        1.8,
			attackRadius: 64.0,
			description:  "Monster should pursue diagonally with minimal oscillation",
		},
		{
			name:         "Near Attack Range (Shake-Prone)",
			startX:       32.0,
			startY:       32.0,
			playerX:      120.0,
			playerY:      32.0,
			speed:        1.8,
			attackRadius: 64.0,
			description:  "Monster should not shake when approaching attack radius",
		},
		{
			name:         "At Attack Boundary",
			startX:       32.0,
			startY:       32.0,
			playerX:      96.0,
			playerY:      32.0,
			speed:        1.5,
			attackRadius: 64.0,
			description:  "Monster should not oscillate between pursuing and attacking states",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create monster with AI config
			m := &Monster3D{
				X:                tc.startX,
				Y:                tc.startY,
				Speed:            tc.speed,
				AttackRadius:     tc.attackRadius,
				State:            StateAlert,
				IsEngagingPlayer: true,
			}

			checker := NewMockCollisionChecker(64.0)

			// Track positions and states
			positions := make([][2]float64, 0)
			states := make([]MonsterState, 0)
			positions = append(positions, [2]float64{m.X, m.Y})
			states = append(states, m.State)

			// Run AI updates for 100 frames
			stateChanges := 0
			for i := 0; i < 100; i++ {
				// Update monster AI (full AI cycle with state transitions)
				m.Update(checker, "test", tc.playerX, tc.playerY)

				positions = append(positions, [2]float64{m.X, m.Y})

				// Track state changes
				if len(states) > 0 && m.State != states[len(states)-1] {
					stateChanges++
				}
				states = append(states, m.State)

				// Stop if in attack range and attacking for a while
				dx := tc.playerX - m.X
				dy := tc.playerY - m.Y
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist <= m.AttackRadius && m.State == StateAttacking && i > 50 {
					break
				}
			}

			// Count position oscillations
			oscillations := 0
			for i := 2; i < len(positions); i++ {
				dx1 := positions[i][0] - positions[i-1][0]
				dy1 := positions[i][1] - positions[i-1][1]
				dx2 := positions[i-1][0] - positions[i-2][0]
				dy2 := positions[i-1][1] - positions[i-2][1]

				axis1 := "none"
				axis2 := "none"
				if math.Abs(dx1) > 0.1 {
					axis1 = "horizontal"
				} else if math.Abs(dy1) > 0.1 {
					axis1 = "vertical"
				}
				if math.Abs(dx2) > 0.1 {
					axis2 = "horizontal"
				} else if math.Abs(dy2) > 0.1 {
					axis2 = "vertical"
				}

				if axis1 != axis2 && axis1 != "none" && axis2 != "none" {
					oscillations++
				}
			}

			// Check for state oscillations (rapid flip-flopping)
			stateOscillations := 0
			for i := 2; i < len(states); i++ {
				if states[i] == StatePursuing && states[i-1] == StateAttacking && states[i-2] == StatePursuing {
					stateOscillations++
				}
				if states[i] == StateAttacking && states[i-1] == StatePursuing && states[i-2] == StateAttacking {
					stateOscillations++
				}
			}

			// Verify no excessive oscillations
			maxOscillations := 15
			if oscillations > maxOscillations {
				t.Errorf("Excessive movement oscillation! %d changes (max: %d)", oscillations, maxOscillations)
				t.Logf("First 20 positions:")
				for i := 0; i < len(positions) && i < 20; i++ {
					t.Logf("  %d: (%.1f, %.1f) State:%v", i, positions[i][0], positions[i][1], states[i])
				}
			}

			// Verify no state oscillations
			if stateOscillations > 3 {
				t.Errorf("State oscillation! Flip-flopped Pursuing/Attacking %d times", stateOscillations)
				t.Logf("State transitions:")
				for i := 1; i < len(states) && i < 30; i++ {
					if states[i] != states[i-1] {
						t.Logf("  Frame %d: %v -> %v", i, states[i-1], states[i])
					}
				}
			}

			// Calculate progress
			initialDist := math.Sqrt(math.Pow(tc.playerX-tc.startX, 2) + math.Pow(tc.playerY-tc.startY, 2))
			finalPos := positions[len(positions)-1]
			finalDist := math.Sqrt(math.Pow(tc.playerX-finalPos[0], 2) + math.Pow(tc.playerY-finalPos[1], 2))
			progress := initialDist - finalDist

			t.Logf("✓ Movement: %d/%d oscillations, States: %d changes, %d flip-flops, Progress: %.1f/%.1f px",
				oscillations, maxOscillations, stateChanges, stateOscillations, progress, initialDist)
		})
	}
}

// =============================================================================
// Combat Engagement Behavior Tests
// =============================================================================

// createTestMonster creates a monster with basic test configuration
func createTestMonster(x, y float64) *Monster3D {
	return &Monster3D{
		X:                x,
		Y:                y,
		HitPoints:        100,
		MaxHitPoints:     100,
		Speed:            1.5,
		AlertRadius:      128.0, // 2 tiles
		AttackRadius:     64.0,  // 1 tile
		State:            StateIdle,
		IsEngagingPlayer: false,
		WasAttacked:      false,
		SpawnX:           x,
		SpawnY:           y,
		TetherRadius:     192.0, // 3 tiles
		Resistances:      make(map[DamageType]int),
		config:           &config.Config{},
	}
}

// TestMonsterEngagesWhenHitFromCloseRange tests monster engagement when hit from close range
func TestMonsterEngagesWhenHitFromCloseRange(t *testing.T) {
	m := createTestMonster(100.0, 100.0)

	// Player at close range (2 tiles away)
	playerX, playerY := 228.0, 100.0

	// Initial state should be idle
	if m.State != StateIdle {
		t.Fatalf("Expected initial state Idle, got %v", m.State)
	}
	if m.IsEngagingPlayer {
		t.Fatalf("Expected IsEngagingPlayer to be false initially")
	}

	// Monster takes damage from close range
	damage := m.TakeDamage(10, DamagePhysical, playerX, playerY)

	// Verify damage was applied
	if damage != 10 {
		t.Errorf("Expected 10 damage, got %d", damage)
	}
	if m.HitPoints != 90 {
		t.Errorf("Expected 90 HP after damage, got %d", m.HitPoints)
	}

	// Monster should now be engaging and alert
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should be engaging player after being hit")
	}
	if m.State != StateAlert {
		t.Errorf("Expected state Alert after being hit, got %v", m.State)
	}
	if !m.WasAttacked {
		t.Errorf("WasAttacked flag should be true after taking damage")
	}
}

// TestMonsterEngagesWhenHitFromLongRange tests monster engagement when hit from long range
func TestMonsterEngagesWhenHitFromLongRange(t *testing.T) {
	m := createTestMonster(100.0, 100.0)

	// Player at long range (10 tiles away = 640 pixels)
	playerX, playerY := 740.0, 100.0

	// Initial state
	if m.IsEngagingPlayer {
		t.Fatalf("Expected IsEngagingPlayer to be false initially")
	}

	// Monster takes damage from long range
	m.TakeDamage(15, DamageFire, playerX, playerY)

	// Monster should engage regardless of distance when hit
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should engage player when hit from long range")
	}
	if m.State != StateAlert {
		t.Errorf("Expected state Alert after being hit from long range, got %v", m.State)
	}
	if !m.WasAttacked {
		t.Errorf("WasAttacked flag should be true after taking damage from long range")
	}
}

// TestMonsterStaysEngagedAfterBeingHit tests that monster doesn't disengage after being hit
func TestMonsterStaysEngagedAfterBeingHit(t *testing.T) {
	m := createTestMonster(100.0, 100.0)
	checker := NewMockCollisionChecker(64.0)

	// Player at very long range (15 tiles away = 960 pixels)
	playerX, playerY := 1060.0, 100.0

	// Hit the monster from long range
	m.TakeDamage(10, DamagePhysical, playerX, playerY)

	// Verify initial engagement
	if !m.IsEngagingPlayer {
		t.Fatalf("Monster should engage after being hit")
	}

	// Run several AI update cycles - monster should stay engaged
	for i := 0; i < 60; i++ {
		m.updatePlayerEngagementWithVision(checker, playerX, playerY)
	}

	// Monster should still be engaging because WasAttacked is true
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should stay engaged after being hit (WasAttacked = %v)", m.WasAttacked)
	}
	if m.State == StateIdle {
		t.Errorf("Monster should not return to Idle after being hit, got %v", m.State)
	}
}

// TestMonsterDisengagesNormallyWithoutBeingHit tests normal disengagement when not attacked
func TestMonsterDisengagesNormallyWithoutBeingHit(t *testing.T) {
	m := createTestMonster(100.0, 100.0)
	checker := NewMockCollisionChecker(64.0)

	// Manually set engaged state (simulating player walked close then walked away)
	m.IsEngagingPlayer = true
	m.State = StateAlert
	m.WasAttacked = false // Not hit, just detected player

	// Player at very long range (beyond 2x detection radius)
	// AlertRadius is 128, so 2x = 256. Player at 400 pixels away should trigger disengage.
	playerX, playerY := 500.0, 100.0

	// Run AI update - monster should disengage since WasAttacked is false
	m.updatePlayerEngagementWithVision(checker, playerX, playerY)

	// Monster should disengage when player is far and WasAttacked is false
	if m.IsEngagingPlayer {
		t.Errorf("Monster should disengage when player is far and wasn't hit")
	}
	if m.State != StateIdle {
		t.Errorf("Expected state Idle after disengaging, got %v", m.State)
	}
}

// TestMonsterResistanceReducesDamage tests that resistances properly reduce damage
func TestMonsterResistanceReducesDamage(t *testing.T) {
	m := createTestMonster(100.0, 100.0)
	m.Resistances[DamageFire] = 50 // 50% fire resistance

	// Hit with fire damage
	damage := m.TakeDamage(20, DamageFire, 200.0, 100.0)

	// Should receive only 50% of damage
	if damage != 10 {
		t.Errorf("Expected 10 damage after 50%% resistance, got %d", damage)
	}
	if m.HitPoints != 90 {
		t.Errorf("Expected 90 HP, got %d", m.HitPoints)
	}

	// Should still engage
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should engage even when damage is reduced by resistance")
	}
}

// TestMonsterDoesNotReengageWhenAlreadyEngaged tests that AI state doesn't change when already engaged
func TestMonsterDoesNotReengageWhenAlreadyEngaged(t *testing.T) {
	m := createTestMonster(100.0, 100.0)

	// Already engaged and pursuing
	m.IsEngagingPlayer = true
	m.State = StatePursuing
	m.StateTimer = 50

	// Take more damage
	m.TakeDamage(10, DamagePhysical, 200.0, 100.0)

	// State should not change (still pursuing, not reset to alert)
	if m.State != StatePursuing {
		t.Errorf("State should remain Pursuing when already engaged, got %v", m.State)
	}
	if m.StateTimer != 50 {
		t.Errorf("StateTimer should not reset when already engaged, got %d", m.StateTimer)
	}
}

// TestMultipleHitsKeepMonsterEngaged tests that multiple hits maintain engagement
func TestMultipleHitsKeepMonsterEngaged(t *testing.T) {
	m := createTestMonster(100.0, 100.0)
	checker := NewMockCollisionChecker(64.0)

	// Player at long range
	playerX, playerY := 800.0, 100.0

	// First hit
	m.TakeDamage(10, DamagePhysical, playerX, playerY)

	// Run some AI updates
	for i := 0; i < 30; i++ {
		m.updatePlayerEngagementWithVision(checker, playerX, playerY)
	}

	// Second hit
	m.TakeDamage(10, DamagePhysical, playerX, playerY)

	// Run more AI updates
	for i := 0; i < 30; i++ {
		m.updatePlayerEngagementWithVision(checker, playerX, playerY)
	}

	// Should still be engaged
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should remain engaged after multiple hits")
	}
	if !m.WasAttacked {
		t.Errorf("WasAttacked should remain true")
	}
}

// TestMonsterChasesPlayerAfterRangedHit tests the full pursuit behavior after being hit
func TestMonsterChasesPlayerAfterRangedHit(t *testing.T) {
	m := createTestMonster(100.0, 100.0)
	checker := NewMockCollisionChecker(64.0)

	// Player 8 tiles away
	playerX, playerY := 612.0, 100.0

	// Hit the monster
	m.TakeDamage(10, DamageFire, playerX, playerY)

	initialX := m.X

	// Run full AI update cycles (not just engagement check)
	for i := 0; i < 100; i++ {
		m.Update(checker, "test_monster", playerX, playerY)
	}

	// Monster should have moved toward player
	if m.X <= initialX {
		t.Errorf("Monster should move toward player after being hit. Initial X: %.1f, Final X: %.1f", initialX, m.X)
	}

	// Should still be engaged
	if !m.IsEngagingPlayer {
		t.Errorf("Monster should remain engaged while chasing player")
	}

	t.Logf("Monster moved from %.1f to %.1f (%.1f tiles)", initialX, m.X, (m.X-initialX)/64.0)
}