package game

import (
	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func (cs *CombatSystem) countCardSummons() int {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return 0
	}
	n := 0
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() && isCardAlly(m) {
			n++
		}
	}
	return n
}

// markCardAlly turns a spawned monster into a party ally summoned by the card
// collection: Bound (hunts enemy monsters, ignores the party), tagged for the
// summon limit, and excluded from map-clear quest counts.
// BoundFramesRemaining 0 = never expires (the bind tick only counts down > 0).
// A card ally is a PURE summon (never was an enemy): it yields the party no
// XP/gold/loot on death and does not follow across maps - it simply crumbles
// when the party leaves (a fresh set is re-summoned there via the proc).
func markCardAlly(m *monsterPkg.Monster3D) {
	markPurePartySummon(m, cardSummonOwner)
}

// mitigateCharacterDamage is the int-shaped shorthand for a hit with no true
// component: same pipeline, same armor step, one number in and out. Balance
// tests read better through it; there is no second formula here.
func (cs *CombatSystem) mitigateCharacterDamage(damage int, damageTypeStr string, char *character.MMCharacter, ignoreArmor bool) int {
	return cs.mitigateCharacterDamageParts(
		damagecalc.Parts{Normal: damage}, damageTypeStr, char, ignoreArmor,
	).Normal
}

// tankTarget exposes the shared tank-index rule to targeting tests.
func (cs *CombatSystem) tankTarget() *character.MMCharacter {
	if i := cs.tankIndex(); i >= 0 {
		return cs.game.party.Members[i]
	}
	return nil
}

// tryCastAoeStun handles AoE-stun effect spells (e.g. Darkness): if the spell
// has StunRadiusTiles > 0, every alive monster within that radius of the caster
// is stunned (RT frames + TB turns), no damage dealt. Tests without a caster
// use this shorthand over the production caster-aware path.
func (cs *CombatSystem) tryCastAoeStun(spellID spells.SpellID, def spells.SpellDefinition) bool {
	return cs.tryCastAoeStunBy(spellID, def, nil)
}

func (gl *GameLoop) monsterAttackFoeTurnBased(attacker, foe *monsterPkg.Monster3D) {
	if foe != nil {
		gl.game.combat.commitMonsterAttack(attacker, monsterAttackDestination{foe: foe}, monsterAttackTurn)
	}
}
