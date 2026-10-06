package game

import (
	"fmt"
	"testing"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
)

// Range-1 formation counts per arc type: bound targets on front, both
// diagonals and both sides; a party standing on the front tile.
var (
	crossfireBoundsByArc = map[int]int{1: 1, 2: 2, 3: 3, 4: 5}
	crossfirePartyByArc  = map[int]int{1: 1, 2: 2, 3: 3, 4: 4}
)

// A crossfire arc has two independent results: it applies the shared arc
// geometry to bound targets and, when that same geometry reaches the party,
// applies the champion's normal formation hit. Cover every arc width here so a
// change to either side cannot silently desynchronise them; arc 0 is the real
// template's own main hand.
func TestChampionCrossfireArcHitsBoundsAndParty(t *testing.T) {
	for _, arc := range []int{0, 1, 2, 3, 4} {
		name := fmt.Sprintf("arc_%d", arc)
		if arc == 0 {
			name = "template"
		}
		t.Run(name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			primeTestChampions(t, cs.game)
			fillTestParty(t, cs.game)
			weapon := "template"
			if arc == 0 {
				arc = championHandArc(t, cs.game.championTemplate("weapon_master", "impossible"), false)
			} else {
				weapon = meleeWeaponWithArc(t, arc)
				overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", weapon)
			}
			for _, mem := range cs.game.party.Members {
				mem.Luck = 0
			}
			ts := float64(cs.game.config.GetTileSize())
			champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
			champ.ChampionTier = "impossible"

			// Front, both diagonals, then both side tiles are the complete range-1
			// formation for arc types 1 through 4.
			positions := [][2]float64{{11, 10}, {11, 9}, {11, 11}, {10, 9}, {10, 11}}
			bounds := make([]*monsterPkg.Monster3D, 0, len(positions))
			for _, pos := range positions {
				bound := monsterPkg.NewMonster3DFromConfig(pos[0]*ts+ts/2, pos[1]*ts+ts/2, "masked_huntress", cs.game.config)
				bound.MaxHitPoints, bound.HitPoints = 5000, 5000
				bound.PerfectDodge = 0 // geometry test; defense parity has dedicated coverage
				markCardAlly(bound)
				bounds = append(bounds, bound)
			}
			cs.game.world.Monsters = append([]*monsterPkg.Monster3D{champ}, bounds...)
			cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
			cs.game.camera.X, cs.game.camera.Y = 11*ts+ts/2, 10*ts+ts/2

			cs.championCrossfireStrike(champ, bounds[0], false)

			hitBounds := 0
			for _, bound := range bounds {
				if bound.HitPoints < bound.MaxHitPoints {
					hitBounds++
				}
			}
			if want := crossfireBoundsByArc[arc]; hitBounds != want {
				t.Fatalf("%s crossfire hit %d bound targets, want %d", weapon, hitBounds, want)
			}
			hitParty := 0
			for _, mem := range cs.game.party.Members {
				if mem.HitPoints < mem.MaxHitPoints {
					hitParty++
				}
			}
			if want := crossfirePartyByArc[arc]; hitParty != want {
				t.Fatalf("%s crossfire hit %d party members, want %d", weapon, hitParty, want)
			}
		})
	}
}

func TestChampionCrossfireArcDoesNotHitPartyOutsideWorldCone(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", meleeWeaponWithArc(t, 1))
	ts := float64(cs.game.config.GetTileSize())

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	foe := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	foe.MaxHitPoints, foe.HitPoints = 5000, 5000
	foe.PerfectDodge = 0 // geometry test; dodge parity has dedicated coverage
	markCardAlly(foe)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, foe}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	// The arc-1 weapon swings east at the summon. The party is equally close
	// but north of the champion, outside the arc-1 cone.
	cs.game.camera.X, cs.game.camera.Y = 10*ts+ts/2, 9*ts+ts/2
	partyHP := make([]int, len(cs.game.party.Members))
	for i, member := range cs.game.party.Members {
		member.MaxHitPoints = 5000
		member.HitPoints = 5000
		partyHP[i] = member.HitPoints
	}

	cs.championCrossfireStrike(champ, foe, false)

	if foe.HitPoints >= foe.MaxHitPoints {
		t.Fatal("summon in front of the champion was not hit")
	}
	for i, member := range cs.game.party.Members {
		if member.HitPoints != partyHP[i] {
			t.Fatalf("party member %d outside the world-space arc was hit: HP %d -> %d",
				i, partyHP[i], member.HitPoints)
		}
	}
}

