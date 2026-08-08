package game

import (
	"fmt"
	"math/rand"
	"sort"
	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/status"
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
			if m.IsInertSetPiece() {
				owner = i
				break
			}
		}
		for i, m := range cluster {
			if i == owner || m.IsInertSetPiece() {
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
		if g.collisionSystem.CanMoveToWithHabitat(m.ID, nx, ny, m.HabitatPrefs, m.Flying) {
			used[key] = true
			m.X, m.Y = nx, ny
			g.collisionSystem.UpdateEntity(m.ID, nx, ny)
			m.ResetPathfinding()
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

	// Cache player position for the loop
	playerX, playerY := gl.game.camera.X, gl.game.camera.Y

	if gl.game.turnBasedMonsterPassesLeft <= 0 {
		// Persistent damage zones (Hot Steam) sear once per monster turn in TB.
		gl.tickPersistentDamageZonesTB()

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
			gl.game.refreshMonsterCollisionState(m)
			continue
		}
		if tickTurnStatuses {
			m.TickPoisonTurn(turnBasedPeriodicEffectFrames(gl.game.config.GetTPS())) // Venom-proc cards; ticks regardless of stun
			m.TickBurnTurn(turnBasedPeriodicEffectFrames(gl.game.config.GetTPS()))   // Drakefang ignite; stacks with poison
			m.TickArmorShredTurn()                                                   // Pit Labrys shred decays regardless of stun
			m.TickSlowTurn()                                                         // Tarn Trident silt decays regardless of stun
			m.TickWeakenTurn()                                                       // Scalebreaker roar decays regardless of stun
			m.TickSoakTurn()                                                         // Champion Stone Skin rated dual clock
			if !m.IsAlive() {
				// Matches RT: HandleMonsterInteractions skips a monster the parallel
				// Update's TickPoison just killed. finalizeIndirectKills (end of
				// frame) does the actual XP/loot/collision cleanup for both modes.
				continue
			}
		}
		if tickTurnStatuses && m.StunTurnsRemaining <= 0 && m.StunDRMemoryTurns > 0 {
			// Stun-free this turn: count toward clearing the diminishing-returns chain.
			m.StunDRMemoryTurns--
			if m.StunDRMemoryTurns == 0 {
				m.StunDRStacks, m.StunDRMemoryFrames = 0, 0
			}
		}
		if tickTurnStatuses && m.StunTurnsRemaining > 0 {
			// Expiry clears the RT clock too, or the stun-star overlay and
			// bossDisabled keep reading the monster as stunned.
			status.TickTurnRated(&m.StunTurnsRemaining, &m.StunFramesRemaining, &m.StunRate)
			gl.game.turnBasedMonsterStunned[m] = true
			gl.game.refreshMonsterCollisionState(m)
			continue
		}
		// Root (bear trap) burns one turn per monster TURN - whether it moves
		// or stands adjacent and attacks (root pins movement, not actions).
		// MUST tick before the Pacified/Bound branches: a bound undead still
		// moves through monsterMoveTurnBased and its root must hold and decay.
		if tickTurnStatuses {
			m.TickRootTurn()
		}

		// CurrentAIBehavior is the mode-independent owner of high-level precedence.
		// Keep every mode explicit here: adding a behavior to the policy without a TB
		// branch must not silently fall through into ordinary party combat.
		behavior := m.CurrentAIBehavior()
		switch behavior {
		case monster.AIBehaviorInert:
			// Sealed bosses, warded warlords, and ward idols hold their placed tile.
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorPacified:
			// Charm must clear a pre-existing pursuit/attack state in TB just as it
			// does in RT; otherwise the actor remains visually combat-active.
			m.StandDownFromCombat()
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorEvasive:
			// Evasive quest bosses still react through tickEvasiveBossesTB above,
			// but never take a normal combat turn.
			m.StandDownFromCombat()
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorBoundAlly:
			// Bound (Bind Undead): strike an enemy in reach or step toward it;
			// without an enemy, follow the party without ever attacking it.
			if m.AIFoe != nil && !m.AIFoe.IsAlive() {
				// The cached target died earlier in this serial monster pass.
				// Do not spend the action walking toward its corpse.
				gl.game.releaseMonsterAttackPost(m)
				gl.game.refreshMonsterCollisionState(m)
				continue
			}
			if foe := m.AIFoe; foe != nil && foe.IsAlive() && gl.tryMonsterAttackFoeTurnBased(m, foe) {
				// Claimed a distinct logical attack post and spent the turn striking.
			} else {
				gl.monsterMoveTurnBased(m) // no enemy in reach - close the distance
			}
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorFleeing:
			elapsedFrames := 0
			if tickTurnStatuses {
				elapsedFrames = gl.game.config.GetTPS()
			}
			if nx, ny, move := m.NextFleeTurnStep(gl.game.collisionSystem, playerX, playerY, elapsedFrames); move && !m.RootHeld() {
				wx, wy := TileCenterFromTile(nx, ny, tileSize)
				gl.commitMonsterMoveTB(m, wx, wy)
			}
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorPassive:
			// Passive monsters mirror RT behavior: no move or attack until hit.
			m.StandDownFromCombat()
			gl.game.refreshMonsterCollisionState(m)
			continue
		case monster.AIBehaviorFightFoe:
			// A previous actor can kill this frame's cached foe. Wait for the next
			// shared retarget instead of falling through to a party action.
			if m.AIFoe == nil || !m.AIFoe.IsAlive() {
				gl.game.releaseMonsterAttackPost(m)
				gl.game.refreshMonsterCollisionState(m)
				continue
			}
		case monster.AIBehaviorRelentlessParty, monster.AIBehaviorSeekParty:
			// These modes continue through the shared combat scheduler below.
		}
		// A sight-only loot guard has one exact seven-tile combat radius in both
		// modes. Ordinary fights intentionally remain sticky in TB, but this
		// objective-specific encounter returns to its prop when the party leaves.
		if m.LootGuardAlerted && m.IsEngagingPlayer && !m.WasAttacked {
			if m.ShouldDisengageFromPlayer(gl.game.collisionSystem, playerX, playerY) {
				m.EndPlayerEngagement()
				gl.game.refreshMonsterCollisionState(m)
				continue
			}
		}
		gl.game.refreshMonsterCollisionState(m)

		// TB does not run Monster3D.Update, so it invokes the exact same normal
		// sight gate as RT. Loot guards receive their exact seven-tile objective
		// range inside that shared rule, but never patrol during TB. Sticky hostility and
		// a bound-ally foe deliberately bypass first sight: a monster already
		// committed to a fight must keep taking turns after cover or a retreat.
		if m.CanStartPlayerEngagement(gl.game.collisionSystem, playerX, playerY) {
			m.BeginPlayerEngagement()
		}
		if m.AIFoe == nil && !m.IsEngagingPlayer && !m.WasAttacked && !m.BossAggro && !m.Relentless {
			continue
		}

		// A bound-ally foe is combat too, even though its target is not the party.
		// Preserve the existing combat marker so bands scatter rather than remain a
		// calm stack while fighting summons. Normal party entries already used the
		// shared BeginPlayerEngagement transition above.
		if !m.IsEngagingPlayer {
			m.BeginCombatEngagement()
		}

		// Each participating monster snaps to the center of its current tile at
		// the start of its turn. Keeps TB strictly tile-to-tile and fixes
		// off-center spawns (e.g. encounter pirates) that would otherwise
		// stand/attack between tiles.
		gl.centerMonsterOnTile(m, tileSize)

		// Boss specials; each TB turn is one action tick. BEFORE the bound-undead
		// check (matching RT order): a boss lured at a summon still sows traps,
		// rallies adds, enrages and blinks.
		// DESIGN: specials roll BEFORE range/movement checks, so an aggressive TB
		// boss may spend its turn on a special instead of closing in. The Inferno
		// nova is the exception with a real gate - bound to its authored
		// inferno_range_tiles in BOTH modes (it used to be map-wide here, which with
		// aggro_whole_map let the Golden Thief Bug burn the party from anywhere).
		if m.IsBoss() {
			if gl.game.combat.runBossSpecials(m, true, true) {
				if !gl.game.combat.bossEvasive(m) {
					gl.game.combat.armMonsterRTAttackCooldowns(m)
				}
				gl.game.refreshMonsterCollisionState(m)
				continue
			}
		}

		// Lured at a bound undead instead of the party: attack it (ranged mobs loose
		// a bolt from within range, melee strike from an adjacent tile), else step
		// toward it; never touch the party.
		if foe := m.AIFoe; foe != nil && foe.IsAlive() {
			if gl.tryMonsterAttackFoeTurnBased(m, foe) {
				// Claimed a distinct logical attack post and spent the turn striking.
			} else {
				gl.monsterMoveTurnBased(m)
			}
			gl.game.refreshMonsterCollisionState(m)
			continue
		}

		// Work in tile space: monsters never enter the player's tile. Any attacker
		// uses melee from a clear adjacent tile; a projectile-capable attacker uses
		// its ranged profile everywhere else and still needs a row/column lane.
		mtx, mty := TileIndex(m.X, tileSize), TileIndex(m.Y, tileSize)
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

		// Pounce: from 2+ tiles away (within pounce range) leap onto an adjacent
		// tile and strike. Brief turn cooldown.
		if m.CanPounce() {
			if tickTurnStatuses {
				m.TickPounceCooldownTurn()
			}
			pounceTiles := int(m.PounceRangePixels / tileSize)
			if m.PounceCDTurns == 0 && manhattan >= 2 && manhattan <= pounceTiles &&
				gl.game.combat.monsterCanPounceParty(m) {
				if gl.game.combat.executePounce(m, playerX, playerY) {
					gl.game.AddCombatMessage(fmt.Sprintf("%s pounces at the party!", m.Name))
					gl.monsterAttackTurnBased(m)
					m.ArmPounceCooldown(gl.game.config.GetTPS(), TurnBasedPounceCooldownTurns)
					gl.game.refreshMonsterCollisionState(m)
					continue
				}
				// Couldn't land adjacent - fall through to a normal step this turn.
			}
		}

		// Both gates: the spatial one (adjacency+LOS) and the delivery selector.
		// A ranged CHAMPION fails the selector and falls through to the lane
		// rule below - otherwise an adjacent diagonal would let it fire where
		// monsterAttackTurnBased resolves the attack as ranged.
		if gl.game.combat.monsterMeleeAdjacentToPoint(m, playerX, playerY) &&
			gl.game.combat.monsterUsesMeleeAgainstPoint(m, playerX, playerY) {
			if gl.game.tryClaimMonsterAttackPost(m) {
				m.State = monster.StateAttacking
				gl.monsterAttackTurnBased(m)
			} else {
				gl.game.releaseMonsterAttackPost(m)
				gl.monsterMoveTurnBased(m)
			}
		} else if m.HasRangedAttack() {
			// Ranged: only fire when on the player's row or column (never
			// diagonal), within range, AND with a clear line of sight; otherwise
			// step toward the player. The LOS check stops a wasted shot into a wall
			// - without it a ranged mob holds at range and plinks the wall forever
			// while the party hides round a corner regening mana. No LOS -> it A*-s
			// toward the party (monsterMoveTurnBased) to round the corner instead.
			rangeTiles := int(m.GetAttackRangePixels() / tileSize)
			if rangeTiles < 1 {
				rangeTiles = 1
			}
			aligned := dxT == 0 || dyT == 0
			axisDist := adX
			if dxT == 0 {
				axisDist = adY
			}
			hasLOS := gl.game.collisionSystem == nil ||
				gl.game.collisionSystem.CheckLineOfSight(m.X, m.Y, playerX, playerY)
			if aligned && axisDist >= 1 && axisDist <= rangeTiles && hasLOS {
				if gl.game.tryClaimMonsterAttackPost(m) {
					m.State = monster.StateAttacking
					gl.monsterAttackTurnBased(m)
				} else {
					gl.game.releaseMonsterAttackPost(m)
					gl.monsterMoveTurnBased(m)
				}
			} else {
				gl.monsterMoveTurnBased(m)
			}
		} else {
			gl.monsterMoveTurnBased(m)
		}

		gl.game.refreshMonsterCollisionState(m)
	}

	// Monsters finished moving: spring any traps they stepped onto, and burn
	// whatever walked into a damage zone. The entry pass must run AFTER the moves
	// and AFTER this round's periodic ticks stamped everyone already standing
	// inside, or a mob in a Firewall takes one hit per round too many.
	gl.game.combat.sweepTrapTriggers()
	gl.applyZoneEntryDamageAll()

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
	gl.forEachMonsterAttackTurnBased(monster, func() bool {
		return len(alivePartyIndices(gl.game.party.Members)) > 0
	}, func() {
		// Same attack wrappers as RT so TB gets the identical roll chain:
		// special ability -> Fireburst -> the shared monster->character hit hub.
		gl.game.combat.performMonsterAttackAgainstParty(monster)
	})
}

// monsterAttackFoeTurnBased resolves a full monster turn against a controlled
// monster. Crossfire must use the same authored attacks-per-round/cooldown
// parity as attacks against the party; otherwise fast monsters silently lose
// swings whenever their target is a bound ally or card summon.
func (gl *GameLoop) monsterAttackFoeTurnBased(attacker, foe *monster.Monster3D) {
	gl.forEachMonsterAttackTurnBased(attacker, func() bool {
		return foe != nil && foe.IsAlive()
	}, func() {
		owner := ProjectileOwnerMonsterAtBound
		if attacker.Bound {
			owner = ProjectileOwnerBoundUndead
		}
		gl.game.combat.performMonsterAttackAgainstMonster(attacker, foe, owner)
	})
}

// tryMonsterAttackFoeTurnBased applies the same logical-post gate used for
// party attacks before a monster attacks a summon, bound undead, or other foe.
// The entities stay physically pass-through; a rejected contender simply keeps
// pursuing another free tile around the target.
func (gl *GameLoop) tryMonsterAttackFoeTurnBased(attacker, foe *monster.Monster3D) bool {
	if gl == nil || gl.game == nil || gl.game.combat == nil ||
		!gl.game.combat.monsterCanAttackMonster(attacker, foe) ||
		!gl.game.tryClaimMonsterAttackPost(attacker) {
		return false
	}
	attacker.State = monster.StateAttacking
	attacker.StateTimer = 0
	gl.monsterAttackFoeTurnBased(attacker, foe)
	return true
}

// forEachMonsterAttackTurnBased is the sole action-count loop for attacks in a
// monster turn. The party and crossfire branches deliberately differ only in
// target selection and delivery; authored attacks-per-round must not drift.
func (gl *GameLoop) forEachMonsterAttackTurnBased(attacker *monster.Monster3D, targetAlive func() bool, attack func()) {
	if attacker == nil || targetAlive == nil || attack == nil {
		return
	}
	gl.game.armMonsterAttackAnimation(attacker)
	attacker.LastMoveTick = gl.game.frameCount
	attacked := false
	for hit := 0; hit < attacker.GetTurnBasedAttackCount() && targetAlive(); hit++ {
		attack()
		attacked = true
	}
	if attacked {
		gl.game.combat.armMonsterRTAttackCooldowns(attacker)
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

// commitMonsterMoveTB moves the monster to the tile-center (wx, wy) when the
// habitat-aware collision check passes, updating its collision entity and turn
// stamp. Returns whether the monster moved.
func (gl *GameLoop) commitMonsterMoveTB(m *monster.Monster3D, wx, wy float64) bool {
	if !gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, wx, wy, m.HabitatPrefs, m.Flying) {
		return false
	}
	tileSize := float64(gl.game.config.GetTileSize())
	movedTile := tileSize > 0 && (TileIndex(m.X, tileSize) != TileIndex(wx, tileSize) || TileIndex(m.Y, tileSize) != TileIndex(wy, tileSize))
	if movedTile {
		gl.game.releaseMonsterAttackPost(m)
	}
	m.X = wx
	m.Y = wy
	if movedTile {
		// TB computes a fresh discrete A* step. Its new position invalidates the
		// continuous RT route, but not a flee/guard objective shared across turns.
		m.ResetPathCache()
	}
	gl.game.collisionSystem.UpdateEntity(m.ID, wx, wy)
	m.LastMoveTick = gl.game.frameCount
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

// monsterMoveTurnBased handles a monster move in turn-based mode
// monsterMoveTurnBased is the ONE turn-based movement entry point: gates and
// the shared step machinery live here; only the goal selection differs by
// attacker kind (melee: adjacent tile; ranged vs party: firing lane; ranged vs
// a monster foe: plain approach).
func (gl *GameLoop) monsterMoveTurnBased(monster *monster.Monster3D) {
	// A mob that is searching for a post is transit even if it reached this
	// method from an old held position. Physical overlap remains allowed; only
	// its attack claim is released.
	gl.game.releaseMonsterAttackPost(monster)
	// Rooted (bear trap): pinned for the whole turn; the per-turn countdown
	// lives in TickRootTurn (root != stun - attacks still happen).
	if monster.RootHeld() {
		return
	}
	// Slowed (Tarn Trident silt): TB movement is tile-stepped, so the RT speed
	// drag converts to skipping this turn's step SlowPct% of the time - the
	// same average ground lost per turn, attacks unaffected. ActiveSlowPct
	// keeps the latched value for the turn that consumed the final tick.
	if pct := monster.ActiveSlowPct(); pct > 0 && rand.Intn(100) < pct {
		return
	}
	tileSize := float64(gl.game.config.GetTileSize())

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
			(gl.game.collisionSystem == nil ||
				gl.game.collisionSystem.CheckLineOfSight(monster.X, monster.Y, targetX, targetY))
		if inFormation {
			return
		}
		if nx, ny, ok := monster.NextPathStepTile(gl.game.collisionSystem, targetX, targetY); ok {
			wx, wy := TileCenterFromTile(nx, ny, tileSize)
			gl.commitMonsterMoveTB(monster, wx, wy)
		}
		return
	}

	// A* FIRST. In TB, melee contact is tile-adjacent only, so do not reuse the
	// RT "within attack radius" goals: they can pick a dead-end bank tile across
	// water as "close enough", then the monster turns around next move.
	if !monster.HasRangedAttack() {
		goals := gl.turnBasedMeleeGoalTiles(monster, targetX, targetY)
		if len(goals) == 0 {
			// All adjacent attack posts are currently unavailable. Route to the
			// next free ring around the target instead of falling back to a greedy
			// cardinal move; this is the normal movement after a failed pounce when
			// allies temporarily surround its landing tiles.
			goals = gl.turnBasedBlockedMeleeApproachGoalTiles(monster, targetX, targetY)
		}
		if gl.moveMonsterAlongTBGoals(monster, goals, tileSize) {
			return
		}
	} else {
		// Ranged hunting the party repositions onto a row/column firing lane;
		// against a monster foe there is no alignment rule - plain approach.
		if monster.AIFoe == nil && !monster.Bound && gl.game.collisionSystem != nil {
			if goals := gl.turnBasedRangedGoalTiles(monster); len(goals) > 0 {
				if nx, ny, ok := monster.NextPathStepTileToAny(gl.game.collisionSystem, goals); ok {
					wx, wy := TileCenterFromTile(nx, ny, tileSize)
					if gl.commitMonsterMoveTB(monster, wx, wy) {
						return
					}
				}
			}
		}
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
	if nx, ny, ok := m.NextPathStepTileToAny(gl.game.collisionSystem, goals); ok {
		wx, wy := TileCenterFromTile(nx, ny, tileSize)
		if gl.commitMonsterMoveTB(m, wx, wy) {
			return true
		}
	}
	return false
}

func (gl *GameLoop) turnBasedMeleeGoalTiles(m *monster.Monster3D, targetX, targetY float64) []monster.TileCoord {
	if m == nil || gl.game == nil || gl.game.collisionSystem == nil {
		return nil
	}
	tileSize := float64(gl.game.config.GetTileSize())
	targetTileX, targetTileY := TileIndex(targetX, tileSize), TileIndex(targetY, tileSize)

	goals := make([]monster.TileCoord, 0, 24)
	addGoal := func(tx, ty int, requireLOS bool) {
		wx, wy := TileCenterFromTile(tx, ty, tileSize)
		if gl.game.monsterHasAttackTarget(m) && gl.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, wx, wy) {
			return
		}
		if !gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, wx, wy, m.HabitatPrefs, m.Flying) {
			return
		}
		if requireLOS && !gl.game.collisionSystem.CheckLineOfSight(wx, wy, targetX, targetY) {
			return
		}
		goals = append(goals, monster.TileCoord{X: tx, Y: ty})
	}

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			addGoal(targetTileX+dx, targetTileY+dy, true)
		}
	}

	return uniqueTileGoals(goals)
}

