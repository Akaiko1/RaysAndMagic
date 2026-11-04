package game

import (
	"slices"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
)

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
		if !b.Clock.Active() || b.OutPercent <= pct {
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

// elementalBuffGain is elementalBuffPacket on a copy of packet, with what the
// buffs added by school as the hit lands; cards name that gain. normal is what
// a component's normal part becomes after armor (melee cards), nil for as is.
func (cs *CombatSystem) elementalBuffGain(packet monsterDamagePacket, normal func(int) int) (monsterDamagePacket, []character.DamageShare) {
	if normal == nil {
		normal = func(n int) int { return n }
	}
	raw := packet.Components
	packet.Components = slices.Clone(raw)
	packet = cs.elementalBuffPacket(packet)
	gains := make([]character.DamageShare, len(raw))
	for i, c := range packet.Components {
		gain := normal(c.Parts.Normal) - normal(raw[i].Parts.Normal) + c.Parts.True - raw[i].Parts.True
		gains[i] = character.DamageShare{School: c.School.String(), Amount: gain}
	}
	return packet, character.MergeDamageShares(gains...)
}

func (cs *CombatSystem) elementalBuffPacket(packet monsterDamagePacket) monsterDamagePacket {
	for i := range packet.Components {
		c := &packet.Components[i]
		c.Parts = cs.game.elementalDamageBuff(c.Parts, c.School.String())
	}
	return packet
}