// Weapon Master's hands keep their own authored arc shapes during crossfire
// against several summons, and the front summon (the foe) is always struck.
func TestWeaponMasterCrossfireUsesEachHandArcAgainstSummons(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = 40*ts, 40*ts

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	positions := [][2]float64{{11, 10}, {11, 9}, {11, 11}}
	bounds := make([]*monsterPkg.Monster3D, 0, len(positions))
	for _, pos := range positions {
		bound := monsterPkg.NewMonster3DFromConfig(pos[0]*ts+ts/2, pos[1]*ts+ts/2, "masked_huntress", cs.game.config)
		bound.MaxHitPoints, bound.HitPoints = 5000, 5000
		markCardAlly(bound)
		bounds = append(bounds, bound)
	}
	cs.game.world.Monsters = append([]*monsterPkg.Monster3D{champ}, bounds...)
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	hitCount := func() int {
		hits := 0
		for _, bound := range bounds {
			if bound.HitPoints < bound.MaxHitPoints {
				hits++
			}
		}
		return hits
	}
	// Front plus both diagonals: arc N reaches min(N, 3) of them.
	template := cs.game.championTemplate("weapon_master", "impossible")
	wantMain := min(championHandArc(t, template, false), len(bounds))
	wantOff := min(championHandArc(t, template, true), len(bounds))
	if wantMain == wantOff {
		t.Fatalf("fixture hands reach the same %d summons; cannot tell them apart", wantMain)
	}
	cs.championCrossfireStrike(champ, bounds[0], false)
	if got := hitCount(); got != wantMain || bounds[0].HitPoints == bounds[0].MaxHitPoints {
		t.Fatalf("main-hand crossfire hit %d summons (front hit=%v), want %d incl. the front",
			got, bounds[0].HitPoints < bounds[0].MaxHitPoints, wantMain)
	}
	for _, bound := range bounds {
		bound.HitPoints = bound.MaxHitPoints
	}
	cs.championCrossfireStrike(champ, bounds[0], true)
	if got := hitCount(); got != wantOff || bounds[0].HitPoints == bounds[0].MaxHitPoints {
		t.Fatalf("off-hand crossfire hit %d summons (front hit=%v), want %d incl. the front",
			got, bounds[0].HitPoints < bounds[0].MaxHitPoints, wantOff)
	}
}

func TestWeaponMasterCrossfireUsesOffHandArcAgainstCaughtParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	foe := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	foe.MaxHitPoints, foe.HitPoints = 5000, 5000
	markCardAlly(foe)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, foe}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
	cs.game.camera.X, cs.game.camera.Y = foe.X, foe.Y

	for _, member := range cs.game.party.Members {
		member.Luck = 0
		member.MaxHitPoints = 5000
		member.HitPoints = 5000
	}
	template := cs.game.championTemplate("weapon_master", "impossible")
	offArc := championHandArc(t, template, true)
	mainDef, _, _ := config.GetWeaponDefinitionByName(championHandWeapon(template, false).Name)
	offDef, _, _ := config.GetWeaponDefinitionByName(championHandWeapon(template, true).Name)
	if mainDef.StunChance == offDef.StunChance {
		t.Fatal("fixture hands share a stun rider; cannot tell which hand armed it")
	}
	cs.championCrossfireStrike(champ, foe, true)

	hitParty := 0
	for _, member := range cs.game.party.Members {
		if member.HitPoints < member.MaxHitPoints {
			hitParty++
		}
	}
	if want := crossfirePartyByArc[offArc]; hitParty != want {
		t.Fatalf("off-hand crossfire hit %d party members, want its arc-%d %d", hitParty, offArc, want)
	}
	if champ.StunCharChance != offDef.StunChance {
		t.Fatalf("off-hand crossfire left stun rider %.2f, want the off hand's own %.2f", champ.StunCharChance, offDef.StunChance)
	}
}

