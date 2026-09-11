package game

// attackLineClear authorizes delivery independently of sticky AI acquisition.
// Coordinates are logical positions, never pulled sprites or camera shake.
func (cs *CombatSystem) attackLineClear(fromX, fromY, toX, toY float64) bool {
	return cs.game.collisionSystem == nil || cs.game.collisionSystem.CheckLineOfSight(fromX, fromY, toX, toY)
}
