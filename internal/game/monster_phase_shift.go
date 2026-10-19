package game

import (
	"cmp"
	"math"
	"slices"

	uitext "ugataima/assets/text"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Phase Shift (abilities.phase_shift) lets a walker blink across a chasm or
// water toward the party. It is a movement fallback only: while the monster's
// own pursuit reaches an attack position within monster_ai.phase_shift's
// detour_factor times the best walk-and-blink plan (counting the cooldown
// still to wait), pursuit runs exactly as before. The blink runs straight
// along a row or column, over at least one gap tile, onto the first walkable
// tile beyond it within range_tiles; walls, doors, exits and solid props stop
// it, and it may pass over the party.
//
// Cost: a replan runs the pursuit's own search plus two flood fills over the
// planning box, several pursuit repaths' worth. It runs only for authored
// phase shifters chasing the party with a gap nearby, so keep phase_shift on
// monsters of small dungeon maps (the Solstice chambers), not open-world ones.
//
// Ordinary code reaches Phase Shift only through these entry points:
//   - refreshPhaseShiftPlan: the RT AI pre-pass (refreshMonsterAIState)
//   - tryPhaseShift: the RT interaction pass (handleMonsterInteraction)
//   - tryPhaseShiftTB, stepTowardPhaseLaunchTB: the TB monster turn
//   - phaseShiftLandingNextTurn: the TB tactical overlay
//
// The monster side (plan, cooldowns, the RT launch walk) is in
// internal/monster/phase_shift.go.

// phaseShiftEligible: a hostile walker chasing the party that cannot strike it
// from where it stands.
func (cs *CombatSystem) phaseShiftEligible(m *monster.Monster3D) bool {
	if !m.CanPhaseShift() || !m.IsAlive() || m.AIFoe != nil || m.Bound || m.IsInertSetPiece() || !m.TargetsParty() {
		return false
	}
	switch m.CurrentAIBehavior() {
	case monster.AIBehaviorSeekParty, monster.AIBehaviorRelentlessParty:
	default:
		return false
	}
	if cs.game.turnBasedMode {
		// Grid reach alone: a walker in attack transit still counts as in reach.
		px, py := cs.logicalCameraXY()
		return !cs.monsterAttackPointInReachTB(m, px, py)
	}
	return !cs.partyInStrikeReach(m, false)
}

// phaseShiftWalk is the monster's own pursuit in the active mode: the attack
// positions it walks to and the length of the route it finds to one of them
// (-1: none). A turn-based ranged attacker needs a row or column lane.
func (cs *CombatSystem) phaseShiftWalk(m *monster.Monster3D, px, py float64) ([]monster.TileCoord, int) {
	g := cs.game
	if !g.turnBasedMode {
		return m.PursuitRoute(g.collisionSystem, px, py)
	}
	goals := g.turnBasedApproachGoals(m, px, py)
	return goals, m.PathTilesToAny(g.collisionSystem, goals, g.attackTargetTile(m))
}

// phaseScratch keeps the planner's buffers between plans. Planning is serial
// (the AI pre-pass, the RT interaction pass, the TB monster turn, the overlay).
type phaseScratch struct {
	walk             []int8 // 0 unknown, 1 traversable, 2 blocked
	toGoal, fromSelf []int
	queue            []monster.TileCoord
}

// phasePlanner is one plan's tile box over the shared scratch, with a
// traversal cache for the walking fields and the blink checks. The party's
// tile is never walkable, as for the pursuit (blockedAttackTargetTile).
// Other monsters do not block a launch or a landing: walkers waiting or
// landing together are a transit stack (the tile fan draws
// them apart) and pick their blinks independently.
type phasePlanner struct {
	cs               *CombatSystem
	m                *monster.Monster3D
	party            monster.TileCoord
	minX, minY, w, h int
	s                *phaseScratch
}

func (cs *CombatSystem) newPhasePlanner(m *monster.Monster3D, party, a, b monster.TileCoord, margin int) *phasePlanner {
	g := cs.game
	minX, minY := max(0, min(a.X, b.X)-margin), max(0, min(a.Y, b.Y)-margin)
	maxX, maxY := min(g.world.Width-1, max(a.X, b.X)+margin), min(g.world.Height-1, max(a.Y, b.Y)+margin)
	p := &phasePlanner{cs: cs, m: m, party: party, minX: minX, minY: minY, w: maxX - minX + 1, h: maxY - minY + 1, s: &cs.phase}
	p.s.walk = slices.Grow(p.s.walk[:0], p.w*p.h)[:p.w*p.h]
	clear(p.s.walk)
	return p
}

func (p *phasePlanner) index(t monster.TileCoord) int {
	if t.X < p.minX || t.Y < p.minY || t.X >= p.minX+p.w || t.Y >= p.minY+p.h {
		return -1
	}
	return (t.Y-p.minY)*p.w + t.X - p.minX
}

// traversable is the monster's own movement rule (habitat, body, blockers).
func (p *phasePlanner) traversable(t monster.TileCoord) bool {
	i := p.index(t)
	if i < 0 || t == p.party {
		return false
	}
	if p.s.walk[i] == 0 {
		p.s.walk[i] = 2
		if p.m.CanTraverseTile(p.cs.game.collisionSystem, t) {
			p.s.walk[i] = 1
		}
	}
	return p.s.walk[i] == 1
}

// gap: a tile the monster cannot walk that fly-over content lets a blink cross
// (chasms, water). Walls and props are never gaps.
func (p *phasePlanner) gap(t monster.TileCoord) bool {
	return !p.traversable(t) && flyOverTile(p.cs.game.world, t)
}

// flyOverTile: chasm or water a blink may cross; walls and props never are.
func flyOverTile(w *world.World3D, t monster.TileCoord) bool {
	if world.GlobalTileManager == nil || t.X < 0 || t.Y < 0 || t.X >= w.Width || t.Y >= w.Height {
		return false
	}
	d := world.GlobalTileManager.GetTileData(w.Tiles[t.Y][t.X])
	return d != nil && d.FlyOver // the tile loader keeps fly_over tiles non-solid
}

// phaseShiftGapNear: a fly-over tile lies within radius tiles of the monster,
// the cheap precondition for any blink from there.
func phaseShiftGapNear(w *world.World3D, m *monster.Monster3D, ts float64, r int) bool {
	tx, ty := TileIndex(m.X, ts), TileIndex(m.Y, ts)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r && flyOverTile(w, monster.TileCoord{X: tx + dx, Y: ty + dy}) {
				return true
			}
		}
	}
	return false
}

