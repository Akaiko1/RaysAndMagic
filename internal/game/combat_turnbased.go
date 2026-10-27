package game

import (
	"math"
	"sort"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// separateStackedMonstersTB repairs accidental non-combat overlaps by moving
// them onto distinct neighbouring tile centres. Combatants with an active target
// deliberately keep shared tiles: that is the common transit-stack model, and
// scattering them here would undo the movement action they just spent passing
// through an occupied attack post. Calm social bands are intentional stacks too.
// Runs once per turn boundary via startPartyTurn and reuses only the read-only
// bandScatterRing order.
func (g *MMGame) separateStackedMonstersTB() {
	if g.world == nil || g.collisionSystem == nil {
		return
	}
	tile := float64(g.config.GetTileSize())
	separationRadius := tile * TurnBasedCalmStackSeparationRadiusTiles
	px, py := g.camera.X, g.camera.Y
	playerTile := [2]int{TileIndex(px, tile), TileIndex(py, tile)}
	byTile := map[[2]int][]*monster.Monster3D{}
	// Only byTile actors may be repaired, but every live actor reserves its
	// current tile. Otherwise a legacy stack can scatter onto a combat transit
	// participant that was deliberately excluded from repair.
	used := map[[2]int]bool{playerTile: true}
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		key := [2]int{TileIndex(m.X, tile), TileIndex(m.Y, tile)}
		used[key] = true
		if combatStackParticipant(g, m) {
			continue
		}
		if (m.Banding || (m.LootGuarding && m.BandID > 0)) && m.IsCalmForSocialBehavior() {
			continue // calm band stack: intentional, and re-stacked same tick anyway
		}
		if Distance(px, py, m.X, m.Y) > separationRadius && !m.IsInCombat() {
			continue // out of this fight - leave it be
		}
		byTile[key] = append(byTile[key], m)
	}
	keys := make([][2]int, 0, len(byTile))
	for k := range byTile {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][0] != keys[b][0] {
			return keys[a][0] < keys[b][0]
		}
		return keys[a][1] < keys[b][1]
	})
	// used is shared across clusters: every occupied tile is off-limits as a
	// destination, so two adjacent stacks can't scatter onto the same free tile
	// (calm-calm pass-through wouldn't stop them), onto a lone calm mob, or onto
	// a combatant intentionally sharing a transit stack.
	for _, k := range keys {
		cluster := byTile[k]
		if len(cluster) < 2 {
			continue
		}
		// Lowest-ID mob keeps the tile (a stable owner -> no role ping-pong); the
		// rest snap onto distinct free neighbours. Set-piece monsters (sealed or
		// warded bosses, warlord idols) must never leave their scripted tile - one
		// of them owns the tile and none of them ever scatters.
		sortMonstersByID(cluster)
		owner := 0
		for i, m := range cluster {
			if !g.monsterCanBeDisplaced(m) {
				owner = i
				break
			}
		}
		for i, m := range cluster {
			if i == owner || !g.monsterCanBeDisplaced(m) {
				continue
			}
			g.scatterMonsterToFreeTile(m, k[0], k[1], tile, used)
		}
	}
}

// scatterMonsterToFreeTile snaps m onto the first walkable tile CENTRE from the
// bandScatterRing search order around (ctx,cty) not already in used, marks it
// used, and replans the mob's path. Returns false if every ring tile is
// blocked/taken (the mob stays put). It reuses the ring ORDER constant only -
// band scatter's own logic is untouched.
func (g *MMGame) scatterMonsterToFreeTile(m *monster.Monster3D, ctx, cty int, tile float64, used map[[2]int]bool) bool {
	for _, d := range bandScatterRing {
		key := [2]int{ctx + d[0], cty + d[1]}
		if used[key] {
			continue
		}
		nx, ny := TileCenterFromTile(key[0], key[1], tile)
		if g.collisionSystem.CanMoveToWithTileOverrides(m.ID, nx, ny, m.WalkableTileOverrides, m.Flying) {
			used[key] = true
			g.placeMonster(m, nx, ny)
			return true
		}
	}
	return false
}

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

	tileSize := float64(gl.game.config.GetTileSize())

	if gl.game.turnBasedMonsterPassesLeft <= 0 {
		// Persistent damage zones (Hot Steam) sear once per monster turn in TB.
		gl.tickPersistentDamageZonesTB()
		gl.game.monsterTurnState.startPasses()
	}
	ready, tickTurnStatuses := gl.game.monsterTurnState.beginPass()
	if !ready {
		return
	}
	if tickTurnStatuses {
		// Resolve status damage and latch disabled turns for every actor before
		// releasing cross-actor specials. Extra passes reuse these results.
		for _, m := range gl.game.world.Monsters {
			if m.IsAlive() && gl.game.tickMonsterTurnStatuses(m, true) {
				gl.game.turnBasedMonsterStunned[m] = true
			}
		}
		gl.game.tickMonsterTelegraphs(true)
		gl.game.tickEnvironment(true)
	}
	gl.game.updateAuthoredBandAggro()

	gl.game.simulateRemoteEcology(true, tickTurnStatuses)

	// Process each monster's turn.
	for _, m := range gl.game.world.Monsters {
		gl.takeMonsterTurnTB(m, tileSize, tickTurnStatuses)
	}

	// Monsters finished moving: spring any traps they stepped onto, and burn
	// whatever walked into a damage zone. The entry pass must run AFTER the moves
	// and AFTER this round's periodic ticks stamped everyone already standing
	// inside, or a mob in a Firewall takes one hit per round too many.
	gl.game.combat.sweepTrapTriggers()
	gl.applyZoneEntryDamageAll()

	delay := gl.game.framesForSeconds(TurnBasedExtraMonsterActionDelaySeconds)
	if !gl.game.monsterTurnState.finishPass(delay) {
		return
	}

	// Always end monster turn and start party turn
	// Even if no monsters acted, we need to return control to the party
	gl.endMonsterTurn()
}

