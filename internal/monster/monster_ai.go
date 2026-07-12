package monster

import (
	"math"
	"math/rand"
	"ugataima/internal/config"
	"ugataima/internal/mathutil"
	"ugataima/internal/status"
)

// CollisionChecker interface for checking movement validity
type CollisionChecker interface {
	CanMoveTo(entityID string, x, y float64) bool
	CanMoveToWithHabitat(entityID string, x, y float64, habitatPrefs []string, flying bool) bool
	// CanOccupyTilesWithHabitat is the tile-only variant: terrain rules apply,
	// entity bodies are ignored. Used where an entity veto would be wrong
	// (e.g. the A* start tile the monster is already standing in).
	CanOccupyTilesWithHabitat(entityID string, x, y float64, habitatPrefs []string, flying bool) bool
	CheckLineOfSight(x1, y1, x2, y2 float64) bool
}

// TileCoord represents a tile coordinate on the grid.
type TileCoord struct {
	X int
	Y int
}

type gridNode struct {
	idx int
	f   float64
	g   float64
}

type nodeHeap struct {
	nodes []gridNode
}

func (h *nodeHeap) reset() {
	h.nodes = h.nodes[:0]
}

func (h *nodeHeap) push(n gridNode) {
	h.nodes = append(h.nodes, n)
	i := len(h.nodes) - 1
	for i > 0 {
		p := (i - 1) / 2
		if h.nodes[p].f <= n.f {
			break
		}
		h.nodes[i] = h.nodes[p]
		i = p
	}
	h.nodes[i] = n
}

func (h *nodeHeap) pop() (gridNode, bool) {
	if len(h.nodes) == 0 {
		return gridNode{}, false
	}
	min := h.nodes[0]
	last := h.nodes[len(h.nodes)-1]
	h.nodes = h.nodes[:len(h.nodes)-1]
	if len(h.nodes) == 0 {
		return min, true
	}
	i := 0
	for {
		left := 2*i + 1
		right := left + 1
		if left >= len(h.nodes) {
			break
		}
		smallest := left
		if right < len(h.nodes) && h.nodes[right].f < h.nodes[left].f {
			smallest = right
		}
		if h.nodes[smallest].f >= last.f {
			break
		}
		h.nodes[i] = h.nodes[smallest]
		i = smallest
	}
	h.nodes[i] = last
	return min, true
}

type pathScratch struct {
	gScore   []float64
	cameFrom []int
	closed   []bool
	goal     []bool
	width    int
	height   int
	minX     int
	minY     int
	heap     nodeHeap
}

func (ps *pathScratch) prepare(width, height int, minX, minY int) {
	size := width * height
	if cap(ps.gScore) < size {
		ps.gScore = make([]float64, size)
		ps.cameFrom = make([]int, size)
		ps.closed = make([]bool, size)
		ps.goal = make([]bool, size)
	} else {
		ps.gScore = ps.gScore[:size]
		ps.cameFrom = ps.cameFrom[:size]
		ps.closed = ps.closed[:size]
		ps.goal = ps.goal[:size]
	}
	for i := 0; i < size; i++ {
		ps.gScore[i] = math.Inf(1)
		ps.cameFrom[i] = -1
		ps.closed[i] = false
		ps.goal[i] = false
	}
	ps.width = width
	ps.height = height
	ps.minX = minX
	ps.minY = minY
	ps.heap.reset()
}

func (ps *pathScratch) index(tile TileCoord) int {
	x := tile.X - ps.minX
	y := tile.Y - ps.minY
	if x < 0 || y < 0 || x >= ps.width || y >= ps.height {
		return -1
	}
	return y*ps.width + x
}

func (ps *pathScratch) coord(idx int) TileCoord {
	x := idx%ps.width + ps.minX
	y := idx/ps.width + ps.minY
	return TileCoord{X: x, Y: y}
}

// Update runs the monster AI with collision checking and player position for
// engagement detection. AI-ONLY: it does not touch collision-entity metadata
// (e.g. the engaged/solid distinction), so a real-time gameplay step must call
// this through game.MonsterWrapper.Update, not on its own - tests that need RT
// fidelity (not just AI/position behavior) should do the same.
func (m *Monster3D) Update(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Sealed/dormant boss: completely inert (no detection, no patrol) so it holds
	// its throne until its quest unseals it. The RT attack loop already no-ops via
	// updateBoss; without this the patrol state would still drift the boss off its
	// spawn tile. Flag is set single-threaded in refreshBoundAllyCache.
	// A warlord idol is likewise immobile - it stands where placed and never moves.
	// A warded warlord HOLDS its plaza (rooted by its idols) until they're broken;
	// without this it would chase the party clear across the map at load.
	if m.BossDormant || m.WarlordIdol || m.BossWarded {
		return
	}
	m.TickPoison()          // Venom-proc cards; ticks regardless of stun/root state
	m.TickArmorShredFrame() // Pit Labrys shred decays regardless of stun/root state
	if m.SoakFrames > 0 {   // Stone Skin soak runs on the stun dual-clock convention
		status.TickFrame(&m.SoakFrames, &m.SoakTurns)
	}
	// RT roots run on frames; a TB-turn hold left over from a mode switch
	// must not keep gating pounce here.
	m.rootHeldThisTurn = false
	if m.StunFramesRemaining > 0 {
		// Expiry clears the TB clock too, or the stun-star overlay stays on.
		status.TickFrame(&m.StunFramesRemaining, &m.StunTurnsRemaining)
		return
	}
	// Stun-free this frame: count toward clearing the stun diminishing-returns chain.
	if m.StunDRMemoryFrames > 0 {
		m.StunDRMemoryFrames--
		if m.StunDRMemoryFrames == 0 {
			m.StunDRStacks, m.StunDRMemoryTurns = 0, 0
		}
	}
	// Rooted (bear trap): the FULL update runs - detection, state machine,
	// attack cadence - but any displacement it produced is undone, so the
	// monster fights from where it stands without being stunned.
	if m.RootFramesRemaining > 0 {
		m.RootFramesRemaining--
		px, py := m.X, m.Y
		defer func() { m.X, m.Y = px, py }()
	}

	m.StateTimer++

	// Safety: if the monster somehow ended up in a blocked position (e.g., spawn overlap or jitter),
	// attempt to gently nudge it to a nearby free spot to avoid getting stuck inside walls/trees.
	if collisionChecker != nil && m.StateTimer%15 == 0 { // throttle checks
		if !collisionChecker.CanMoveToWithHabitat(m.ID, m.X, m.Y, m.HabitatPrefs, m.Flying) {
			m.unstuckFromObstacles(collisionChecker)
		}
	}

	// Check for player detection and engagement with line-of-sight (trees reduce detection)
	m.updatePlayerEngagementWithVision(collisionChecker, playerX, playerY)

	switch m.State {
	case StateIdle:
		m.updateIdle(playerX, playerY)
	case StatePatrolling:
		m.updatePatrolling(collisionChecker, playerX, playerY)
	case StatePursuing:
		m.updatePursuing(collisionChecker, playerX, playerY)
	case StateAlert:
		m.updateAlert(collisionChecker, playerX, playerY)
	case StateAttacking:
		m.updateAttacking(collisionChecker, playerX, playerY)
	case StateFleeing:
		m.updateFleeing(collisionChecker, playerX, playerY)
	}
}

