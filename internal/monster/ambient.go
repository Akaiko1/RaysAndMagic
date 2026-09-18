package monster

import "ugataima/internal/config"

// IsAmbient separates persistent wildlife and travelers from authored hostile
// rosters. It does not replace creature type (beast, undead, dragon, etc.).
func (m *Monster3D) IsAmbient() bool { return m != nil && m.Disposition != "" }
func (m *Monster3D) Hunts(target *Monster3D) bool {
	if m == nil || target == nil || m.Disposition != "wildlife" || target.Disposition != "wildlife" {
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
	if m == nil || target == nil || m == target || !m.IsAlive() || !target.IsAlive() || m.IsInertSetPiece() || m.Pacified || m.BossEvasive {
		return false
	}
	if m.Bound {
		return !target.IsPartyControlled() && target.Disposition != "caravan" && !target.IsInertSetPiece()
	}
	if m.Disposition == "caravan" {
		return false
	}
	if m.Disposition == "wildlife" {
		return !m.AmbientFlee && m.Hunts(target) && !target.IsPartyControlled()
	}
	return target.IsPartyControlled() || target.Disposition == "caravan"
}

// UpdateAmbient uses the same terrain/path policy as hostile movement. TB
// chooses one grid step; RT follows that path at the authored movement speed.
func (m *Monster3D) UpdateAmbient(checker CollisionChecker, tx, ty float64, turn bool) {
	if m.MovementHeld(turn) || checker == nil {
		return
	}
	if turn && !m.SpendAmbientTurnMove() {
		return
	}
	m.IsEngagingPlayer = false
	m.WasAttacked = false
	if m.AmbientFlee {
		m.State = StateFleeing
		goal, ok := m.pickFleeTarget(checker, tx, ty)
		if !ok {
			return
		}
		tx, ty = float64(goal.X)*m.tileSize()+m.tileSize()/2, float64(goal.Y)*m.tileSize()+m.tileSize()/2
	} else if m.Disposition == "wildlife" {
		m.State = StatePatrolling
		if !turn {
			m.updatePatrolling(checker)
			return
		}
		goal, ok := m.pickPatrolTarget(checker)
		if !ok {
			return
		}
		tx, ty = float64(goal.X)*m.tileSize()+m.tileSize()/2, float64(goal.Y)*m.tileSize()+m.tileSize()/2
	} else {
		m.State = StatePatrolling
	}
	if turn {
		x, y, ok := m.NextPathStepTileToAny(checker, []TileCoord{{X: int(tx / m.tileSize()), Y: int(ty / m.tileSize())}}, nil)
		if ok {
			nx, ny := (float64(x)+.5)*m.tileSize(), (float64(y)+.5)*m.tileSize()
			if checker.CanMoveToWithTileOverrides(m.ID, nx, ny, m.WalkableTileOverrides, m.Flying) {
				m.X, m.Y = nx, ny
			}
		}
	} else {
		m.followPathToTile(checker, int(tx/m.tileSize()), int(ty/m.tileSize()))
	}
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
