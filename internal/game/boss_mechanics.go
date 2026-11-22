package game

import (
	"math/rand"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func (g *MMGame) bossMechanicValue(m *monster.Monster3D, stat string, fallback int) int {
	if g == nil || m == nil || m.SummonedBy != "" || m.Bound || config.GlobalBossMechanics == nil {
		return fallback
	}
	wm := world.GlobalWorldManager
	if wm == nil {
		return fallback
	}
	mc := wm.MapConfigs[m.HomeMap]
	if mc == nil || mc.Adventure == nil || mc.Adventure.Boss == nil || mc.Adventure.Boss.Monster != m.Key {
		return fallback
	}
	for _, r := range config.GlobalBossMechanics.Maps[m.HomeMap] {
		if r.Stat == stat {
			var controls map[string]bool
			if visit := g.adventure.Visits[m.HomeMap]; visit != nil {
				controls = visit.Controls
			}
			return r.Value(controls)
		}
	}
	return fallback
}

// Roll once when the outgoing hit is created. Projectiles and their splash keep
// that packet even when a switch is operated before impact. True damage stays
// true. crit reports the boss crit it rolled.
func (cs *CombatSystem) monsterOutgoingDamage(m *monster.Monster3D, parts damagecalc.Parts) (damagecalc.Parts, bool) {
	return cs.bossCriticalDamage(m, m.OutgoingDamage(parts))
}
func (cs *CombatSystem) bossCriticalDamage(m *monster.Monster3D, parts damagecalc.Parts) (damagecalc.Parts, bool) {
	if m.IsChampion() {
		return parts, false // a champion crits through its own weapon or spell roll
	}
	if chance := cs.game.bossMechanicValue(m, "critical_chance", 0); chance > 0 && rand.Intn(100) < chance {
		parts.Normal *= CritDamageMultiplier
		return parts, true
	}
	return parts, false
}
func (cs *CombatSystem) hitFromMonster(m *monster.Monster3D, damage int, school string, ignoreArmor bool, disintegrate float64, melee, spell bool) monsterCharacterHit {
	hit := hitFromMonster(m, damage, school, ignoreArmor, disintegrate, melee, spell)
	if m != nil {
		var crit bool
		hit.Parts, crit = cs.bossCriticalDamage(m, hit.Parts)
		hit.Critical = hit.Critical || crit
	}
	return hit
}