// A crossfire target must not reduce Weapon Master to one swing in TB. The
// scheduler uses the champion's authored attack count and alternates hands for
// every one of those actions, exactly as it does against the party.
func TestWeaponMasterCrossfireTurnBasedUsesAllActionsAndHands(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = 40*ts, 40*ts

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	champ.AttacksPerRound = 2
	bounds := []*monsterPkg.Monster3D{
		monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config),
		monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 9*ts+ts/2, "masked_huntress", cs.game.config),
		monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 11*ts+ts/2, "masked_huntress", cs.game.config),
	}
	for _, bound := range bounds {
		bound.MaxHitPoints, bound.HitPoints = 5000, 5000
		markCardAlly(bound)
	}
	cs.game.world.Monsters = append([]*monsterPkg.Monster3D{champ}, bounds...)
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	gl := &GameLoop{game: cs.game}
	cs.game.refreshMonsterAIState()
	gl.monsterAttackFoeTurnBased(champ, bounds[0])

	for i, bound := range bounds {
		if bound.HitPoints == bound.MaxHitPoints {
			t.Fatalf("bound %d was not hit by the two-action main/off-hand crossfire turn", i)
		}
	}
	if champ.NextHandOff {
		t.Fatal("two crossfire actions must consume main then off hand and return to main")
	}
}

// A melee AoE champion does not multiply its sweep by arc width: it reaches
// every bound target in the AoE and then performs one whole-party AoE strike.
func TestChampionCrossfireMeleeAoEHitsBoundsAndParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", "tonbogiri")
	for _, mem := range cs.game.party.Members {
		mem.Luck = 0
	}
	ts := float64(cs.game.config.GetTileSize())
	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	bounds := []*monsterPkg.Monster3D{
		monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config),
		monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 11*ts+ts/2, "masked_huntress", cs.game.config),
	}
	for _, bound := range bounds {
		bound.MaxHitPoints, bound.HitPoints = 5000, 5000
		markCardAlly(bound)
	}
	cs.game.world.Monsters = append([]*monsterPkg.Monster3D{champ}, bounds...)
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
	cs.game.camera.X, cs.game.camera.Y = 11*ts+ts/2, 10*ts+ts/2

	cs.championCrossfireStrike(champ, bounds[0], false)

	for i, bound := range bounds {
		if bound.HitPoints >= bound.MaxHitPoints {
			t.Fatalf("bound target %d untouched by melee AoE", i)
		}
	}
	for i, mem := range cs.game.party.Members {
		if mem.HitPoints >= mem.MaxHitPoints {
			t.Fatalf("party member %d untouched by melee AoE", i)
		}
	}
}

func TestChampionCrossfireTransitTargetsSkipArcButTakeAoe(t *testing.T) {
	setup := func(t *testing.T, weapon string) (*CombatSystem, *monsterPkg.Monster3D, *monsterPkg.Monster3D, *monsterPkg.Monster3D) {
		t.Helper()
		cs := newTestCombatSystemWithConfig(t)
		primeTestChampions(t, cs.game)
		fillTestParty(t, cs.game)
		overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", weapon)
		ts := float64(cs.game.config.GetTileSize())
		cs.game.camera.X, cs.game.camera.Y = 40*ts, 40*ts

		champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
		champ.ChampionTier = "impossible"
		front := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
		transit := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 11*ts+ts/2, "masked_huntress", cs.game.config)
		for _, bound := range []*monsterPkg.Monster3D{front, transit} {
			bound.MaxHitPoints, bound.HitPoints = 5000, 5000
			bound.PerfectDodge = 0 // transit/arc test; defense parity is separate
			markCardAlly(bound)
		}
		transit.AttackTransit = true
		cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, front, transit}
		cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
		return cs, champ, front, transit
	}

	t.Run("arc", func(t *testing.T) {
		cs, champ, front, transit := setup(t, "steel_mace")
		cs.championCrossfireStrike(champ, front, false)
		if front.HitPoints >= front.MaxHitPoints {
			t.Fatal("front summon was not hit by the champion arc")
		}
		if transit.HitPoints != transit.MaxHitPoints {
			t.Fatal("transit summon sharing an attack post must be skipped by an arc")
		}
	})

	t.Run("aoe", func(t *testing.T) {
		cs, champ, front, transit := setup(t, "tonbogiri")
		cs.championCrossfireStrike(champ, front, false)
		if front.HitPoints >= front.MaxHitPoints || transit.HitPoints >= transit.MaxHitPoints {
			t.Fatal("melee AoE must hit both the settled and transit summons")
		}
	})
}