// field fills dist (reused) with the monster's four-way walking distance to
// the nearest of starts inside the box, up to depth; -1 = unreachable or
// farther. Start tiles count as reached, except the party's own tile.
func (p *phasePlanner) field(starts []monster.TileCoord, dist []int, depth int) []int {
	dist = slices.Grow(dist[:0], p.w*p.h)[:p.w*p.h]
	for i := range dist {
		dist[i] = -1
	}
	queue := p.s.queue[:0]
	for _, start := range starts {
		if i := p.index(start); i >= 0 && dist[i] < 0 && start != p.party {
			dist[i] = 0
			queue = append(queue, start)
		}
	}
	for head := 0; head < len(queue); head++ {
		t := queue[head]
		d := dist[p.index(t)]
		if d >= depth {
			continue
		}
		for _, n := range [4]monster.TileCoord{{X: t.X + 1, Y: t.Y}, {X: t.X - 1, Y: t.Y}, {X: t.X, Y: t.Y + 1}, {X: t.X, Y: t.Y - 1}} {
			if i := p.index(n); i >= 0 && dist[i] < 0 && p.traversable(n) {
				dist[i] = d + 1
				queue = append(queue, n)
			}
		}
	}
	p.s.queue = queue
	return dist
}

// blinkDirections are the four straight lines a blink may take, in row-major
// order of their (dy, dx) so ties break the same way every time.
var blinkDirections = [4][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}} // {dx, dy}: up, left, right, down

// blinkLanding walks a straight line from launch: one or more gaps, then the
// first walkable tile within reach is the landing (the party's tile is passed
// over). Anything else on the line - a wall, a door, an exit, a solid prop or
// sight blocker - stops it. Returns the landing and its distance in tiles.
func (p *phasePlanner) blinkLanding(launch monster.TileCoord, dir [2]int, reach int) (monster.TileCoord, int, bool) {
	crossed := false
	for k := 1; k <= reach; k++ {
		t := monster.TileCoord{X: launch.X + dir[0]*k, Y: launch.Y + dir[1]*k}
		switch {
		case p.gap(t):
			crossed = true
		case t == p.party:
			if !crossed {
				return monster.TileCoord{}, 0, false
			}
		case p.traversable(t):
			if !crossed || !p.sightClear(launch, t) {
				return monster.TileCoord{}, 0, false
			}
			return t, k, true
		default:
			return monster.TileCoord{}, 0, false
		}
	}
	return monster.TileCoord{}, 0, false
}

