package game

import (
	"testing"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func isolateTrueDamageMember(member *character.MMCharacter, resist int) {
	member.Equipment = map[items.EquipSlot]items.Item{}
	member.Skills = map[character.SkillType]*character.Skill{}
	member.Luck = 0
	if resist != 0 {
		member.Equipment[items.SlotRing1] = items.Item{
			Attributes: map[string]int{"resist_physical": resist},
		}
	}
}

func TestPartyMitigation_ResistanceAppliesToBothFlatOnlyToNormal(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	isolateTrueDamageMember(member, 50)
	cs.game.combatBuffs = []TimedCombatBuff{{InReduce: 10}}

	got := cs.mitigateCharacterDamageParts(
		damagecalc.Parts{Normal: 100, True: 40},
		monsterPkg.DamagePhysical.String(),
		member,
		false,
	)
	want := damagecalc.Parts{Normal: 40, True: 20}
	if got != want {
		t.Fatalf("mitigated parts = %+v, want %+v", got, want)
	}

	isolateTrueDamageMember(member, 100)
	got = cs.mitigateCharacterDamageParts(
		damagecalc.Parts{True: 40},
		monsterPkg.DamagePhysical.String(),
		member,
		false,
	)
	if got.Total() != 0 {
		t.Fatalf("100%% resistance left true damage %+v, want zero", got)
	}
}

func TestMonsterTrueDamage_LandsThroughPartyDodgeButUsesResistance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	isolateTrueDamageMember(member, 50)
	member.Luck = 100 * LuckToDodgeDivisor
	member.HitPoints, member.MaxHitPoints = 200, 200
	attacker := mkTestMonster("True Striker", 500)
	cs.game.cardSlots = [MaxCardSlots]cardSlot{}
	cs.game.cardSlots[0].key = "vengeful_ningyo_card"

	cs.monsterHitCharacter(attacker, member, attacker.Name, monsterCharacterHit{
		Parts:      damagecalc.Parts{Normal: 100, True: 20},
		DamageType: monsterPkg.DamagePhysical.String(),
	})

	if got := 200 - member.HitPoints; got != 10 {
		t.Fatalf("damage through 100%% dodge = %d, want 10 resistance-reduced true damage", got)
	}
	if got := 500 - attacker.HitPoints; got != 1 {
		t.Fatalf("thorns reflected %d from true damage through dodge, want 1", got)
	}
}

