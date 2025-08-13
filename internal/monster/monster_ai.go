package monster

import (
	"math"
	"math/rand"
)

// CollisionChecker interface for checking movement validity
type CollisionChecker interface {
	CanMoveTo(entityID string, x, y float64) bool
	CheckLineOfSight(x1, y1, x2, y2 float64) bool
}

// Update runs the monster AI with collision checking and player position for engagement detection
func (m *Monster3D) Update(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
    m.StateTimer++

    // Safety: if the monster somehow ended up in a blocked position (e.g., spawn overlap or jitter),
    // attempt to gently nudge it to a nearby free spot to avoid getting stuck inside walls/trees.
    if collisionChecker != nil && m.StateTimer%15 == 0 { // throttle checks
        if !collisionChecker.CanMoveTo(monsterID, m.X, m.Y) {
            m.unstuckFromObstacles(collisionChecker, monsterID)
        }
    }

	// Check for player detection and engagement
	m.updatePlayerEngagement(playerX, playerY)

	switch m.State {
	case StateIdle:
		m.updateIdle(playerX, playerY)
	case StatePatrolling:
		m.updatePatrolling(collisionChecker, monsterID, playerX, playerY)
	case StatePursuing:
		m.updatePursuing(collisionChecker, monsterID, playerX, playerY)
	case StateAlert:
		m.updateAlert(playerX, playerY)
	case StateAttacking:
		m.updateAttacking(playerX, playerY)
	case StateFleeing:
		m.updateFleeing(collisionChecker, monsterID, playerX, playerY)
	}
}

// updatePlayerEngagement handles player detection and engagement logic
func (m *Monster3D) updatePlayerEngagement(playerX, playerY float64) {
	// Don't process engagement while fleeing - flee state takes priority
	if m.State == StateFleeing {
		return
	}

	// Calculate distance to player
	dx := playerX - m.X
	dy := playerY - m.Y
	distanceToPlayer := math.Sqrt(dx*dx + dy*dy)

	// Get detection radius (use AlertRadius or default)
	detectionRadius := m.AlertRadius
	if detectionRadius <= 0 {
		detectionRadius = 256.0 // 4 tiles default detection radius (4 * 64 pixels)
	}

	// Check if player is within detection range
	if distanceToPlayer <= detectionRadius {
		if !m.IsEngagingPlayer {
			// Start engaging player - switch to alert state
			m.IsEngagingPlayer = true
			m.State = StateAlert
			m.StateTimer = 0
			m.AttackCount = 0 // Reset attack counter for new engagement
		}
	} else if distanceToPlayer > detectionRadius*2 { // Hysteresis - lose engagement at double distance
		if m.IsEngagingPlayer {
			// Stop engaging player - return to idle
			m.IsEngagingPlayer = false
			m.State = StateIdle
			m.StateTimer = 0
			m.AttackCount = 0 // Reset attack counter when disengaging
		}
	}
}

func (m *Monster3D) updateIdle(playerX, playerY float64) {
	// Get AI config values
	var idlePatrolTimer int = 300        // Default value
	var idleToPatrolChance float64 = 0.1 // Default value

	if m.config != nil {
		idlePatrolTimer = m.config.MonsterAI.IdlePatrolTimer
		idleToPatrolChance = m.config.MonsterAI.IdleToPatrolChance
	}

	// If not engaging player and outside tether, return to spawn
	if !m.IsEngagingPlayer && !m.IsWithinTetherRadius() {
		m.State = StatePatrolling
		m.StateTimer = 0
		m.Direction = m.GetDirectionToSpawn() // Head back to spawn
		return
	}

	// Occasionally start patrolling if within tether or engaging player
	if m.StateTimer > idlePatrolTimer && rand.Float64() < idleToPatrolChance {
		m.State = StatePatrolling
		m.StateTimer = 0
		if m.IsEngagingPlayer {
			// If engaging player, move towards them
			dx := playerX - m.X
			dy := playerY - m.Y
			m.Direction = math.Atan2(dy, dx)
		} else {
			// Random direction within tether
			m.Direction = rand.Float64() * 2 * math.Pi
		}
	}
}

