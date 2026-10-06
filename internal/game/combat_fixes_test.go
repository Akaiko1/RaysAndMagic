package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// A flying projectile keeps its SHOOTER's mastery: selection moving to another
// hero before impact must not change whose skill tier applies.
func TestProjectileKeepsItsAuthor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	archer := g.party.Members[3] // Silvelyn: bow skill
	knight := g.party.Members[0]
	archer.Skills[character.SkillBow] = &character.Skill{Mastery: character.MasteryGrandMaster}
	delete(knight.Skills, character.SkillBow)

	g.selectedChar = 3
	cs.createArrowAttack(20, items.SlotMainHand, "")
	if len(g.arrows) == 0 {
		t.Fatal("no arrow spawned")
	}
	arrow := &g.arrows[len(g.arrows)-1]
	if arrow.Attacker != archer {
		t.Fatalf("arrow stamped with %v, want the archer", arrow.Attacker)
	}

	wantTrue := 3 * MasteryWeaponTrueDamagePerTier
	if arrow.TrueDamage != wantTrue || !arrow.IgnoresDodge {
		t.Fatalf("spawned arrow mastery = (%d,%v), want GM archer's (%d,true)",
			arrow.TrueDamage, arrow.IgnoresDodge, wantTrue)
	}

	// Selection auto-advances to the knight - and the tavern even swaps the
	// archer out - while the arrow flies. The projectile keeps both its author
	// and the attack properties resolved when it was fired.
	g.selectedChar = 0
	g.party.Members[3] = knight
	if arrow.Attacker != archer || arrow.TrueDamage != wantTrue || !arrow.IgnoresDodge {
		t.Errorf("in-flight arrow changed to author=%v mastery=(%d,%v), want archer and (%d,true)",
			arrow.Attacker, arrow.TrueDamage, arrow.IgnoresDodge, wantTrue)
	}
}

func TestDamageSchoolNormalizationUsesOneCanonicalKey(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.cardSlots[0].key = "golden_thief_bug_card"
	member := cs.game.party.Members[0]

	if got := normalizeDamageTypeStr(" FIRE "); got != "fire" {
		t.Fatalf("normalized school = %q, want fire", got)
	}
	want := cs.game.schoolResistPct(member, "fire")
	if want <= 0 {
		t.Fatal("golden_thief_bug_card grants no fire resistance; fixture cannot tell keys apart")
	}
	if got := cs.game.schoolResistPct(member, " FIRE "); got != want {
		t.Fatalf("card fire resistance through spaced/mixed-case key = %d, want canonical %d", got, want)
	}
	loaded := monsterPkg.MonsterConfig
	monsterPkg.MonsterConfig = nil
	defer func() { monsterPkg.MonsterConfig = loaded }()
	if got := convertToMonsterDamageType(" FIRE "); got != monsterPkg.DamageFire {
		t.Fatalf("config-independent monster damage type = %v, want fire", got)
	}
}

// An off-hand shield's armor_class_base counts toward total AC.
func TestShieldContributesAC(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0] // knight: shield skill from kit
	char.Equipment = map[items.EquipSlot]items.Item{}
	base := cs.CalculateTotalArmorClass(char)

	shield := items.CreateItemFromYAML("elven_shield") // armor_class_base 3, offhand
	if _, _, ok := char.EquipItem(shield); !ok {
		t.Fatal("failed to equip elven_shield")
	}
	got := cs.CalculateTotalArmorClass(char)
	if got <= base {
		t.Errorf("shield added no AC: %d -> %d", base, got)
	}
}