// takeMonsterTurnTB is one monster's turn-based action, on the party's world
// and on a remote one (simulateRemoteEcology) alike: engagement, the return
// home, the behavior's own branch, telegraphs, boss specials and the combat
// step. A stunned monster or one whose cached foe already died waits.
func (gl *GameLoop) takeMonsterTurnTB(m *monster.Monster3D, tileSize float64, tickTurnStatuses bool) {
	// Earlier actors and telegraphs can move the party during this pass.
	playerX, playerY := gl.game.combat.logicalCameraXY()
	if !m.IsAlive() {
		return
	}

	m.LimitPlayerEngagement(playerX, playerY)
	if gl.game.turnBasedMonsterStunned[m] {
		gl.game.refreshMonsterCollisionState(m)
		return
	}

	// CurrentAIBehavior is the mode-independent owner of high-level precedence.
	// Keep every mode explicit here: adding a behavior to the policy without a TB
	// branch must not silently fall through into ordinary party combat.
	m.UpdatePlayerEngagement(gl.game.collisionSystem, playerX, playerY, m.AITargetX, m.AITargetY)
	if _, _, returning := m.ReturnHomeTarget(); returning {
		m.WalkTurn(func(bool) bool {
			nx, ny, move := m.NextReturnHomeStep(gl.game.collisionSystem)
			return move && gl.stepMonsterTB(m, nx, ny, tileSize)
		}, gl.turnBasedStepHazards(m))
		m.ReturnHomeTarget()
		gl.game.refreshMonsterCollisionState(m)
		return
	}
	behavior := m.CurrentAIBehavior()
	switch behavior {
	case monster.AIBehaviorAmbient:
		hazards, perceive := gl.turnBasedStepHazards(m), gl.game.ambientStepPerception(m)
		m.UpdateAmbient(gl.game.collisionSystem, m.AITargetX, m.AITargetY, true, func() bool { return hazards() && perceive() })
		gl.game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorInert:
		// Sealed bosses, warded warlords, and ward idols hold their placed tile.
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorPacified:
		// Charm must clear a pre-existing pursuit/attack state in TB just as it
		// does in RT; otherwise the actor remains visually combat-active.
		m.StandDownFromCombat()
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorEvasive:
		// Evasive quest bosses still react through tickEvasiveBossesTB above,
		// but never take a normal combat turn.
		m.StandDownFromCombat()
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorBoundAlly:
		// Bound (Bind Undead): strike an enemy in reach or step toward it;
		// without an enemy, follow the party without ever attacking it.
		if m.AIFoe != nil && !m.AIFoe.IsAlive() {
			// The cached target died earlier in this serial monster pass.
			// Do not spend the action walking toward its corpse.
			gl.game.releaseMonsterAttackPost(m)
			gl.game.refreshMonsterCollisionState(m)
			return
		}
		if foe := m.AIFoe; foe != nil && foe.IsAlive() && gl.tryMonsterAttackFoeTurnBased(m, foe) {
			// Claimed a distinct logical attack post and spent the turn striking.
		} else {
			gl.monsterMoveTurnBased(m) // no enemy in reach - close the distance
		}
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorFleeing:
		elapsedFrames := 0
		if tickTurnStatuses {
			elapsedFrames = gl.game.config.GetTPS()
		}
		// The first call advances the flee clock once per turn, moving or not.
		nx, ny, move := m.NextFleeTurnStep(gl.game.collisionSystem, playerX, playerY, elapsedFrames)
		m.WalkTurn(func(first bool) bool {
			if !first {
				nx, ny, move = m.NextFleeTurnStep(gl.game.collisionSystem, playerX, playerY, 0)
			}
			return move && gl.stepMonsterTB(m, nx, ny, tileSize)
		}, gl.turnBasedStepHazards(m))
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorPassive:
		// Passive monsters neither move nor attack until hit. Like every idle
		// monster in TB, they hold their tile: TB never patrols.
		m.StandDownFromCombat()
		gl.game.refreshMonsterCollisionState(m)
		return
	case monster.AIBehaviorFightFoe:
		// A previous actor can kill this frame's cached foe. Wait for the next
		// shared retarget instead of falling through to a party action.
		if m.AIFoe == nil || !m.AIFoe.IsAlive() {
			gl.game.releaseMonsterAttackPost(m)
			gl.game.refreshMonsterCollisionState(m)
			return
		}
	case monster.AIBehaviorRelentlessParty, monster.AIBehaviorSeekParty:
		// These modes return through the shared combat scheduler below.
	}
	gl.game.refreshMonsterCollisionState(m)
	if m.AIFoe == nil && !m.TargetsParty() {
		return
	}
	if !m.IsEngagingPlayer {
		m.BeginCombatEngagement()
	}

	// Each participating monster snaps to the center of its current tile at
	// the start of its turn. Keeps TB strictly tile-to-tile and fixes
	// off-center spawns (e.g. encounter pirates) that would otherwise
	// stand/attack between tiles.
	gl.centerMonsterOnTile(m, tileSize)

	if gl.game.combat.runTelegraphedAction(m, true) {
		gl.game.refreshMonsterCollisionState(m)
		return
	}

	// Boss specials; each TB turn is one action tick. BEFORE the bound-undead
	// check (matching RT order): a boss lured at a summon still sows traps,
	// rallies adds, enrages and blinks.
	// DESIGN: specials roll BEFORE range/movement checks, so an aggressive TB
	// boss may spend its turn on a special instead of closing in. A nova
	// is the exception with a real gate - bound to its authored
	// range_tiles in BOTH modes (it used to be map-wide here, which with
	// aggro_whole_map let the Golden Thief Bug burn the party from anywhere).
	if m.IsBoss() {
		if gl.game.combat.runBossSpecials(m, true, true) {
			if !gl.game.combat.bossEvasive(m) {
				gl.game.combat.armMonsterRTAttackCooldowns(m)
			}
		} else {
			gl.monsterCombatStepTB(m)
		}
	} else {
		gl.monsterCombatStepTB(m)
	}
	gl.game.refreshMonsterCollisionState(m)
}

