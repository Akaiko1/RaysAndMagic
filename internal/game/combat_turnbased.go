package game

import (
	"fmt"
	"math"
	"math/rand"
	"ugataima/internal/character"
	"ugataima/internal/mathutil"
	"ugataima/internal/monster"
)

// updateMonstersTurnBased handles monster updates in turn-based mode.
// A monster turn usually gives every participating monster one action pass.
// If the party attacked/cast and then retreated in the previous party turn,
// monsters get a second action pass as catch-up pressure.
func (gl *GameLoop) updateMonstersTurnBased() {
	if gl.game.currentTurn != 1 { // Not monster turn
		return
	}
	if gl.game.monsterTurnResolved {
		return
	}

	// Only monsters within vision range participate in turn-based combat
	tileSize := float64(gl.game.config.GetTileSize())
	visionRange := tileSize * TurnBasedVisionRangeTiles

	// Cache player position for the loop
	playerX, playerY := gl.game.camera.X, gl.game.camera.Y

	if gl.game.turnBasedMonsterPassesLeft <= 0 {
		// Persistent damage zones (Hot Steam) sear once per monster turn in TB.
		gl.tickSteamZonesTB()

		gl.game.turnBasedMonsterPassesLeft = 1
		if gl.game.turnBasedExtraMonsterAction {
			gl.game.turnBasedMonsterPassesLeft = 2
			gl.game.turnBasedExtraMonsterAction = false
		}
		gl.game.turnBasedMonsterStatusTick = false
		gl.game.turnBasedMonsterStunned = make(map[*monster.Monster3D]bool)
	}

	if gl.game.turnBasedMonsterPassDelay > 0 {
		gl.game.turnBasedMonsterPassDelay--
		return
	}

	tickTurnStatuses := !gl.game.turnBasedMonsterStatusTick
	gl.game.turnBasedMonsterStatusTick = true

	// Process each monster's turn (only those in vision range).
	for _, m := range gl.game.world.Monsters {
		if !m.IsAlive() {
			continue
		}
		if gl.game.turnBasedMonsterStunned[m] {
			gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
			continue
		}
		if tickTurnStatuses && m.StunTurnsRemaining > 0 {
			m.StunTurnsRemaining--
			gl.game.turnBasedMonsterStunned[m] = true
			gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
			continue
		}
		// Root (bear trap) burns one turn per monster TURN — whether it moves
		// or stands adjacent and attacks (root pins movement, not actions).
		// MUST tick before the Pacified/Bound branches: a bound undead still
		// moves through monsterMoveTurnBased and its root must hold and decay.
		if tickTurnStatuses {
			m.TickRootTurn()
		}

		// Match real-time AI: sealed bosses, warded warlords, and ward idols are
		// inert in TB too. They hold their placed tile and never spend the monster
		// turn moving or attacking while the seal/ward condition is active.
		if m.BossDormant || m.BossWarded || m.WarlordIdol {
			gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
			continue
		}

		// Pacified (Charm): holds position, never acts against the party.
		if m.Pacified {
			continue
		}
		// Bound (Bind Undead): strikes an enemy in reach or steps toward the
		// nearest one. Never acts against the party. Spends its whole turn here.
		if m.Bound {
			if !gl.combat.boundAttackNearest(m) {
				gl.monsterMoveTurnBased(m) // no enemy in reach — close the distance
			}
			continue
		}

		// Passive monsters mirror RT behaviour: no move, no attack until hit.
		// The RT path enforces this in updatePlayerEngagementWithVision; the
		// TB scheduler skips engagement updates entirely, so re-check here.
		if m.PassiveUntilAttacked && !m.WasAttacked && !m.HatesActiveTrait() {
			continue
		}
		gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)

		// Skip monsters outside vision range unless engaged by a hit (they stay
		// fully idle — no centering, no move).
		if Distance(playerX, playerY, m.X, m.Y) > visionRange && !m.IsEngagingPlayer {
			continue
		}

		// Each participating monster snaps to the center of its current tile at
		// the start of its turn. Keeps TB strictly tile-to-tile and fixes
		// off-center spawns (e.g. encounter pirates) that would otherwise
		// stand/attack between tiles.
		gl.centerMonsterOnTile(m, tileSize)

		// Lured at a bound undead instead of the party: attack it (ranged mobs loose
		// a bolt from within range, melee strike from an adjacent tile), else step
		// toward it; never touch the party.
		if foe := m.AIFoe; foe != nil && foe.IsAlive() {
			mtx, mty := int(m.X/tileSize), int(m.Y/tileSize)
			ftx, fty := int(foe.X/tileSize), int(foe.Y/tileSize)
			adx, ady := mathutil.IntAbs(ftx-mtx), mathutil.IntAbs(fty-mty)
			manh := adx + ady
			chebyshev := adx
			if ady > chebyshev {
				chebyshev = ady
			}
			if m.HasRangedAttack() {
				rangeTiles := int(m.GetAttackRangePixels() / tileSize)
				if rangeTiles < 1 {
					rangeTiles = 1
				}
				if manh >= 1 && manh <= rangeTiles {
					m.AttackAnimFrames = MonsterAttackAnimFrames
					gl.combat.spawnMonsterRangedAttackAtMonster(m, foe, ProjectileOwnerMonsterAtBound)
				} else {
					gl.monsterMoveTurnBased(m)
				}
			} else if chebyshev == 1 {
				// Monster-vs-monster melee allows a diagonal-adjacent strike (unlike
				// the cardinal-only party rule) so crowded mobs can still connect.
				m.AttackAnimFrames = MonsterAttackAnimFrames
				gl.combat.monsterStrikeMonster(m, foe)
			} else {
				gl.monsterMoveTurnBased(m)
			}
			gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
			continue
		}

		// Boss specials (blink / Inferno); each TB turn is one action tick.
		// Runs AFTER the bound-undead check (matching RT order): a boss lured
		// at a bound foe spends its turn on that fight, not on party novas.
		// DESIGN: specials are rolled BEFORE range/movement checks, so in TB an
		// aggressive boss may cast Inferno or its low-HP blink from across the
		// room instead of closing in. RT gates these to the attack moment; the
		// asymmetry is intentional TB flavor — do not "fix" toward RT.
		if gl.combat.isBoss(m) {
			if gl.combat.updateBoss(m, m.BossCD == 0, true) {
				gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
				continue
			}
		}

		// Work in tile space: monsters never enter the player's tile and only
		// act from cardinally-aligned (N/S/E/W) tiles.
		mtx, mty := int(m.X/tileSize), int(m.Y/tileSize)
		ptx, pty := gl.game.GetPlayerTilePosition()
		dxT, dyT := ptx-mtx, pty-mty
		adX, adY := dxT, dyT
		if adX < 0 {
			adX = -adX
		}
		if adY < 0 {
			adY = -adY
		}
		manhattan := adX + adY

		// Pounce: from 2+ tiles away (within pounce range) leap onto a
		// cardinally-adjacent tile and strike. Brief turn cooldown.
		if m.CanPounce() {
			if m.PounceCDTurns > 0 {
				m.PounceCDTurns--
			}
			pounceTiles := int(m.PounceRangePixels / tileSize)
			if m.PounceCDTurns == 0 && manhattan >= 2 && manhattan <= pounceTiles {
				if _, landed := gl.combat.executePounce(m, playerX, playerY); landed {
					gl.game.AddCombatMessage(fmt.Sprintf("%s pounces at the party!", m.Name))
					gl.monsterAttackTurnBased(m)
					m.PounceCDTurns = 2
					gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
					continue
				}
				// Couldn't land adjacent — fall through to a normal step this turn.
			}
		}

		if m.HasRangedAttack() {
			// Ranged: only fire when on the player's row or column (never
			// diagonal) and within range; otherwise step toward the player.
			rangeTiles := int(m.GetAttackRangePixels() / tileSize)
			if rangeTiles < 1 {
				rangeTiles = 1
			}
			aligned := dxT == 0 || dyT == 0
			axisDist := adX
			if dxT == 0 {
				axisDist = adY
			}
			if aligned && axisDist >= 1 && axisDist <= rangeTiles {
				gl.monsterAttackTurnBased(m)
			} else {
				gl.monsterMoveTurnBased(m)
			}
		} else {
			// Melee: attack only from a cardinally-adjacent tile (Manhattan 1);
			// otherwise step one tile toward the player (never onto their tile).
			if manhattan == 1 {
				gl.monsterAttackTurnBased(m)
			} else {
				gl.monsterMoveTurnBased(m)
			}
		}

		gl.game.updateMonsterCollisionEngagement(m, playerX, playerY)
	}

	// Monsters finished moving: spring any traps they stepped onto.
	gl.combat.sweepTrapTriggers()

	gl.game.turnBasedMonsterPassesLeft--
	if gl.game.turnBasedMonsterPassesLeft > 0 {
		gl.game.turnBasedMonsterPassDelay = int(TurnBasedExtraMonsterActionDelaySeconds * float64(gl.game.config.GetTPS()))
		if gl.game.turnBasedMonsterPassDelay < 1 {
			gl.game.turnBasedMonsterPassDelay = 1
		}
		return
	}

	gl.game.turnBasedMonsterPassDelay = 0
	gl.game.turnBasedMonsterStatusTick = false
	gl.game.turnBasedMonsterStunned = nil

	// Mark monster turn as processed before ending turn
	gl.game.monsterTurnResolved = true

	// Always end monster turn and start party turn
	// Even if no monsters acted, we need to return control to the party
	gl.endMonsterTurn()
}

