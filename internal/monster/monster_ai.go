package monster

import (
	"math"
	"math/rand"
	"ugataima/internal/config"
	"ugataima/internal/mathutil"
)

// CollisionChecker interface for checking movement validity
type CollisionChecker interface {
	CanMoveTo(entityID string, x, y float64) bool
	CanMoveToWithHabitat(entityID string, x, y float64, habitatPrefs []string, flying bool) bool
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

// Update runs the monster AI with collision checking and player position for engagement detection
func (m *Monster3D) Update(collisionChecker CollisionChecker, playerX, playerY float64) {
	if m.StunFramesRemaining > 0 {
		m.StunFramesRemaining--
		return
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
		m.updateAlert(playerX, playerY)
	case StateAttacking:
		m.updateAttacking(playerX, playerY)
	case StateFleeing:
		m.updateFleeing(collisionChecker, playerX, playerY)
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
	distanceToPlayer := distance(m.X, m.Y, playerX, playerY)

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
		spawnTileX := worldToTile(m.SpawnX)
		spawnTileY := worldToTile(m.SpawnY)
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

	currentCenterX, currentCenterY := worldToTileCenter(m.X, m.Y)
	targetX := currentCenterX + float64(dirX)*tileSize
	targetY := currentCenterY + float64(dirY)*tileSize

	if !collisionChecker.CanMoveToWithHabitat(m.ID, targetX, targetY, m.HabitatPrefs, m.Flying) {
		return false
	}

	dirAngle := math.Atan2(float64(dirY), float64(dirX))
	newX := m.X + math.Cos(dirAngle)*speed
	newY := m.Y + math.Sin(dirAngle)*speed

	if collisionChecker.CanMoveToWithHabitat(m.ID, newX, newY, m.HabitatPrefs, m.Flying) {
		m.X = newX
		m.Y = newY
		m.Direction = dirAngle
		return true
	}

	return false
}

// updatePursuing moves monster towards player using grid-based cardinal movement
func (m *Monster3D) updatePursuing(collisionChecker CollisionChecker, playerX, playerY float64) {
	// Calculate distance to player
	distanceToPlayer := distance(m.X, m.Y, playerX, playerY)
	attackRange := m.GetAttackRangePixels()

	// Check if close enough to attack
	if distanceToPlayer <= attackRange {
		m.State = StateAttacking
		m.StateTimer = 0
		return
	}

	// Safety check for collision system
	if collisionChecker == nil {
		return
	}

	// Use A* pathfinding toward the player
	m.followPathToTarget(collisionChecker, playerX, playerY)
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

func (m *Monster3D) hasMoveTarget(state MonsterState) bool {
	return m.HasMoveTarget && m.MoveTargetState == state
}

func (m *Monster3D) currentMoveTarget() TileCoord {
	return TileCoord{X: m.MoveTargetTileX, Y: m.MoveTargetTileY}
}

func (m *Monster3D) isAtTile(tileX, tileY int) bool {
	return worldToTile(m.X) == tileX && worldToTile(m.Y) == tileY
}

func (m *Monster3D) pickPatrolTarget(collisionChecker CollisionChecker) (TileCoord, bool) {
	if collisionChecker == nil {
		return TileCoord{}, false
	}

	tileRadius := int(math.Ceil(m.TetherRadius / tileSize))
	if tileRadius < 1 {
		tileRadius = 1
	}

	spawnTileX := worldToTile(m.SpawnX)
	spawnTileY := worldToTile(m.SpawnY)

	const attempts = 20
	for i := 0; i < attempts; i++ {
		dx := rand.Intn(tileRadius*2+1) - tileRadius
		dy := rand.Intn(tileRadius*2+1) - tileRadius
		if dx == 0 && dy == 0 {
			continue
		}
		tileX := spawnTileX + dx
		tileY := spawnTileY + dy
		centerX, centerY := tileToWorldCenter(tileX, tileY)
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

	fleeDistance := tileSize * 4
	if m.config != nil && m.config.MonsterAI.FleeVisionDistance > 0 {
		fleeDistance = m.config.MonsterAI.FleeVisionDistance
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
		tileX := worldToTile(targetX)
		tileY := worldToTile(targetY)
		centerX, centerY := tileToWorldCenter(tileX, tileY)
		if collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying) {
			return TileCoord{X: tileX, Y: tileY}, true
		}
	}

	return TileCoord{}, false
}

// followPathToTarget computes (or reuses) an A* path and moves toward the next tile.
func (m *Monster3D) followPathToTarget(collisionChecker CollisionChecker, targetX, targetY float64) bool {
	if collisionChecker == nil {
		return false
	}

	targetTileX := worldToTile(targetX)
	targetTileY := worldToTile(targetY)

	shouldRepath := len(m.PathTiles) == 0 || m.PathIndex >= len(m.PathTiles)
	targetChanged := m.PathTargetTileX != targetTileX || m.PathTargetTileY != targetTileY
	if targetChanged {
		shouldRepath = true
	}

	if shouldRepath {
		m.PathTiles = m.findPathToTarget(collisionChecker, targetX, targetY)
		m.PathIndex = 0
		m.PathTargetTileX = targetTileX
		m.PathTargetTileY = targetTileY
		m.LastPathCalcTick = m.StateTimer
	}

	if len(m.PathTiles) == 0 {
		return false
	}

	currentTileX := worldToTile(m.X)
	currentTileY := worldToTile(m.Y)

	if m.PathIndex == 0 {
		if m.PathTiles[0].X == currentTileX && m.PathTiles[0].Y == currentTileY {
			m.PathIndex = 1
		} else {
			for i := range m.PathTiles {
				if m.PathTiles[i].X == currentTileX && m.PathTiles[i].Y == currentTileY {
					m.PathIndex = i + 1
					break
				}
			}
			if m.PathIndex == 0 {
				m.PathTiles = nil
				return false
			}
		}
	}

	if m.PathIndex >= len(m.PathTiles) {
		return false
	}

	next := m.PathTiles[m.PathIndex]
	targetCenterX, targetCenterY := tileToWorldCenter(next.X, next.Y)

	dx := targetCenterX - m.X
	dy := targetCenterY - m.Y
	dist := math.Hypot(dx, dy)
	speed := m.speedPerTick()
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
		m.Direction = math.Atan2(dy, dx)
		return true
	}

	// Path blocked - drop it and fall back to grid movement
	m.PathTiles = nil
	return false
}

// followPathToTile computes (or reuses) an A* path to a tile and moves toward it.
func (m *Monster3D) followPathToTile(collisionChecker CollisionChecker, targetTileX, targetTileY int) bool {
	if collisionChecker == nil {
		return false
	}

	pathCheckFrequency := 30
	if m.config != nil && m.config.MonsterAI.PathCheckFrequency > 0 {
		pathCheckFrequency = m.config.MonsterAI.PathCheckFrequency
	}

	shouldRepath := len(m.PathTiles) == 0 || m.PathIndex >= len(m.PathTiles)
	targetChanged := m.PathTargetTileX != targetTileX || m.PathTargetTileY != targetTileY
	if targetChanged && !shouldRepath && m.canRepath(pathCheckFrequency) {
		shouldRepath = true
	}

	if shouldRepath {
		m.PathTiles = m.findPathToTile(collisionChecker, targetTileX, targetTileY)
		m.PathIndex = 0
		m.PathTargetTileX = targetTileX
		m.PathTargetTileY = targetTileY
		m.LastPathCalcTick = m.StateTimer
	}

	if len(m.PathTiles) == 0 {
		return false
	}

	currentTileX := worldToTile(m.X)
	currentTileY := worldToTile(m.Y)

	if m.PathIndex == 0 {
		if m.PathTiles[0].X == currentTileX && m.PathTiles[0].Y == currentTileY {
			m.PathIndex = 1
		} else {
			for i := range m.PathTiles {
				if m.PathTiles[i].X == currentTileX && m.PathTiles[i].Y == currentTileY {
					m.PathIndex = i + 1
					break
				}
			}
			if m.PathIndex == 0 {
				m.PathTiles = nil
				return false
			}
		}
	}

	if m.PathIndex >= len(m.PathTiles) {
		return false
	}

	speed := m.movementSpeed(m.State)
	if speed <= 0 {
		return false
	}

	next := m.PathTiles[m.PathIndex]
	targetCenterX, targetCenterY := tileToWorldCenter(next.X, next.Y)

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
		m.Direction = math.Atan2(dy, dx)
		return true
	}

	m.PathTiles = nil
	return false
}

func (m *Monster3D) findPathToTarget(collisionChecker CollisionChecker, targetX, targetY float64) []TileCoord {
	start := TileCoord{X: worldToTile(m.X), Y: worldToTile(m.Y)}
	targetTileX := worldToTile(targetX)
	targetTileY := worldToTile(targetY)

	goals := m.collectGoalTiles(collisionChecker, targetX, targetY)
	if len(goals) == 0 {
		return nil
	}

	rangeTiles := int(math.Ceil(m.AlertRadius / tileSize))
	if rangeTiles < 4 {
		rangeTiles = 4
	}
	rangeTiles *= 2

	minX := mathutil.IntMin(start.X, targetTileX) - rangeTiles
	maxX := mathutil.IntMax(start.X, targetTileX) + rangeTiles
	minY := mathutil.IntMin(start.Y, targetTileY) - rangeTiles
	maxY := mathutil.IntMax(start.Y, targetTileY) + rangeTiles

	return m.findPathAStar(collisionChecker, start, goals, minX, maxX, minY, maxY)
}

func (m *Monster3D) findPathToTile(collisionChecker CollisionChecker, targetTileX, targetTileY int) []TileCoord {
	start := TileCoord{X: worldToTile(m.X), Y: worldToTile(m.Y)}
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

	if !ps.goal[startIdx] && !m.isPassableTile(collisionChecker, start) {
		return nil
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
	maxNodes := 500 // Reduced from 2000 - typical search area is ~200-400 tiles

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
	targetTileX := int(targetX / tileSize)
	targetTileY := int(targetY / tileSize)
	radiusTiles := int(math.Ceil(m.AttackRadius / tileSize))
	if radiusTiles < 1 {
		radiusTiles = 1
	}

	var goals []TileCoord
	for dy := -radiusTiles; dy <= radiusTiles; dy++ {
		for dx := -radiusTiles; dx <= radiusTiles; dx++ {
			tileX := targetTileX + dx
			tileY := targetTileY + dy
			centerX, centerY := tileToWorldCenter(tileX, tileY)
			if distance(targetX, targetY, centerX, centerY) > m.AttackRadius+0.1 {
				continue
			}
			if collisionChecker.CanMoveToWithHabitat(m.ID, centerX, centerY, m.HabitatPrefs, m.Flying) {
				goals = append(goals, TileCoord{X: tileX, Y: tileY})
			}
		}
	}

	return goals
}

func (m *Monster3D) isPassableTile(collisionChecker CollisionChecker, tile TileCoord) bool {
	centerX, centerY := tileToWorldCenter(tile.X, tile.Y)
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

func (m *Monster3D) updateAlert(playerX, playerY float64) {
	if m.IsEngagingPlayer {
		// Calculate distance to player
		distanceToPlayer := distance(m.X, m.Y, playerX, playerY)

		// If close enough to attack, switch to attacking
		// Use a slightly tighter radius to prevent shaking at the boundary
		attackRange := m.GetAttackRangePixels()
		if distanceToPlayer <= attackRange*0.9 {
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

	// Reset RT target when state changes
	if m.MoveTargetState != StateFleeing {
		m.clearMoveTarget()
	}

	shouldPickNewTarget := !m.hasMoveTarget(StateFleeing)

	if shouldPickNewTarget {
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

	// Stop fleeing after a while
	if m.StateTimer > fleeDuration {
		m.State = StateIdle
		m.StateTimer = 0
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
				m.Direction = angle
				return
			}
		}
	}
	// As a last resort, try the spawn position if within reasonable distance
	if collisionChecker.CanMoveToWithHabitat(m.ID, m.SpawnX, m.SpawnY, m.HabitatPrefs, m.Flying) {
		m.X = m.SpawnX
		m.Y = m.SpawnY
		m.Direction = m.GetDirectionToSpawn()
	}
}
