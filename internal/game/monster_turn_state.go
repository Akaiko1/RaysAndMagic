package game

import "ugataima/internal/monster"

// monsterTurnState is the persisted TB scheduler state. The embedded field
// names preserve the save adapter and existing fixtures, while all normal
// pass transitions go through beginPass/finishPass/resetPasses.
type monsterTurnState struct {
	monsterTurnResolved         bool
	turnBasedExtraMonsterAction bool
	turnBasedMonsterPassesLeft  int
	turnBasedMonsterPassDelay   int
	turnBasedMonsterStatusTick  bool
	turnBasedMonsterStunned     map[*monster.Monster3D]bool
}

func (s *monsterTurnState) startPasses() {
	s.turnBasedMonsterPassesLeft = 1
	if s.turnBasedExtraMonsterAction {
		s.turnBasedMonsterPassesLeft = 2
	}
	s.turnBasedExtraMonsterAction = false
	s.turnBasedMonsterStatusTick = false
	s.turnBasedMonsterStunned = make(map[*monster.Monster3D]bool)
}

// beginPass advances the frame delay independently of turn-status cadence.
// A restored second pass must not tick poison/stun a second time.
func (s *monsterTurnState) beginPass() (ready, tickStatuses bool) {
	if s.turnBasedMonsterPassDelay > 0 {
		s.turnBasedMonsterPassDelay--
		return false, false
	}
	tickStatuses = !s.turnBasedMonsterStatusTick
	s.turnBasedMonsterStatusTick = true
	return true, tickStatuses
}

func (s *monsterTurnState) finishPass(delay int) bool {
	s.turnBasedMonsterPassesLeft--
	if s.turnBasedMonsterPassesLeft > 0 {
		s.turnBasedMonsterPassDelay = max(1, delay)
		return false
	}
	s.resetPasses()
	s.monsterTurnResolved = true
	return true
}

func (s *monsterTurnState) resetPasses() {
	s.turnBasedMonsterPassesLeft = 0
	s.turnBasedMonsterPassDelay = 0
	s.turnBasedMonsterStatusTick = false
	s.turnBasedMonsterStunned = nil
}