// updatePatrolling moves monster randomly for normal wandering behavior
func (m *Monster3D) updatePatrolling(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
	// Get AI config values
	var normalSpeedMult float64 = 0.5 // Default value
	var directionTimer int = 120      // Default value
	var directionChance float64 = 0.2 // Default value
	var patrolIdleTimer int = 600     // Default value

	if m.config != nil {
		normalSpeedMult = m.config.MonsterAI.NormalSpeedMultiplier
		directionTimer = m.config.MonsterAI.PatrolDirectionTimer
		directionChance = m.config.MonsterAI.PatrolDirectionChance
		patrolIdleTimer = m.config.MonsterAI.PatrolIdleTimer
	}

	// Safety check for collision system
	if collisionChecker == nil {
		// Fallback to simple movement if no collision system
		newX := m.X + math.Cos(m.Direction)*m.Speed*normalSpeedMult
		newY := m.Y + math.Sin(m.Direction)*m.Speed*normalSpeedMult

		// Simple bounds checking
		if newX >= 0 && newX < 50*64 && newY >= 0 && newY < 50*64 {
			m.X = newX
			m.Y = newY
		} else {
			m.Direction += math.Pi
		}
		return
	}

	// Calculate next position
	newX := m.X + math.Cos(m.Direction)*m.Speed*normalSpeedMult
	newY := m.Y + math.Sin(m.Direction)*m.Speed*normalSpeedMult

	// Tether checking - only move if within tether
	canMoveWithinTether := m.CanMoveWithinTether(newX, newY)

	if canMoveWithinTether {
		// Check collision before moving
		if collisionChecker.CanMoveTo(monsterID, newX, newY) {
			m.X = newX
			m.Y = newY
		} else {
			// Obstacle - choose new random direction
			m.chooseNewDirection(collisionChecker)
		}
	} else {
		// Outside tether - return to spawn area
		returnX := m.X + math.Cos(m.GetDirectionToSpawn())*m.Speed*normalSpeedMult
		returnY := m.Y + math.Sin(m.GetDirectionToSpawn())*m.Speed*normalSpeedMult

		if collisionChecker.CanMoveTo(monsterID, returnX, returnY) {
			m.Direction = m.GetDirectionToSpawn()
			m.X = returnX
			m.Y = returnY
		} else {
			// Find alternate path back to spawn
			m.chooseNewDirectionTowardsTarget(collisionChecker, m.SpawnX, m.SpawnY)
		}
	}

	// Change direction occasionally for natural movement
	if m.StateTimer > directionTimer && rand.Float64() < directionChance {
		m.chooseNewDirection(collisionChecker)
		m.StateTimer = 0
	}

	// Return to idle after a while
	if m.StateTimer > patrolIdleTimer {
		m.State = StateIdle
		m.StateTimer = 0
	}
}

// updatePursuing moves monster directly towards player at full speed with occasional strafing
func (m *Monster3D) updatePursuing(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
	// Calculate distance and direction to player
	dx := playerX - m.X
	dy := playerY - m.Y
	distanceToPlayer := math.Sqrt(dx*dx + dy*dy)
	directionToPlayer := math.Atan2(dy, dx)

	// Determine movement direction (with occasional strafing)
	movementDirection := directionToPlayer

	// Random strafing: 15% chance every 45 frames to strafe perpendicular for short bursts
	if m.StateTimer%45 == 0 && rand.Float64() < 0.15 {
		// Strafe perpendicular to player direction
		if rand.Float64() < 0.5 {
			movementDirection = directionToPlayer + math.Pi/2 // Strafe left
		} else {
			movementDirection = directionToPlayer - math.Pi/2 // Strafe right
		}
	} else if m.StateTimer%45 < 15 && m.StateTimer%45 != 0 {
		// Continue strafing for 15 frames (~0.25 seconds)
		// Use the stored direction from previous frame
		movementDirection = m.Direction
	}

	// Safety check for collision system
	if collisionChecker == nil {
		// Fallback to simple movement if no collision system
		newX := m.X + math.Cos(movementDirection)*m.Speed
		newY := m.Y + math.Sin(movementDirection)*m.Speed

		// Simple bounds checking
		if newX >= 0 && newX < 50*64 && newY >= 0 && newY < 50*64 {
			m.X = newX
			m.Y = newY
		} else {
			m.Direction += math.Pi
		}
		return
	}

	// Calculate next position at full speed
	newX := m.X + math.Cos(movementDirection)*m.Speed
	newY := m.Y + math.Sin(movementDirection)*m.Speed

	// Check collision before moving
	if collisionChecker.CanMoveTo(monsterID, newX, newY) {
		m.X = newX
		m.Y = newY
		m.Direction = movementDirection // Update stored direction
	} else {
		// Obstacle - find path around it towards player
		m.chooseNewDirectionTowardsTarget(collisionChecker, playerX, playerY)
	}

	// Check if close enough to attack
	if distanceToPlayer <= m.AttackRadius {
		m.State = StateAttacking
		m.StateTimer = 0
	}
}

