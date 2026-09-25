package game

import (
	"fmt"
	"time"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func quietActorCombat(attacker, target *monster.Monster3D) bool {
	if target == nil {
		return false
	}
	if target.IsCaravan() {
		return true
	}
	return attacker != nil && attacker.IsWildlife() && target.IsWildlife() &&
		!attacker.IsPartyControlled() && !target.IsPartyControlled()
}

func (g *MMGame) addActorCombatMessage(attacker, target *monster.Monster3D, format string, args ...any) {
	if !quietActorCombat(attacker, target) {
		g.AddCombatMessage(fmt.Sprintf(format, args...))
	}
}

// Remote encounters notify the live campaign, sharing one wall-clock cooldown
// across attackers, maps, projectiles and RT/TB. Loading starts a fresh HUD
// throttle; it is presentation state, not a saved gameplay timer.
func (g *MMGame) notifyCaravanAttack(target *monster.Monster3D) {
	if target == nil || !target.IsCaravan() || !target.IsAlive() || config.GlobalEcology == nil {
		return
	}
	owner := g.rewardOwner()
	// Under attack, leave a scheduled stop instead of waiting out the rest.
	g.ecology.StopFrames = 0
	owner.ecology.StopFrames = 0
	now := time.Now()
	if now.Before(owner.caravanAttackAlertUntil) {
		return
	}
	owner.caravanAttackAlertUntil = now.Add(time.Duration(config.GlobalEcology.Caravan.AttackAlertCooldownSeconds) * time.Second)
	owner.AddColoredCombatMessage(uitext.Text("caravan.under_attack"), combatMessageRed)
}

// Loss is campaign state, independent of the attack notification throttle.
// Persisting RespawnDay deduplicates combat, ecology sweeps and save reloads.
func (g *MMGame) recordCaravanLoss(actorID string) {
	owner := g.rewardOwner()
	if actorID == "" || owner.ecology.ActorID != actorID || owner.ecology.RespawnDay != 0 {
		return
	}
	owner.ecology.RespawnDay = owner.currentCalendarDay() + 1
	g.ecology.RespawnDay = owner.ecology.RespawnDay
	owner.AddColoredCombatMessage(uitext.Text("caravan.destroyed"), combatMessageRed)
}