// monsterCombatStepTB is a fighting monster's TB action after its specials:
// strike its foe or the party in reach, pounce, or step toward them.
func (gl *GameLoop) monsterCombatStepTB(m *monster.Monster3D) {
	// Lured at a bound undead instead of the party: attack it (ranged mobs loose
	// a bolt from within range, melee strike from an adjacent tile), else step
	// toward it; never touch the party.
	if foe := m.AIFoe; foe != nil && foe.IsAlive() {
		if gl.tryMonsterAttackFoeTurnBased(m, foe) {
			// Claimed a distinct logical attack post and spent the turn striking.
		} else if !gl.tryPhaseShiftTB(m) {
			// Phase Shift is this turn's movement; without it the walk runs.
			gl.monsterMoveTurnBased(m)
		}
		return
	}

	// Resolve aim after any earlier actor or special has displaced the party.
	playerX, playerY := gl.game.combat.logicalCameraXY()

	// Pounce: from 2+ tiles away (within pounce range) leap onto an adjacent
	// tile and strike. Brief turn cooldown.
	if m.CanPounce() {
		if gl.game.combat.monsterPouncePointInReachTB(m, playerX, playerY, m.PounceCD.Turns) {
			if gl.game.combat.executePounce(m, playerX, playerY) {
				gl.game.AddCombatMessage(uitext.Text("combat.pounces_at_the_party", m.Name))
				gl.monsterAttackTurnBased(m)
				m.ArmPounceCooldown(TurnBasedPounceCooldownTurns)
				return
			}
			// Couldn't land adjacent - fall through to a normal step this turn.
		}
	}

	if gl.game.combat.monsterAttackPointInReachTB(m, playerX, playerY) {
		if gl.game.tryClaimMonsterAttackPost(m) {
			m.State = monster.StateAttacking
			gl.monsterAttackTurnBased(m)
		} else {
			gl.game.releaseMonsterAttackPost(m)
			gl.monsterMoveTurnBased(m)
		}
	} else if !gl.tryPhaseShiftTB(m) {
		// Phase Shift is this turn's movement; without it the walk runs.
		gl.monsterMoveTurnBased(m)
	}
}

