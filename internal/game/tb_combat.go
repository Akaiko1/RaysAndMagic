package game

import (
	"fmt"
	"math"
	"math/rand"
	"ugataima/internal/character"
	"ugataima/internal/mathutil"
	"ugataima/internal/monster"
)

// updateMonstersTurnBased handles monster updates in turn-based mode
// This function only executes ONCE per monster turn (when monsterTurnResolved is false)
func (gl *GameLoop) updateMonstersTurnBased() {
	if gl.game.currentTurn != 1 { // Not monster turn
		return
	}
	if gl.game.monsterTurnResolved {
		return
	}

	// Only monsters within 6-tile vision range participate in turn-based combat
	tileSize := float64(gl.game.config.GetTileSize())
	visionRange := tileSize * 6.0 // 6 tiles

	// Cache player radius ONCE before the loop (was being looked up for each monster)
	playerRadius := 8.0 // default if entity not found
	if ent := gl.game.collisionSystem.GetEntityByID("player"); ent != nil && ent.BoundingBox != nil {
		playerRadius = math.Min(ent.BoundingBox.Width, ent.BoundingBox.Height) / 2
	}

	// Cache player position for the loop
	playerX, playerY := gl.game.camera.X, gl.game.camera.Y

	// Process each monster's turn (only those in vision range)
	for _, m := range gl.game.world.Monsters {
		if !m.IsAlive() {
			continue
		}

		// Calculate distance to player
		dist := Distance(playerX, playerY, m.X, m.Y)

		// Skip monsters outside vision range unless they've been engaged by a hit
		if dist > visionRange && !m.IsEngagingPlayer {
			continue
		}

		// Get monster radius for attack range calculation
		monsterRadius := 16.0 // default
		if ent := gl.game.collisionSystem.GetEntityByID(m.ID); ent != nil && ent.BoundingBox != nil {
			monsterRadius = math.Min(ent.BoundingBox.Width, ent.BoundingBox.Height) / 2
		}

		freeSpace := dist - (playerRadius + monsterRadius)
		reach := m.GetAttackRangePixels()
		if reach <= 0 {
			reach = tileSize * 0.25 // conservative fallback reach
		}

		inPerpendicularPosition := gl.isMonsterPerpendicularToPlayer(m, tileSize)
		canAttack := freeSpace <= reach
		if canAttack && inPerpendicularPosition {
			// Attack only from perpendicular positions (N/E/S/W)
			gl.monsterAttackTurnBased(m)
		} else {
			// Move 1 tile towards player using perpendicular steps
			gl.monsterMoveTurnBased(m)
		}
	}

	// Mark monster turn as processed before ending turn
	gl.game.monsterTurnResolved = true

	// Always end monster turn and start party turn
	// Even if no monsters acted, we need to return control to the party
	gl.endMonsterTurn()
}

// monsterAttackTurnBased handles a monster attack in turn-based mode
func (gl *GameLoop) monsterAttackTurnBased(monster *monster.Monster3D) {
	const attackAnimFrames = 8
	monster.AttackAnimFrames = attackAnimFrames
	monster.LastMoveTick = gl.game.frameCount

	if monster.HasRangedAttack() {
		gl.game.combat.spawnMonsterRangedAttack(monster)
		return
	}

	// Attack a random party character
	targetIndex := rand.Intn(len(gl.game.party.Members))
	target := gl.game.party.Members[targetIndex]

	damage := monster.GetAttackDamage()

	// Apply armor damage reduction
	finalDamage := gl.combat.ApplyArmorDamageReduction(damage, target)

	// Perfect Dodge: luck/5% roll to avoid all damage
	if dodged, _ := gl.combat.RollPerfectDodge(target); !dodged {
		target.HitPoints -= finalDamage
		if target.HitPoints < 0 {
			target.HitPoints = 0
		}
		if target.HitPoints == 0 {
			target.AddCondition(character.ConditionUnconscious)
		}
		// Trigger damage blink effect
		gl.game.TriggerDamageBlink(targetIndex)

		gl.game.AddCombatMessage(fmt.Sprintf("%s attacks %s for %d damage!", monster.Name, target.Name, finalDamage))
	} else {
		gl.game.AddCombatMessage(fmt.Sprintf("Perfect Dodge! %s evades %s's attack!", target.Name, monster.Name))
	}
}

