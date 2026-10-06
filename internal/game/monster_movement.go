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

// monsterCanBeDisplaced gates external pushes and stack separation alike.
// Canopy trajectories and scripted set pieces must retain their anchors.
func (g *MMGame) monsterCanBeDisplaced(m *monster.Monster3D) bool {
	return m != nil && m.Arbor.Phase == "" && !m.IsInertSetPiece() && !g.monsterMovementHeld(m)
}

// movementGrantsExtraMonsterPass is shared by the turn handoff and its preview.
func (g *MMGame) movementGrantsExtraMonsterPass() bool {
	return g.partyActionsUsed > g.partyTechniqueActionsUsed
}

func (g *MMGame) monsterCanEnterTileTB(m *monster.Monster3D, tx, ty int) bool {
	if g.monsterMovementHeld(m) || g.collisionSystem == nil {
		return false
	}
	_, x, y, hasTarget := g.monsterAttackTarget(m)
	ts := g.config.GetTileSize()
	if hasTarget && tx == TileIndex(x, ts) && ty == TileIndex(y, ts) {
		return false
	}
	return m.CanTraverseTile(g.collisionSystem, monster.TileCoord{X: tx, Y: ty})
}
