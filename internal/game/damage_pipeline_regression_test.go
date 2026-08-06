package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestMonsterVsMonsterUsesTargetArmorAndDodge(t *testing.T) {
	t.Run("melee armor", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		source := mkTestMonster("Attacker", 1000)
		source.DamageMin, source.DamageMax = 100, 100
		target := mkTestMonster("Armored summon", 1000)
		target.ArmorClass = 100
		target.PerfectDodge = 0

		cs.monsterStrikeMonster(source, target)
		want := applyMonsterArmor(100, monsterPkg.DamagePhysical.String(), target.EffectiveArmorClass(), false)
		if got := 1000 - target.HitPoints; got != want {
			t.Fatalf("melee dealt %d through target armor, want %d", got, want)
		}
	})

	t.Run("true through dodge", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		source := mkTestMonster("Attacker", 1000)
		source.DamageMin, source.DamageMax = 100, 100
		source.TrueDamage = 20
		target := mkTestMonster("Dodging summon", 1000)
		target.PerfectDodge = 100
		target.Resistances[monsterPkg.DamagePhysical] = 50

		cs.monsterStrikeMonster(source, target)
		if got := 1000 - target.HitPoints; got != 10 {
			t.Fatalf("dodge took %d, want only 10 resistance-reduced true damage", got)
		}
	})

	t.Run("ranged elemental armor", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		source := mkTestMonster("Bound caster", 1000)
		source.Bound = true
		target := mkTestMonster("Armored target", 1000)
		target.ArmorClass = 100
		target.PerfectDodge = 0
		bolt := &Arrow{
			ID: "armor_bolt", Active: true, LifeTime: 1, Damage: 100,
			DamageType: monsterPkg.DamageFire.String(),
			Owner:      ProjectileOwnerBoundUndead, SourceMonster: source, SourceName: source.Name,
		}

		cs.resolveMonsterProjectileVsMonster(bolt, "arrow", target, bolt.ID)
		want := applyMonsterArmor(100, monsterPkg.DamageFire.String(), target.EffectiveArmorClass(), true)
		if got := 1000 - target.HitPoints; got != want {
			t.Fatalf("ranged fire dealt %d through target armor, want %d", got, want)
		}
	})
}

func TestMonsterDamagePacketMitigationOrder(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	target := mkTestMonster("Layered defenses", 1000)
	target.ArmorClass = 100
	target.Resistances[monsterPkg.DamagePhysical] = 50
	target.SoakDamage = 5
	target.SoakFrames = 1

	packet := singleMonsterDamagePacket(
		damagecalc.Parts{Normal: 100, True: 20},
		monsterPkg.DamagePhysical.String(),
		0,
	)
	got := cs.applyMonsterDamagePacket(target, packet, monsterDamageOptions{
		PostArmorNormal: func(damage int) int { return damage * 2 },
	})

	afterArmor := applyMonsterArmor(100, monsterPkg.DamagePhysical.String(), target.ArmorClass, false)
	wantNormal := afterArmor*2*50/100 - target.SoakDamage
	wantTrue := 20 * 50 / 100
	if wantNormal < 0 {
		wantNormal = 0
	}
	if got.Normal != wantNormal || got.True != wantTrue {
		t.Fatalf(
			"damage packet = %+v, want normal=%d true=%d from armor -> modifier -> resistance -> normal-only soak",
			got, wantNormal, wantTrue,
		)
	}
}

func TestPartyConversionKeepsModifiersAndOneSoakInAoe(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	isolateTrueDamageMember(g.party.Members[g.selectedChar], 0)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "masked_hexer_girl_card"     // 20% physical -> dark
	g.cardSlots[1].key = "masked_serpent_dancer_card" // +20% melee normal damage

	ts := float64(g.config.GetTileSize())
	center := mkTestMonster("Center", 1000)
	center.X, center.Y = 10*ts, 10*ts
	center.SoakDamage, center.SoakFrames = 10, 1
	near := mkTestMonster("Near", 1000)
	near.X, near.Y = center.X+ts, center.Y
	near.SoakDamage, near.SoakFrames = 10, 1
	g.world.Monsters = []*monsterPkg.Monster3D{center, near}

	cs.ApplyDamageToMonster(center, 100, "Idol-Breaker, the Warlord's Maul", false)
	// 80 physical + 20 dark; +20% applies to both => 96 + 24. One hit pays
	// Stone Skin once: 120 - 10 = 110 for primary and splash alike.
	for _, target := range []*monsterPkg.Monster3D{center, near} {
		if got := 1000 - target.HitPoints; got != 110 {
			t.Fatalf("%s took %d, want 110 from one converted/modded hit", target.Name, got)
		}
	}
}

