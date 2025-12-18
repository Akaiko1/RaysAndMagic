package game

import (
    "fmt"
    "math"
    "math/rand"
    "ugataima/internal/character"
    "ugataima/internal/monster"
)

// updateMonstersTurnBased handles monster updates in turn-based mode
func (gl *GameLoop) updateMonstersTurnBased() {
    if gl.game.currentTurn != 1 { // Not monster turn
        return
    }

    // Only monsters within 6-tile vision range participate in turn-based combat
    tileSize := float64(gl.game.config.GetTileSize())
    visionRange := tileSize * 6.0 // 6 tiles

    // Process each monster's turn (only those in vision range)
    monstersActed := 0
    for _, monster := range gl.game.world.Monsters {
        if !monster.IsAlive() {
            continue
        }

        // Calculate distance to player
        dx := monster.X - gl.game.camera.X
        dy := monster.Y - gl.game.camera.Y
        distance := math.Sqrt(dx*dx + dy*dy)

        // Skip monsters outside vision range - they don't participate in turn-based combat
        if distance > visionRange {
            continue
        }

        // Monster can either move 1 tile OR attack if within reach
        // Use collision radii and monster-specific attack radius for robust range checks
        playerRadius := 8.0 // default if entity not found
        if ent := gl.game.collisionSystem.GetEntityByID("player"); ent != nil && ent.BoundingBox != nil {
            playerRadius = math.Min(ent.BoundingBox.Width, ent.BoundingBox.Height) / 2
        }
        monsterRadius := 16.0 // default if entity not found
        if ent := gl.game.collisionSystem.GetEntityByID(monster.ID); ent != nil && ent.BoundingBox != nil {
            monsterRadius = math.Min(ent.BoundingBox.Width, ent.BoundingBox.Height) / 2
        }
        freeSpace := distance - (playerRadius + monsterRadius)
        reach := monster.AttackRadius
        if reach <= 0 {
            reach = tileSize * 0.25 // conservative fallback reach
        }

        if freeSpace <= reach {
            // Attack the current character
            gl.monsterAttackTurnBased(monster)
        } else {
            // Move 1 tile towards player (or preferred direction)
            gl.monsterMoveTurnBased(monster)
        }

        monstersActed++
    }

    // Always end monster turn and start party turn
    // Even if no monsters acted, we need to return control to the party
    gl.endMonsterTurn()
}

// monsterAttackTurnBased handles a monster attack in turn-based mode
func (gl *GameLoop) monsterAttackTurnBased(monster *monster.Monster3D) {
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

    // Calculate direction to player
    dx := gl.game.camera.X - monster.X
    dy := gl.game.camera.Y - monster.Y
    distance := math.Sqrt(dx*dx + dy*dy)

    if distance == 0 {
        return // Already at player position
    }

    // Normalize direction and move 1 tile
    dirX := dx / distance
    dirY := dy / distance

    newX := monster.X + dirX*tileSize
    newY := monster.Y + dirY*tileSize

    // Check if the monster can move to the new position
    if gl.game.collisionSystem.CanMoveTo(monster.ID, newX, newY) {
        monster.X = newX
        monster.Y = newY
        gl.game.collisionSystem.UpdateEntity(monster.ID, newX, newY)
        return
    }

    // Direct path blocked - in turn-based mode, teleport to closest valid tile towards player
    // This prevents monsters wasting turns stuck behind obstacles
    gl.teleportMonsterTowardsPlayer(monster, tileSize)
}

// teleportMonsterTowardsPlayer finds the closest valid position towards the player and teleports there
func (gl *GameLoop) teleportMonsterTowardsPlayer(m *monster.Monster3D, tileSize float64) {
    playerX := gl.game.camera.X
    playerY := gl.game.camera.Y

    // Check 8 adjacent tiles, pick the one closest to player
    bestX, bestY := m.X, m.Y
    bestDist := math.MaxFloat64

    for dy := -1; dy <= 1; dy++ {
        for dx := -1; dx <= 1; dx++ {
            if dx == 0 && dy == 0 {
                continue
            }
            testX := m.X + float64(dx)*tileSize
            testY := m.Y + float64(dy)*tileSize

            if gl.game.collisionSystem.CanMoveTo(m.ID, testX, testY) {
                dist := (testX-playerX)*(testX-playerX) + (testY-playerY)*(testY-playerY)
                if dist < bestDist {
                    bestDist = dist
                    bestX, bestY = testX, testY
                }
            }
        }
    }

    if bestDist < math.MaxFloat64 {
        m.X = bestX
        m.Y = bestY
        gl.game.collisionSystem.UpdateEntity(m.ID, bestX, bestY)
    }
    // If no valid position found, monster stays put (loses turn)
}

// endMonsterTurn ends the monster turn and starts party turn
func (gl *GameLoop) endMonsterTurn() {
    gl.game.currentTurn = 0 // Party turn
    gl.game.partyActionsUsed = 0
    // Don't spam combat log with turn messages
}