// monsterAttackPointInReachTB is the shared spatial rule for normal party attacks
// and the tactical overlay. It does not spend actions or reserve an attack post.
func (cs *CombatSystem) monsterAttackPointInReachTB(m *monster.Monster3D, x, y float64) bool {
	if cs == nil || cs.game == nil || m == nil {
		return false
	}
	ts := float64(cs.game.config.GetTileSize())
	if cs.monsterMeleeAdjacentToPoint(m, x, y) && cs.monsterUsesMeleeAgainstPoint(m, x, y) {
		return true
	}
	if !m.HasRangedAttack() {
		// Authored long melee reach (for example, a dragon's bite) remains
		// radial. Only projectile delivery requires a cardinal firing lane.
		return Distance(m.X, m.Y, x, y) <= m.GetAttackRangePixels() && cs.attackLineClear(m.X, m.Y, x, y)
	}
	dx, dy := TileIndex(x, ts)-TileIndex(m.X, ts), TileIndex(y, ts)-TileIndex(m.Y, ts)
	distance := absInt(dx) + absInt(dy)
	return (dx == 0 || dy == 0) && distance >= 1 &&
		distance <= max(1, int(m.GetAttackRangePixels()/ts)) &&
		Distance(m.X, m.Y, x, y) <= m.GetAttackRangePixels() && cs.attackLineClear(m.X, m.Y, x, y)
}

// monsterPouncePointInReachTB includes the landing rule, not just leap radius.
// Preview callers may supply the cooldown after the next turn's status tick.
func (cs *CombatSystem) monsterPouncePointInReachTB(m *monster.Monster3D, x, y float64, cooldown int) bool {
	if !m.CanPounce() || !m.TargetsParty() || cooldown > 0 {
		return false
	}
	if !cs.pounceGridReach(m, x, y) || !cs.attackLineClear(m.X, m.Y, x, y) {
		return false
	}
	_, _, ok := cs.pounceLanding(m, x, y)
	return ok
}

// pounceGridReach is the one pounce reach in both modes: the target's tile 2
// to range_tiles grid steps (|dx|+|dy|) from the monster's, so a leap covers
// the same tiles in RT and TB.
func (cs *CombatSystem) pounceGridReach(m *monster.Monster3D, x, y float64) bool {
	ts := float64(cs.game.config.GetTileSize())
	d := absInt(TileIndex(x, ts)-TileIndex(m.X, ts)) + absInt(TileIndex(y, ts)-TileIndex(m.Y, ts))
	return d >= 2 && d <= int(m.PounceRangePixels()/ts)
}

// monsterAttackTurnBased handles a monster attack in turn-based mode.
func (gl *GameLoop) monsterAttackTurnBased(m *monster.Monster3D) {
	gl.game.combat.commitMonsterAttack(m, monsterAttackDestination{}, monsterAttackTurn)
}

