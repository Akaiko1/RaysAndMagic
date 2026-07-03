package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"ugataima/internal/character"
	"ugataima/internal/mathutil"
	"ugataima/internal/monster"
)

// separateStackedMonstersTB pulls in-play monsters off shared tiles onto distinct
// neighbouring tile centres. Turn-based combat fires down rows/columns, so two
// mobs the real-time pixel push left stacked or half-a-tile offset straddle the
// aim line and a shot threads the gap between them. Runs once per turn boundary
// (via startPartyTurn — which fires on TB entry and at every party turn); being
// turn-discrete it can't oscillate the way a per-frame RT snap would (that
// jittered because pursuit re-converged the pair every frame). Real time keeps
// its smooth pixel push (separateOverlappingMonsters); this is TB-only. Calm
// band stacks are skipped — stacking is the banding feature, and stackMonsterBand
// would snap them back the same tick anyway; it only reuses the read-only
// bandScatterRing order.
func (g *MMGame) separateStackedMonstersTB() {
	if g.world == nil || g.collisionSystem == nil {
		return
	}
	tile := float64(g.config.GetTileSize())
	vision := tile * TurnBasedVisionRangeTiles
	px, py := g.camera.X, g.camera.Y
	playerTile := [2]int{int(px / tile), int(py / tile)}
	byTile := map[[2]int][]*monster.Monster3D{}
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		if m.Banding && !m.IsEngagingPlayer {
			continue // calm band stack: intentional, and re-stacked same tick anyway
		}
		if Distance(px, py, m.X, m.Y) > vision && !m.IsEngagingPlayer {
			continue // out of this fight — leave it be
		}
		key := [2]int{int(m.X / tile), int(m.Y / tile)}
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
	// (calm-calm pass-through wouldn't stop them) or onto a lone calm mob.
	used := map[[2]int]bool{playerTile: true}
	for k := range byTile {
		used[k] = true
	}
	for _, k := range keys {
		cluster := byTile[k]
		if len(cluster) < 2 {
			continue
		}
		// Lowest-ID mob keeps the tile (a stable owner → no role ping-pong); the
		// rest snap onto distinct free neighbours. Set-piece monsters (sealed or
		// warded bosses, warlord idols) must never leave their scripted tile — one
		// of them owns the tile and none of them ever scatters.
		sortMonstersByID(cluster)
		owner := 0
		for i, m := range cluster {
			if m.BossDormant || m.BossWarded || m.WarlordIdol {
				owner = i
				break
			}
		}
		for i, m := range cluster {
			if i == owner || m.BossDormant || m.BossWarded || m.WarlordIdol {
				continue
			}
			g.scatterMonsterToFreeTile(m, k[0], k[1], tile, used)
		}
	}
}

