package game

import (
	"math/rand"
	"ugataima/internal/monster"
)

func (g *MMGame) monsterMovementHeld(m *monster.Monster3D) bool {
	return m.MovementHeld(g.turnBasedMode) || (g.turnBasedMode && g.turnBasedMonsterStunned[m])
}

// Roll once per movement action, before trying alternative path destinations.
// Fleeing advances its clock even when this gate denies the step.
func (gl *GameLoop) monsterCanStepTB(m *monster.Monster3D) bool {
	if gl.game.monsterMovementHeld(m) {
		return false
	}
	pct := m.ActiveSlowPct()
	return pct <= 0 || rand.Intn(100) >= pct
}