func TestPartyAoeAppliesCardBonusVsPerVictim(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "elf_archer_card" // x1.25 vs Dragon
	ts := float64(g.config.GetTileSize())
	center := mkTestMonster("Center", 1000)
	dragon := mkTestMonster("Dragon", 1000)
	center.X, center.Y = 10*ts, 10*ts
	dragon.X, dragon.Y = center.X+ts, center.Y
	g.world.Monsters = []*monsterPkg.Monster3D{center, dragon}

	attack := cs.newPartyMonsterAttack(100, 0, "fire", 0, nil, "Fireball", false, true, false)
	cs.applyAoeSplash(center, attack, 2)
	if got := 1000 - dragon.HitPoints; got != 125 {
		t.Fatalf("dragon splash took %d, want 125 with its target-specific card bonus", got)
	}
}

func TestTrapDamageDoesNotInheritAttackOnlyCardBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "elf_archer_card" // x1.25 vs Dragon attacks, not traps
	target := mkTestMonster("Dragon", 1000)

	cs.applyTrapDamage(target, 100, monsterPkg.DamagePhysical.String(), "Test Trap")
	if got := 1000 - target.HitPoints; got != 100 {
		t.Fatalf("trap dealt %d, want 100 without attack-only card bonus", got)
	}
}

func TestChampionFormulaExcludesPartyCards(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	primeTestChampions(t, g)
	fillTestParty(t, g)
	champion := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", g.config)
	champion.ChampionTier = "impossible"
	template := g.championTemplateFor(champion)
	if template == nil {
		t.Fatal("champion template missing")
	}
	weapon := template.Equipment[items.SlotMainHand]
	baseCrit := cs.CalculateWeaponCritChance(weapon, template)
	baseAC := cs.CalculateTotalArmorClass(template)
	partyMember := g.party.Members[0]
	partyBaseAC := cs.CalculateTotalArmorClass(partyMember)

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "ronin_marksman_card" // +5% PARTY crit
	g.cardSlots[1].key = "treant_card"         // +10 PARTY AC

	if got := cs.CalculateWeaponCritChance(weapon, template); got != baseCrit {
		t.Fatalf("party card crit leaked into champion: %d -> %d", baseCrit, got)
	}
	if got := cs.CalculateTotalArmorClass(template); got != baseAC {
		t.Fatalf("Treant Card leaked into champion AC: %d -> %d", baseAC, got)
	}
	if got := cs.CalculateTotalArmorClass(partyMember); got != partyBaseAC+10 {
		t.Fatalf("Treant Card no longer buffs party: AC %d -> %d, want +10", partyBaseAC, got)
	}
}

func TestBanditBoltDoesNotInheritFallbackBowCombatRules(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	attacker := g.party.Members[g.selectedChar]
	isolateTrueDamageMember(attacker, 0)
	attacker.Luck = 1000 // an inherited weapon crit would be guaranteed
	attacker.Skills[character.SkillBow] = &character.Skill{Mastery: character.MasteryGrandMaster}
	mace, err := items.TryCreateWeaponFromYAML("holy_mace")
	if err != nil {
		t.Fatalf("holy_mace: %v", err)
	}
	attacker.Equipment[items.SlotMainHand] = mace

	if !cs.createArrowAttack(30, items.SlotMainHand, "Bandit Bolt") {
		t.Fatal("bonus bolt did not spawn")
	}
	if len(g.arrows) != 1 {
		t.Fatalf("bonus bolt spawned %d projectiles, want 1", len(g.arrows))
	}
	bolt := &g.arrows[0]
	if bolt.Crit || bolt.TrueDamage != 0 || bolt.IgnoresDodge {
		t.Fatalf("bonus bolt inherited weapon rules: crit=%v true=%d ignore_dodge=%v",
			bolt.Crit, bolt.TrueDamage, bolt.IgnoresDodge)
	}
	if bolt.DamageType != monsterPkg.DamagePhysical.String() {
		t.Fatalf("bonus bolt school = %q, want physical", bolt.DamageType)
	}

	target := mkTestMonster("Dodger", 100)
	target.PerfectDodge = 100
	cs.applyProjectileDamage(bolt, "arrow", target, bolt.ID)
	if target.HitPoints != target.MaxHitPoints {
		t.Fatalf("Bandit Bolt pierced dodge via fallback bow mastery: HP %d/%d", target.HitPoints, target.MaxHitPoints)
	}
}