// monsterAttackTurnBased handles a monster attack in turn-based mode
func (gl *GameLoop) monsterAttackTurnBased(monster *monster.Monster3D) {
	monster.AttackAnimFrames = MonsterAttackAnimFrames
	monster.LastMoveTick = gl.game.frameCount

	attacks := monster.GetTurnBasedAttackCount()
	for hit := 0; hit < attacks; hit++ {
		if gl.game.combat.tryMonsterSpecialAbility(monster) {
			continue
		}
		if monster.HasRangedAttack() {
			gl.game.combat.spawnMonsterRangedAttackNormal(monster)
			continue
		}

		// Re-filter every iteration: a previous attack may have just KO'd
		// the only remaining target, in which case the rest are no-ops.
		alive := alivePartyIndices(gl.game.party.Members)
		if len(alive) == 0 {
			return
		}
		targetIndex := alive[rand.Intn(len(alive))]
		target := gl.game.party.Members[targetIndex]

		damage := monster.GetAttackDamage()
		// Armour-piercing attackers (Golden Thief Bug) bypass armor class; resistances
		// and buff mitigation still apply.
		finalDamage := gl.combat.mitigateCharacterDamage(damage, "physical", target, monster.IgnoresArmor)

		// Perfect Dodge: luck/5% roll to avoid all damage
		if dodged, _ := gl.combat.RollPerfectDodge(target); dodged {
			gl.game.AddCombatMessage(fmt.Sprintf("Perfect Dodge! %s evades %s's attack!", target.Name, monster.Name))
			continue
		}

		target.HitPoints -= finalDamage
		if target.HitPoints < 0 {
			target.HitPoints = 0
		}
		if target.HitPoints == 0 {
			target.AddCondition(character.ConditionUnconscious)
		}
		gl.game.TriggerDamageBlink(targetIndex)
		gl.combat.tryApplyMonsterPoison(monster, target)

		suffix := ""
		if attacks > 1 {
			suffix = fmt.Sprintf(" (%d/%d)", hit+1, attacks)
		}
		gl.game.AddCombatMessage(fmt.Sprintf("%s attacks %s for %d damage!%s", monster.Name, target.Name, finalDamage, suffix))
	}
}

