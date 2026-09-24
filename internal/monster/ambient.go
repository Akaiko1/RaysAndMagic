package monster

import "ugataima/internal/config"

const (
	DispositionFish     = "fish"
	DispositionCaravan  = "caravan"
	DispositionWildlife = "wildlife"
)

func (m *Monster3D) IsCaravan() bool  { return m != nil && m.Disposition == DispositionCaravan }
func (m *Monster3D) IsWildlife() bool { return m != nil && m.Disposition == DispositionWildlife }

// IsFish identifies transient ecology fish independently of their species.
func (m *Monster3D) IsFish() bool { return m != nil && m.Disposition == DispositionFish }

// IsAmbient separates persistent wildlife and travelers from authored hostile
// rosters. It does not replace creature type (beast, undead, dragon, etc.).
func (m *Monster3D) IsAmbient() bool { return m != nil && m.Disposition != "" }
func (m *Monster3D) Hunts(target *Monster3D) bool {
	if m == nil || target == nil || !m.IsWildlife() || !target.IsWildlife() {
		return false
	}
	for _, key := range m.Prey {
		if key == target.Key {
			return true
		}
	}
	return false
}

// CanAttackActor is the shared relationship check used at selection AND commit.
func (m *Monster3D) CanAttackActor(target *Monster3D) bool {
	if m == nil || target == nil || m == target || m.IsFish() || target.IsFish() || !m.IsAlive() || !target.IsAlive() || m.IsInertSetPiece() || m.Pacified || m.BossEvasive {
		return false
	}
	if m.Bound {
		return !target.IsPartyControlled() && !target.IsCaravan() && !target.IsInertSetPiece()
	}
	if m.IsCaravan() {
		return false
	}
	if m.IsWildlife() {
		return !m.AmbientFlee && m.Hunts(target) && !target.IsPartyControlled()
	}
	return target.IsPartyControlled() || target.IsCaravan()
}

// UpdateAmbient uses the same terrain/path policy as hostile movement. TB
// chooses one grid step; RT follows that path at the authored movement speed.
func (m *Monster3D) UpdateAmbient(checker CollisionChecker, tx, ty float64, turn bool) {
	if m.AmbientFlee && m.Arbor.Phase == "" {
		x, y := m.X, m.Y
		defer func() {
			if m.X == x && m.Y == y && m.Arbor.Phase == "" {
				m.State = StateIdle
			}
		}()
	}
	if m.MovementHeld(turn) || checker == nil {
		return
	}
	if m.advanceAmbientThreat(turn) {
		return
	}
	if turn && !m.SpendAmbientTurnMove() {
		return
	}
	m.IsEngagingPlayer = false
	m.WasAttacked = false
	if m.updateArboreal(checker, tx, ty, turn) {
		return
	}
	if m.AmbientFlee {
		m.updateAmbientThreatMovement(checker, tx, ty, turn)
		return
	}
	if m.IsWildlife() {
		m.State = StatePatrolling
		if !turn {
			m.updatePatrolling(checker)
			return
		}
		goal, ok := m.pickPatrolTarget(checker)
		if !ok {
			return
		}
		m.setMoveTarget(StatePatrolling, goal.X, goal.Y)
		tx, ty = float64(goal.X)*m.tileSize()+m.tileSize()/2, float64(goal.Y)*m.tileSize()+m.tileSize()/2
	} else {
		m.State = StatePatrolling
	}
	m.stepAmbientPath(checker, TileCoord{X: int(tx / m.tileSize()), Y: int(ty / m.tileSize())}, turn)
}

func (m *Monster3D) stepAmbientPath(checker CollisionChecker, goal TileCoord, turn bool) bool {
	if !turn {
		return m.followPathToTile(checker, goal.X, goal.Y)
	}
	x, y, ok := m.NextPathStepTileToAny(checker, []TileCoord{goal}, nil)
	if !ok {
		return false
	}
	nx, ny := m.tileToWorldCenter(x, y)
	if !checker.CanMoveToWithTileOverrides(m.ID, nx, ny, m.WalkableTileOverrides, m.Flying) {
		return false
	}
	m.X, m.Y = nx, ny
	return true
}

// SpendAmbientTurnMove preserves relative walking speeds in tile-based combat.
func (m *Monster3D) SpendAmbientTurnMove() bool {
	if !m.IsAmbient() || config.GlobalEcology == nil {
		return true
	}
	m.AmbientMoveCredit += m.EffectiveSpeed() / config.GlobalEcology.TurnStepSpeed
	if m.AmbientMoveCredit < 1 {
		return false
	}
	m.AmbientMoveCredit -= 1
	if m.AmbientMoveCredit > 1 {
		m.AmbientMoveCredit = 1
	}
	return true
}