// meleeTileAdjacent reports whether a MELEE attacker stands on one of the 8 tiles
// around the target WITH a clear line of sight to it. A diagonal neighbour is
// ~1.41 tiles away - just past the 1-tile pixel attack range - so the pixel
// checks alone leave a melee monster pursuing forever at a diagonal (walk
// animation looping). The LoS requirement matters at a tree/wall CORNER: a
// diagonal contact whose line is blocked is NOT real reach, so the monster keeps
// pursuing (routes around) instead of freezing in an attack stance it can never
// land. Ranged attackers never count as melee-adjacent.
func (m *Monster3D) meleeTileAdjacent(targetX, targetY float64, checker CollisionChecker) bool {
	if m.HasRangedAttack() {
		return false
	}
	ts := m.tileSize()
	if ts <= 0 {
		return false
	}
	dx := mathutil.IntAbs(int(m.X/ts) - int(targetX/ts))
	dy := mathutil.IntAbs(int(m.Y/ts) - int(targetY/ts))
	if dx > 1 || dy > 1 || (dx == 0 && dy == 0) {
		return false
	}
	return checker == nil || checker.CheckLineOfSight(m.X, m.Y, targetX, targetY)
}

// pursueRelentlessly closes on (targetX, targetY), ignoring detection range, LoS
// and the flee cycle. Shared by bound undead and aggressive bosses.
func (m *Monster3D) pursueRelentlessly(checker CollisionChecker, targetX, targetY float64) {
	m.IsEngagingPlayer = true
	los := checker == nil || checker.CheckLineOfSight(m.X, m.Y, targetX, targetY)
	inReach := (distance(m.X, m.Y, targetX, targetY) <= m.GetAttackRangePixels() && los) ||
		m.meleeTileAdjacent(targetX, targetY, checker)
	if !inReach {
		if m.State != StatePursuing {
			m.State = StatePursuing
			m.StateTimer = 0
		}
	} else if m.State != StateAttacking {
		m.State = StateAttacking
		m.StateTimer = 0
	}
}