// sightClear: no dynamic sight blocker stands between the two tile centers.
func (p *phasePlanner) sightClear(from, to monster.TileCoord) bool {
	ts := float64(p.cs.game.config.GetTileSize())
	x0, y0 := TileCenterFromTile(from.X, from.Y, ts)
	x1, y1 := TileCenterFromTile(to.X, to.Y, ts)
	return p.cs.game.collisionSystem.CheckLineOfSight(x0, y0, x1, y1)
}

// planPhaseShift picks the cheapest walk-to-launch + blink + walk-to-attack
// plan and keeps it only when the monster's own pursuit cannot reach an attack
// position or that walk is far longer. Both walks end on the same positions
// (phaseShiftWalk): a melee walker whose near ring is taken targets the queue
// ring, as its pursuit does; with neither free it waits where it is.
// wait is the rest of the cooldown in walking tiles (PhaseShiftWaitTiles): the
// blink goes once both the launch walk and the wait are over.
func (cs *CombatSystem) planPhaseShift(m *monster.Monster3D, wait int) monster.PhaseShiftPlan {
	g := cs.game
	tune := g.config.MonsterAI.PhaseShift
	ts := float64(g.config.GetTileSize())
	px, py := cs.logicalCameraXY()
	from := monster.TileCoord{X: TileIndex(m.X, ts), Y: TileIndex(m.Y, ts)}
	party := monster.TileCoord{X: TileIndex(px, ts), Y: TileIndex(py, ts)}
	plan := monster.PhaseShiftPlan{Planned: true, From: from, Party: party}
	r := m.Abilities.PhaseShift.RangeTiles
	if g.world == nil || g.collisionSystem == nil ||
		max(absInt(from.X-party.X), absInt(from.Y-party.Y)) > tune.WindowTiles ||
		!cs.phaseShiftEligible(m) || !phaseShiftGapNear(g.world, m, ts, tune.MaxWalkTiles+r) {
		return plan
	}
	goals, direct := cs.phaseShiftWalk(m, px, py)
	if len(goals) == 0 {
		return plan
	}
	limit := math.MaxInt
	if direct >= 0 {
		limit = (direct - 1) / tune.DetourFactor
	}
	if wait > limit {
		return plan // walking gets there before the cooldown does
	}
	p := cs.newPhasePlanner(m, party, from, party, tune.WindowTiles)
	p.s.toGoal = p.field(goals, p.s.toGoal, limit)
	toGoal := p.s.toGoal
	p.s.fromSelf = p.field([]monster.TileCoord{from}, p.s.fromSelf, min(tune.MaxWalkTiles, limit))
	fromSelf := p.s.fromSelf
	// Candidates rank by cost, launch walk, landing walk, then launch tile and
	// direction in row-major order.
	best := [7]int{math.MaxInt}
	for _, launch := range p.s.queue { // fromSelf's BFS order: walks never shrink
		walk := fromSelf[p.index(launch)]
		start := max(walk, wait)
		if start+1 > min(limit, best[0]) {
			break // every later launch costs at least as much
		}
		for _, dir := range blinkDirections {
			landing, dist, ok := p.blinkLanding(launch, dir, r)
			i := p.index(landing)
			if !ok || i < 0 || toGoal[i] < 0 {
				continue
			}
			cand := [7]int{start + dist + toGoal[i], walk, toGoal[i], launch.Y, launch.X, dir[1], dir[0]}
			if cand[0] <= limit && slices.Compare(cand[:], best[:]) < 0 {
				best, plan.Active, plan.Launch, plan.Landing, plan.Walk = cand, true, launch, landing, walk
			}
		}
	}
	return plan
}

// refreshPhaseShiftPlan keeps an RT plan current for the parallel pursuit
// step: an active plan drops the frame the monster stops being eligible or
// able to move; plans refresh when either tile changes, and at least every
// replan_frames. TB plans at each monster turn instead.
func (cs *CombatSystem) refreshPhaseShiftPlan(m *monster.Monster3D) {
	if !m.CanPhaseShift() {
		return // ordinary monsters never pay for Phase Shift
	}
	if cs.game.turnBasedMode || cs.game.monsterMovementHeld(m) {
		// TB plans at the monster's own turn; a stale RT plan must not steer it.
		m.PhasePlan = monster.PhaseShiftPlan{}
		return
	}
	ts := float64(cs.game.config.GetTileSize())
	px, py := cs.logicalCameraXY()
	p := &m.PhasePlan
	if p.Planned && p.Age < cs.game.config.MonsterAI.PhaseShift.ReplanFrames &&
		p.From == (monster.TileCoord{X: TileIndex(m.X, ts), Y: TileIndex(m.Y, ts)}) &&
		p.Party == (monster.TileCoord{X: TileIndex(px, ts), Y: TileIndex(py, ts)}) {
		if p.Active && !cs.phaseShiftEligible(m) {
			m.PhasePlan = monster.PhaseShiftPlan{}
			return
		}
		p.Age++
		return
	}
	m.PhasePlan = cs.planPhaseShift(m, m.PhaseShiftWaitTiles(false, 0, 0)) // planning checks eligibility
}