// scatterMonsterToFreeTile snaps m onto the first walkable tile CENTRE from the
// bandScatterRing search order around (ctx,cty) not already in used, marks it
// used, and replans the mob's path. Returns false if every ring tile is
// blocked/taken (the mob stays put). It reuses the ring ORDER constant only —
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
			gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
			continue
		}
		if tickTurnStatuses && m.StunTurnsRemaining <= 0 && m.StunDRMemoryTurns > 0 {
			// Stun-free this turn: count toward clearing the diminishing-returns chain.
			m.StunDRMemoryTurns--
			if m.StunDRMemoryTurns == 0 {
				m.StunDRStacks, m.StunDRMemoryFrames = 0, 0
			}
		}
		if tickTurnStatuses && m.StunTurnsRemaining > 0 {
			m.StunTurnsRemaining--
			gl.game.turnBasedMonsterStunned[m] = true
			gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
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
			gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
			continue
		}

		// Pacified (Charm): holds position, never acts against the party.
		if m.Pacified {
			continue
		}
		// Bound (Bind Undead): strikes an enemy in reach or steps toward the
		// nearest one. Never acts against the party. Spends its whole turn here.
		if m.Bound {
			if !gl.game.combat.boundAttackNearest(m) {
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
		gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)

		// Skip monsters outside vision range unless engaged by a hit (they stay
		// fully idle — no centering, no move).
		if Distance(playerX, playerY, m.X, m.Y) > visionRange && !m.IsEngagingPlayer {
			continue
		}

		// Acting against the party IS engagement. RT sets this on sight in
		// updatePlayerEngagementWithVision, which the TB scheduler never runs;
		// without it a banded flock stays "calm" by flags, keeps re-stacking
		// every frame and chases the party as one pile — sight aggro must
		// scatter a band in any mode, damage must not be required.
		m.IsEngagingPlayer = true

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
					gl.game.combat.spawnMonsterRangedAttackAtMonster(m, foe, ProjectileOwnerMonsterAtBound)
				} else {
					gl.monsterMoveTurnBased(m)
				}
			} else if chebyshev == 1 {
				// Monster-vs-monster melee uses the same adjacent-tile contact as
				// party melee so crowded mobs can still connect.
				m.AttackAnimFrames = MonsterAttackAnimFrames
				gl.game.combat.monsterStrikeMonster(m, foe)
			} else {
				gl.monsterMoveTurnBased(m)
			}
			gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
			continue
		}

		// Boss specials (blink / Inferno); each TB turn is one action tick.
		// Runs AFTER the bound-undead check (matching RT order): a boss lured
		// at a bound foe spends its turn on that fight, not on party novas.
		// DESIGN: specials are rolled BEFORE range/movement checks, so in TB an
		// aggressive boss may cast Inferno or its low-HP blink from across the
		// room instead of closing in. RT gates these to the attack moment; the
		// asymmetry is intentional TB flavor — do not "fix" toward RT.
		if gl.game.combat.isBoss(m) {
			if gl.game.combat.updateBoss(m, m.BossCD == 0, true) {
				gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
				continue
			}
		}

		// Work in tile space: monsters never enter the player's tile. Melee can
		// attack from any adjacent tile (including diagonals); ranged attackers
		// still need a row/column firing lane.
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
		chebyshev := adX
		if adY > chebyshev {
			chebyshev = adY
		}

		// Pounce: from 2+ tiles away (within pounce range) leap onto an adjacent
		// tile and strike. Brief turn cooldown.
		if m.CanPounce() {
			if m.PounceCDTurns > 0 {
				m.PounceCDTurns--
			}
			pounceTiles := int(m.PounceRangePixels / tileSize)
			if m.PounceCDTurns == 0 && manhattan >= 2 && manhattan <= pounceTiles {
				if _, landed := gl.game.combat.executePounce(m, playerX, playerY); landed {
					gl.game.AddCombatMessage(fmt.Sprintf("%s pounces at the party!", m.Name))
					gl.monsterAttackTurnBased(m)
					m.PounceCDTurns = 2
					gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
					continue
				}
				// Couldn't land adjacent — fall through to a normal step this turn.
			}
		}

		if m.HasRangedAttack() {
			// Ranged: only fire when on the player's row or column (never
			// diagonal), within range, AND with a clear line of sight; otherwise
			// step toward the player. The LOS check stops a wasted shot into a wall
			// — without it a ranged mob holds at range and plinks the wall forever
			// while the party hides round a corner regening mana. No LOS → it A*-s
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
				gl.monsterAttackTurnBased(m)
			} else {
				gl.monsterMoveRangedTurnBased(m, rangeTiles)
			}
		} else {
			// Melee: attack from any adjacent tile (including diagonals);
			// otherwise step one tile toward the player (never onto their tile).
			if chebyshev == 1 && manhattan > 0 &&
				(gl.game.collisionSystem == nil || gl.game.collisionSystem.CheckLineOfSight(m.X, m.Y, playerX, playerY)) {
				gl.monsterAttackTurnBased(m)
			} else {
				gl.monsterMoveTurnBased(m)
			}
		}

		gl.game.refreshMonsterCollisionSolidity(m, playerX, playerY)
	}

	// Monsters finished moving: spring any traps they stepped onto.
	gl.game.combat.sweepTrapTriggers()

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

		// Route through the shared hit hub (same as RT melee/ranged) so TB melee
		// gets the FULL treatment: armor/resist mitigation, true damage, true-through-
		// dodge, knockOut (Lich Card cheat-death), damage blink, and the on-hit riders
		// (poison/ignite/char-stun/dispel). Disintegrate is a separate special ability
		// (tryMonsterSpecialAbility above), so 0 here.
		gl.game.combat.monsterHitCharacter(monster, target, monster.Name, monster.GetAttackDamage(), "physical", monster.IgnoresArmor, 0)
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
	m.X = wx
	m.Y = wy
	gl.game.collisionSystem.UpdateEntity(m.ID, wx, wy)
	m.LastMoveTick = gl.game.frameCount
	return true
}

