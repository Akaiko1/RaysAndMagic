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

	// Check for player detection and engagement with line-of-sight (trees reduce detection)
	m.updatePlayerEngagementWithVision(collisionChecker, playerX, playerY)

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

// updatePlayerEngagementWithVision handles player detection with line-of-sight checks
// Trees and other opaque obstacles reduce detection radius
func (m *Monster3D) updatePlayerEngagementWithVision(collisionChecker CollisionChecker, playerX, playerY float64) {
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

	// If outside tether, monster is more alert (was lured or is lost)
	// This prevents them from immediately returning to spawn when switching from TB to RT mode
	if !m.IsWithinTetherRadius() {
		detectionRadius *= 2.0 // Double range when far from home
	}

	// Check line of sight - if obstructed (trees, walls), halve detection radius
	// Only apply penalty if we are inside our territory. If outside, we stay alert.
	if m.IsWithinTetherRadius() && collisionChecker != nil && !collisionChecker.CheckLineOfSight(m.X, m.Y, playerX, playerY) {
		detectionRadius *= 0.5 // Trees block vision, reduce detection range
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
	var idlePatrolTimer int = 60         // Default value (1 second)
	var idleToPatrolChance float64 = 0.1 // Default value

	if m.config != nil {
		idlePatrolTimer = m.config.MonsterAI.IdlePatrolTimer
		// If config value is the old default (300), override it to 60 for better responsiveness
		// unless explicitly set in config (we assume 300 is the "unset" default here for safety)
		if idlePatrolTimer == 300 {
			idlePatrolTimer = 60
		}
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

// updatePatrolling moves monster randomly for normal wandering behavior using grid-based movement
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
		return
	}

	speed := m.Speed * normalSpeedMult

	// Check if outside tether - return to spawn using grid movement
	if !m.IsWithinTetherRadius() {
		m.moveGridBased(collisionChecker, monsterID, m.SpawnX, m.SpawnY)
		return
	}

	// Convert current direction to cardinal (0=E, 1=S, 2=W, 3=N)
	cardinalDir := m.getCardinalDirection()

	// Change direction occasionally
	if m.StateTimer > directionTimer && rand.Float64() < directionChance {
		// Pick a new random cardinal direction
		cardinalDir = rand.Intn(4)
		m.StateTimer = 0
	}

	// Try to move in current cardinal direction
	dirs := [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} // E, S, W, N
	dirX, dirY := dirs[cardinalDir][0], dirs[cardinalDir][1]

	// Check if target would be within tether
	currentTileX := math.Floor(m.X/tileSize)*tileSize + tileSize/2
	currentTileY := math.Floor(m.Y/tileSize)*tileSize + tileSize/2
	targetX := currentTileX + float64(dirX)*tileSize
	targetY := currentTileY + float64(dirY)*tileSize

	if m.CanMoveWithinTether(targetX, targetY) {
		if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, dirX, dirY, speed) {
			return
		}
	}

	// Current direction blocked or outside tether - try other directions
	for i := 1; i < 4; i++ {
		newDir := (cardinalDir + i) % 4
		dirX, dirY = dirs[newDir][0], dirs[newDir][1]
		targetX = currentTileX + float64(dirX)*tileSize
		targetY = currentTileY + float64(dirY)*tileSize

		if m.CanMoveWithinTether(targetX, targetY) {
			if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, dirX, dirY, speed) {
				// Update stored direction
				m.Direction = math.Atan2(float64(dirY), float64(dirX))
				return
			}
		}
	}

	// Return to idle after a while
	if m.StateTimer > patrolIdleTimer {
		m.State = StateIdle
		m.StateTimer = 0
	}
}

// getCardinalDirection converts the monster's current direction to a cardinal index (0=E, 1=S, 2=W, 3=N)
func (m *Monster3D) getCardinalDirection() int {
	// Normalize direction to 0-2π
	dir := m.Direction
	for dir < 0 {
		dir += 2 * math.Pi
	}
	for dir >= 2*math.Pi {
		dir -= 2 * math.Pi
	}

	// Convert to cardinal (each quadrant is π/2)
	quadrant := int((dir + math.Pi/4) / (math.Pi / 2))
	return quadrant % 4
}