// A ranged champion's AoE bolt (Dark Elf Sorceress' archmage staff, radius 3)
// splashes every summon in the blast AND the party when it stands in the radius.
func TestChampionRangedAoESplashesSummonsAndParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())

	sorc := monsterPkg.NewMonster3DFromConfig(0, 0, "dark_elf_sorceress", cs.game.config)
	sorc.ChampionTier = "impossible"
	cs.game.mirrorChampionStats(sorc)
	if !sorc.IsChampion() {
		t.Fatal("dark_elf_sorceress must be a champion")
	}

	// Two huntresses one tile apart (both inside a 3-tile blast), party alongside.
	target := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	other := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	for _, h := range []*monsterPkg.Monster3D{target, other} {
		h.MaxHitPoints, h.HitPoints = 5000, 5000
		h.PerfectDodge = 0
		markCardAlly(h)
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{sorc, target, other}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
	cs.game.camera.X, cs.game.camera.Y = 11*ts+ts/2, 11*ts+ts/2 // within 3 tiles of the impact

	partyHP0 := make([]int, len(cs.game.party.Members))
	for i, mem := range cs.game.party.Members {
		mem.Luck = 0
		partyHP0[i] = mem.HitPoints
	}
	bolt := &Arrow{
		ID: "test_bolt", Active: true, LifeTime: 10, Damage: 80,
		BowKey: "archmage_staff", DamageType: "fire", SourceName: sorc.Name,
		SourceMonster: sorc, Owner: ProjectileOwnerMonsterAtBound,
	}
	cs.resolveMonsterProjectileVsMonster(bolt, "arrow", target, bolt.ID)

	if target.HitPoints >= 5000 {
		t.Error("the bolt's direct target must be hit")
	}
	if other.HitPoints >= 5000 {
		t.Error("the AoE bolt must splash the second summon in the blast")
	}
	for i, mem := range cs.game.party.Members {
		if mem.HitPoints >= partyHP0[i] {
			t.Errorf("party member %d inside the blast was not hit (%d -> %d)", i, partyHP0[i], mem.HitPoints)
		}
	}
}

// The direct monster-projectile AoE endpoint is the non-crossfire route used
// when a champion fires at a clean party. It must retain whole-party splash.
func TestChampionRangedAoEHitsCleanParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	sorc := monsterPkg.NewMonster3DFromConfig(0, 0, "dark_elf_sorceress", cs.game.config)
	sorc.ChampionTier = "impossible"
	cs.game.mirrorChampionStats(sorc)
	for _, mem := range cs.game.party.Members {
		mem.Luck = 0
	}

	cs.applyMonsterProjectileDamageAoE(sorc, sorc.Name, hitFromMonster(sorc, 80, "fire", false, 0, false, false))

	for i, mem := range cs.game.party.Members {
		if mem.HitPoints >= mem.MaxHitPoints {
			t.Fatalf("party member %d untouched by champion ranged AoE", i)
		}
	}
}

// Crossfire AoE must not reuse player splash targeting: a champion's projectile
// may harm the bound allies it was aimed at, but never itself or its ordinary
// monster allies standing in the blast.
func TestChampionCrossfireAoESparesSourceAndEnemyAllies(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())

	sorc := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "dark_elf_sorceress", cs.game.config)
	sorc.ChampionTier = "impossible"
	cs.game.mirrorChampionStats(sorc)
	target := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	otherAlly := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 11*ts+ts/2, "masked_huntress", cs.game.config)
	for _, h := range []*monsterPkg.Monster3D{target, otherAlly} {
		h.MaxHitPoints, h.HitPoints = 5000, 5000
		h.PerfectDodge = 0 // this test isolates AoE faction targeting, not dodge
		markCardAlly(h)
	}
	enemyAlly := monsterPkg.NewMonster3DFromConfig(12*ts+ts/2, 10*ts+ts/2, "goblin", cs.game.config)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{sorc, target, otherAlly, enemyAlly}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	sorcHP, enemyAllyHP := sorc.HitPoints, enemyAlly.HitPoints
	bolt := &Arrow{
		ID: "crossfire_aoe", Active: true, LifeTime: 10, Damage: 80,
		BowKey: "archmage_staff", DamageType: "fire", SourceName: sorc.Name,
		SourceMonster: sorc, Owner: ProjectileOwnerMonsterAtBound,
	}
	cs.resolveMonsterProjectileVsMonster(bolt, "arrow", target, bolt.ID)

	if sorc.HitPoints != sorcHP {
		t.Errorf("crossfire source HP = %d, want %d", sorc.HitPoints, sorcHP)
	}
	if enemyAlly.HitPoints != enemyAllyHP {
		t.Errorf("ordinary enemy ally HP = %d, want %d", enemyAlly.HitPoints, enemyAllyHP)
	}
	if otherAlly.HitPoints >= 5000 {
		t.Error("bound ally in the blast must take crossfire AoE damage")
	}
}
