package game

import "ugataima/internal/collision"

// attackLineClear authorizes delivery independently of sticky AI acquisition.
// Coordinates are logical positions, never pulled sprites or camera shake.
func (cs *CombatSystem) attackLineClear(fromX, fromY, toX, toY float64) bool {
	return cs.game.collisionSystem == nil || collision.AttackLineClear(cs.game.collisionSystem, fromX, fromY, toX, toY)
}

func (cs *CombatSystem) attackOriginBlocked(x, y float64) bool {
	if cs.game.collisionSystem != nil {
		return !cs.game.collisionSystem.CanAttackFrom(x, y)
	}
	w := cs.game.GetCurrentWorld()
	return w != nil && !w.CanProjectileMoveTo(x, y)
}