// alivePartyIndices returns indices of party members who can still take a hit
// (HP > 0 and not unconscious). Order is preserved.
func alivePartyIndices(members []*character.MMCharacter) []int {
	indices := make([]int, 0, len(members))
	for i, m := range members {
		if m.CanAct() {
			indices = append(indices, i)
		}
	}
	return indices
}

// centerMonsterOnTile snaps the monster to the center of the tile it currently
// occupies, so turn-based movement stays strictly tile-to-tile. No-op if the
// tile center isn't reachable for this monster (wall/occupied).
func (gl *GameLoop) centerMonsterOnTile(m *monster.Monster3D, tileSize float64) {
	cx, cy := TileCenterFromTile(int(m.X/tileSize), int(m.Y/tileSize), tileSize)
	if cx == m.X && cy == m.Y {
		return
	}
	if gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, cx, cy, m.HabitatPrefs, m.Flying) {
		m.X, m.Y = cx, cy
		gl.game.collisionSystem.UpdateEntity(m.ID, cx, cy)
		m.LastMoveTick = gl.game.frameCount
	}
}

// monsterMoveTurnBased handles a monster move in turn-based mode
func (gl *GameLoop) monsterMoveTurnBased(monster *monster.Monster3D) {
	// Rooted (bear trap): pinned for the whole turn; the per-turn countdown
	// lives in TickRootTurn (root != stun — attacks still happen).
	if monster.RootHeld() {
		return
	}
	tileSize := float64(gl.game.config.GetTileSize())

	// Step toward the monster's AI target (party by default; a charmed monster is
	// redirected — bound undead toward its enemy, pacified toward itself = no move).
	monsterTileX := int(monster.X / tileSize)
	monsterTileY := int(monster.Y / tileSize)
	targetX, targetY := gl.combat.monsterAITargetPoint(monster)
	playerTileX, playerTileY := int(targetX/tileSize), int(targetY/tileSize)

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

// teleportMonsterTowardsPlayer finds the closest valid position towards the
// monster's AI target (party, or a charmed monster's redirected target) and
// teleports there.
func (gl *GameLoop) teleportMonsterTowardsPlayer(m *monster.Monster3D, tileSize float64) {
	playerX, playerY := gl.combat.monsterAITargetPoint(m)

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
	ptx, pty := gl.game.GetPlayerTilePosition()
	bestX, bestY := m.X, m.Y
	for _, offset := range offsets {
		testX := m.X + float64(offset[0])*tileSize
		testY := m.Y + float64(offset[1])*tileSize
		// Never teleport onto the player's tile: the player collision entity is
		// non-solid, so CanMoveToWithHabitat would otherwise allow a mob to stand
		// inside the party — the blocked-diagonal fallback's offsets can include it.
		if int(testX/tileSize) == ptx && int(testY/tileSize) == pty {
			continue
		}
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

// endMonsterTurn ends the monster turn and starts a fresh party turn. The
// slot refill + selectedChar reset live inside startPartyTurn.
func (gl *GameLoop) endMonsterTurn() {
	gl.game.currentTurn = 0 // Party turn
	gl.game.partyActionsUsed = 0
	gl.game.turnBasedMonsterPassesLeft = 0
	gl.game.turnBasedMonsterPassDelay = 0
	gl.game.turnBasedMonsterStatusTick = false
	gl.game.turnBasedMonsterStunned = nil
	gl.game.startPartyTurn()
	gl.game.monsterTurnResolved = true
	// Don't spam combat log with turn messages
}
