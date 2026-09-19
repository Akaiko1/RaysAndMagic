package monster

import "ugataima/internal/config"

// AmbientThreat preserves a short memory across occlusion and save/load.
// Its position uses the same map coordinate space as the animal.
type AmbientThreat struct {
	X, Y    float64
	Seconds float64 `json:"seconds"`
	// A selected detour keeps its original clearance until the escape ends or
	// danger moves. Recomputing this from each step would allow inward drift.
	DetourClearance float64 `json:"detour_clearance,omitempty"`
}

func (s AmbientThreat) MapPosition(f func(float64, float64) (float64, float64)) AmbientThreat {
	if s.Seconds > 0 {
		s.X, s.Y = f(s.X, s.Y)
	}
	return s
}

// Awareness extends beyond the escape distance so safe roaming can continue
// without immediately forgetting nearby danger.
func (m *Monster3D) AmbientAwarenessRadius() float64 {
	return m.ambientSafeDistance() + m.tileSize()
}

func (m *Monster3D) ambientSafeDistance() float64 { return m.AlertRadius + m.tileSize() }

// Ground paths honor the selected clearance, including a bounded detour
// when an outward route is unavailable. Terrain-only start checks remain unrestricted.
// This policy is used by both A* and the final continuous/tile movement step.
type ambientThreatChecker struct {
	CollisionChecker
	x, y, clearance float64
}

func (c ambientThreatChecker) CanMoveTo(entityID string, x, y float64) bool {
	return distance(x, y, c.x, c.y)+1e-6 >= c.clearance && c.CollisionChecker.CanMoveTo(entityID, x, y)
}

func (c ambientThreatChecker) CanMoveToWithTileOverrides(entityID string, x, y float64, overrides []string, flying bool) bool {
	return distance(x, y, c.x, c.y)+1e-6 >= c.clearance && c.CollisionChecker.CanMoveToWithTileOverrides(entityID, x, y, overrides, flying)
}

func (m *Monster3D) RememberAmbientThreat(x, y float64) {
	if m.Threat.DetourClearance > 0 && (m.Threat.X != x || m.Threat.Y != y) {
		m.Threat.DetourClearance = 0
		m.ResetPathfinding()
	}
	m.Threat.X, m.Threat.Y, m.Threat.Seconds = x, y, 3
}

func (m *Monster3D) advanceAmbientThreat(turn bool) bool {
	if m.Threat.Seconds <= 0 {
		return false
	}
	tps := config.GetTargetTPS()
	if m.config != nil {
		tps = m.config.GetTPS()
	}
	dt := 1 / float64(max(1, tps))
	if turn {
		speed := m.Speed
		if config.GlobalEcology != nil {
			speed = config.GlobalEcology.TurnStepSpeed
		}
		dt = m.tileSize() / (60 * max(.01, speed))
	}
	m.Threat.Seconds -= dt
	if m.Threat.Seconds > 0 {
		return false
	}
	m.Threat = AmbientThreat{}
	m.AmbientFlee = false
	// Wildlife resumes roaming around its refuge, not a spawn beside danger.
	m.SpawnX, m.SpawnY = m.X, m.Y
	m.ResetPathfinding()
	m.State = StateIdle
	return true
}

// One route policy serves ground escapes and safe roaming in RT and TB.
// Arboreal trajectories finish before this method takes ownership.
func (m *Monster3D) updateAmbientThreatMovement(terrain CollisionChecker, tx, ty float64, turn bool) {
	d := distance(m.X, m.Y, tx, ty)
	escaping := d < m.ambientSafeDistance() || (m.Threat.DetourClearance > 0 && d < m.Threat.DetourClearance+m.tileSize())
	state := StatePatrolling
	clearance := min(d, m.ambientSafeDistance())
	if escaping {
		state = StateFleeing
		if m.Threat.DetourClearance > 0 {
			clearance = m.Threat.DetourClearance
		}
	} else {
		m.Threat.DetourClearance = 0
		// A canopy route can leave MoveTargetState at Patrolling. The actual
		// refuge must still be reachable, safe, and inside the roaming tether.
		if m.MoveTargetState != StatePatrolling || !m.IsWithinTetherRadius() || distance(m.SpawnX, m.SpawnY, tx, ty) < m.ambientSafeDistance() {
			m.SpawnX, m.SpawnY = m.X, m.Y
			m.ResetPathfinding()
		}
	}
	checker := ambientThreatChecker{CollisionChecker: terrain, x: tx, y: ty, clearance: clearance}
	goal := m.currentMoveTarget()
	gx, gy := m.tileToWorldCenter(goal.X, goal.Y)
	if !m.hasMoveTarget(state) || m.isAtTile(goal.X, goal.Y) || (escaping && distance(gx, gy, tx, ty) <= d) {
		var ok bool
		if !escaping {
			goal, ok = m.pickPatrolTarget(checker)
			ok = ok && !m.isAtTile(goal.X, goal.Y)
		}
		if !ok {
			// Prefer an outward escape even when an ordinary patrol cannot leave
			// a pocket at the safe-distance boundary.
			state = StateFleeing
			checker.clearance = d
			goal, ok = m.pickFleeTarget(checker, tx, ty)
			if !ok && d > m.tileSize() {
				clearance = m.Threat.DetourClearance
				if clearance == 0 {
					clearance = max(m.tileSize(), d-m.tileSize())
				}
				checker.clearance = clearance
				goal, ok = m.pickFleeTarget(checker, tx, ty)
				if ok {
					m.Threat.DetourClearance = clearance
				}
			}
		}
		if !ok {
			m.ResetPathfinding()
			m.State = StateIdle
			return
		}
		m.setMoveTarget(state, goal.X, goal.Y)
	}
	m.State = state
	if !m.stepAmbientPath(checker, goal, turn) {
		m.ResetPathfinding()
	}
}