// updatePlayerEngagementWithVision handles player detection with line-of-sight checks
// Trees and other opaque obstacles reduce detection radius
func (m *Monster3D) updatePlayerEngagementWithVision(collisionChecker CollisionChecker, playerX, playerY float64) {
	// A pacified (Charm) monster stands down - it never aggros or pursues the
	// party, but it still idly wanders. Drop any aggressive state ONCE (so it
	// stops chasing), then skip detection so it can't re-engage; the idle/patrol
	// states below drive its wandering as normal. (Resetting the state every frame
	// would freeze it - the idle->patrol timer could never elapse.)
	if m.Pacified {
		m.IsEngagingPlayer = false
		switch m.State {
		case StateAlert, StatePursuing, StateAttacking, StateFleeing:
			m.State = StateIdle
			m.StateTimer = 0
		}
		return
	}

	// A bound undead (Bind Undead) always pursues the target it was handed (its
	// enemy, picked by the game's AI-target logic) regardless of normal detection
	// range - it actively hunts, and never flees. It only enters its attack stance
	// once within real attack range; beyond that it keeps closing. When it has no
	// enemy the game hands it the party position, so the ally follows the party.
	if m.Bound {
		m.pursueRelentlessly(collisionChecker, playerX, playerY)
		return
	}

	// A monster handed a bound-ally FOE (playerX/Y is that foe's position, set as
	// the AI target) hunts it relentlessly, exactly like a bound ally hunts its
	// own enemy - ignoring this mob's detection range. A small-sighted mob (a
	// goblin sees 3 tiles) would otherwise never engage a ranged summon peppering
	// it from beyond that, and would wander off instead of closing.
	if m.AIFoe != nil && m.AIFoe.IsAlive() {
		m.pursueRelentlessly(collisionChecker, playerX, playerY)
		return
	}

	// Aggressive boss OR revenge-rallied mob (Amazons after their Warlord dies):
	// pursue the party relentlessly, ignoring detection range / LoS / flee.
	if m.relentlessHunter() {
		m.pursueRelentlessly(collisionChecker, playerX, playerY)
		return
	}

	// Don't process engagement while fleeing - flee state takes priority
	if m.State == StateFleeing {
		return
	}

	// Calculate distance to player
	distanceToPlayer := distance(m.X, m.Y, playerX, playerY)

	if m.PassiveUntilAttacked && !m.WasAttacked && !m.HatesActiveTrait() {
		if m.IsEngagingPlayer {
			m.IsEngagingPlayer = false
			m.State = StateIdle
			m.StateTimer = 0
			m.AttackCount = 0
		}
		return
	}

	// A monster handed hostility directly (encounter spawn, save restore) never
	// saw the !IsEngagingPlayer edge below, so it would idle/patrol forever while
	// "engaged". Engagement is a level, not an edge: snap it into the combat loop.
	if m.IsEngagingPlayer && (m.State == StateIdle || m.State == StatePatrolling) {
		m.State = StateAlert
		m.StateTimer = 0
	}

	// Detection tuning from config (monster_ai section), with code fallbacks for
	// configless contexts (tests). Distances are in tiles.
	defaultRadiusTiles, outsideTetherMult, losBlockedMult, disengageMult := 4.0, 2.0, 0.5, 2.0
	if m.config != nil {
		ai := &m.config.MonsterAI
		if ai.DefaultAlertRadiusTiles > 0 {
			defaultRadiusTiles = ai.DefaultAlertRadiusTiles
		}
		if ai.AlertOutsideTetherMultiplier > 0 {
			outsideTetherMult = ai.AlertOutsideTetherMultiplier
		}
		if ai.AlertLosBlockedMultiplier > 0 {
			losBlockedMult = ai.AlertLosBlockedMultiplier
		}
		if ai.DisengageDistanceMultiplier > 0 {
			disengageMult = ai.DisengageDistanceMultiplier
		}
	}

	// Get detection radius (use AlertRadius or the configured default)
	detectionRadius := m.AlertRadius
	if detectionRadius <= 0 {
		detectionRadius = defaultRadiusTiles * m.tileSize()
	}

	// If outside tether, monster is more alert (was lured or is lost)
	// This prevents them from immediately returning to spawn when switching from TB to RT mode
	if !m.IsWithinTetherRadius() {
		detectionRadius *= outsideTetherMult
	}

	// Check line of sight - if obstructed (trees, walls), reduce detection radius
	// Only apply penalty if we are inside our territory. If outside, we stay alert.
	// The LOS trace (a per-monster DDA walk) is skipped when the player is beyond
	// the largest radius either branch below could use: past that distance the
	// engage/disengage outcome is identical with or without the LOS multiplier.
	losRelevant := distanceToPlayer <= detectionRadius*math.Max(1, losBlockedMult)*math.Max(1, disengageMult)
	if losRelevant && m.IsWithinTetherRadius() && collisionChecker != nil && !collisionChecker.CheckLineOfSight(m.X, m.Y, playerX, playerY) {
		detectionRadius *= losBlockedMult
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
	} else if distanceToPlayer > detectionRadius*disengageMult { // Hysteresis - lose engagement further out
		if m.IsEngagingPlayer && !m.WasAttacked {
			// Stop engaging player - return to idle (only if not recently attacked)
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

// updatePatrolling moves monster randomly for normal wandering behavior using pathfinding
func (m *Monster3D) updatePatrolling(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Get AI config values
	var patrolIdleTimer int = 600 // Default value

	if m.config != nil {
		patrolIdleTimer = m.config.MonsterAI.PatrolIdleTimer
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	// Reset RT target when state changes
	if m.MoveTargetState != StatePatrolling {
		m.clearMoveTarget()
	}

	// Check if outside tether - return to spawn using pathfinding
	if !m.IsWithinTetherRadius() {
		spawnTileX := m.worldToTile(m.SpawnX)
		spawnTileY := m.worldToTile(m.SpawnY)
		m.setMoveTarget(StatePatrolling, spawnTileX, spawnTileY)
		if !m.followPathToTile(collisionChecker, spawnTileX, spawnTileY) {
			m.clearMoveTarget()
		}
		return
	}

	shouldPickNewTarget := !m.hasMoveTarget(StatePatrolling)

	if shouldPickNewTarget {
		if target, ok := m.pickPatrolTarget(collisionChecker); ok {
			m.setMoveTarget(StatePatrolling, target.X, target.Y)
		}
	}

	if m.hasMoveTarget(StatePatrolling) {
		target := m.currentMoveTarget()
		if m.isAtTile(target.X, target.Y) {
			m.clearMoveTarget()
			return
		}
		if !m.followPathToTile(collisionChecker, target.X, target.Y) {
			m.clearMoveTarget()
		}
	}

	// Return to idle after a while
	if m.StateTimer > patrolIdleTimer {
		m.State = StateIdle
		m.StateTimer = 0
	}
}

// tryMoveCardinal attempts to move in a cardinal direction using the current state's speed.
func (m *Monster3D) tryMoveCardinal(collisionChecker CollisionChecker, dirX, dirY int) bool {
	if dirX == 0 && dirY == 0 {
		return false
	}

	speed := m.movementSpeed(m.State)
	if speed <= 0 {
		return false
	}

	currentCenterX, currentCenterY := m.worldToTileCenter(m.X, m.Y)
	targetX := currentCenterX + float64(dirX)*m.tileSize()
	targetY := currentCenterY + float64(dirY)*m.tileSize()

	if !collisionChecker.CanMoveToWithHabitat(m.ID, targetX, targetY, m.HabitatPrefs, m.Flying) {
		return false
	}

	dirAngle := math.Atan2(float64(dirY), float64(dirX))
	newX := m.X + math.Cos(dirAngle)*speed
	newY := m.Y + math.Sin(dirAngle)*speed

	if collisionChecker.CanMoveToWithHabitat(m.ID, newX, newY, m.HabitatPrefs, m.Flying) {
		m.X = newX
		m.Y = newY
		return true
	}

	return false
}

// updatePursuing moves monster towards player using grid-based cardinal movement
func (m *Monster3D) updatePursuing(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Calculate distance to player
	distanceToPlayer := distance(m.X, m.Y, playerX, playerY)
	attackRange := m.GetAttackRangePixels()
	hasLOS := collisionChecker == nil || collisionChecker.CheckLineOfSight(m.X, m.Y, playerX, playerY)

	// Check if close enough to attack (pixel range, or melee tile-adjacency so a
	// diagonal neighbour commits instead of pursuing in place).
	if (distanceToPlayer <= attackRange && hasLOS) || m.meleeTileAdjacent(playerX, playerY, collisionChecker) {
		m.State = StateAttacking
		m.StateTimer = 0
		return
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	// Stall watchdog: the cached path is only recomputed when the target tile
	// changes, so a route that an engaged packmate now bodily blocks (wedged
	// behind it in a one-tile corridor) is micro-danced on forever. No net
	// progress in half a second -> drop the path and replan against current
	// entity positions: flank if a route exists, else wait for the gap.
	m.stallTimer++
	if m.stallTimer >= 60 {
		if distance(m.X, m.Y, m.stallAnchorX, m.stallAnchorY) < 2 {
			m.ResetPathfinding()
		}
		m.stallAnchorX, m.stallAnchorY = m.X, m.Y
		m.stallTimer = 0
	}

	// Arbitration invariant: the A* route moves first; greedy fallbacks run
	// ONLY when it produced no move - a greedy step that can preempt a live
	// path 2-cycles with it at an obstacle corner.
	if m.followPathToTarget(collisionChecker, playerX, playerY) {
		return
	}

	// Final approach: steer straight at the target. A*'s goals are TILE
	// CENTERS within attack reach, and that ring can be empty - an off-center
	// player puts adjacent centers just past reach while their own tile center
	// is blocked by the player's body - leaving a melee monster frozen a few
	// pixels out of range. A direct collision-checked step has no such
	// quantization; walls still win.
	if distanceToPlayer <= m.tileSize()*1.5 && m.stepToward(collisionChecker, playerX, playerY) {
		return
	}

	// Last resort: wedged at a diagonal whose LoS a wall/tree corner seals,
	// with no walkable route this tick - nudge out of the diagonal so the next
	// repath can route around.
	m.stepOutOfBlockedMeleeDiagonal(collisionChecker, playerX, playerY)
}

// entersTargetTile reports whether (x, y) would land a MELEE monster on the
// target's own tile. RT melee must hold one tile out - mirroring the TB rule and
// the A* goal ring, which already excludes the target tile. The player is a
// non-solid collision entity, so CanMoveToWithHabitat won't stop a pixel step
// from crossing onto the party and overlapping their sprite ("wolf on the head");
// this guard is what keeps the final-approach steps off the player tile.
// TODO: ranged mobs are skipped here because they should stop at firing range,
// but LOS/pathing edge cases can still let them step onto the party tile.
func (m *Monster3D) entersTargetTile(x, y, targetX, targetY float64) bool {
	if m.HasRangedAttack() {
		return false
	}
	return m.worldToTile(x) == m.worldToTile(targetX) && m.worldToTile(y) == m.worldToTile(targetY)
}

// stepToward takes one collision-checked step straight toward (tx, ty) at the
// monster's pursuit speed, sliding along whichever axis stays clear when the
// diagonal grazes an obstacle. Returns false when fully blocked or when the only
// available step would put a melee monster onto the target's tile (caller then
// falls through to A*, whose goal ring stops one tile out).
func (m *Monster3D) stepToward(collisionChecker CollisionChecker, tx, ty float64) bool {
	dx := tx - m.X
	dy := ty - m.Y
	dist := math.Hypot(dx, dy)
	if dist < 1e-6 {
		return false
	}
	step := m.speedPerTick()
	if dist < step {
		step = dist
	}
	newX := m.X + dx/dist*step
	newY := m.Y + dy/dist*step
	if !m.entersTargetTile(newX, newY, tx, ty) && collisionChecker.CanMoveToWithHabitat(m.ID, newX, newY, m.HabitatPrefs, m.Flying) {
		m.X, m.Y = newX, newY
		return true
	}
	if dx != 0 && !m.entersTargetTile(newX, m.Y, tx, ty) && collisionChecker.CanMoveToWithHabitat(m.ID, newX, m.Y, m.HabitatPrefs, m.Flying) {
		m.X = newX
		return true
	}
	if dy != 0 && !m.entersTargetTile(m.X, newY, tx, ty) && collisionChecker.CanMoveToWithHabitat(m.ID, m.X, newY, m.HabitatPrefs, m.Flying) {
		m.Y = newY
		return true
	}
	return false
}

func (m *Monster3D) stepOutOfBlockedMeleeDiagonal(collisionChecker CollisionChecker, targetX, targetY float64) bool {
	if collisionChecker == nil || m.HasRangedAttack() {
		return false
	}
	ts := m.tileSize()
	if ts <= 0 {
		return false
	}
	dxTile := m.worldToTile(m.X) - m.worldToTile(targetX)
	dyTile := m.worldToTile(m.Y) - m.worldToTile(targetY)
	if mathutil.IntAbs(dxTile) != 1 || mathutil.IntAbs(dyTile) != 1 {
		return false
	}
	if collisionChecker.CheckLineOfSight(m.X, m.Y, targetX, targetY) {
		return false
	}
	step := m.speedPerTick()
	candidates := [2]struct {
		x, y float64
	}{
		{x: m.X + float64(mathutil.IntSign(dxTile))*step, y: m.Y},
		{x: m.X, y: m.Y + float64(mathutil.IntSign(dyTile))*step},
	}
	for _, c := range candidates {
		if !m.entersTargetTile(c.x, c.y, targetX, targetY) && collisionChecker.CanMoveToWithHabitat(m.ID, c.x, c.y, m.HabitatPrefs, m.Flying) {
			m.X, m.Y = c.x, c.y
			m.ResetPathfinding()
			return true
		}
	}
	return false
}

func (m *Monster3D) setMoveTarget(state MonsterState, tileX, tileY int) {
	m.MoveTargetState = state
	m.MoveTargetTileX = tileX
	m.MoveTargetTileY = tileY
	m.HasMoveTarget = true
}

func (m *Monster3D) clearMoveTarget() {
	m.HasMoveTarget = false
	m.MoveTargetTileX = 0
	m.MoveTargetTileY = 0
}

// ResetPathfinding drops the cached A* path and move-target so the monster
// repaths from its current position. Call after teleporting it (e.g. a boss blink).
func (m *Monster3D) ResetPathfinding() {
	m.PathTiles = nil
	m.PathIndex = 0
	m.PathTargetTileX = 0
	m.PathTargetTileY = 0
	m.LastPathCalcTick = 0 // also lifts the failed-search retry gate
	m.clearMoveTarget()
}

func (m *Monster3D) hasMoveTarget(state MonsterState) bool {
	return m.HasMoveTarget && m.MoveTargetState == state
}

func (m *Monster3D) currentMoveTarget() TileCoord {
	return TileCoord{X: m.MoveTargetTileX, Y: m.MoveTargetTileY}
}

func (m *Monster3D) isAtTile(tileX, tileY int) bool {
	return m.worldToTile(m.X) == tileX && m.worldToTile(m.Y) == tileY
}

func (m *Monster3D) pickPatrolTarget(collisionChecker CollisionChecker) (TileCoord, bool) {
	if collisionChecker == nil {
		return TileCoord{}, false
	}

	tileRadius := int(math.Ceil(m.TetherRadius / m.tileSize()))
	if tileRadius < 1 {
		tileRadius = 1
	}

	spawnTileX := m.worldToTile(m.SpawnX)
	spawnTileY := m.worldToTile(m.SpawnY)

	const attempts = 20
	for i := 0; i < attempts; i++ {
		dx := rand.Intn(tileRadius*2+1) - tileRadius
		dy := rand.Intn(tileRadius*2+1) - tileRadius
		if dx == 0 && dy == 0 {
			continue
		}
		tileX := spawnTileX + dx
		tileY := spawnTileY + dy
		centerX, centerY := m.tileToWorldCenter(tileX, tileY)
		if distance(centerX, centerY, m.SpawnX, m.SpawnY) > m.TetherRadius {
			continue
		}
		if collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying) {
			return TileCoord{X: tileX, Y: tileY}, true
		}
	}

	if collisionChecker.CanMoveToWithHabitat(m.ID, m.SpawnX, m.SpawnY, m.HabitatPrefs, m.Flying) {
		return TileCoord{X: spawnTileX, Y: spawnTileY}, true
	}

	return TileCoord{}, false
}

func (m *Monster3D) pickFleeTarget(collisionChecker CollisionChecker, playerX, playerY float64) (TileCoord, bool) {
	if collisionChecker == nil {
		return TileCoord{}, false
	}

	fleeDistance := m.tileSize() * 4
	if m.config != nil && m.config.MonsterAI.FleeDistanceTiles > 0 {
		fleeDistance = m.config.MonsterAI.FleeDistanceTiles * m.tileSize()
	}

	dx := m.X - playerX
	dy := m.Y - playerY
	dist := math.Hypot(dx, dy)
	if dist < 0.01 {
		angle := rand.Float64() * 2 * math.Pi
		dx = math.Cos(angle)
		dy = math.Sin(angle)
		dist = 1
	}

	const attempts = 12
	for i := 0; i < attempts; i++ {
		angleOffset := (float64(i) - float64(attempts-1)/2) * 0.15
		angle := math.Atan2(dy, dx) + angleOffset
		targetX := m.X + math.Cos(angle)*fleeDistance
		targetY := m.Y + math.Sin(angle)*fleeDistance
		tileX := m.worldToTile(targetX)
		tileY := m.worldToTile(targetY)
		if m.isAtTile(tileX, tileY) {
			// The own tile is never a flee target: "reaching" it instantly
			// re-looped the picker forever (and skipped the flee timeout).
			continue
		}
		centerX, centerY := m.tileToWorldCenter(tileX, tileY)
		if collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying) {
			return TileCoord{X: tileX, Y: tileY}, true
		}
	}

	return TileCoord{}, false
}

// pathCheckFrequency returns the configured repath throttle in ticks (default 30).
func (m *Monster3D) pathCheckFrequency() int {
	if m.config != nil && m.config.MonsterAI.PathCheckFrequency > 0 {
		return m.config.MonsterAI.PathCheckFrequency
	}
	return 30
}

// followPathToTarget computes (or reuses) an A* path and moves toward the next tile.
func (m *Monster3D) followPathToTarget(collisionChecker CollisionChecker, targetX, targetY float64) bool {
	if collisionChecker == nil {
		return false
	}

	targetTileX := m.worldToTile(targetX)
	targetTileY := m.worldToTile(targetY)

	shouldRepath := len(m.PathTiles) == 0 || m.PathIndex >= len(m.PathTiles)
	targetChanged := m.PathTargetTileX != targetTileX || m.PathTargetTileY != targetTileY
	if targetChanged {
		shouldRepath = true
	}

	// A failed search (boxed in: previous A* toward this same target found no
	// path) retries at pathCheckFrequency, not every tick - otherwise a wedged
	// pursuer reruns a full A* 120x/s for as long as it stays blocked.
	if shouldRepath && !targetChanged && len(m.PathTiles) == 0 && m.LastPathCalcTick > 0 && !m.canRepath(m.pathCheckFrequency()) {
		return false
	}

	return m.followPathStep(collisionChecker, targetTileX, targetTileY, shouldRepath,
		func() []TileCoord { return m.findPathToTarget(collisionChecker, targetX, targetY) },
		m.speedPerTick(), false, true)
}

// followPathToTile computes (or reuses) an A* path to a tile and moves toward it.
func (m *Monster3D) followPathToTile(collisionChecker CollisionChecker, targetTileX, targetTileY int) bool {
	if collisionChecker == nil {
		return false
	}

	shouldRepath := len(m.PathTiles) == 0 || m.PathIndex >= len(m.PathTiles)
	targetChanged := m.PathTargetTileX != targetTileX || m.PathTargetTileY != targetTileY
	if targetChanged && !shouldRepath && m.canRepath(m.pathCheckFrequency()) {
		shouldRepath = true
	}

	return m.followPathStep(collisionChecker, targetTileX, targetTileY, shouldRepath,
		func() []TileCoord { return m.findPathToTile(collisionChecker, targetTileX, targetTileY) },
		m.movementSpeed(m.State), true, false)
}

// followPathStep advances one tick along m.PathTiles toward (targetTileX,
// targetTileY), recomputing the path via computePath when shouldRepath. It snaps
// onto the next tile centre when within one step, else moves straight toward it.
// haltOnZeroSpeed returns early on speed <= 0 (tile variant); cornerSlide enables
// the axis-slide fallback (target variant only).
func (m *Monster3D) followPathStep(collisionChecker CollisionChecker, targetTileX, targetTileY int, shouldRepath bool, computePath func() []TileCoord, speed float64, haltOnZeroSpeed, cornerSlide bool) bool {
	if shouldRepath {
		m.PathTiles = computePath()
		m.PathIndex = 0
		m.PathTargetTileX = targetTileX
		m.PathTargetTileY = targetTileY
		m.LastPathCalcTick = m.StateTimer
	}

	if len(m.PathTiles) == 0 {
		return false
	}

	if !m.advancePathIndexToCurrentTile() {
		return false
	}

	if m.PathIndex >= len(m.PathTiles) {
		return false
	}

	if haltOnZeroSpeed && speed <= 0 {
		return false
	}

	next := m.PathTiles[m.PathIndex]
	targetCenterX, targetCenterY := m.tileToWorldCenter(next.X, next.Y)

	dx := targetCenterX - m.X
	dy := targetCenterY - m.Y
	dist := math.Hypot(dx, dy)
	step := speed
	if dist < step {
		step = dist
	}

	if dist <= step {
		if collisionChecker.CanMoveToWithHabitat(m.ID, targetCenterX, targetCenterY, m.HabitatPrefs, m.Flying) {
			m.X = targetCenterX
			m.Y = targetCenterY
			m.PathIndex++
			return true
		}
		m.PathTiles = nil
		return false
	}

	newX := m.X + dx/dist*step
	newY := m.Y + dy/dist*step

	if collisionChecker.CanMoveToWithHabitat(m.ID, newX, newY, m.HabitatPrefs, m.Flying) {
		m.X = newX
		m.Y = newY
		return true
	}

	if cornerSlide {
		// The diagonal step clips a wall corner (the box grazes the inside edge
		// while rounding it). Instead of giving up and freezing, slide along
		// whichever axis is still clear so the monster rounds the corner; only
		// if BOTH axes are blocked do we repath.
		if dx != 0 && collisionChecker.CanMoveToWithHabitat(m.ID, newX, m.Y, m.HabitatPrefs, m.Flying) {
			m.X = newX
			return true
		}
		if dy != 0 && collisionChecker.CanMoveToWithHabitat(m.ID, m.X, newY, m.HabitatPrefs, m.Flying) {
			m.Y = newY
			return true
		}
	}

	// Blocked - drop the path and repath next tick.
	m.PathTiles = nil
	return false
}

// advancePathIndexToCurrentTile aligns PathIndex with the monster's current tile
// when a fresh path begins (PathIndex 0): it skips the node the monster already
// stands on, scanning forward to locate it. Returns false (and drops the path)
// when the current tile isn't on the path at all, so the caller repaths.
func (m *Monster3D) advancePathIndexToCurrentTile() bool {
	if m.PathIndex != 0 {
		return true
	}
	currentTileX := m.worldToTile(m.X)
	currentTileY := m.worldToTile(m.Y)
	if m.PathTiles[0].X == currentTileX && m.PathTiles[0].Y == currentTileY {
		m.PathIndex = 1
		return true
	}
	for i := range m.PathTiles {
		if m.PathTiles[i].X == currentTileX && m.PathTiles[i].Y == currentTileY {
			m.PathIndex = i + 1
			return true
		}
	}
	m.PathTiles = nil
	return false
}

func (m *Monster3D) findPathToTarget(collisionChecker CollisionChecker, targetX, targetY float64) []TileCoord {
	start := TileCoord{X: m.worldToTile(m.X), Y: m.worldToTile(m.Y)}
	targetTileX := m.worldToTile(targetX)
	targetTileY := m.worldToTile(targetY)

	goals := m.collectGoalTiles(collisionChecker, targetX, targetY)
	if len(goals) == 0 {
		return nil
	}

	rangeTiles := int(math.Ceil(m.AlertRadius / m.tileSize()))
	if rangeTiles < 4 {
		rangeTiles = 4
	}
	rangeTiles *= 2
	if m.relentlessHunter() {
		// Map-wide pursuit: widen the window to hold maze detours (48 covers a 50x50 map).
		rangeTiles = 48
	}

	minX := mathutil.IntMin(start.X, targetTileX) - rangeTiles
	maxX := mathutil.IntMax(start.X, targetTileX) + rangeTiles
	minY := mathutil.IntMin(start.Y, targetTileY) - rangeTiles
	maxY := mathutil.IntMax(start.Y, targetTileY) + rangeTiles

	return m.findPathAStar(collisionChecker, start, goals, minX, maxX, minY, maxY)
}

// NextPathStepTile returns the next cardinal tile this monster should step to en
// route to (targetX,targetY) via the same A* the real-time AI uses, plus ok=true.
// ok=false means it's already adjacent or no path exists. Turn-based movement uses
// this so mobs route around barriers (across a bridge/ford) instead of oscillating
// at the edge - where they could otherwise be safely ranged down.
func (m *Monster3D) NextPathStepTile(collisionChecker CollisionChecker, targetX, targetY float64) (tileX, tileY int, ok bool) {
	path := m.findPathToTarget(collisionChecker, targetX, targetY)
	if len(path) < 2 {
		return 0, 0, false // path[0] is the current tile; need at least one step
	}
	return path[1].X, path[1].Y, true
}

// NextPathStepTileToAny returns the next cardinal tile toward one of the given
// goal tiles. It is used by callers with mode-specific goals, e.g. turn-based
// ranged monsters that need a row/column firing lane rather than any tile inside
// their circular projectile range.
func (m *Monster3D) NextPathStepTileToAny(collisionChecker CollisionChecker, goals []TileCoord) (tileX, tileY int, ok bool) {
	if collisionChecker == nil || len(goals) == 0 {
		return 0, 0, false
	}
	start := TileCoord{X: m.worldToTile(m.X), Y: m.worldToTile(m.Y)}

	minGoalX, maxGoalX := goals[0].X, goals[0].X
	minGoalY, maxGoalY := goals[0].Y, goals[0].Y
	for _, goal := range goals[1:] {
		if goal.X < minGoalX {
			minGoalX = goal.X
		}
		if goal.X > maxGoalX {
			maxGoalX = goal.X
		}
		if goal.Y < minGoalY {
			minGoalY = goal.Y
		}
		if goal.Y > maxGoalY {
			maxGoalY = goal.Y
		}
	}

	rangeTiles := int(math.Ceil(m.AlertRadius / m.tileSize()))
	if rangeTiles < 4 {
		rangeTiles = 4
	}
	rangeTiles *= 2
	if m.relentlessHunter() {
		rangeTiles = 48
	}

	minX := mathutil.IntMin(start.X, minGoalX) - rangeTiles
	maxX := mathutil.IntMax(start.X, maxGoalX) + rangeTiles
	minY := mathutil.IntMin(start.Y, minGoalY) - rangeTiles
	maxY := mathutil.IntMax(start.Y, maxGoalY) + rangeTiles

	path := m.findPathAStar(collisionChecker, start, goals, minX, maxX, minY, maxY)
	if len(path) < 2 {
		return 0, 0, false
	}
	return path[1].X, path[1].Y, true
}

func (m *Monster3D) findPathToTile(collisionChecker CollisionChecker, targetTileX, targetTileY int) []TileCoord {
	start := TileCoord{X: m.worldToTile(m.X), Y: m.worldToTile(m.Y)}
	goal := TileCoord{X: targetTileX, Y: targetTileY}

	if !m.isPassableTile(collisionChecker, goal) {
		return nil
	}

	dx := math.Abs(float64(goal.X - start.X))
	dy := math.Abs(float64(goal.Y - start.Y))
	rangeTiles := int(math.Max(dx, dy)) + 8
	if rangeTiles < 6 {
		rangeTiles = 6
	}

	minX := mathutil.IntMin(start.X, goal.X) - rangeTiles
	maxX := mathutil.IntMax(start.X, goal.X) + rangeTiles
	minY := mathutil.IntMin(start.Y, goal.Y) - rangeTiles
	maxY := mathutil.IntMax(start.Y, goal.Y) + rangeTiles

	return m.findPathAStar(collisionChecker, start, []TileCoord{goal}, minX, maxX, minY, maxY)
}

func (m *Monster3D) findPathAStar(collisionChecker CollisionChecker, start TileCoord, goals []TileCoord, minX, maxX, minY, maxY int) []TileCoord {
	if maxX < minX || maxY < minY {
		return nil
	}
	width := maxX - minX + 1
	height := maxY - minY + 1
	if width <= 0 || height <= 0 {
		return nil
	}

	ps := &m.pathScratch
	ps.prepare(width, height, minX, minY)

	startIdx := ps.index(start)
	if startIdx < 0 {
		return nil
	}

	for _, g := range goals {
		if idx := ps.index(g); idx >= 0 {
			ps.goal[idx] = true
		}
	}

	// The start tile is checked terrain-only: the monster already stands there,
	// and an entity check would let an interlocked engaged neighbor (two mobs
	// aggroed in the same tile - each covering the shared tile center) abort
	// every path attempt, freezing both in place.
	if !ps.goal[startIdx] {
		startCX, startCY := m.tileToWorldCenter(start.X, start.Y)
		if !collisionChecker.CanOccupyTilesWithHabitat(m.ID, startCX, startCY, m.HabitatPrefs, m.Flying) {
			return nil
		}
	}

	heuristic := func(c TileCoord) float64 {
		best := math.MaxFloat64
		for _, g := range goals {
			d := math.Abs(float64(g.X-c.X)) + math.Abs(float64(g.Y-c.Y))
			if d < best {
				best = d
			}
		}
		if best == math.MaxFloat64 {
			return 0
		}
		return best
	}

	ps.gScore[startIdx] = 0
	ps.heap.push(gridNode{idx: startIdx, g: 0, f: heuristic(start)})

	nodesSearched := 0
	maxNodes := 500 // typical mob search area is ~200-400 tiles
	if m.relentlessHunter() {
		// Map-wide pursuit may path across a whole maze - well beyond a normal budget.
		maxNodes = 4000
	}

	for len(ps.heap.nodes) > 0 && nodesSearched < maxNodes {
		current, ok := ps.heap.pop()
		if !ok {
			break
		}
		if ps.closed[current.idx] {
			continue
		}
		if current.g > ps.gScore[current.idx] {
			continue
		}

		if ps.goal[current.idx] {
			return reconstructPathGrid(ps, current.idx)
		}

		ps.closed[current.idx] = true
		nodesSearched++

		coord := ps.coord(current.idx)
		for _, dir := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			neighbor := TileCoord{X: coord.X + dir[0], Y: coord.Y + dir[1]}
			nidx := ps.index(neighbor)
			if nidx < 0 {
				continue
			}
			if ps.closed[nidx] {
				continue
			}
			if neighbor != start && !m.isPassableTile(collisionChecker, neighbor) {
				continue
			}
			tentativeG := ps.gScore[current.idx] + 1
			if tentativeG < ps.gScore[nidx] {
				ps.cameFrom[nidx] = current.idx
				ps.gScore[nidx] = tentativeG
				f := tentativeG + heuristic(neighbor)
				ps.heap.push(gridNode{idx: nidx, g: tentativeG, f: f})
			}
		}
	}

	return nil
}

func (m *Monster3D) collectGoalTiles(collisionChecker CollisionChecker, targetX, targetY float64) []TileCoord {
	targetTileX := int(targetX / m.tileSize())
	targetTileY := int(targetY / m.tileSize())
	// Pursue to within the monster's actual attack reach - the ranged range for
	// ranged attackers, melee AttackRadius otherwise. Melee treats the 8 tiles
	// around the party as valid contact so mobs can surround instead of queueing
	// only on N/S/E/W.
	// Using only the melee radius made ranged mobs (e.g. dragons) path to melee
	// distance; when those near tiles were unreachable (party blocking a bridge)
	// they orbited without ever stopping at firing range.
	reach := m.GetAttackRangePixels()
	melee := !m.HasRangedAttack()
	radiusTiles := int(math.Ceil(reach / m.tileSize()))
	if radiusTiles < 1 {
		radiusTiles = 1
	}

	var goals []TileCoord
	// Ranged approach fallback: walkable tiles adjacent to the target, used only
	// when NO in-reach tile has a fire lane - the mob then closes distance to
	// win a line of sight instead of freezing parked-but-blind (see below).
	var approach []TileCoord
	for dy := -radiusTiles; dy <= radiusTiles; dy++ {
		for dx := -radiusTiles; dx <= radiusTiles; dx++ {
			tileX := targetTileX + dx
			tileY := targetTileY + dy
			centerX, centerY := m.tileToWorldCenter(tileX, tileY)
			adx, ady := mathutil.IntAbs(dx), mathutil.IntAbs(dy)
			adjacent := adx <= 1 && ady <= 1 && !(dx == 0 && dy == 0)
			if melee {
				if dx == 0 && dy == 0 {
					continue
				}
				if adjacent && !collisionChecker.CheckLineOfSight(centerX, centerY, targetX, targetY) {
					continue
				}
				if !adjacent && distance(targetX, targetY, centerX, centerY) > reach+0.1 {
					continue
				}
			} else {
				if distance(targetX, targetY, centerX, centerY) > reach+0.1 {
					continue
				}
			}
			if !collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying) {
				continue
			}
			if !melee && adjacent {
				approach = append(approach, TileCoord{X: tileX, Y: tileY}) // no-LOS fallback
			}
			// Ranged: a goal needs a FIRE LANE, not just range - a tile within
			// reach but walled off leaves the mob parked there, in range yet
			// forever unable to shoot (the attack gate requires LOS). LOS is the
			// costliest test, so it runs last and only on walkable candidates.
			if !melee && !collisionChecker.CheckLineOfSight(centerX, centerY, targetX, targetY) {
				continue
			}
			goals = append(goals, TileCoord{X: tileX, Y: tileY})
		}
	}

	// No firing lane anywhere in reach: fall back to closing on the target so a
	// walled-off ranged mob repositions for LOS instead of standing still.
	if len(goals) == 0 && !melee {
		return approach
	}
	return goals
}

func (m *Monster3D) isPassableTile(collisionChecker CollisionChecker, tile TileCoord) bool {
	centerX, centerY := m.tileToWorldCenter(tile.X, tile.Y)
	return collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying)
}

