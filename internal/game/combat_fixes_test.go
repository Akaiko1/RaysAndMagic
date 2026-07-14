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

	// Selection auto-advances to the knight - and the tavern even swaps the
	// archer out - while the arrow flies: the POINTER still names the shooter.
	g.selectedChar = 0
	g.party.Members[3] = knight
	bowDef := lookupWeaponConfigByKey(arrow.BowKey)
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(arrow.Attacker, bowDef)
	if trueDmg != 3*MasteryWeaponTrueDamagePerTier || !ignoreDodge {
		t.Errorf("impact mastery = (%d,%v), want GM archer's (%d,true) - not the selected knight's",
			trueDmg, ignoreDodge, 3*MasteryWeaponTrueDamagePerTier)
	}
}

func TestDamageSchoolNormalizationUsesOneCanonicalKey(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cs.game.cardSlots[0].key = "golden_thief_bug_card"
	member := cs.game.party.Members[0]

	if got := normalizeDamageTypeStr(" FIRE "); got != "fire" {
		t.Fatalf("normalized school = %q, want fire", got)
	}
	if got := cs.game.schoolResistPct(member, " FIRE "); got != 100 {
		t.Fatalf("card fire resistance through spaced/mixed-case key = %d, want 100", got)
	}
	if got := convertToMonsterDamageType(" FIRE "); got != monsterPkg.DamageFire {
		t.Fatalf("normalized monster damage type = %v, want fire", got)
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

// Heroism/Hour of Power flat outgoing bonus applies to MELEE, not only projectiles.
func TestCombatBuffOutBonus_BoostsMelee(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	g := cs.game
	g.selectedChar = 0
	mon := monsterPkg.NewMonster3DFromConfig(200, 0, "goblin", g.config)
	mon.PerfectDodge = 0
	mon.ArmorClass = 0 // isolate the buff: % armor would scale the bonus down too
	g.world.Monsters = append(g.world.Monsters, mon)
	hpBefore := mon.HitPoints
	cs.ApplyDamageToMonster(mon, 10, "Iron Sword", false)
	plainDmg := hpBefore - mon.HitPoints

	g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 600, OutBonus: 7})
	hpBefore = mon.HitPoints
	cs.ApplyDamageToMonster(mon, 10, "Iron Sword", false)
	buffedDmg := hpBefore - mon.HitPoints

	// With no armor on the target, the full +7 outgoing bonus comes through.
	if buffedDmg-plainDmg != 7 {
		t.Errorf("melee under Heroism dealt +%d, want +7 (plain %d, buffed %d)", buffedDmg-plainDmg, plainDmg, buffedDmg)
	}
}
