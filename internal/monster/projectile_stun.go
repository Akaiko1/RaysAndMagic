package monster

import "ugataima/internal/config"

// StunEffect is the one attack-rider profile used by combat and descriptions.
type StunEffect struct {
	Chance         float64
	Seconds, Turns int
}

// An explicit monster stun overrides the projectile's default rider. This
// preserves authored boss mechanics without stacking two independent stuns.
func (d MonsterDefinition) ProjectileStun(spellID string) StunEffect {
	if d.Abilities.Stun.Chance > 0 {
		return StunEffect{d.Abilities.Stun.Chance, d.Abilities.Stun.Seconds, d.Abilities.Stun.Turns}
	}
	if def, ok := config.GetSpellDefinition(spellID); ok && def != nil {
		return StunEffect{def.StunChance, def.StunDurationSeconds, def.StunDurationTurns}
	}
	return StunEffect{}
}

func (m *Monster3D) ProjectileStun(spellID string) StunEffect {
	d := MonsterDefinition{}
	if m != nil {
		if m.IsChampion() {
			// Champion runtime fields describe the last weapon hand, not the spell.
			if MonsterConfig != nil {
				d = MonsterConfig.Monsters[m.Key]
			}
		} else {
			d.Abilities.Stun.Chance, d.Abilities.Stun.Seconds, d.Abilities.Stun.Turns = m.Abilities.Stun.Chance, m.Abilities.Stun.Seconds, m.Abilities.Stun.Turns
		}
	}
	return d.ProjectileStun(spellID)
}
