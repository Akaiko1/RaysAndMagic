package game

import (
	"fmt"
	"testing"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

func TestElementalDefenseOrderAndVictimFX(t *testing.T) {
	cases := []struct {
		name                       string
		resist, soak, armor, dodge int
		physical, elemental, fx    int
	}{
		{"bare", 0, 0, 0, 0, 110, 220, 2},
		{"resist", 50, 0, 0, 0, 55, 110, 2},
		{"resist_soak", 50, 10, 0, 0, 45, 100, 2},
		{"immunity", 100, 0, 0, 0, 0, 0, 1},
		{"dodge_true", 50, 10, 0, 100, 5, 10, 1},
		// At the armor caps, 100 normal + 10 true becomes 25+10 physical
		// or 134+20 elemental; resistance halves each component before flat soak.
		{"armor_resist", 50, 0, 1000000, 0, 17, 77, 2},
		{"armor_resist_soak", 50, 10, 1000000, 0, 7, 67, 2},
	}
	for _, target := range []string{"party", "monster"} {
		for _, tc := range cases {
			for _, proc := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/proc=%t", target, tc.name, proc), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					elementalTestBiome(t, cs, "earth")
					cs.elementalAttackRoll = func() float64 {
						if proc {
							return 0
						}
						return 1
					}
					m := mkTestMonster("Attacker", 1000)
					m.DamageMin, m.DamageMax, m.TrueDamage = 100, 100, 10
					got := 0
					if target == "party" {
						cs.game.party.Members = cs.game.party.Members[:1]
						ch := cs.game.party.Members[0]
						isolateTrueDamageMember(ch, 0)
						ch.Equipment[items.SlotArmor] = items.Item{Attributes: map[string]int{"resist_earth": tc.resist, "resist_physical": tc.resist, "armor_class_base": tc.armor}}
						ch.Luck = tc.dodge * LuckToDodgeDivisor
						ch.HitPoints, ch.MaxHitPoints = 1000, 1000
						cs.game.combatBuffs = []TimedCombatBuff{{InReduce: tc.soak}}
						cs.applyMonsterMeleeDamage(m)
						got = 1000 - ch.HitPoints
					} else {
						foe := mkTestMonster("Victim", 1000)
						foe.Resistances[monster.DamageEarth], foe.Resistances[monster.DamagePhysical] = tc.resist, tc.resist
						foe.ArmorClass, foe.PerfectDodge = tc.armor, tc.dodge
						foe.SoakDamage, foe.SoakFrames = tc.soak, 100
						cs.monsterStrikeMonster(m, foe)
						got = 1000 - foe.HitPoints
					}
					want, wantFX := tc.physical, 0
					if proc {
						want, wantFX = tc.elemental, tc.fx
					}
					if got != want {
						t.Fatalf("damage=%d want %d", got, want)
					}
					if len(cs.game.elementalAttackEffects) != wantFX {
						t.Fatalf("FX=%d want %d", len(cs.game.elementalAttackEffects), wantFX)
					}
				})
			}
		}
	}
}
