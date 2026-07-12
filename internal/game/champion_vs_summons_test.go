package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// TestChampionVsCardSummonedAllies pins the arena scenario: fighting a champion
// while the Orc Warlord Card has summoned Masked Huntresses (Bound party allies).
// Covered for BOTH champion archetypes at the impossible tier - the melee Weapon
// Master and the ranged Dark Elf Sorceress.
//
//   - Targeting: a Bound huntress hunts the nearest enemy monster (the champion);
//     the champion, a normal enemy, targets the nearer Bound ally instead of the
//     party (only while she is closer than the party).
//   - Champion -> huntress: the champion's damage comes from monsterAttackDamage,
//     which for a champion is its REAL weapon swing (championSwingDamage), well
//     above the huntress's own band. The melee Weapon Master strikes; the ranged
//     Sorceress delivers the same damage as a staff bolt (both are crossfire, NOT
//     the champion's party-only spell casts).
//   - Huntress -> champion: her own authored band draws down the champion's tier
//     HP pool (neither champion resists physical).
func TestChampionVsCardSummonedAllies(t *testing.T) {
	for _, tc := range []struct {
		champion string
		ranged   bool
	}{
		{"weapon_master", false},
		{"dark_elf_sorceress", true},
	} {
		t.Run(tc.champion, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			primeTestChampions(t, cs.game)
			fillTestParty(t, cs.game)
			ts := float64(cs.game.config.GetTileSize())

			cs.game.camera.X, cs.game.camera.Y = 20*ts, 20*ts

			// Champion 3 tiles east of the party, at the impossible tier.
			champ := monsterPkg.NewMonster3DFromConfig(cs.game.camera.X+3*ts, cs.game.camera.Y, tc.champion, cs.game.config)
			champ.ChampionTier = "impossible"
			if !champ.IsChampion() {
				t.Fatalf("%s must be a champion mob", tc.champion)
			}
			// Card-summoned huntress BETWEEN the party and the champion, so the
			// champion's "nearer than the party" gate targets her, not the party.
			huntress := monsterPkg.NewMonster3DFromConfig(cs.game.camera.X+1.5*ts, cs.game.camera.Y, "masked_huntress", cs.game.config)
			markCardAlly(huntress) // Bound, SummonedBy = card_collection

			cs.game.world.Monsters = []*monsterPkg.Monster3D{champ, huntress}
			cs.game.refreshBoundAllyCache() // mirrors the champion + resolves AIFoe both ways

			// The delivery mechanism differs by archetype (documents HOW each attacks).
			if champ.HasRangedAttack() != tc.ranged {
				t.Fatalf("%s ranged = %v, want %v", tc.champion, champ.HasRangedAttack(), tc.ranged)
			}

			// --- Targeting ---
			if huntress.AIFoe != champ {
				t.Fatalf("bound huntress AIFoe = %v, want the champion", huntress.AIFoe)
			}
			if champ.AIFoe != huntress {
				t.Fatalf("champion AIFoe = %v, want the nearer bound huntress (not the party)", champ.AIFoe)
			}

			// --- Champion's damage source is its own weapon swing ---
			// monsterAttackDamage(champion) = championSwingDamage(main hand); its
			// range sits well above the huntress's 36-54 band. Sample so a low roll
			// can't make this flaky.
			maxSwing := 0
			for i := 0; i < 40; i++ {
				if d := cs.monsterAttackDamage(champ); d > maxSwing {
					maxSwing = d
				}
			}
			if maxSwing <= huntress.DamageMax {
				t.Errorf("champion max swing over 40 rolls = %d, want above the huntress band max %d (champion uses its own weapon)", maxSwing, huntress.DamageMax)
			}
			// A landed blow reduces the huntress's HP (monsterStrikeMonster is the
			// shared damage sink both the melee strike and the ranged bolt reach).
			hpBefore := huntress.HitPoints
			cs.monsterStrikeMonster(champ, huntress)
			if huntress.HitPoints >= hpBefore {
				t.Errorf("champion did not damage the summoned huntress (HP %d -> %d)", hpBefore, huntress.HitPoints)
			}

			// --- Huntress strikes the champion for her own authored band ---
			// Neither champion resists physical, so the tier HP pool takes the raw band.
			champHPBefore := champ.HitPoints
			cs.monsterStrikeMonster(huntress, champ)
			hit := champHPBefore - champ.HitPoints
			if hit < huntress.DamageMin || hit > huntress.DamageMax {
				t.Errorf("huntress hit the champion for %d, want her authored band [%d,%d]", hit, huntress.DamageMin, huntress.DamageMax)
			}
			if champ.HitPoints >= champ.MaxHitPoints {
				t.Errorf("champion tier HP pool untouched (%d/%d) after a huntress blow", champ.HitPoints, champ.MaxHitPoints)
			}
		})
	}
}

