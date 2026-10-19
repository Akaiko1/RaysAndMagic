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

// instakillOutcome is what an instakill that landed does to its target:
// disintegrate, Destroy Undead, the Maw.
type instakillOutcome uint8

const (
	instakillNone instakillOutcome = iota
	instakillKill
	// instakillSear: a boss is never erased; it takes instakillSearDamage
	// true damage instead.
	instakillSear
)

// instakillOn is the one rule for an instakill that landed on target.
func instakillOn(target *monsterPkg.Monster3D) instakillOutcome {
	if target.IsBoss() {
		return instakillSear
	}
	return instakillKill
}

func rollMonsterDisintegrate(target *monsterPkg.Monster3D, chance float64) instakillOutcome {
	if target == nil || chance <= 0 || monsterImmuneToDisintegrate(target) || rand.Float64() >= chance {
		return instakillNone
	}
	return instakillOn(target)
}

// instakillSearDamage is the true damage a boss takes in place of an
// instakill: config.InstakillSearMaxHPPct of its max HP.
func instakillSearDamage(target *monsterPkg.Monster3D) int {
	return max(1, target.MaxHitPoints*config.InstakillSearMaxHPPct/100)
}

// withInstakillSear adds the sear's true damage to both packets of one
// victim's attack, in the attack's own school. Packets are cloned: the
// source attack is shared with every AoE victim.
func withInstakillSear(attack partyMonsterAttack, boss *monsterPkg.Monster3D) partyMonsterAttack {
	sear := instakillSearDamage(boss)
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
