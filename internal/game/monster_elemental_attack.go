package game

import (
	"fmt"
	"math/rand"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// monsterEffectContext resolves the inspected/attacking monster's own region.
// Catalog previews deliberately pass no school rather than guess a biome.
func (g *MMGame) monsterEffectContext(m *monster.Monster3D) monster.CombatEffectContext {
	var ctx monster.CombatEffectContext
	if g == nil || g.config == nil {
		return ctx
	}
	ctx.ElementalAttack = g.config.MonsterCombat.ElementalAttack
	if m != nil && world.GlobalWorldManager != nil {
		ts := float64(g.config.GetTileSize())
		biome := g.biomeAtTile(TileIndex(m.X, ts), TileIndex(m.Y, ts))
		ctx.ElementalSchool = world.GlobalWorldManager.Biomes[biome].ElementalAttackSchool
	}
	return ctx
}

// normalMonsterMeleeHit is the sole proc boundary for ordinary melee delivery
// against either faction. Specials and champion weapon packets bypass it.
func (cs *CombatSystem) normalMonsterMeleeHit(m *monster.Monster3D, damage int) monsterCharacterHit {
	hit := hitFromMonster(m, damage, monster.DamagePhysical.String(), m.IgnoresArmor, 0, true, false)
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
		cs.game.AddCombatMessage(fmt.Sprintf("%s uses Elemental Attack (%s)!", m.Name, school))
		cs.game.addMonsterElementalAttackFX(m, school)
	}
	return hit
}

// MonsterCombatEffectLines feeds game inspection and is also available to
// previews that already have a staged monster and map context.
func (g *MMGame) MonsterCombatEffectLines(m *monster.Monster3D) []monster.EffectLine {
	if m == nil || monster.MonsterConfig == nil {
		return nil
	}
	d, err := monster.MonsterConfig.GetMonsterByKey(m.Key)
	if err != nil {
		return nil
	}
	d.ProjectileSpell, d.ProjectileWeapon = m.ProjectileSpell, m.ProjectileWeapon
	return d.CombatEffectLines(g.monsterEffectContext(m))
}

// MonsterCatalogEffectContext provides the same YAML settings without assuming
// that a catalog entry belongs to the map currently displayed by the editor.
func MonsterCatalogEffectContext(cfg *config.Config) monster.CombatEffectContext {
	if cfg == nil {
		return monster.CombatEffectContext{}
	}
	return monster.CombatEffectContext{ElementalAttack: cfg.MonsterCombat.ElementalAttack}
}
