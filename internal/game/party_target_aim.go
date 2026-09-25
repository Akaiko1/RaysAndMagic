package game

import (
	"math"
	"ugataima/internal/monster"
)

// beginPartyTargetAim scopes explicit aim to one synchronous action. Camera
// presentation remains independent, including turns or arrivals during a cast.
// Projectiles copy this aim at launch; no target state is persisted.
func (cs *CombatSystem) beginPartyTargetAim(target *monster.Monster3D) func() {
	previous, angle := cs.partyAimTarget, cs.partyAimAngle
	cs.partyAimTarget = target
	cs.partyAimAngle = math.Atan2(target.Y-cs.game.camera.Y, target.X-cs.game.camera.X)
	return func() {
		cs.partyAimTarget, cs.partyAimAngle = previous, angle
	}
}

func (cs *CombatSystem) partyAttackAngle() float64 {
	if cs.partyAimTarget != nil {
		return cs.partyAimAngle
	}
	return cs.game.camera.Angle
}
