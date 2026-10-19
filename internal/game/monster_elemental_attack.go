package game

import (
	"math/rand"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// monsterEffectContext resolves the attacking monster's own region: a region
// without an element grants no Elemental Attack, so none is described either.
// Catalog previews deliberately pass no school rather than guess a biome.
func (g *MMGame) monsterEffectContext(m *monster.Monster3D) monster.CombatEffectContext {
	if g == nil || g.config == nil || m == nil {
		return monster.CombatEffectContext{}
	}
	school := g.elementalDomainAt(m.X, m.Y)
	if school == "" {
		return monster.CombatEffectContext{}
	}
	return monster.CombatEffectContext{ElementalAttack: g.config.MonsterCombat.ElementalAttack, ElementalSchool: school}
}

// normalMonsterMeleeHitRolled is normalMonsterMeleeHit for the monster's own
// attack roll, its crit included.
func (cs *CombatSystem) normalMonsterMeleeHitRolled(m *monster.Monster3D) monsterCharacterHit {
	damage, crit := cs.monsterAttackDamage(m)
	hit := cs.normalMonsterMeleeHit(m, damage)
	hit.Critical = hit.Critical || crit
	return hit
}

// normalMonsterMeleeHit is the sole proc boundary for ordinary melee delivery
// against either faction. Specials and champion weapon packets bypass it.
func (cs *CombatSystem) normalMonsterMeleeHit(m *monster.Monster3D, damage int) monsterCharacterHit {
	hit := cs.hitFromMonster(m, damage, monster.DamagePhysical.String(), false, 0, true, false)
	ctx := cs.game.monsterEffectContext(m)
	profile := m.MeleeProfile(ctx.ElementalAttack, ctx.ElementalSchool)
	roll := cs.elementalAttackRoll
	if roll == nil {
		roll = rand.Float64
	}
	parts, school, elemental := profile.Resolve(hit.Parts, roll)
	if school != "" {
		hit.Parts, hit.DamageType = parts, school
	}
	hit.ElementalAttack = elemental
	if elemental {
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.uses", logSlot, logSlot, logSlot), logMonsterName(m), logAbility("Elemental Attack"), logSchoolWord(school, school))
		cs.game.addMonsterElementalAttackFX(m, school)
	}
	return hit
}

// MonsterCatalogEffectContext provides the same YAML settings without assuming
// that a catalog entry belongs to the map currently displayed by the editor.
func MonsterCatalogEffectContext(cfg *config.Config) monster.CombatEffectContext {
	if cfg == nil {
		return monster.CombatEffectContext{}
	}
	return monster.CombatEffectContext{ElementalAttack: cfg.MonsterCombat.ElementalAttack}
}