// tryMonsterAttackFoeTurnBased applies the same logical-post gate used for
// party attacks before a monster attacks a summon, bound undead, or other foe.
// The entities stay physically pass-through; a rejected contender simply keeps
// pursuing another free tile around the target.
func (gl *GameLoop) tryMonsterAttackFoeTurnBased(attacker, foe *monster.Monster3D) bool {
	if gl == nil || gl.game == nil || gl.game.combat == nil || foe == nil {
		return false
	}
	if !gl.game.combat.commitMonsterAttack(attacker, monsterAttackDestination{foe: foe}, monsterAttackTurn) {
		return false
	}
	attacker.StateTimer = 0
	return true
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

// commitMonsterMoveTB moves the monster to the tile-center (wx, wy) when the
// monster-terrain-aware collision check passes, updating its collision entity and turn
// stamp. Returns whether the monster moved.
func (gl *GameLoop) commitMonsterMoveTB(m *monster.Monster3D, wx, wy float64) bool {
	if !gl.game.monsterCanEnterTileTB(m, TileIndex(wx, gl.game.config.GetTileSize()), TileIndex(wy, gl.game.config.GetTileSize())) {
		return false
	}
	tileSize := float64(gl.game.config.GetTileSize())
	movedTile := tileSize > 0 && (TileIndex(m.X, tileSize) != TileIndex(wx, tileSize) || TileIndex(m.Y, tileSize) != TileIndex(wy, tileSize))
	if movedTile {
		gl.game.releaseMonsterAttackPost(m)
	}
	oldX, oldY := m.X, m.Y
	m.X = wx
	m.Y = wy
	if movedTile {
		// TB computes a fresh discrete A* step. Its new position invalidates the
		// continuous RT route, but not a flee/guard objective shared across turns.
		m.ResetPathCache()
	}
	gl.game.collisionSystem.UpdateEntity(m.ID, wx, wy)
	if movedTile {
		gl.game.observeOverwatchMovement(m, oldX, oldY)
	}
	return true
}

// centerMonsterOnTile snaps the monster to the center of the tile it currently
// occupies, so turn-based movement stays strictly tile-to-tile. No-op if the
// tile center isn't reachable for this monster (wall/occupied).
func (gl *GameLoop) centerMonsterOnTile(m *monster.Monster3D, tileSize float64) {
	cx, cy := TileCenterFromTile(TileIndex(m.X, tileSize), TileIndex(m.Y, tileSize), tileSize)
	if cx == m.X && cy == m.Y {
		return
	}
	gl.commitMonsterMoveTB(m, cx, cy)
}

// monsterMoveTurnBased is the ONE turn-based approach entry point: one
// movement action at the monster's step rate (monster.WalkTurn). Extra steps
// stop as soon as the monster can strike its target from a post it can claim,
// so moving and attacking stay separate actions; a held post is walked through.
// A foe that died mid-walk ends the walk. Goal selection differs by attacker kind (melee:
// adjacent tile; ranged vs party: firing lane; ranged vs a monster foe: plain
// approach).
func (gl *GameLoop) monsterMoveTurnBased(m *monster.Monster3D) {
	tileSize := float64(gl.game.config.GetTileSize())
	m.WalkTurn(func(first bool) bool {
		if first {
			// A mob that is searching for a post is transit even if it reached
			// this method from an old held position. Physical overlap remains
			// allowed; only its attack claim is released.
			gl.game.releaseMonsterAttackPost(m)
			if !gl.monsterCanStepTB(m) {
				return false
			}
		} else if foe := m.AIFoe; (foe != nil && !foe.IsAlive()) || gl.monsterCanStrikeTB(m) {
			return false
		}
		tx, ty := TileIndex(m.X, tileSize), TileIndex(m.Y, tileSize)
		gl.monsterApproachStepTB(m, tileSize)
		return TileIndex(m.X, tileSize) != tx || TileIndex(m.Y, tileSize) != ty
	}, gl.turnBasedStepHazards(m))
}

// stepMonsterTB commits one walking step to tile (tx, ty) if movement is not
// held; it reports whether the monster moved.
func (gl *GameLoop) stepMonsterTB(m *monster.Monster3D, tx, ty int, tileSize float64) bool {
	if !gl.monsterCanStepTB(m) {
		return false
	}
	wx, wy := TileCenterFromTile(tx, ty, tileSize)
	return gl.commitMonsterMoveTB(m, wx, wy)
}

// turnBasedStepHazards runs between a fast monster's steps: the tile it just
// crossed springs its traps and burns its damage zones as an RT walk would (the
// last tile is left to the end-of-pass sweep), and a monster the hazard killed,
// rooted or stunned stops walking. Only the walker meets these hazards.
func (gl *GameLoop) turnBasedStepHazards(m *monster.Monster3D) func() bool {
	return func() bool {
		gl.game.combat.springTrapsUnder(m)
		gl.applyZoneEntryDamageFor(m)
		return m.IsAlive() && gl.monsterCanStepTB(m)
	}
}

// monsterCanStrikeTB reports whether the monster already stands where it can
// attack its target from a post it can claim, so extra steps end at an attack
// position. An escort without a foe has no attack position: its step stops in
// formation instead.
func (gl *GameLoop) monsterCanStrikeTB(m *monster.Monster3D) bool {
	cs := gl.game.combat
	if foe := m.AIFoe; foe != nil {
		if !foe.IsAlive() || !cs.monsterCanAttackMonster(m, foe) {
			return false
		}
	} else {
		if m.Bound || !m.TargetsParty() {
			return false
		}
		x, y := cs.logicalCameraXY()
		if !cs.monsterAttackPointInReachTB(m, x, y) {
			return false
		}
	}
	_, open := gl.game.monsterAttackPostOpen(m)
	return open
}

// monsterApproachStepTB is one approach step toward the monster's target.
func (gl *GameLoop) monsterApproachStepTB(monster *monster.Monster3D, tileSize float64) {
	// Step toward the monster's AI target (party by default; a charmed monster is
	// redirected - bound undead toward its enemy, pacified toward itself = no move).
	monsterTileX := TileIndex(monster.X, tileSize)
	monsterTileY := TileIndex(monster.Y, tileSize)
	targetX, targetY := gl.game.combat.monsterAITargetPoint(monster)
	playerTileX, playerTileY := TileIndex(targetX, tileSize), TileIndex(targetY, tileSize)

	dxTiles := playerTileX - monsterTileX
	dyTiles := playerTileY - monsterTileY

	if dxTiles == 0 && dyTiles == 0 {
		// A pre-existing overlap (for example an old save made before generic
		// attack posts) must self-heal instead of leaving the monster unable to
		// move or attack on its target's tile.
		if gl.game.monsterHasAttackTarget(monster) {
			gl.moveMonsterOffAttackTargetTileTB(monster, targetX, targetY, tileSize)
		}
		return // Already at player position
	}

	// Escorting ally (Bound, no enemy): holds the follow distance instead of its
	// attack reach, melee allies included - no adjacent post to claim.
	if monster.Bound && monster.AIFoe == nil {
		// Distance alone would let an ally "keep formation" from the far side of a
		// wall, so it must also SEE the party - the same pair the RT pursuit uses.
		inFormation := Distance(monster.X, monster.Y, targetX, targetY) <= monster.PursuitReachPixels() &&
			gl.game.attackLineClear(monster.X, monster.Y, targetX, targetY)
		if inFormation {
			return
		}
		if nx, ny, ok := monster.NextPathStepTile(gl.game.collisionSystem, targetX, targetY); ok {
			wx, wy := TileCenterFromTile(nx, ny, tileSize)
			gl.commitMonsterMoveTB(monster, wx, wy)
		}
		return
	}

	// Phase Shift plan: no walking route worth taking, so walk to the launch
	// tile and hold there; the blink is a later turn's movement.
	if gl.stepTowardPhaseLaunchTB(monster, tileSize) {
		return
	}

	// A* FIRST, toward the TB strike goals (turnBasedApproachGoals). A ranged
	// attacker without a lane goal walks straight at its target.
	if gl.moveMonsterAlongTBGoals(monster, gl.game.turnBasedApproachGoals(monster, targetX, targetY), tileSize) {
		return
	}
	if monster.HasRangedAttack() {
		if nx, ny, ok := monster.NextPathStepTile(gl.game.collisionSystem, targetX, targetY); ok {
			wx, wy := TileCenterFromTile(nx, ny, tileSize)
			if gl.commitMonsterMoveTB(monster, wx, wy) {
				return
			}
		}
	}

	// A* has no legal route (or its next tile became occupied). Do not fall back
	// to a separate greedy movement rule: it can enter a dead end that the path
	// planner deliberately avoided, producing visible back-and-forth jitter.
	// Holding this turn lets the next serial AI pass re-evaluate the same source
	// of truth after moving actors have updated their positions.
}

// moveMonsterAlongTBGoals is the common serial TB A* commit path. Combatants
// are already pass-through at the collision layer; logical attack posts decide
// who may stop and attack around a target.
func (gl *GameLoop) moveMonsterAlongTBGoals(m *monster.Monster3D, goals []monster.TileCoord, tileSize float64) bool {
	if m == nil || len(goals) == 0 || tileSize <= 0 || gl == nil || gl.game == nil || gl.game.collisionSystem == nil {
		return false
	}
	if nx, ny, ok := m.NextPathStepTileToAny(gl.game.collisionSystem, goals, gl.game.attackTargetTile(m)); ok {
		wx, wy := TileCenterFromTile(nx, ny, tileSize)
		if gl.commitMonsterMoveTB(m, wx, wy) {
			return true
		}
	}
	return false
}

// turnBasedApproachGoals is where a TB walker heads to strike its target, for
// the mover and the Phase Shift planner alike: a melee attacker's reach ring
// (with every post taken, the next free ring - never a greedy cardinal step),
// or a ranged attacker hunting the party, a row or column firing lane. Nil:
// walk straight at the target.
func (g *MMGame) turnBasedApproachGoals(m *monster.Monster3D, targetX, targetY float64) []monster.TileCoord {
	if !m.HasRangedAttack() {
		if goals := g.turnBasedMeleeGoalTiles(m, targetX, targetY); len(goals) > 0 {
			return goals
		}
		return g.turnBasedBlockedMeleeApproachGoalTiles(m, targetX, targetY)
	}
	// Against a monster foe there is no alignment rule - plain approach.
	if m.AIFoe == nil && !m.Bound && g.collisionSystem != nil {
		return g.turnBasedRangedGoalTiles(m)
	}
	return nil
}

// turnBasedMeleeGoalTiles are the tiles a melee attacker strikes its target
// from under the TB reach rule (monsterAttackPointInReachTB): the eight
// neighbours, and for an authored long reach (a dragon's bite) every tile
// within it on a clear line. Never the RT "within attack radius" goals: they
// can pick a dead-end bank tile across water as "close enough".
func (g *MMGame) turnBasedMeleeGoalTiles(m *monster.Monster3D, targetX, targetY float64) []monster.TileCoord {
	if m == nil || g == nil || g.collisionSystem == nil {
		return nil
	}
	tileSize := float64(g.config.GetTileSize())
	targetTileX, targetTileY := TileIndex(targetX, tileSize), TileIndex(targetY, tileSize)

	goals := make([]monster.TileCoord, 0, 24)
	addGoal := func(tx, ty int, requireLOS bool) {
		wx, wy := TileCenterFromTile(tx, ty, tileSize)
		if g.monsterHasAttackTarget(m) && g.collisionSystem.IsMonsterAttackPostReserved(m.ID, wx, wy) {
			return
		}
		if !g.collisionSystem.CanMoveToWithTileOverrides(m.ID, wx, wy, m.WalkableTileOverrides, m.Flying) {
			return
		}
		if requireLOS && !g.combat.attackLineClear(wx, wy, targetX, targetY) {
			return
		}
		goals = append(goals, monster.TileCoord{X: tx, Y: ty})
	}

	reach := m.GetAttackRangePixels()
	ring := max(1, int(math.Ceil(reach/tileSize)))
	for dy := -ring; dy <= ring; dy++ {
		for dx := -ring; dx <= ring; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if absInt(dx) > 1 || absInt(dy) > 1 {
				wx, wy := TileCenterFromTile(targetTileX+dx, targetTileY+dy, tileSize)
				if Distance(wx, wy, targetX, targetY) > reach {
					continue
				}
			}
			addGoal(targetTileX+dx, targetTileY+dy, true)
		}
	}

	return uniqueTileGoals(goals)
}

