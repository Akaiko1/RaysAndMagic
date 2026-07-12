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
	cs.game.refreshBoundAllyCache()

	cs.championCrossfireStrike(champ, front)

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
				markCardAlly(bound)
				bounds = append(bounds, bound)
			}
			cs.game.world.Monsters = append([]*monsterPkg.Monster3D{champ}, bounds...)
			cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)
			cs.game.camera.X, cs.game.camera.Y = 11*ts+ts/2, 10*ts+ts/2

			cs.championCrossfireStrike(champ, bounds[0])

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

	cs.championCrossfireStrike(champ, bounds[0])

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

	cs.applyMonsterProjectileDamageAoE(sorc, sorc.Name, 80, "fire", 0)

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
	cs.game.refreshBoundAllyCache()

	partyHP0 := 0
	for _, mem := range cs.game.party.Members {
		partyHP0 += mem.HitPoints
	}
	cs.championCrossfireStrike(champ, foe)

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
