package game

import (
	"ugataima/internal/monster"
)

func (g *MMGame) monsterMovementHeld(m *monster.Monster3D) bool {
	return m.MovementHeld(g.turnBasedMode) || (g.turnBasedMode && g.turnBasedMonsterStunned[m])
}

// monsterCanStepTB gates one movement action, before trying alternative path
// destinations. Slow does not roll here: it skips whole turns instead.
// Fleeing advances its clock even when this gate denies the step.
func (gl *GameLoop) monsterCanStepTB(m *monster.Monster3D) bool {
	return !gl.game.monsterMovementHeld(m)
}