// turnBasedBlockedMeleeApproachGoalTiles supplies a second A* goal ring only
// when no adjacent attack post is currently free. These are never attack posts:
// they let a pouncer advance after its landing ring is occupied without using a
// separate greedy step that could disagree with terrain override pathing.
// The ring itself is shared with RT pursuit (MeleeApproachRingGoals).
func (g *MMGame) turnBasedBlockedMeleeApproachGoalTiles(m *monster.Monster3D, targetX, targetY float64) []monster.TileCoord {
	if m == nil || g == nil || g.collisionSystem == nil {
		return nil
	}
	return m.MeleeApproachRingGoals(g.collisionSystem, targetX, targetY)
}

// moveMonsterOffAttackTargetTileTB repairs an attacker that starts a turn on
// its target's tile. It is a recovery path only; normal movement already
// selects surrounding attack posts before this can happen.
func (gl *GameLoop) moveMonsterOffAttackTargetTileTB(m *monster.Monster3D, targetX, targetY, tileSize float64) bool {
	if m == nil || gl == nil || gl.game == nil || gl.game.collisionSystem == nil || tileSize <= 0 {
		return false
	}
	tx, ty := TileIndex(targetX, tileSize), TileIndex(targetY, tileSize)
	for _, offset := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		x, y := TileCenterFromTile(tx+offset[0], ty+offset[1], tileSize)
		if gl.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, x, y) {
			continue
		}
		if gl.commitMonsterMoveTB(m, x, y) {
			return true
		}
	}
	return false
}

