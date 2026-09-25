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

func rollMonsterDisintegrate(target *monsterPkg.Monster3D, chance float64) bool {
	return target != nil && chance > 0 && !monsterImmuneToDisintegrate(target) && rand.Float64() < chance
}

// monsterMatchesBonusTarget is shared by weapon and card bonus_vs selectors.
// A selector matches once even when name, key and type describe the same family.
func monsterMatchesBonusTarget(target *monsterPkg.Monster3D, selector string) bool {
	return target != nil && selector != "" &&
		(strings.EqualFold(selector, target.Name) ||
			strings.EqualFold(selector, target.Key) ||
			strings.EqualFold(selector, target.MonsterType))
}