// turnBasedBlockedMeleeApproachGoalTiles supplies a second A* goal ring only
// when no adjacent attack post is currently free. These are never attack posts:
// they let a pouncer advance after its landing ring is occupied without using a
// separate greedy step that could disagree with terrain/habitat pathing.
// The ring itself is shared with RT pursuit (MeleeApproachRingGoals).
func (gl *GameLoop) turnBasedBlockedMeleeApproachGoalTiles(m *monster.Monster3D, targetX, targetY float64) []monster.TileCoord {
	if m == nil || gl == nil || gl.game == nil || gl.game.collisionSystem == nil {
		return nil
	}
	return m.MeleeApproachRingGoals(gl.game.collisionSystem, targetX, targetY)
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
func (gl *GameLoop) turnBasedRangedGoalTiles(m *monster.Monster3D) []monster.TileCoord {
	if m == nil || gl.game == nil || gl.game.collisionSystem == nil {
		return nil
	}
	tileSize := float64(gl.game.config.GetTileSize())
	rangeTiles := int(m.GetAttackRangePixels() / tileSize)
	if rangeTiles < 1 {
		rangeTiles = 1
	}
	ptx, pty := gl.game.GetPlayerTilePosition()
	playerX, playerY := gl.game.camera.X, gl.game.camera.Y

	goals := make([]monster.TileCoord, 0, rangeTiles*4)
	addGoal := func(tx, ty int) {
		wx, wy := TileCenterFromTile(tx, ty, tileSize)
		if gl.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, wx, wy) {
			return
		}
		if !gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, wx, wy, m.HabitatPrefs, m.Flying) {
			return
		}
		if !gl.game.collisionSystem.CheckLineOfSight(wx, wy, playerX, playerY) {
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
	gl.game.turnBasedMonsterPassesLeft = 0
	gl.game.turnBasedMonsterPassDelay = 0
	gl.game.turnBasedMonsterStatusTick = false
	gl.game.turnBasedMonsterStunned = nil
	gl.game.startPartyTurn()
	gl.game.monsterTurnResolved = true
	// Don't spam combat log with turn messages
}