func uniqueTileGoals(goals []monster.TileCoord) []monster.TileCoord {
	if len(goals) < 2 {
		return goals
	}
	out := goals[:0]
	seen := make(map[monster.TileCoord]bool, len(goals))
	for _, goal := range goals {
		if seen[goal] {
			continue
		}
		seen[goal] = true
		out = append(out, goal)
	}
	return out
}

// turnBasedRangedGoalTiles lists a party-hunting ranged monster's turn-based
// firing lanes: free tiles on the party's row or column, within range and with
// line of sight. The generic monster A* uses circular attack reach, which is
// correct for RT ranged mobs but bad for TB: it can pick a diagonal in-range
// tile where the monster still cannot shoot, causing archers to shuffle around
// other archers instead of taking a second firing position.
func (g *MMGame) turnBasedRangedGoalTiles(m *monster.Monster3D) []monster.TileCoord {
	if m == nil || g == nil || g.collisionSystem == nil {
		return nil
	}
	tileSize := float64(g.config.GetTileSize())
	rangeTiles := int(m.GetAttackRangePixels() / tileSize)
	if rangeTiles < 1 {
		rangeTiles = 1
	}
	playerX, playerY := g.partyFightPos() // logical: the TB overlay reads this during Draw's shake
	ptx, pty := TileIndex(playerX, tileSize), TileIndex(playerY, tileSize)

	goals := make([]monster.TileCoord, 0, rangeTiles*4)
	addGoal := func(tx, ty int) {
		wx, wy := TileCenterFromTile(tx, ty, tileSize)
		if g.collisionSystem.IsMonsterAttackPostReserved(m.ID, wx, wy) {
			return
		}
		if !g.collisionSystem.CanMoveToWithTileOverrides(m.ID, wx, wy, m.WalkableTileOverrides, m.Flying) {
			return
		}
		if !g.combat.attackLineClear(wx, wy, playerX, playerY) {
			return
		}
		goals = append(goals, monster.TileCoord{X: tx, Y: ty})
	}

	for d := 1; d <= rangeTiles; d++ {
		addGoal(ptx+d, pty)
		addGoal(ptx-d, pty)
		addGoal(ptx, pty+d)
		addGoal(ptx, pty-d)
	}
	return goals
}

