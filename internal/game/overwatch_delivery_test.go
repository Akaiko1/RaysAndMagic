package game

import (
	"fmt"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestOverwatchRequiresHostileDelivery(t *testing.T) {
	for _, cadence := range []monsterAttackCadence{monsterAttackRealtime, monsterAttackTurn, monsterAttackPounce} {
		for _, kind := range []string{"healing", "melee", "dodged", "zero damage", "mixed", "buff", "spell", "dual mixed"} {
			if (kind == "mixed" && cadence != monsterAttackTurn) || (kind == "dual mixed" && cadence != monsterAttackRealtime) {
				continue
			}
			t.Run(fmt.Sprintf("cadence=%d/%s", cadence, kind), func(t *testing.T) {
				g, _, ch, ts := sniperFixture(t, cadence == monsterAttackTurn)
				ch.HitPoints, ch.MaxHitPoints = 100000, 100000
				// The mixed-action control observes HP loss. Its attack must
				// land; the dedicated dodged row explicitly opts into dodge.
				ch.Luck = 0
				key := "ningyo"
				if kind == "buff" || kind == "spell" {
					key = "wild_druid"
				}
				if kind == "dual mixed" {
					key = "weapon_master"
				}
				if key != "ningyo" {
					primeTestChampions(t, g)
					def := config.GetChampionDefinition(key)
					original := *def
					t.Cleanup(func() { *def = original })
					spell := "stone_skin"
					if kind == "spell" {
						spell = "fireball"
					}
					def.OpeningSpell, def.OpeningSpellTiers = spell, nil
					def.SpellCastChance, def.SpellSchools, def.ExtraSpells = 1, nil, []string{spell}
				}
				m := spawnMonsterAtTile(g, key, 6, 10, ts)
				m.State, m.StateTimer, m.AttackCDFrames, m.OffHandCDFrames = monster.StateAttacking, 1, 0, 0
				m.ChampionTier = "normal"
				m.HitPoints, m.MaxHitPoints = 100, 500
				if key == "ningyo" {
					m.AllyHealChance, m.AllyHealAmount = 1, 200
					if kind == "melee" || kind == "dodged" || kind == "zero damage" {
						m.AllyHealChance = 0
					}
					if kind == "dodged" {
						ch.Luck = 500
					}
					if kind == "zero damage" {
						m.DamageMin, m.DamageMax, m.TrueDamage = 0, 0, 0
					}
					if kind == "mixed" {
						m.MaxHitPoints = 300
						m.AttacksPerRound = 2
					}
				}
				want := 1
				if kind == "healing" || kind == "buff" {
					want = 0
				}
				if !g.combat.commitMonsterAttack(m, monsterAttackDestination{}, cadence) {
					t.Fatal("fixture action was not committed")
				}
				if got := overwatchShots(g); got != want {
					t.Fatalf("reaction shots=%d want %d for delivered %s", got, want, kind)
				}
				if g.combat.monsterAction != nil {
					t.Fatal("action observation leaked to later actions")
				}
				if kind == "healing" && m.HitPoints <= 100 {
					t.Fatal("healing control did not execute")
				}
				if kind == "spell" {
					delivered := false
					for _, projectile := range g.magicProjectiles {
						if projectile.Owner == ProjectileOwnerMonster && projectile.SpellType == "fireball" {
							delivered = true
						}
					}
					if !delivered {
						t.Fatal("offensive spell control did not deliver a fireball")
					}
				}
				if kind == "buff" && m.SoakFrames == 0 {
					t.Fatal("buff control did not execute")
				}
				if kind == "mixed" || kind == "dual mixed" {
					if ch.HitPoints == ch.MaxHitPoints {
						t.Fatal("mixed action never delivered its hostile part")
					}
				}
			})
		}
	}
}
