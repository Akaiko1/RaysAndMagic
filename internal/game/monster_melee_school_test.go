package game

import (
	"testing"

	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// Melee blows land in the authored melee_damage_type school, so per-school
// party resists mitigate them; unauthored monsters stay physical.
func TestMonsterMeleeDamageType_UsesAuthoredSchool(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.party.Members = cs.game.party.Members[:1] // deterministic target pick
	member := cs.game.party.Members[0]
	isolateTrueDamageMember(member, 0)
	member.Equipment[items.SlotRing1] = items.Item{
		Attributes: map[string]int{"resist_earth": 100},
	}
	member.HitPoints, member.MaxHitPoints = 400, 400

	attacker := mkTestMonster("Earth Fist", 500)
	attacker.DamageMin, attacker.DamageMax = 50, 50
	attacker.MeleeDamageType = monsterPkg.DamageEarth.String()

	cs.applyMonsterMeleeDamage(attacker)
	if member.HitPoints != 400 {
		t.Fatalf("earth melee through 100%% earth resist dealt %d, want 0", 400-member.HitPoints)
	}

	attacker.MeleeDamageType = ""
	cs.applyMonsterMeleeDamage(attacker)
	if member.HitPoints != 350 {
		t.Fatalf("physical melee vs earth-only resist dealt %d, want 50", 400-member.HitPoints)
	}
}

// The YAML pass stays honest: every school handed out in monsters.yaml is a
// known school and survives the config load (validation would fail the load).
func TestMonstersYAML_MeleeSchoolsLoad(t *testing.T) {
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	want := map[string]string{
		"mummy":             "body",
		"gorilla_titan":     "body",
		"isis":              "spirit",
		"alien":             "spirit",
		"pixie":             "mind",
		"mountain_troll":    "earth",
		"grandfather_clock": "earth",
		"lich_king":         "dark",
		"ningyo":            "water",
	}
	for key, school := range want {
		def, ok := monsterPkg.MonsterConfig.Monsters[key]
		if !ok {
			t.Fatalf("monster %s missing", key)
		}
		if def.MeleeDamageType != school {
			t.Fatalf("%s melee_damage_type = %q, want %q", key, def.MeleeDamageType, school)
		}
	}
}
