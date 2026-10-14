package monster

import (
	"math"

	"ugataima/internal/config"
	"ugataima/internal/status"
)

// Phase Shift (abilities.phase_shift), monster side: the plan the serial
// planner hands down (internal/game/monster_phase_shift.go), the cooldown
// clocks, and the real-time walk to the launch tile, which takes over the
// pursuit while a plan is active. Ordinary movement only calls
// followPhaseShiftPlan.
//
// Planning is expensive (the pursuit's own search plus two flood fills per
// replan, several pursuit repaths' worth), so author phase_shift for monsters
// of small dungeon maps (the Solstice chambers), not open-world packs.

// MaxPhaseShiftRangeTiles caps an authored Phase Shift reach.
const MaxPhaseShiftRangeTiles = 8

// PhaseShiftPlan is the serial planner's answer for a Phase Shift walker: walk
// to Launch, then blink to Landing. Active is false while walking is fine.
type PhaseShiftPlan struct {
	Planned, Active bool
	Launch, Landing TileCoord
	Walk            int // tiles from the planning tile to Launch
	// The tiles the plan was made for; a change forces a fresh plan.
	From, Party TileCoord
	Age         int
}

// CanPhaseShift reports an authored Phase Shift on a walker.
func (m *Monster3D) CanPhaseShift() bool {
	return m != nil && m.Abilities.PhaseShift.RangeTiles > 0 && !m.Flying
}

// PhaseShiftReady reports whether the active mode's cooldown has run out.
func (m *Monster3D) PhaseShiftReady(turnBased bool) bool {
	if turnBased {
		return m.PhaseShiftCDTurns <= 0
	}
	return m.PhaseShiftCDFrames <= 0
}

// PhaseShiftWaitTiles is the rest of the cooldown as walking: the tiles the
// monster covers at its pursuit pace (RT) or its turn steps (TB) meanwhile.
// The TB cooldown ticks once a round: it walks the passes left in this round,
// then perRound passes in each later round before the blink is ready.
func (m *Monster3D) PhaseShiftWaitTiles(turnBased bool, passesLeft, perRound int) int {
	if turnBased {
		if m.PhaseShiftCDTurns <= 0 {
			return 0
		}
		return m.PeekTurnBasedSteps(max(0, passesLeft) + (m.PhaseShiftCDTurns-1)*max(1, perRound))
	}
	if m.PhaseShiftCDFrames <= 0 {
		return 0
	}
	return int(math.Ceil(float64(m.PhaseShiftCDFrames) * m.movementSpeed(StatePursuing) / m.tileSize()))
}

// ArmPhaseShiftCooldown starts both mode clocks from the authored whole rounds.
func (m *Monster3D) ArmPhaseShiftCooldown(tps int) {
	if m == nil {
		return
	}
	if tps <= 0 {
		tps = config.GetTargetTPS()
	}
	seconds := m.Abilities.PhaseShift.CooldownSeconds
	frames := max(1, int(math.Round(float64(seconds*tps)*m.SlowCadenceFactor())))
	status.RefreshDualRated(&m.PhaseShiftCDFrames, &m.PhaseShiftCDTurns, &m.PhaseShiftCDRate, frames, config.CombatRounds(seconds))
}

func (m *Monster3D) TickPhaseShiftCooldownFrame() {
	if m == nil {
		return
	}
	status.TickFrameRated(&m.PhaseShiftCDFrames, &m.PhaseShiftCDTurns, &m.PhaseShiftCDRate)
}

func (m *Monster3D) TickPhaseShiftCooldownTurn() {
	if m == nil {
		return
	}
	status.TickTurnRated(&m.PhaseShiftCDTurns, &m.PhaseShiftCDFrames, &m.PhaseShiftCDRate)
}

// followPhaseShiftPlan walks an active plan's launch tile and holds on its
// center for the blink, which is planned and checked from there. A launch this
// walk cannot reach drops the plan until the next one: pursuing and launching
// share one path cache, so trying both every tick would search twice a tick.
// False = no plan: the pursuit runs.
func (m *Monster3D) followPhaseShiftPlan(collisionChecker CollisionChecker, playerX, playerY float64) bool {
	p := m.PhasePlan
	if !p.Active {
		return false
	}
	if m.isAtTile(p.Launch.X, p.Launch.Y) {
		m.stepToTileCenter(collisionChecker, p.Launch)
		return true
	}
	if m.followPathToLaunch(collisionChecker, p.Launch, playerX, playerY) {
		return true
	}
	m.PhasePlan.Active = false
	return false
}

// AtTileCenter: the monster stands on t's center, where a blink leaves from.
func (m *Monster3D) AtTileCenter(t TileCoord) bool {
	cx, cy := m.tileToWorldCenter(t.X, t.Y)
	return math.Hypot(m.X-cx, m.Y-cy) <= 0.5
}

// stepToTileCenter moves at pursuit pace toward the center of the tile the
// monster already stands in (stepToward refuses its own target tile).
func (m *Monster3D) stepToTileCenter(collisionChecker CollisionChecker, t TileCoord) {
	cx, cy := m.tileToWorldCenter(t.X, t.Y)
	dx, dy := cx-m.X, cy-m.Y
	d := math.Hypot(dx, dy)
	if d <= 0.5 {
		m.X, m.Y = cx, cy
		return
	}
	step := min(m.speedPerTick(), d)
	nx, ny := m.X+dx/d*step, m.Y+dy/d*step
	if collisionChecker.CanMoveToWithTileOverrides(m.ID, nx, ny, m.WalkableTileOverrides, m.Flying) {
		m.X, m.Y = nx, ny
	}
}

// followPathToLaunch walks to a Phase Shift launch tile on the pursuit's terms
// (followPathToTarget): a new launch repaths at once, the route never enters
// the target's tile, and a failed search retries at pathCheckFrequency.
func (m *Monster3D) followPathToLaunch(collisionChecker CollisionChecker, launch TileCoord, targetX, targetY float64) bool {
	if collisionChecker == nil {
		return false
	}
	blocked := m.blockedAttackTargetTile(targetX, targetY)
	targetChanged := m.PathTargetTileX != launch.X || m.PathTargetTileY != launch.Y
	shouldRepath := targetChanged || len(m.PathTiles) == 0 || m.PathIndex >= len(m.PathTiles)
	if shouldRepath && !targetChanged && len(m.PathTiles) == 0 && m.LastPathCalcTick > 0 && !m.canRepath(m.pathCheckFrequency()) {
		return false
	}
	return m.followPathStep(collisionChecker, launch.X, launch.Y, shouldRepath,
		func() []TileCoord { return m.pathToAny(collisionChecker, []TileCoord{launch}, blocked) },
		m.speedPerTick(), false, true, blocked)
}