// monsterMoveTurnBased handles a monster move in turn-based mode
func (gl *GameLoop) monsterMoveTurnBased(monster *monster.Monster3D) {
	tileSize := float64(gl.game.config.GetTileSize())

	// Calculate grid deltas to player (tile-based)
	monsterTileX := int(monster.X / tileSize)
	monsterTileY := int(monster.Y / tileSize)
	playerTileX, playerTileY := gl.game.GetPlayerTilePosition()

	dxTiles := playerTileX - monsterTileX
	dyTiles := playerTileY - monsterTileY

	if dxTiles == 0 && dyTiles == 0 {
		return // Already at player position
	}

	// Move 1 tile in a perpendicular (cardinal) direction towards the player
	stepX, stepY := 0, 0
	if math.Abs(float64(dxTiles)) >= math.Abs(float64(dyTiles)) {
		stepX = mathutil.IntSign(dxTiles)
	} else {
		stepY = mathutil.IntSign(dyTiles)
	}

	newX := monster.X + float64(stepX)*tileSize
	newY := monster.Y + float64(stepY)*tileSize

	// Check if the monster can move to the new position
	if gl.game.collisionSystem.CanMoveToWithHabitat(monster.ID, newX, newY, monster.HabitatPrefs, monster.Flying) {
		monster.X = newX
		monster.Y = newY
		gl.game.collisionSystem.UpdateEntity(monster.ID, newX, newY)
		monster.LastMoveTick = gl.game.frameCount
		return
	}

	// Try the other perpendicular direction if the preferred one is blocked
	if stepX != 0 && dyTiles != 0 {
		altX := monster.X
		altY := monster.Y + float64(mathutil.IntSign(dyTiles))*tileSize
		if gl.game.collisionSystem.CanMoveToWithHabitat(monster.ID, altX, altY, monster.HabitatPrefs, monster.Flying) {
			monster.X = altX
			monster.Y = altY
			gl.game.collisionSystem.UpdateEntity(monster.ID, altX, altY)
			monster.LastMoveTick = gl.game.frameCount
			return
		}
	} else if stepY != 0 && dxTiles != 0 {
		altX := monster.X + float64(mathutil.IntSign(dxTiles))*tileSize
		altY := monster.Y
		if gl.game.collisionSystem.CanMoveToWithHabitat(monster.ID, altX, altY, monster.HabitatPrefs, monster.Flying) {
			monster.X = altX
			monster.Y = altY
			gl.game.collisionSystem.UpdateEntity(monster.ID, altX, altY)
			monster.LastMoveTick = gl.game.frameCount
			return
		}
	}

	// Direct path blocked - in turn-based mode, teleport to closest valid tile towards player
	// This prevents monsters wasting turns stuck behind obstacles
	gl.teleportMonsterTowardsPlayer(monster, tileSize)
}

// teleportMonsterTowardsPlayer finds the closest valid position towards the player and teleports there
func (gl *GameLoop) teleportMonsterTowardsPlayer(m *monster.Monster3D, tileSize float64) {
	playerX := gl.game.camera.X
	playerY := gl.game.camera.Y

	// Check perpendicular adjacent tiles first, then diagonals as fallback
	var bestX, bestY float64
	bestDist := math.MaxFloat64

	cardinalOffsets := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	diagOffsets := [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

	bestX, bestY, bestDist = gl.pickBestTeleportOffset(m, tileSize, playerX, playerY, cardinalOffsets, bestDist)
	if bestDist == math.MaxFloat64 {
		bestX, bestY, bestDist = gl.pickBestTeleportOffset(m, tileSize, playerX, playerY, diagOffsets, bestDist)
	}

	if bestDist < math.MaxFloat64 {
		m.X = bestX
		m.Y = bestY
		gl.game.collisionSystem.UpdateEntity(m.ID, bestX, bestY)
		m.LastMoveTick = gl.game.frameCount
	}
	// If no valid position found, monster stays put (loses turn)
}

func (gl *GameLoop) pickBestTeleportOffset(m *monster.Monster3D, tileSize, playerX, playerY float64, offsets [][2]int, bestDist float64) (float64, float64, float64) {
	bestX, bestY := m.X, m.Y
	for _, offset := range offsets {
		testX := m.X + float64(offset[0])*tileSize
		testY := m.Y + float64(offset[1])*tileSize
		if gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, testX, testY, m.HabitatPrefs, m.Flying) {
			dist := (testX-playerX)*(testX-playerX) + (testY-playerY)*(testY-playerY)
			if dist < bestDist {
				bestDist = dist
				bestX, bestY = testX, testY
			}
		}
	}
	return bestX, bestY, bestDist
}

func (gl *GameLoop) isMonsterPerpendicularToPlayer(monster *monster.Monster3D, tileSize float64) bool {
	monsterTileX := int(monster.X / tileSize)
	monsterTileY := int(monster.Y / tileSize)
	playerTileX, playerTileY := gl.game.GetPlayerTilePosition()
	return monsterTileX == playerTileX || monsterTileY == playerTileY
}

// endMonsterTurn ends the monster turn and starts party turn
func (gl *GameLoop) endMonsterTurn() {
	gl.game.currentTurn = 0 // Party turn
	gl.game.partyActionsUsed = 0
	gl.game.monsterTurnResolved = true
	// Don't spam combat log with turn messages
}
