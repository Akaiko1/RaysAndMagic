package game

import (
	"math/rand"
	"strings"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
)

// partyDisintegrateChance combines the attack's authored chance with the card
// collection once. Projectiles snapshot this at launch; melee resolves it on hit.
func (g *MMGame) partyDisintegrateChance(base float64) float64 {
	return min(1, max(0, base+float64(g.cardDisintegratePct())/100))
}

func (g *MMGame) weaponDisintegrateChance(def *config.WeaponDefinitionConfig) float64 {
	base := 0.0
	if def != nil {
		base = def.DisintegrateChance
	}
	return g.partyDisintegrateChance(base)
}

// disintegrateOutcome is what a disintegrate roll does to its target.
type disintegrateOutcome uint8

const (
	disintegrateNone disintegrateOutcome = iota
	disintegrateKill
	// disintegrateSear: a boss is never erased; the proc adds
	// disintegrateSearDamage true damage to the hit.
	disintegrateSear
)

func rollMonsterDisintegrate(target *monsterPkg.Monster3D, chance float64) disintegrateOutcome {
	if target == nil || chance <= 0 || monsterImmuneToDisintegrate(target) || rand.Float64() >= chance {
		return disintegrateNone
	}
	if target.IsBoss() {
		return disintegrateSear
	}
	return disintegrateKill
}

// disintegrateSearDamage is the true damage a boss takes in place of the
// disintegrate kill: config.DisintegrateBossMaxHPPct of its max HP.
func disintegrateSearDamage(boss *monsterPkg.Monster3D) int {
	return max(1, boss.MaxHitPoints*config.DisintegrateBossMaxHPPct/100)
}

// withDisintegrateSear adds the boss proc's true damage to both packets of
// one victim's attack, in the attack's own school. Packets are cloned: the
// source attack is shared with every AoE victim.
func withDisintegrateSear(attack partyMonsterAttack, boss *monsterPkg.Monster3D) partyMonsterAttack {
	sear := disintegrateSearDamage(boss)
	attack.Packet = attack.Packet.withTrue(sear)
	attack.CriticalPacket = attack.CriticalPacket.withTrue(sear)
	return attack
}

// monsterMatchesBonusTarget is shared by weapon and card bonus_vs selectors.
// A selector matches once even when name, key and type describe the same family.
func monsterMatchesBonusTarget(target *monsterPkg.Monster3D, selector string) bool {
	return target != nil && selector != "" &&
		(strings.EqualFold(selector, target.Name) ||
			strings.EqualFold(selector, target.Key) ||
			strings.EqualFold(selector, target.MonsterType))
}
