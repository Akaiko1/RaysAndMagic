package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// A melee champion's swing at a summon uses its real weapon ARC: it catches
// more than the single foe (the front target plus one flank), exactly like the
// party's PvE arc - not the old single-target monster-vs-monster blow.
func TestChampionArcHitsMultipleSummons(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())

	// Park the party far away so it is never caught in this swing.
	cs.game.camera.X, cs.game.camera.Y = 40*ts, 40*ts

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config) // steel_mace, arc 2
	champ.ChampionTier = "impossible"

	// Front + both flanks around the champion, all adjacent (facing = east at the
	// front huntress). Arc 2 = front always + ONE flank -> exactly two of three.
	front := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	left := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 9*ts+ts/2, "masked_huntress", cs.game.config)
	right := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 11*ts+ts/2, "masked_huntress", cs.game.config)
	for _, h := range []*monsterPkg.Monster3D{front, left, right} {
		h.MaxHitPoints, h.HitPoints = 5000, 5000
		markCardAlly(h)
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, front, left, right}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
	cs.game.refreshMonsterAIState()

	cs.championCrossfireStrike(champ, front, false)

	damaged := 0
	for _, h := range []*monsterPkg.Monster3D{front, left, right} {
		if h.HitPoints < 5000 {
			damaged++
		}
	}
	if front.HitPoints >= 5000 {
		t.Error("the front summon (the foe) must be hit")
	}
	if damaged < 2 {
		t.Errorf("arc-2 swing should catch the front summon plus one flank (>=2), hit %d", damaged)
	}
}

// A crossfire arc has two independent results: it applies the shared arc
// geometry to bound targets and, when that same geometry reaches the party,
// applies the champion's normal formation hit. Cover every arc width here so a
// change to either side cannot silently desynchronise them.
func TestChampionCrossfireArcHitsBoundsAndParty(t *testing.T) {
	for _, tc := range []struct {
		name, weapon  string
		bounds, party int
	}{
		{name: "arc_1", weapon: "magic_dagger", bounds: 1, party: 1},
		{name: "arc_2", weapon: "steel_mace", bounds: 2, party: 2},
		{name: "arc_3", weapon: "muramasa", bounds: 3, party: 3},
		{name: "arc_4", weapon: "gorehorn_greataxe", bounds: 5, party: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			primeTestChampions(t, cs.game)
			fillTestParty(t, cs.game)
			overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", tc.weapon)
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
			if hitBounds != tc.bounds {
				t.Fatalf("%s crossfire hit %d bound targets, want %d", tc.weapon, hitBounds, tc.bounds)
			}
			hitParty := 0
			for _, mem := range cs.game.party.Members {
				if mem.HitPoints < mem.MaxHitPoints {
					hitParty++
				}
			}
			if hitParty != tc.party {
				t.Fatalf("%s crossfire hit %d party members, want %d", tc.weapon, hitParty, tc.party)
			}
		})
	}
}

func TestChampionCrossfireArcDoesNotHitPartyOutsideWorldCone(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", "magic_dagger")
	ts := float64(cs.game.config.GetTileSize())

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	foe := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	foe.MaxHitPoints, foe.HitPoints = 5000, 5000
	foe.PerfectDodge = 0 // geometry test; dodge parity has dedicated coverage
	markCardAlly(foe)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, foe}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	// The dagger swings east at the summon. The party is equally close but
	// north of the champion, outside the arc-1 cone.
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

// Weapon Master's hands keep their own authored arc shapes during crossfire:
// impossible-tier main-hand Steel Mace is arc 2, while off-hand Muramasa is
// arc 3. This guards the multi-summon case specifically, not just party arcs.
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
	cs.championCrossfireStrike(champ, bounds[0], false)
	if got := hitCount(); got != 2 {
		t.Fatalf("main-hand Steel Mace crossfire hit %d summons, want arc 2", got)
	}
	for _, bound := range bounds {
		bound.HitPoints = bound.MaxHitPoints
	}
	cs.championCrossfireStrike(champ, bounds[0], true)
	if got := hitCount(); got != 3 {
		t.Fatalf("off-hand Muramasa crossfire hit %d summons, want arc 3", got)
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
	cs.championCrossfireStrike(champ, foe, true)

	hitParty := 0
	for _, member := range cs.game.party.Members {
		if member.HitPoints < member.MaxHitPoints {
			hitParty++
		}
	}
	if hitParty != 3 {
		t.Fatalf("off-hand Muramasa crossfire hit %d party members, want its arc 3", hitParty)
	}
	if champ.StunCharChance != 0 {
		t.Fatalf("off-hand Muramasa crossfire left main-hand Steel Mace stun rider %.2f", champ.StunCharChance)
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

// When the same swing's arc reaches the party, the party takes the champion's
// normal vs-party hit too (the additional action) - vs-party handling reused.
func TestChampionCrossfireAlsoHitsCaughtParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	ts := float64(cs.game.config.GetTileSize())

	champ := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "weapon_master", cs.game.config)
	champ.ChampionTier = "impossible"
	// The party stands right in front of the champion (east, adjacent) - inside
	// the swing that also strikes the summon foe there.
	cs.game.camera.X, cs.game.camera.Y = 11*ts+ts/2, 10*ts+ts/2
	foe := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "masked_huntress", cs.game.config)
	foe.MaxHitPoints, foe.HitPoints = 5000, 5000
	markCardAlly(foe)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, foe}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
	cs.game.refreshMonsterAIState()

	partyHP0 := 0
	for _, mem := range cs.game.party.Members {
		partyHP0 += mem.HitPoints
	}
	cs.championCrossfireStrike(champ, foe, false)

	if foe.HitPoints >= 5000 {
		t.Error("the summon foe must be struck")
	}
	partyHP1 := 0
	for _, mem := range cs.game.party.Members {
		partyHP1 += mem.HitPoints
	}
	if partyHP1 >= partyHP0 {
		t.Errorf("the party caught in the swing must take the champion's hit (party HP %d -> %d)", partyHP0, partyHP1)
	}
}