func TestPartyCardTrueDamage_DodgeResistanceAndMeleeMultiplier(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	attacker := g.party.Members[g.selectedChar]
	isolateTrueDamageMember(attacker, 0)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "samurai_card"               // +20 true
	g.cardSlots[1].key = "masked_serpent_dancer_card" // +20% normal melee

	target := mkTestMonster("Target", 1000)
	cs.ApplyDamageToMonster(target, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - target.HitPoints; got != 140 {
		t.Fatalf("100 normal with +20%% normal and +20 true dealt %d, want 140", got)
	}

	g.cardSlots[1] = cardSlot{}
	dodger := mkTestMonster("Dodger", 100)
	dodger.PerfectDodge = 100
	dodger.Resistances[monsterPkg.DamagePhysical] = 50
	cs.ApplyDamageToMonster(dodger, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 100 - dodger.HitPoints; got != 10 {
		t.Fatalf("card true through dodge dealt %d, want 10 after physical resistance", got)
	}
}

func TestPartyWeaponMasteryTrueDamage_InheritsWeaponElement(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	attacker := cs.game.party.Members[cs.game.selectedChar]
	isolateTrueDamageMember(attacker, 0)
	attacker.Skills[character.SkillMace] = &character.Skill{Mastery: character.MasteryGrandMaster}
	cs.game.cardSlots = [MaxCardSlots]cardSlot{}
	weapon := lookupWeaponConfigByName("Holy Mace")
	if weapon == nil {
		t.Fatal("Holy Mace definition missing")
	}
	if trueDamage, _ := cs.weaponMasteryStrike(attacker, weapon); trueDamage != 9 {
		t.Fatalf("GM mace mastery true damage = %d, want 9", trueDamage)
	}

	target := mkTestMonster("Element Ward", 100)
	target.Resistances[monsterPkg.DamagePhysical] = 100
	target.Resistances[monsterPkg.DamageLight] = 50

	cs.ApplyDamageToMonster(target, 0, "Holy Mace", false)

	// GM mace mastery contributes 9 true damage. Holy Mace makes it Light:
	// physical immunity is irrelevant and 50% Light resistance rounds it to 4.
	if got := 100 - target.HitPoints; got != 4 {
		t.Fatalf("elemental weapon true damage = %d, want 4 Light damage", got)
	}
}

func TestPartyTrueDamage_ReachesWeaponAoeVictims(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	attacker := g.party.Members[g.selectedChar]
	isolateTrueDamageMember(attacker, 0)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "samurai_card"

	tile := float64(g.config.GetTileSize())
	center := mkTestMonster("Center", 1000)
	center.X, center.Y = 10*tile, 10*tile
	near := mkTestMonster("Near", 1000)
	near.X, near.Y = center.X+tile, center.Y
	near.Resistances[monsterPkg.DamagePhysical] = 50
	near.SoakDamage, near.SoakFrames = 999, 1
	g.world.Monsters = []*monsterPkg.Monster3D{center, near}

	cs.ApplyDamageToMonster(center, 0, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - near.HitPoints; got != 10 {
		t.Fatalf("AoE victim took %d, want 10 resistance-reduced true damage through soak", got)
	}
}

func TestMonsterCrossfireProjectile_CarriesTrueDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	source := mkTestMonster("Bound Archer", 100)
	source.Bound = true
	target := mkTestMonster("Target", 100)
	target.Resistances[monsterPkg.DamagePhysical] = 50
	target.SoakDamage, target.SoakFrames = 999, 1

	bolt := &Arrow{
		ID:            "true_crossfire",
		Active:        true,
		LifeTime:      1,
		DamageType:    monsterPkg.DamagePhysical.String(),
		TrueDamage:    20,
		Owner:         ProjectileOwnerBoundUndead,
		SourceName:    source.Name,
		SourceMonster: source,
	}
	cs.resolveMonsterProjectileVsMonster(bolt, "arrow", target, bolt.ID)

	if got := 100 - target.HitPoints; got != 10 {
		t.Fatalf("crossfire projectile dealt %d, want 10 resistance-reduced true damage through soak", got)
	}
}

func TestMonsterSpecialAoe_CarriesAuthoredTrueDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	isolateTrueDamageMember(member, 0)
	member.Equipment[items.SlotRing1] = items.Item{
		Attributes: map[string]int{"resist_fire": 50},
	}
	member.HitPoints, member.MaxHitPoints = 200, 200
	cs.game.combatBuffs = []TimedCombatBuff{{InReduce: 999}}
	attacker := &monsterPkg.Monster3D{
		Name:               "Burst Caster",
		FireburstDamageMin: 10,
		FireburstDamageMax: 10,
		TrueDamage:         20,
	}

	cs.applyMonsterFireburst(attacker)
	if got := 200 - member.HitPoints; got != 10 {
		t.Fatalf("Fireburst dealt %d, want only 10 resistance-reduced true through flat ward", got)
	}
}

func TestChampionSpellUsesOnlySpellMasteryTrueDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	champion := monsterPkg.NewMonster3DFromConfig(64, 64, "dark_elf_sorceress", cs.game.config)
	champion.ChampionTier = "impossible"
	template := cs.game.championTemplateFor(champion)
	if template == nil {
		t.Fatal("champion template missing")
	}

	champion.TrueDamage = 99
	champion.IgnoresDodge = true
	before := len(cs.game.magicProjectiles)
	cs.championCastSpell(champion, template, spells.SpellID("fireball"))
	if len(cs.game.magicProjectiles) != before+1 {
		t.Fatal("champion spell did not spawn a projectile")
	}
	projectile := cs.game.magicProjectiles[len(cs.game.magicProjectiles)-1]
	wantTrue := int(character.MasteryGrandMaster) * MasterySpellEffectPerLevel
	if projectile.TrueDamage != wantTrue || projectile.IgnoresDodge {
		t.Fatalf("champion spell riders: true=%d ignoreDodge=%v, want spell true=%d and no dodge ignore",
			projectile.TrueDamage, projectile.IgnoresDodge, wantTrue)
	}
}
