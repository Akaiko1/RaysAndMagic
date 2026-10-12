package monster

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
func (m *Monster3D) IsAmbient() bool { return m != nil && isAmbientDisposition(m.Disposition) }

// IsAmbient reports a definition whose monsters are ambient (any disposition).
func (d MonsterDefinition) IsAmbient() bool { return isAmbientDisposition(d.Disposition) }

func isAmbientDisposition(disposition string) bool { return disposition != "" }
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
	if m.BerserkFramesRemaining > 0 {
		return !target.IsInertSetPiece()
	}
	if m.IsCaravan() {
		return false
	}
	if m.IsWildlife() {
		return !m.AmbientFlee && m.Hunts(target) && !target.IsPartyControlled()
	}
	return target.IsPartyControlled() || target.IsCaravan()
}

// UpdateAmbient uses the same terrain/path policy as hostile movement. RT
// follows that path at the authored movement speed. TB spends one movement
// action at the monster's turn-based step rate (WalkTurn); between runs before
// each extra step (nil = nothing to check).
func (m *Monster3D) UpdateAmbient(checker CollisionChecker, tx, ty float64, turn bool, between func() bool) {
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
	if !turn {
		m.IsEngagingPlayer = false
		m.WasAttacked = false
		m.ambientStep(checker, tx, ty, false, nil)
		return
	}
	// A patrol picks its goal once per action, so extra steps keep heading the
	// same way instead of re-rolling a random destination every tile.
	var patrol *TileCoord
	m.WalkTurn(func(first bool) bool {
		if first {
			m.IsEngagingPlayer = false
			m.WasAttacked = false
		}
		x, y, arbor := m.X, m.Y, m.Arbor
		m.ambientStep(checker, tx, ty, true, &patrol)
		return m.X != x || m.Y != y || m.Arbor != arbor
	}, between)
}

// ambientStep is one ambient move: a canopy phase, a flight from the threat,
// a patrol leg or a caravan leg. In TB patrol holds the action's goal.
func (m *Monster3D) ambientStep(checker CollisionChecker, tx, ty float64, turn bool, patrol **TileCoord) {
	if m.updateArboreal(checker, tx, ty, turn) {
		return
	}
	if m.AmbientFlee {
		m.updateAmbientThreatMovement(checker, tx, ty, turn)
		return
	}
	m.State = StatePatrolling
	if m.IsWildlife() {
		if !turn {
			m.updatePatrolling(checker)
			return
		}
		if *patrol == nil {
			goal, ok := m.pickPatrolTarget(checker)
			if !ok {
				return
			}
			m.setMoveTarget(StatePatrolling, goal.X, goal.Y)
			*patrol = &goal
		}
		tx, ty = m.tileToWorldCenter((*patrol).X, (*patrol).Y)
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