func TestRangedProjectileSoundDefinitionUsesSpawnedWeaponOnlyForBonusBolt(t *testing.T) {
	equipped := &config.WeaponDefinitionConfig{Category: "blaster"}
	spawned := &config.WeaponDefinitionConfig{Category: "bow"}
	tests := []struct {
		name        string
		bonusBolt   bool
		equippedDef *config.WeaponDefinitionConfig
		want        *config.WeaponDefinitionConfig
	}{
		{name: "normal attack", equippedDef: equipped, want: equipped},
		{name: "bonus bolt", bonusBolt: true, equippedDef: nil, want: spawned},
		{name: "invalid normal attack stays invalid", equippedDef: nil, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rangedProjectileSoundDefinition(test.bonusBolt, test.equippedDef, spawned); got != test.want {
				t.Fatalf("sound definition = %p, want %p", got, test.want)
			}
		})
	}
}

func TestDamageTooltipsUseLiveSourceFormula(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	char := g.party.Members[0]
	isolateTrueDamageMember(char, 0)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "samurai_card"
	g.cardSlots[1].key = "masked_serpent_dancer_card"
	g.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 600, OutBonus: 5, OutDamageType: "all"})

	weapon, err := items.TryCreateWeaponFromYAML("idol_breakers_maul")
	if err != nil {
		t.Fatalf("idol_breakers_maul: %v", err)
	}
	preview := cs.calculateWeaponDamagePreview(weapon, char)
	tooltip := GetItemTooltip(weapon, char, cs, true)
	for _, want := range []string{
		fmt.Sprintf("Normal Damage: %d", preview.Normal),
		fmt.Sprintf("Total Damage: %d", preview.Total),
		fmt.Sprintf("Critical Damage: %d", preview.CriticalTotal),
		"Cards: +20 True",
		"Cards: +20% melee damage",
	} {
		if !strings.Contains(tooltip, want) {
			t.Errorf("weapon tooltip missing %q:\n%s", want, tooltip)
		}
	}

	equipped, err := items.TryCreateWeaponFromYAML("iron_sword")
	if err != nil {
		t.Fatalf("iron_sword: %v", err)
	}
	char.Equipment[items.SlotMainHand] = equipped
	equippedPreview := cs.calculateWeaponDamagePreview(equipped, char)
	comparison := GetItemComparisonTooltip(weapon, char, cs)
	wantComparison := fmt.Sprintf(
		"Total Damage: %d vs %d (%+d)",
		preview.Total,
		equippedPreview.Total,
		preview.Total-equippedPreview.Total,
	)
	if !strings.Contains(comparison, wantComparison) {
		t.Fatalf("weapon comparison missing live formula %q:\n%s", wantComparison, comparison)
	}

	steam, err := spells.GetSpellDefinitionByID("hot_steam")
	if err != nil {
		t.Fatalf("hot_steam: %v", err)
	}
	steamTooltip := buildSpellTooltipUnified(steam, char, cs, true)
	wantTick := cs.CalculateSteamZoneTickDamage(steam, char) + 5
	if want := fmt.Sprintf("Total per tick: %d", wantTick); !strings.Contains(steamTooltip, want) {
		t.Fatalf("Hot Steam tooltip missing live tick %q:\n%s", want, steamTooltip)
	}
}
