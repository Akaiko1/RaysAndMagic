package game

import (
	"maps"
	"os"
	"slices"
	"sync"
	"testing"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"

	"gopkg.in/yaml.v3"
)

// Every attack on several members is one hit per member: each member rolls
// its own dodge. The attacker's riders land only on the members it struck,
// and only from an ability that authors riders: true (the shipped novas say
// false). The party attacks come from the
// catalog, so a new entry in monsters.yaml joins the table by itself.
func TestWholePartyAttacksRollEachMember(t *testing.T) {
	type row struct {
		name   string
		riders bool
		fire   func(cs *CombatSystem, m *monster.Monster3D)
	}
	rows := []row{
		{"telegraph", true, func(cs *CombatSystem, m *monster.Monster3D) {
			cs.game.strikeTelegraphParty(m, "Heat", damagecalc.Fire.String(), damagecalc.Parts{Normal: 100}, true)
		}},
		{"brood trap", true, func(cs *CombatSystem, m *monster.Monster3D) {
			armTestTrapVolley(m, 100)
			cs.game.detonateBossFireTrap(m)
		}},
	}
	for _, a := range catalogPartyAttacks(t) {
		rows = append(rows, row{a.Key, a.Riders.Carried(), func(cs *CombatSystem, m *monster.Monster3D) { castTestPartyAttack(cs, m, a, 100) }})
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			for i, c := range g.party.Members {
				isolateTrueDamageMember(c, 0)
				c.HitPoints, c.MaxHitPoints = 10000, 10000
				if i == 0 {
					c.Luck = 100 * LuckToDodgeDivisor // always dodges
				}
			}
			m := mkTestMonster("Scorcher", 1000)
			m.Abilities.Ignite.Chance, m.Abilities.Ignite.DurationSeconds = 1, 3
			r.fire(cs, m)
			struckAny := false
			for i, c := range g.party.Members {
				struck := c.HitPoints < c.MaxHitPoints
				burning := c.HasCondition(character.ConditionBurning)
				struckAny = struckAny || struck
				if burning != (struck && r.riders) || (i == 0 && struck) {
					t.Errorf("member %d: struck=%v burning=%v; the dodger is never struck and riders (%v) follow the hit", i, struck, burning, r.riders)
				}
			}
			if !struckAny {
				t.Error("the attack struck nobody")
			}
		})
	}
}

// Every ability authored as a share of the attack - a party attack, a
// telegraph's strike and field, the trap volley - takes it from the current
// attack, an enrage included; the calm monster is the positive control.
func TestAreaDamageFollowsTheEnragedAttack(t *testing.T) {
	for _, enraged := range []bool{false, true} {
		m := mkTestMonster("Brute", 1000)
		m.DamageMin, m.DamageMax = 100, 100
		m.Abilities.Enrage = monster.EnrageAbility{AtHP: 500, DamageMult: 1.5}
		m.HitPoints = 1000
		if enraged {
			m.HitPoints = 400
		}
		want := 100
		if enraged {
			want = 150
		}
		a := &monster.PartyAttack{DamagePercent: 100}
		for name, got := range map[string]int{"party attack": partyAttackDamage(m, a), "telegraph or volley share": attackShare(m, 100)} {
			if got != want {
				t.Errorf("enraged=%v %s: %d, want %d", enraged, name, got, want)
			}
		}
	}
}

// shippedPartyAttacks is one entry of each party attack key in the shipped
// catalog, in key order. It reads the file once and leaves the global
// catalog alone.
var shippedPartyAttacks = sync.OnceValue(func() []monster.PartyAttack {
	data, err := os.ReadFile("../../assets/monsters.yaml")
	if err != nil {
		panic(err)
	}
	var catalog monster.MonsterYAMLConfig
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		panic(err)
	}
	byKey := map[string]monster.PartyAttack{}
	for _, name := range slices.Sorted(maps.Keys(catalog.Monsters)) {
		for _, a := range catalog.Monsters[name].Abilities.PartyAttacks {
			if _, seen := byKey[a.Key]; !seen {
				byKey[a.Key] = a
			}
		}
	}
	var out []monster.PartyAttack
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[key])
	}
	return out
})

// catalogPartyAttacks is every shipped party attack kind; a new entry in
// monsters.yaml joins each table built on it.
func catalogPartyAttacks(t *testing.T) []monster.PartyAttack {
	t.Helper()
	if len(shippedPartyAttacks()) == 0 {
		t.Fatal("the catalog authors no party attacks")
	}
	return shippedPartyAttacks()
}

// catalogPartyAttack is the shipped entry of key.
func catalogPartyAttack(key string) monster.PartyAttack {
	for _, a := range shippedPartyAttacks() {
		if a.Key == key {
			return a
		}
	}
	panic("the catalog authors no party attack " + key)
}

// castTestPartyAttack makes m cast a as its only party attack, its damage
// fixed at damage.
func castTestPartyAttack(cs *CombatSystem, m *monster.Monster3D, a monster.PartyAttack, damage int) bool {
	return cs.castMonsterPartyAttack(m, armTestPartyAttack(m, a, damage))
}

// armTestPartyAttack gives m a as its only party attack, sure to roll, its
// damage fixed at damage (the full attack, pinned to damage).
func armTestPartyAttack(m *monster.Monster3D, a monster.PartyAttack, damage int) *monster.PartyAttack {
	a.Chance, a.DamagePercent = 1, 100
	m.DamageMin, m.DamageMax = damage, damage
	m.Abilities.PartyAttacks = []monster.PartyAttack{a}
	return &m.Abilities.PartyAttacks[0]
}

// armTestTrapVolley fixes m's brood-fire trap hit at damage (the full
// attack, pinned to damage).
func armTestTrapVolley(m *monster.Monster3D, damage int) {
	m.Abilities.TrapVolley.DamagePercent, m.Abilities.TrapVolley.Riders = 100, monster.CarryRiders(true)
	m.DamageMin, m.DamageMax = damage, damage
}

// setPartyAttackChance pins the roll of m's party attacks of key.
func setPartyAttackChance(m *monster.Monster3D, key string, chance float64) {
	for i := range m.Abilities.PartyAttacks {
		if m.Abilities.PartyAttacks[i].Key == key {
			m.Abilities.PartyAttacks[i].Chance = chance
		}
	}
}

// monsterPartyAttack is m's party attack of key, or nil.
func monsterPartyAttack(m *monster.Monster3D, key string) *monster.PartyAttack {
	for i := range m.Abilities.PartyAttacks {
		if m.Abilities.PartyAttacks[i].Key == key {
			return &m.Abilities.PartyAttacks[i]
		}
	}
	return nil
}
