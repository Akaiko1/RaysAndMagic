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

// Update runs the monster AI with collision checking
func (m *Monster3D) Update(collisionChecker CollisionChecker, monsterID string) {
	m.StateTimer++

	switch m.State {
	case StateIdle:
		m.updateIdle()
	case StatePatrolling:
		m.updatePatrolling(collisionChecker, monsterID)
	case StateAlert:
		m.updateAlert()
	case StateAttacking:
		m.updateAttacking()
	case StateFleeing:
		m.updateFleeing(collisionChecker, monsterID)
	}
}

func (m *Monster3D) updateIdle() {
	// Get AI config values
	var idlePatrolTimer int = 300        // Default value
	var idleToPatrolChance float64 = 0.1 // Default value

	if m.config != nil {
		idlePatrolTimer = m.config.MonsterAI.IdlePatrolTimer
		idleToPatrolChance = m.config.MonsterAI.IdleToPatrolChance
	}

	// Occasionally start patrolling
	if m.StateTimer > idlePatrolTimer && rand.Float64() < idleToPatrolChance {
		m.State = StatePatrolling
		m.StateTimer = 0
		m.Direction = rand.Float64() * 2 * math.Pi
	}
}

// updatePatrolling moves monster with proper collision detection and vision
func (m *Monster3D) updatePatrolling(collisionChecker CollisionChecker, monsterID string) {
	// Get AI config values
	var normalSpeedMult float64 = 0.5 // Default value
	var visionDistance float64 = 80.0 // Default value
	var pathCheckFreq int = 30        // Default value
	var directionTimer int = 120      // Default value
	var directionChance float64 = 0.2 // Default value
	var patrolIdleTimer int = 600     // Default value

	if m.config != nil {
		normalSpeedMult = m.config.MonsterAI.NormalSpeedMultiplier
		visionDistance = m.config.MonsterAI.PatrolVisionDistance
		pathCheckFreq = m.config.MonsterAI.PathCheckFrequency
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

	// Check vision ahead before moving
	lookAheadX := m.X + math.Cos(m.Direction)*visionDistance
	lookAheadY := m.Y + math.Sin(m.Direction)*visionDistance

	// If path ahead is blocked, choose a new direction (but not too frequently)
	if !collisionChecker.CheckLineOfSight(m.X, m.Y, lookAheadX, lookAheadY) && m.StateTimer%pathCheckFreq == 0 {
		m.chooseNewDirection(collisionChecker)
	}

	// Move in current direction
	newX := m.X + math.Cos(m.Direction)*m.Speed*normalSpeedMult
	newY := m.Y + math.Sin(m.Direction)*m.Speed*normalSpeedMult

	// Check collision before moving (backup safety check)
	if collisionChecker.CanMoveTo(monsterID, newX, newY) {
		m.X = newX
		m.Y = newY
	} else {
		// Immediate obstacle - emergency direction change
		m.chooseNewDirection(collisionChecker)
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

func (m *Monster3D) updateAlert() {
	// Get AI config values
	var alertTimeout int = 180 // Default value

	if m.config != nil {
		alertTimeout = m.config.MonsterAI.AlertTimeout
	}

	// Look for targets - this would need player position reference
	// For now, just timeout back to idle
	if m.StateTimer > alertTimeout {
		m.State = StateIdle
		m.StateTimer = 0
	}
}

func (m *Monster3D) updateAttacking() {
	// Get AI config values
	var attackCooldown int = 60 // Default value

	if m.config != nil {
		attackCooldown = m.config.MonsterAI.AttackCooldown
	}

	// Attack animation/cooldown
	if m.StateTimer > attackCooldown {
		m.State = StateAlert
		m.StateTimer = 0
	}
}

// updateFleeing moves monster with proper collision detection and vision when fleeing
func (m *Monster3D) updateFleeing(collisionChecker CollisionChecker, monsterID string) {
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