// The arena uses the turn-based scheduler, not the direct combat helper above.
// A Weapon Master must therefore acquire a card summon, walk into melee, and
// actually spend a monster turn striking it. This is the gameplay regression
// that a direct championCrossfireStrike/monsterStrikeMonster test cannot see.
func TestWeaponMasterFightsCardSummonThroughTurnBasedAI(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	primeTestChampions(t, game)
	fillTestParty(t, game)
	placePlayerAtTile(game, 5, 10, ts)

	champ := monsterPkg.NewMonster3DFromConfig(20*ts+ts/2, 10*ts+ts/2, "weapon_master", game.config)
	champ.ChampionTier = "impossible"
	champ.MaxHitPoints, champ.HitPoints = 5000, 5000
	huntress := monsterPkg.NewMonster3DFromConfig(24*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	huntress.MaxHitPoints, huntress.HitPoints = 5000, 5000
	markCardAlly(huntress)
	game.world.Monsters = []*monsterPkg.Monster3D{champ, huntress}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	d0 := Distance(champ.X, champ.Y, huntress.X, huntress.Y)
	hp0 := huntress.HitPoints
	for turn := 0; turn < 8; turn++ {
		game.refreshBoundAllyCache()
		if champ.AIFoe != huntress {
			t.Fatalf("turn %d: Weapon Master AIFoe = %v, want card summon", turn, champ.AIFoe)
		}
		runOneMonsterTurn(game, gl)
	}
	if d := Distance(champ.X, champ.Y, huntress.X, huntress.Y); d >= d0 {
		t.Fatalf("Weapon Master did not close on card summon (%.0f -> %.0f)", d0, d)
	}
	if huntress.HitPoints >= hp0 {
		t.Fatalf("Weapon Master never struck card summon through TB AI (HP %d -> %d)", hp0, huntress.HitPoints)
	}
}

// The same acquisition and strike must work through the real-time movement and
// interaction loop, where the champion's crossfire action is cadence-gated.
func TestWeaponMasterFightsCardSummonThroughRealTimeAI(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	primeTestChampions(t, game)
	fillTestParty(t, game)
	placePlayerAtTile(game, 5, 10, ts)

	champ := monsterPkg.NewMonster3DFromConfig(20*ts+ts/2, 10*ts+ts/2, "weapon_master", game.config)
	champ.ChampionTier = "impossible"
	champ.MaxHitPoints, champ.HitPoints = 5000, 5000
	huntress := monsterPkg.NewMonster3DFromConfig(24*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	huntress.MaxHitPoints, huntress.HitPoints = 5000, 5000
	markCardAlly(huntress)
	game.world.Monsters = []*monsterPkg.Monster3D{champ, huntress}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	d0 := Distance(champ.X, champ.Y, huntress.X, huntress.Y)
	hp0 := huntress.HitPoints
	runRTFoeTicks(game, 6*game.config.GetTPS())

	if d := Distance(champ.X, champ.Y, huntress.X, huntress.Y); d >= d0 {
		t.Fatalf("Weapon Master did not close on card summon in RT (%.0f -> %.0f)", d0, d)
	}
	if huntress.HitPoints >= hp0 {
		t.Fatalf("Weapon Master never struck card summon through RT AI (HP %d -> %d)", hp0, huntress.HitPoints)
	}
}

// An adjacent tile is valid melee contact even when two moving standees sit at
// opposite edges of their diagonal tiles. The AI already stops there; crossfire
// must use that same contact rule instead of a smaller radial distance gate.
func TestWeaponMasterCrossfireHitsOffCenterAdjacentCardSummon(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	primeTestChampions(t, game)
	fillTestParty(t, game)
	placePlayerAtTile(game, 5, 10, ts)

	champ := monsterPkg.NewMonster3DFromConfig(20*ts+1, 10*ts+1, "weapon_master", game.config)
	champ.ChampionTier = "impossible"
	ally := monsterPkg.NewMonster3DFromConfig(22*ts-1, 12*ts-1, "masked_huntress", game.config)
	ally.MaxHitPoints, ally.HitPoints = 5000, 5000
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{champ, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshBoundAllyCache()

	if champ.AIFoe != ally {
		t.Fatal("Weapon Master should acquire the adjacent card summon")
	}
	if d := Distance(champ.X, champ.Y, ally.X, ally.Y); d <= 1.5*ts {
		t.Fatalf("test setup needs an off-centre distance beyond the old 1.5-tile gate, got %.1f", d)
	}
	if !game.combat.monsterCanAttackMonster(champ, ally) {
		t.Fatal("tile-adjacent Weapon Master should be allowed to crossfire")
	}

	hp0 := ally.HitPoints
	game.combat.HandleMonsterInteractions()
	if ally.HitPoints >= hp0 {
		t.Fatalf("Weapon Master did not strike off-centre adjacent summon (HP %d -> %d)", hp0, ally.HitPoints)
	}
}

// Crossfire must preserve Weapon Master's RT dual-wield cadence. The first
// contact fires both ready hands; while the main hand cools down, a ready off
// hand continues to strike instead of falling back to the old one-second
// fixed crossfire cadence.
func TestWeaponMasterCrossfireUsesIndependentHandCooldowns(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	primeTestChampions(t, game)
	fillTestParty(t, game)
	placePlayerAtTile(game, 5, 10, ts)

	champ := monsterPkg.NewMonster3DFromConfig(20*ts+ts/2, 10*ts+ts/2, "weapon_master", game.config)
	champ.ChampionTier = "impossible"
	ally := monsterPkg.NewMonster3DFromConfig(21*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	ally.MaxHitPoints, ally.HitPoints = 5000, 5000
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{champ, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshBoundAllyCache()

	game.combat.HandleMonsterInteractions()
	if champ.AttackCDFrames <= 0 || champ.OffHandCDFrames <= 0 {
		t.Fatalf("first crossfire contact must arm both hands, main=%d off=%d", champ.AttackCDFrames, champ.OffHandCDFrames)
	}
	champ.AttackCDFrames = 10
	champ.OffHandCDFrames = 0
	hp0 := ally.HitPoints
	game.combat.HandleMonsterInteractions()
	if champ.AttackCDFrames != 9 {
		t.Fatalf("main-hand cooldown should only tick, got %d", champ.AttackCDFrames)
	}
	if champ.OffHandCDFrames <= 0 {
		t.Fatal("ready off hand did not arm its own cooldown during crossfire")
	}
	if ally.HitPoints >= hp0 {
		t.Fatalf("ready off hand did not strike during main-hand cooldown (HP %d -> %d)", hp0, ally.HitPoints)
	}
}