func (m *Monster3D) updateAlert(playerX, playerY float64) {
	if m.IsEngagingPlayer {
		// Calculate distance to player
		dx := playerX - m.X
		dy := playerY - m.Y
		distanceToPlayer := math.Sqrt(dx*dx + dy*dy)

		// If close enough to attack, switch to attacking
		if distanceToPlayer <= m.AttackRadius {
			m.State = StateAttacking
			m.StateTimer = 0
		} else {
			// Move towards player
			m.Direction = math.Atan2(dy, dx)
			m.State = StatePursuing
			m.StateTimer = 0
		}
	} else {
		// Not engaging player - go directly to idle (engagement system handles transitions)
		m.State = StateIdle
		m.StateTimer = 0
	}
}

func (m *Monster3D) updateAttacking(playerX, playerY float64) {
	// Get AI config values
	var attackCooldown int = 60 // Default value

	if m.config != nil {
		attackCooldown = m.config.MonsterAI.AttackCooldown
	}

	// Attack delay from config
	if m.StateTimer > attackCooldown {
		// Increment attack counter
		m.AttackCount++

		// After 5 attacks, 50% chance to flee for 7 seconds
		if m.AttackCount >= 5 && rand.Float64() < 0.5 {
			// Start fleeing
			m.State = StateFleeing
			m.StateTimer = 0
			m.IsEngagingPlayer = false // Disengage from player
			m.AttackCount = 0          // Reset attack counter

			// Flee in random direction
			m.Direction = rand.Float64() * 2 * math.Pi
		} else {
			// Continue engagement cycle
			m.State = StateAlert
			m.StateTimer = 0
		}
	}
}

// updateFleeing moves monster with proper collision detection and vision when fleeing
func (m *Monster3D) updateFleeing(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
	// Get AI config values
	var fleeSpeedMult float64 = 1.5   // Default value
	var fleeVisionDist float64 = 60.0 // Default value
	var fleeCheckFreq int = 20        // Default value
	var fleeDuration int = 300        // Default value

	if m.config != nil {
		fleeSpeedMult = m.config.MonsterAI.FleeSpeedMultiplier
		fleeVisionDist = m.config.MonsterAI.FleeVisionDistance
		fleeCheckFreq = m.config.MonsterAI.FleeCheckFrequency
		fleeDuration = m.config.MonsterAI.FleeDuration
	}

	// Safety check for collision system
	if collisionChecker == nil {
		// Fallback to simple movement if no collision system
		newX := m.X + math.Cos(m.Direction)*m.Speed*fleeSpeedMult
		newY := m.Y + math.Sin(m.Direction)*m.Speed*fleeSpeedMult

		// Simple bounds checking
		if newX >= 0 && newX < 50*64 && newY >= 0 && newY < 50*64 {
			m.X = newX
			m.Y = newY
		}
		return
	}

	// Check if current direction is clear before moving
	lookAheadX := m.X + math.Cos(m.Direction)*fleeVisionDist
	lookAheadY := m.Y + math.Sin(m.Direction)*fleeVisionDist

	// If path ahead is blocked, find a new escape route (but not too frequently)
	if !collisionChecker.CheckLineOfSight(m.X, m.Y, lookAheadX, lookAheadY) && m.StateTimer%fleeCheckFreq == 0 {
		m.chooseNewDirection(collisionChecker)
	}

	// Move away from threat
	newX := m.X + math.Cos(m.Direction)*m.Speed*fleeSpeedMult
	newY := m.Y + math.Sin(m.Direction)*m.Speed*fleeSpeedMult

	// Check collision before moving
	if collisionChecker.CanMoveTo(monsterID, newX, newY) {
		m.X = newX
		m.Y = newY
	} else {
		// Immediate obstacle - emergency direction change
		m.chooseNewDirection(collisionChecker)
	}

	// Stop fleeing after a while
	if m.StateTimer > fleeDuration {
		m.State = StateIdle
		m.StateTimer = 0
	}
}