// tryPhaseShift blinks a monster that stands on its plan's launch tile center
// (where the blink was planned and is checked from) with the active mode's
// cooldown spent. The landing is checked again first, against where the party
// stands now.
func (cs *CombatSystem) tryPhaseShift(m *monster.Monster3D, turnBased bool) bool {
	plan := m.PhasePlan
	if !plan.Active || !m.PhaseShiftReady(turnBased) || !m.AtTileCenter(plan.Launch) ||
		cs.game.monsterMovementHeld(m) || !cs.phaseShiftEligible(m) {
		return false
	}
	ts := float64(cs.game.config.GetTileSize())
	px, py := cs.logicalCameraXY()
	party := monster.TileCoord{X: TileIndex(px, ts), Y: TileIndex(py, ts)}
	p := cs.newPhasePlanner(m, party, plan.Launch, plan.Landing, 1)
	dir := [2]int{cmp.Compare(plan.Landing.X, plan.Launch.X), cmp.Compare(plan.Landing.Y, plan.Launch.Y)}
	if landing, _, ok := p.blinkLanding(plan.Launch, dir, m.Abilities.PhaseShift.RangeTiles); !ok || landing != plan.Landing {
		m.PhasePlan = monster.PhaseShiftPlan{}
		return false
	}
	cs.executePhaseShift(m, plan.Landing)
	return true
}

// executePhaseShift moves the monster to the landing tile: a movement, so it
// claims no attack and keeps its attack cadence.
func (cs *CombatSystem) executePhaseShift(m *monster.Monster3D, landing monster.TileCoord) {
	g := cs.game
	oldX, oldY := m.X, m.Y
	x, y := TileCenterFromTile(landing.X, landing.Y, float64(g.config.GetTileSize()))
	g.relocateMonster(m, x, y)
	cs.settleBlink(m, oldX, oldY)
	m.ArmPhaseShiftCooldown(g.config.GetTPS())
	m.PhasePlan = monster.PhaseShiftPlan{}
	g.refreshMonsterCollisionState(m)
	g.logCombatLine(logToneBad, uitext.Text("combat.toward_the_party", logSlot, logSlot), logMonsterName(m), logAbility("phase-shifts"))
}

// tryPhaseShiftTB plans from the monster's tile at its turn and blinks when it
// stands on the launch tile; otherwise the plan steers this turn's walk.
func (gl *GameLoop) tryPhaseShiftTB(m *monster.Monster3D) bool {
	if !m.CanPhaseShift() {
		return false
	}
	m.PhasePlan = gl.game.combat.planPhaseShift(m, m.PhaseShiftWaitTiles(true, gl.game.monsterTurnPasses(m), gl.game.monsterTurnState.plannedPasses()))
	return gl.game.combat.tryPhaseShift(m, true)
}

// stepTowardPhaseLaunchTB walks an active plan's launch tile and holds there
// for the blink. False = no plan, no route or a refused step: ordinary TB
// movement runs.
func (gl *GameLoop) stepTowardPhaseLaunchTB(m *monster.Monster3D, tileSize float64) bool {
	plan := m.PhasePlan
	if !plan.Active || m.AIFoe != nil || m.Bound {
		return false
	}
	if TileIndex(m.X, tileSize) == plan.Launch.X && TileIndex(m.Y, tileSize) == plan.Launch.Y {
		return true
	}
	return gl.moveMonsterAlongTBGoals(m, []monster.TileCoord{plan.Launch}, tileSize)
}

// phaseShiftLandingNextTurn is where Phase Shift puts the monster on its
// coming TB turn, for the tactical overlay: the cooldown is spent by then, and
// the walk to its plan's launch tile fits the passes before the last one,
// which the blink takes.
func (g *MMGame) phaseShiftLandingNextTurn(m *monster.Monster3D) ([2]int, bool) {
	if !m.CanPhaseShift() || g.combatAuraTurnCooldown(m.PhaseShiftCD.Turns) > 0 {
		return [2]int{}, false
	}
	passes := g.monsterTurnPasses(m)
	if passes == 0 {
		return [2]int{}, false
	}
	if plan := g.combat.planPhaseShift(m, 0); plan.Active && plan.Walk <= m.PeekTurnBasedSteps(passes-1) {
		return [2]int{plan.Landing.X, plan.Landing.Y}, true
	}
	return [2]int{}, false
}
