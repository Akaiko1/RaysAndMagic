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