// chooseNewDirection intelligently picks a direction with clear path ahead
func (m *Monster3D) chooseNewDirection(collisionChecker CollisionChecker) {
	// Get AI config values
	var directionVisionDist float64 = 50.0 // Default value
	var maxAttempts int = 6                // Default value

	if m.config != nil {
		directionVisionDist = m.config.MonsterAI.DirectionVisionDistance
		maxAttempts = m.config.MonsterAI.MaxDirectionAttempts
	}

	// Safety check
	if collisionChecker == nil {
		m.Direction = rand.Float64() * 2 * math.Pi
		return
	}

	// Try different directions to find a clear path
	for i := 0; i < maxAttempts; i++ {
		testDirection := rand.Float64() * 2 * math.Pi
		lookAheadX := m.X + math.Cos(testDirection)*directionVisionDist
		lookAheadY := m.Y + math.Sin(testDirection)*directionVisionDist

		// If this direction has a clear path, use it
		if collisionChecker.CheckLineOfSight(m.X, m.Y, lookAheadX, lookAheadY) {
			m.Direction = testDirection
			return
		}
	}

	// If no clear path found, just turn 90 degrees with some randomness
	m.Direction += math.Pi/2 + (rand.Float64()-0.5)*0.3
	if m.Direction > 2*math.Pi {
		m.Direction -= 2 * math.Pi
	}
}

// chooseNewDirectionTowardsTarget intelligently picks a direction towards a target (player or spawn)
func (m *Monster3D) chooseNewDirectionTowardsTarget(collisionChecker CollisionChecker, targetX, targetY float64) {
	// Get AI config values
	var directionVisionDist float64 = 50.0 // Default value
	var maxAttempts int = 6                // Default value

	if m.config != nil {
		directionVisionDist = m.config.MonsterAI.DirectionVisionDistance
		maxAttempts = m.config.MonsterAI.MaxDirectionAttempts
	}

	// Safety check
	if collisionChecker == nil {
		dx := targetX - m.X
		dy := targetY - m.Y
		m.Direction = math.Atan2(dy, dx)
		return
	}

	// First try direct path to target
	dx := targetX - m.X
	dy := targetY - m.Y
	directDirection := math.Atan2(dy, dx)

	lookAheadX := m.X + math.Cos(directDirection)*directionVisionDist
	lookAheadY := m.Y + math.Sin(directDirection)*directionVisionDist

	if collisionChecker.CheckLineOfSight(m.X, m.Y, lookAheadX, lookAheadY) {
		m.Direction = directDirection
		return
	}

	// If direct path blocked, try angles around the target direction
	angleStep := math.Pi / 4 // 45 degree steps
	for i := 1; i <= maxAttempts/2; i++ {
		// Try both sides alternating
		for _, sign := range []float64{1, -1} {
			testDirection := directDirection + sign*angleStep*float64(i)
			lookAheadX := m.X + math.Cos(testDirection)*directionVisionDist
			lookAheadY := m.Y + math.Sin(testDirection)*directionVisionDist

			// If this direction has a clear path, use it
			if collisionChecker.CheckLineOfSight(m.X, m.Y, lookAheadX, lookAheadY) {
				m.Direction = testDirection
				return
			}
		}
	}

    // If no clear path found towards target, use fallback random direction
    m.chooseNewDirection(collisionChecker)
}

// unstuckFromObstacles tries to move the monster to the nearest non-blocked position
// Useful when a monster ends up overlapping a solid tile (e.g., trees) due to edge cases
func (m *Monster3D) unstuckFromObstacles(collisionChecker CollisionChecker, monsterID string) {
    if collisionChecker == nil {
        return
    }

    // Search outwards in rings for a free spot
    // Use small steps to avoid teleporting too far
    radii := []float64{8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 128}
    const samples = 16

    for _, r := range radii {
        for i := 0; i < samples; i++ {
            angle := (2 * math.Pi * float64(i)) / samples
            nx := m.X + math.Cos(angle)*r
            ny := m.Y + math.Sin(angle)*r
            if collisionChecker.CanMoveTo(monsterID, nx, ny) {
                m.X = nx
                m.Y = ny
                m.Direction = angle
                return
            }
        }
    }
    // As a last resort, try the spawn position if within reasonable distance
    if collisionChecker.CanMoveTo(monsterID, m.SpawnX, m.SpawnY) {
        m.X = m.SpawnX
        m.Y = m.SpawnY
        m.Direction = m.GetDirectionToSpawn()
    }
}