// centerMonsterOnTile snaps the monster to the center of the tile it currently
// occupies, so turn-based movement stays strictly tile-to-tile. No-op if the
// tile center isn't reachable for this monster (wall/occupied).
func (gl *GameLoop) centerMonsterOnTile(m *monster.Monster3D, tileSize float64) {
	cx, cy := TileCenterFromTile(int(m.X/tileSize), int(m.Y/tileSize), tileSize)
	if cx == m.X && cy == m.Y {
		return
	}
	gl.commitMonsterMoveTB(m, cx, cy)
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
	targetX, targetY := gl.game.combat.monsterAITargetPoint(monster)
	playerTileX, playerTileY := int(targetX/tileSize), int(targetY/tileSize)

	dxTiles := playerTileX - monsterTileX
	dyTiles := playerTileY - monsterTileY

	if dxTiles == 0 && dyTiles == 0 {
		return // Already at player position
	}

	// A* FIRST — match the real-time AI, which is pure A*. Trying the naive cardinal
	// step first (below) looks like progress but oscillates when the route runs
	// PERPENDICULAR to it: a mob across a river from an offset party shuffles along
	// its own bank (each bank step counts as "toward the party" on one axis) and
	// never commits to the path down to the ford — so it could be ranged down for
	// free. A* always finds the crossing here, so follow it; the greedy/perpendicular/
	// teleport steps below are only a fallback for when A* finds no path at all.
	if nx, ny, ok := monster.NextPathStepTile(gl.game.collisionSystem, targetX, targetY); ok {
		wx, wy := TileCenterFromTile(nx, ny, tileSize)
		if gl.commitMonsterMoveTB(monster, wx, wy) {
			return
		}
	}

	// Fallback (A* found no path / its next tile is transiently occupied): step one
	// tile in the dominant cardinal direction towards the player.
	stepX, stepY := 0, 0
	if math.Abs(float64(dxTiles)) >= math.Abs(float64(dyTiles)) {
		stepX = mathutil.IntSign(dxTiles)
	} else {
		stepY = mathutil.IntSign(dyTiles)
	}

	newX := monster.X + float64(stepX)*tileSize
	newY := monster.Y + float64(stepY)*tileSize

	if gl.commitMonsterMoveTB(monster, newX, newY) {
		return
	}

	// Try the other perpendicular direction if the preferred one is blocked
	if stepX != 0 && dyTiles != 0 {
		altY := monster.Y + float64(mathutil.IntSign(dyTiles))*tileSize
		if gl.commitMonsterMoveTB(monster, monster.X, altY) {
			return
		}
	} else if stepY != 0 && dxTiles != 0 {
		altX := monster.X + float64(mathutil.IntSign(dxTiles))*tileSize
		if gl.commitMonsterMoveTB(monster, altX, monster.Y) {
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
	playerX, playerY := gl.game.combat.monsterAITargetPoint(m)

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

// monsterMoveRangedTurnBased moves a ranged monster toward an actual turn-based
// firing lane: a free tile on the party's row or column, within range and with
// line of sight. The generic monster A* uses circular attack reach, which is
// correct for RT ranged mobs but bad for TB: it can pick a diagonal in-range
// tile where the monster still cannot shoot, causing archers to shuffle around
// other archers instead of taking a second firing position.
func (gl *GameLoop) monsterMoveRangedTurnBased(m *monster.Monster3D, rangeTiles int) {
	if m == nil {
		return
	}
	if gl.game == nil || gl.game.collisionSystem == nil {
		gl.monsterMoveTurnBased(m)
		return
	}
	goals := gl.turnBasedRangedGoalTiles(m, rangeTiles)
	if len(goals) > 0 {
		tileSize := float64(gl.game.config.GetTileSize())
		if nx, ny, ok := m.NextPathStepTileToAny(gl.game.collisionSystem, goals); ok {
			wx, wy := TileCenterFromTile(nx, ny, tileSize)
			if gl.commitMonsterMoveTB(m, wx, wy) {
				return
			}
		}
	}
	gl.monsterMoveTurnBased(m)
}

func (gl *GameLoop) turnBasedRangedGoalTiles(m *monster.Monster3D, rangeTiles int) []monster.TileCoord {
	if m == nil || gl.game == nil || gl.game.collisionSystem == nil {
		return nil
	}
	if rangeTiles < 1 {
		rangeTiles = 1
	}
	tileSize := float64(gl.game.config.GetTileSize())
	ptx, pty := gl.game.GetPlayerTilePosition()
	playerX, playerY := gl.game.camera.X, gl.game.camera.Y

	goals := make([]monster.TileCoord, 0, rangeTiles*4)
	addGoal := func(tx, ty int) {
		wx, wy := TileCenterFromTile(tx, ty, tileSize)
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