// endMonsterTurn ends the monster turn and starts a fresh party turn. The
// slot refill + selectedChar reset live inside startPartyTurn.
func (gl *GameLoop) endMonsterTurn() {
	gl.game.currentTurn = 0 // Party turn
	gl.game.partyActionsUsed = 0
	gl.game.partyPenaltyFreeActionsUsed = 0
	gl.game.monsterTurnState.resetPasses()
	gl.game.startPartyTurn()
	gl.game.monsterTurnResolved = true
	// Don't spam combat log with turn messages
}

// attackTargetTile uses the same resolved target as movement and combat.
func (g *MMGame) attackTargetTile(m *monster.Monster3D) *monster.TileCoord {
	_, x, y, ok := g.monsterAttackTarget(m)
	if !ok {
		return nil
	}
	ts := g.config.GetTileSize()
	return &monster.TileCoord{X: TileIndex(x, ts), Y: TileIndex(y, ts)}
}

// tickMonsterTurnStatuses owns the status clock for visible and remote turns.
// It returns whether stun or the slow cadence consumed this actor's turn.
func (g *MMGame) tickMonsterTurnStatuses(m *monster.Monster3D, tickTurnStatuses bool) bool {
	if tickTurnStatuses {
		m.TickAbilityClocks(true)
		m.TickPoisonTurn(turnBasedPeriodicEffectFrames(g.config.GetTPS())) // Venom-proc cards; ticks regardless of stun
		m.TickBurnTurn(turnBasedPeriodicEffectFrames(g.config.GetTPS()))   // Drakefang ignite; stacks with poison
		m.TickDebuffs(true)
		if !m.IsAlive() {
			// Matches RT: HandleMonsterInteractions skips a monster the parallel
			// Update's TickPoison just killed. finalizeIndirectKills (end of
			// frame) does the actual XP/loot/collision cleanup for both modes.
			return false
		}
	}
	if tickTurnStatuses && !m.ControlDRActive() {
		// Recovery starts only while free of every control in the shared chain.
		m.StunDR().Forget(true)
	}
	if tickTurnStatuses && m.Stun.Turns > 0 {
		// Expiry clears the RT clock too, or the stun-star overlay and
		// bossDisabled keep reading the monster as stunned.
		m.Stun.TickTurn()
		return true
	}
	// Root (bear trap) burns one turn per monster TURN - whether it moves
	// or stands adjacent and attacks (root pins movement, not actions).
	// MUST tick before the Pacified/Bound branches: a bound undead still
	// moves through monsterMoveTurnBased and its root must hold and decay.
	if tickTurnStatuses {
		m.TickRootTurn()
	}
	// Slow costs one whole turn (movement and attacks) in every configured
	// cadence; the root above still burned its turn.
	if tickTurnStatuses && m.ConsumeSlowedTurn(config.SlowSkipEveryTurns()) {
		return true
	}

	return false
}