// tryMoveCardinalWithSpeed attempts to move in a cardinal direction with custom speed
func (m *Monster3D) tryMoveCardinalWithSpeed(collisionChecker CollisionChecker, monsterID string, dirX, dirY int, speed float64) bool {
	if dirX == 0 && dirY == 0 {
		return false
	}

	currentTileX := math.Floor(m.X/tileSize)*tileSize + tileSize/2
	currentTileY := math.Floor(m.Y/tileSize)*tileSize + tileSize/2
	targetX := currentTileX + float64(dirX)*tileSize
	targetY := currentTileY + float64(dirY)*tileSize

	if !collisionChecker.CanMoveTo(monsterID, targetX, targetY) {
		return false
	}

	dirAngle := math.Atan2(float64(dirY), float64(dirX))
	newX := m.X + math.Cos(dirAngle)*speed
	newY := m.Y + math.Sin(dirAngle)*speed

	if collisionChecker.CanMoveTo(monsterID, newX, newY) {
		m.X = newX
		m.Y = newY
		m.Direction = dirAngle
		return true
	}

	return false
}

// Tile size constant for grid-based movement
const tileSize = 64.0

// updatePursuing moves monster towards player using grid-based cardinal movement
func (m *Monster3D) updatePursuing(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
	// Calculate distance to player
	dx := playerX - m.X
	dy := playerY - m.Y
	distanceToPlayer := math.Sqrt(dx*dx + dy*dy)

	// Check if close enough to attack
	if distanceToPlayer <= m.AttackRadius {
		m.State = StateAttacking
		m.StateTimer = 0
		return
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	// Use grid-based movement
	m.moveGridBased(collisionChecker, monsterID, playerX, playerY)
}

// moveGridBased moves the monster in cardinal directions (N/S/E/W) towards target
// Monsters move between tile centers to avoid getting stuck in gaps
func (m *Monster3D) moveGridBased(collisionChecker CollisionChecker, monsterID string, targetX, targetY float64) {
	// Calculate direction to target for determining which tile center to aim for
	dxToTarget := targetX - m.X
	dyToTarget := targetY - m.Y

	// Get current tile
	myTileX := int(math.Floor(m.X / tileSize))
	myTileY := int(math.Floor(m.Y / tileSize))
	targetTileX := int(math.Floor(targetX / tileSize))
	targetTileY := int(math.Floor(targetY / tileSize))

	// Already at target tile
	if myTileX == targetTileX && myTileY == targetTileY {
		return
	}

	// Determine the next tile we should move to (one step toward target)
	nextTileX := myTileX
	nextTileY := myTileY
	dxTiles := targetTileX - myTileX
	dyTiles := targetTileY - myTileY

	// Choose primary direction based on larger delta
	if abs(dxTiles) >= abs(dyTiles) && dxTiles != 0 {
		nextTileX += sign(dxTiles)
	} else if dyTiles != 0 {
		nextTileY += sign(dyTiles)
	}

	// Calculate the center of the next tile we want to reach
	nextCenterX := float64(nextTileX)*tileSize + tileSize/2
	nextCenterY := float64(nextTileY)*tileSize + tileSize/2

	// Current tile center (used for lane alignment)
	currentCenterX := float64(myTileX)*tileSize + tileSize/2
	currentCenterY := float64(myTileY)*tileSize + tileSize/2

	// Determine intended cardinal step (one axis only)
	stepX := 0.0
	stepY := 0.0
	if nextTileX != myTileX {
		stepX = float64(sign(nextTileX - myTileX))
	} else if nextTileY != myTileY {
		stepY = float64(sign(nextTileY - myTileY))
	}

	// Before moving to the next tile center, align to the current tile center on the perpendicular axis.
	// This keeps movement "center-to-center" and prevents the hitbox from grazing adjacent blocking tiles.
	if stepX != 0 {
		delta := currentCenterY - m.Y
		if math.Abs(delta) > m.Speed {
			alignY := m.Y + math.Copysign(m.Speed, delta)
			if collisionChecker.CanMoveTo(monsterID, m.X, alignY) {
				m.Y = alignY
				m.Direction = math.Atan2(math.Copysign(1, delta), 0)
				m.StuckCounter = 0
				return
			}
		}
	} else if stepY != 0 {
		delta := currentCenterX - m.X
		if math.Abs(delta) > m.Speed {
			alignX := m.X + math.Copysign(m.Speed, delta)
			if collisionChecker.CanMoveTo(monsterID, alignX, m.Y) {
				m.X = alignX
				m.Direction = math.Atan2(0, math.Copysign(1, delta))
				m.StuckCounter = 0
				return
			}
		}
	}

	// Check if the target tile is actually valid to stand on
	if collisionChecker.CanMoveTo(monsterID, nextCenterX, nextCenterY) {
		// Calculate distance to next tile center
		dxToNext := nextCenterX - m.X
		dyToNext := nextCenterY - m.Y
		distToNext := math.Sqrt(dxToNext*dxToNext + dyToNext*dyToNext)

		// If very close to target tile center, snap to it
		if distToNext < m.Speed*0.5 {
			m.X = nextCenterX
			m.Y = nextCenterY
			m.StuckCounter = 0
			return
		}

		// Step cardinally toward the next tile center, clamping so we don't overshoot.
		newX := m.X + stepX*m.Speed
		newY := m.Y + stepY*m.Speed
		if stepX != 0 {
			// keep lane aligned
			newY = currentCenterY
			if (stepX > 0 && newX > nextCenterX) || (stepX < 0 && newX < nextCenterX) {
				newX = nextCenterX
			}
		} else if stepY != 0 {
			newX = currentCenterX
			if (stepY > 0 && newY > nextCenterY) || (stepY < 0 && newY < nextCenterY) {
				newY = nextCenterY
			}
		}

		if collisionChecker.CanMoveTo(monsterID, newX, newY) {
			m.X = newX
			m.Y = newY
			m.Direction = math.Atan2(stepY, stepX)
			m.StuckCounter = 0
			return
		}
	}

	// Primary direction blocked - try secondary direction
	secondNextTileX := myTileX
	secondNextTileY := myTileY

	if abs(dxTiles) >= abs(dyTiles) && dyTiles != 0 {
		secondNextTileY += sign(dyTiles)
	} else if dxTiles != 0 {
		secondNextTileX += sign(dxTiles)
	}

	if secondNextTileX != myTileX || secondNextTileY != myTileY {
		secondCenterX := float64(secondNextTileX)*tileSize + tileSize/2
		secondCenterY := float64(secondNextTileY)*tileSize + tileSize/2

		secondStepX := 0.0
		secondStepY := 0.0
		if secondNextTileX != myTileX {
			secondStepX = float64(sign(secondNextTileX - myTileX))
		} else if secondNextTileY != myTileY {
			secondStepY = float64(sign(secondNextTileY - myTileY))
		}

		// Check if the secondary target tile is valid
		if collisionChecker.CanMoveTo(monsterID, secondCenterX, secondCenterY) {
			dxToSecond := secondCenterX - m.X
			dyToSecond := secondCenterY - m.Y
			distToSecond := math.Sqrt(dxToSecond*dxToSecond + dyToSecond*dyToSecond)
			if distToSecond < m.Speed*0.5 {
				m.X = secondCenterX
				m.Y = secondCenterY
				m.StuckCounter = 0
				return
			}

			newX := m.X + secondStepX*m.Speed
			newY := m.Y + secondStepY*m.Speed
			if secondStepX != 0 {
				newY = currentCenterY
				if (secondStepX > 0 && newX > secondCenterX) || (secondStepX < 0 && newX < secondCenterX) {
					newX = secondCenterX
				}
			} else if secondStepY != 0 {
				newX = currentCenterX
				if (secondStepY > 0 && newY > secondCenterY) || (secondStepY < 0 && newY < secondCenterY) {
					newY = secondCenterY
				}
			}

			if collisionChecker.CanMoveTo(monsterID, newX, newY) {
				m.X = newX
				m.Y = newY
				m.Direction = math.Atan2(secondStepY, secondStepX)
				m.StuckCounter = 0
				return
			}
		}
	}

	// Both directions blocked - try perpendicular directions to go around obstacle
	m.StuckCounter++

	if m.StuckCounter > 5 {
		// Try perpendicular directions
		var perpDirs [][2]int
		if math.Abs(dxToTarget) >= math.Abs(dyToTarget) {
			// Was trying to move horizontally, try vertical
			perpDirs = [][2]int{{0, 1}, {0, -1}}
		} else {
			// Was trying to move vertically, try horizontal
			perpDirs = [][2]int{{1, 0}, {-1, 0}}
		}

		// Use LastChosenDir to pick consistent side
		startIdx := 0
		if m.LastChosenDir < 0 {
			startIdx = 1
		}

		for i := 0; i < 2; i++ {
			idx := (startIdx + i) % 2
			perpTileX := myTileX + perpDirs[idx][0]
			perpTileY := myTileY + perpDirs[idx][1]
			perpCenterX := float64(perpTileX)*tileSize + tileSize/2
			perpCenterY := float64(perpTileY)*tileSize + tileSize/2

			if !collisionChecker.CanMoveTo(monsterID, perpCenterX, perpCenterY) {
				continue
			}

			dxPerp := perpCenterX - m.X
			dyPerp := perpCenterY - m.Y
			distPerp := math.Sqrt(dxPerp*dxPerp + dyPerp*dyPerp)
			if distPerp < m.Speed*0.5 {
				m.X = perpCenterX
				m.Y = perpCenterY
				m.StuckCounter = 0
				return
			}

			perpStepX := 0.0
			perpStepY := 0.0
			if perpTileX != myTileX {
				perpStepX = float64(sign(perpTileX - myTileX))
			} else if perpTileY != myTileY {
				perpStepY = float64(sign(perpTileY - myTileY))
			}

			newX := m.X + perpStepX*m.Speed
			newY := m.Y + perpStepY*m.Speed
			if perpStepX != 0 {
				newY = currentCenterY
				if (perpStepX > 0 && newX > perpCenterX) || (perpStepX < 0 && newX < perpCenterX) {
					newX = perpCenterX
				}
			} else if perpStepY != 0 {
				newX = currentCenterX
				if (perpStepY > 0 && newY > perpCenterY) || (perpStepY < 0 && newY < perpCenterY) {
					newY = perpCenterY
				}
			}

			if collisionChecker.CanMoveTo(monsterID, newX, newY) {
				m.X = newX
				m.Y = newY
				m.Direction = math.Atan2(perpStepY, perpStepX)
				// Remember which perpendicular direction worked
				if idx == 0 {
					m.LastChosenDir = 1
				} else {
					m.LastChosenDir = -1
				}
				return
			}
		}
	}
}

// tryMoveCardinal attempts to move one step in a cardinal direction
// Returns true if movement was successful
func (m *Monster3D) tryMoveCardinal(collisionChecker CollisionChecker, monsterID string, dirX, dirY int) bool {
	if dirX == 0 && dirY == 0 {
		return false
	}

	// Calculate target position (next tile center in this direction)
	currentTileX := math.Floor(m.X/tileSize)*tileSize + tileSize/2
	currentTileY := math.Floor(m.Y/tileSize)*tileSize + tileSize/2
	targetX := currentTileX + float64(dirX)*tileSize
	targetY := currentTileY + float64(dirY)*tileSize

	// Check if target tile is walkable
	if !collisionChecker.CanMoveTo(monsterID, targetX, targetY) {
		return false
	}

	// Move towards target tile center
	dirAngle := math.Atan2(float64(dirY), float64(dirX))
	newX := m.X + math.Cos(dirAngle)*m.Speed
	newY := m.Y + math.Sin(dirAngle)*m.Speed

	if collisionChecker.CanMoveTo(monsterID, newX, newY) {
		m.X = newX
		m.Y = newY
		m.Direction = dirAngle
		m.StuckCounter = 0
		return true
	}

	return false
}

// abs returns absolute value of an int
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// sign returns -1, 0, or 1 based on the sign of x
func sign(x int) int {
	if x > 0 {
		return 1
	} else if x < 0 {
		return -1
	}
	return 0
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

// updateFleeing moves monster away from player using grid-based movement
func (m *Monster3D) updateFleeing(collisionChecker CollisionChecker, monsterID string, playerX, playerY float64) {
	// Get AI config values
	var fleeSpeedMult float64 = 1.5 // Default value
	var fleeDuration int = 300      // Default value

	if m.config != nil {
		fleeSpeedMult = m.config.MonsterAI.FleeSpeedMultiplier
		fleeDuration = m.config.MonsterAI.FleeDuration
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	speed := m.Speed * fleeSpeedMult

	// Calculate direction away from player
	dx := m.X - playerX
	dy := m.Y - playerY

	// Determine best cardinal direction to flee (opposite of player direction)
	var fleeDirX, fleeDirY int
	if math.Abs(dx) >= math.Abs(dy) {
		fleeDirX = sign(int(dx))
		fleeDirY = 0
	} else {
		fleeDirX = 0
		fleeDirY = sign(int(dy))
	}

	// Try primary flee direction
	if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, fleeDirX, fleeDirY, speed) {
		return
	}

	// Try perpendicular directions
	if fleeDirX != 0 {
		// Was fleeing horizontally, try vertical
		if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, 0, 1, speed) {
			return
		}
		if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, 0, -1, speed) {
			return
		}
	} else {
		// Was fleeing vertically, try horizontal
		if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, 1, 0, speed) {
			return
		}
		if m.tryMoveCardinalWithSpeed(collisionChecker, monsterID, -1, 0, speed) {
			return
		}
	}

	// Stop fleeing after a while
	if m.StateTimer > fleeDuration {
		m.State = StateIdle
		m.StateTimer = 0
	}
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