func reconstructPathGrid(ps *pathScratch, endIdx int) []TileCoord {
	path := make([]TileCoord, 0, 16)
	current := endIdx
	for current >= 0 {
		path = append(path, ps.coord(current))
		current = ps.cameFrom[current]
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func (m *Monster3D) speedPerTick() float64 {
	tps := 60
	if m.config != nil {
		tps = m.config.GetTPS()
	} else {
		tps = config.GetTargetTPS()
	}
	if tps <= 0 {
		return m.Speed
	}
	return m.Speed * (60.0 / float64(tps))
}

type movementSpeedMultipliers struct {
	Patrol float64
	Flee   float64
}

// movementSpeed returns the per-tick speed for the given state (search: move-speed).
func (m *Monster3D) movementSpeed(state MonsterState) float64 {
	base := m.speedPerTick()
	mults := m.movementSpeedMultipliers()
	switch state {
	case StatePatrolling:
		return base * mults.Patrol
	case StateFleeing:
		return base * mults.Flee
	default:
		return base
	}
}

func (m *Monster3D) movementSpeedMultipliers() movementSpeedMultipliers {
	mults := movementSpeedMultipliers{
		Patrol: 0.5,
		Flee:   1.5,
	}
	if m.config != nil {
		if m.config.MonsterAI.NormalSpeedMultiplier > 0 {
			mults.Patrol = m.config.MonsterAI.NormalSpeedMultiplier
		}
		if m.config.MonsterAI.FleeSpeedMultiplier > 0 {
			mults.Flee = m.config.MonsterAI.FleeSpeedMultiplier
		}
	}
	return mults
}

func (m *Monster3D) canRepath(pathCheckFrequency int) bool {
	if pathCheckFrequency <= 0 {
		return true
	}
	return m.StateTimer-m.LastPathCalcTick >= pathCheckFrequency
}

func (m *Monster3D) updateAlert(collisionChecker CollisionChecker, playerX, playerY float64) {
	if m.IsEngagingPlayer {
		// Calculate distance to player
		distanceToPlayer := distance(m.X, m.Y, playerX, playerY)

		// If close enough to attack, switch to attacking
		// Use a slightly tighter radius to prevent shaking at the boundary
		attackRange := m.GetAttackRangePixels()
		enterFraction := 0.9
		if m.config != nil && m.config.MonsterAI.AttackEnterRangeFraction > 0 {
			enterFraction = m.config.MonsterAI.AttackEnterRangeFraction
		}
		hasLOS := collisionChecker == nil || collisionChecker.CheckLineOfSight(m.X, m.Y, playerX, playerY)
		if (distanceToPlayer <= attackRange*enterFraction && hasLOS) || m.meleeTileAdjacent(playerX, playerY, collisionChecker) {
			m.State = StateAttacking
			m.StateTimer = 0
		} else {
			// Move towards player
			dx := playerX - m.X
			dy := playerY - m.Y
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

// AttackCooldownFrames is the real-time minimum interval between this monster's
// attacks (config base x its per-monster multiplier, >=1). Single source for both
// the attacking-state dwell and the persistent AttackCDFrames gate in combat.
func (m *Monster3D) AttackCooldownFrames() int {
	cd := 60
	if m.config != nil {
		cd = m.config.MonsterAI.AttackCooldown
	}
	if m.AttackCooldownMultiplier > 0 {
		cd = int(math.Round(float64(cd) * m.AttackCooldownMultiplier))
	}
	if m.IsEnraged() && m.EnrageCooldownMult > 0 {
		cd = int(math.Round(float64(cd) * m.EnrageCooldownMult))
	}
	if cd < 1 {
		cd = 1
	}
	return cd
}

// tbAttacksForCooldownMult maps a real-time attack-cooldown multiplier to the
// turn-based swing count that keeps the two modes at parity: a faster RT cadence
// (mult < 1) grants proportionally more TB swings. Power-of-two buckets so the
// count stays integer - mult >= 1 -> 1, [0.5,1) -> 2, [0.25,0.5) -> 4, ... (capped at
// 8). Used both for cooldown-only static configs and dynamic enrage multipliers.
func tbAttacksForCooldownMult(mult float64) int {
	if mult <= 0 {
		return 1
	}
	n := 1
	for mult < 1.0 && n < 8 {
		n *= 2
		mult *= 2
	}
	return n
}

func (m *Monster3D) updateAttacking(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Target stepped out of reach -> resume the chase immediately instead of
	// swinging at air for the rest of the cooldown. (updateAlert re-enters attack
	// at <=0.9xrange, so exiting at >range keeps a clean hysteresis band.)
	hasLOS := collisionChecker == nil || collisionChecker.CheckLineOfSight(m.X, m.Y, playerX, playerY)
	if m.IsEngagingPlayer && (distance(m.X, m.Y, playerX, playerY) > m.GetAttackRangePixels() || !hasLOS) &&
		!m.meleeTileAdjacent(playerX, playerY, collisionChecker) {
		m.State = StatePursuing
		m.StateTimer = 0
		return
	}

	// Attack delay from config
	if m.StateTimer > m.AttackCooldownFrames() {
		// Increment attack counter
		m.AttackCount++

		// After enough consecutive attacks, roll the configured chance to flee
		fleeAfter, fleeChance := 5, 0.5
		if m.config != nil {
			ai := &m.config.MonsterAI
			if ai.FleeAfterAttacks > 0 {
				fleeAfter = ai.FleeAfterAttacks
			}
			if ai.FleeAfterAttacksChance > 0 {
				fleeChance = ai.FleeAfterAttacksChance
			}
		}
		if m.AttackCount >= fleeAfter && rand.Float64() < fleeChance {
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

// updateFleeing moves monster away from player using pathfinding
func (m *Monster3D) updateFleeing(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Get AI config values
	var fleeDuration int = 300 // Default value

	if m.config != nil {
		fleeDuration = m.config.MonsterAI.FleeDuration
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	// Flee is over - reconsider instead of standing dazed: the party still in
	// reach (the same hysteresis radius that keeps engagement alive) -> rejoin
	// the fight; party gone -> wander home. This check runs FIRST: the movement
	// code below has early returns, and detection is fully disabled while
	// fleeing - a skipped timeout meant a permanently blind, frozen monster.
	if m.StateTimer > fleeDuration {
		m.clearMoveTarget()
		m.StateTimer = 0

		detectionRadius := m.AlertRadius
		if detectionRadius <= 0 {
			detectionRadius = 4.0 * m.tileSize()
		}
		disengageMult := 2.0
		if m.config != nil && m.config.MonsterAI.DisengageDistanceMultiplier > 0 {
			disengageMult = m.config.MonsterAI.DisengageDistanceMultiplier
		}
		if distance(m.X, m.Y, playerX, playerY) <= detectionRadius*disengageMult {
			m.IsEngagingPlayer = true
			m.State = StateAlert
		} else {
			m.State = StatePatrolling
			m.Direction = m.GetDirectionToSpawn()
		}
		return
	}

	// Reset RT target when state changes
	if m.MoveTargetState != StateFleeing {
		m.clearMoveTarget()
	}

	if !m.hasMoveTarget(StateFleeing) {
		if target, ok := m.pickFleeTarget(collisionChecker, playerX, playerY); ok {
			m.setMoveTarget(StateFleeing, target.X, target.Y)
		}
	}

	if m.hasMoveTarget(StateFleeing) {
		target := m.currentMoveTarget()
		if m.isAtTile(target.X, target.Y) {
			m.clearMoveTarget()
			return
		}
		if !m.followPathToTile(collisionChecker, target.X, target.Y) {
			m.clearMoveTarget()
		}
	}
}

// unstuckFromObstacles tries to move the monster to the nearest non-blocked position
// Useful when a monster ends up overlapping a solid tile (e.g., trees) due to edge cases
func (m *Monster3D) unstuckFromObstacles(collisionChecker CollisionChecker) {
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
			if collisionChecker.CanMoveToWithHabitat(m.ID, nx, ny, m.HabitatPrefs, m.Flying) {
				m.X = nx
				m.Y = ny
				return
			}
		}
	}
	// As a last resort, try the spawn position if within reasonable distance
	if collisionChecker.CanMoveToWithHabitat(m.ID, m.SpawnX, m.SpawnY, m.HabitatPrefs, m.Flying) {
		m.X = m.SpawnX
		m.Y = m.SpawnY
	}
}
