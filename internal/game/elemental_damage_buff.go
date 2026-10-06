package game

import damagecalc "ugataima/internal/damage"

// A stronger draught takes precedence while active. Different elemental
// schools coexist, and a weaker active draught resumes when a stronger expires.
func (g *MMGame) elementalDamageBuff(parts damagecalc.Parts, school string) damagecalc.Parts {
	if g == nil {
		return parts
	}
	target, err := damagecalc.ParseType(school)
	if err != nil || target == damagecalc.Physical {
		return parts
	}
	pct := 0
	for _, b := range g.combatBuffs {
		if b.Frames <= 0 || b.OutPercent <= pct {
			continue
		}
		if typed, err := damagecalc.ParseType(b.OutDamageType); err == nil && typed == target {
			pct = b.OutPercent
		}
	}
	parts.Normal += parts.Normal * pct / 100
	parts.True += parts.True * pct / 100
	return parts
}

func (cs *CombatSystem) elementalBuffPacket(packet monsterDamagePacket) monsterDamagePacket {
	for i := range packet.Components {
		c := &packet.Components[i]
		c.Parts = cs.game.elementalDamageBuff(c.Parts, c.School.String())
	}
	return packet
}
