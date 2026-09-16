package game

import (
	"fmt"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestAIChampionSpellsFollowTarget(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, redirected := range []bool{false, true} {
			for _, ranged := range []bool{false, true} {
				for _, opening := range []bool{false, true} {
					for _, spell := range []string{"stone_skin", "fireball", "stun"} {
						t.Run(fmt.Sprintf("TB=%v/foe=%v/ranged=%v/opening=%v/%s", tb, redirected, ranged, opening, spell), func(t *testing.T) {
							g, gl, tile := tbBehaviorGame(t, 40, 40)
							g.turnBasedMode = tb
							primeTestChampions(t, g)
							def := config.GetChampionDefinition("wild_druid")
							original := *def
							t.Cleanup(func() { *def = original })
							def.Ranged = ranged
							def.OpeningSpell = spell
							def.OpeningSpellTiers = nil
							def.SpellCastChance = 1
							def.SpellSchools = nil
							def.ExtraSpells = []string{spell}
							placePlayerAtTile(g, 11, 10, tile)
							m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "wild_druid", g.config)
							m.ChampionTier = "normal"
							m.OpeningSpellDone = !opening
							m.WasAttacked = true
							m.BeginPlayerEngagement()
							g.world.Monsters = []*monster.Monster3D{m}
							var foe *monster.Monster3D
							if redirected {
								placePlayerAtTile(g, 30, 30, tile)
								foe = monster.NewMonster3DFromConfig(11.5*tile, 10.5*tile, "skeleton", g.config)
								foe.Bound = true
								foe.Speed = 0
								foe.HitPoints = 100000
								foe.MaxHitPoints = 100000
								// Caster acts last so the target cannot tick down a newly applied stun.
								g.world.Monsters = []*monster.Monster3D{foe, m}
							}
							g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
							g.refreshMonsterAIState()
							// Mirror the edited build's delivery without changing champion stats.
							if ranged {
								m.ProjectileWeapon = "archmage_staff"
							} else {
								m.ProjectileWeapon = ""
								m.ProjectileSpell = ""
							}
							if tb {
								runOneMonsterTurn(g, gl)
							} else {
								m.State = monster.StateAttacking
								m.StateTimer = 1
								gl.reconcileMonsterAttackPosts()
								g.combat.HandleMonsterInteractions()
							}
							if opening && !m.OpeningSpellDone {
								t.Fatal("attack bypassed guaranteed opener")
							}
							switch spell {
							case "stone_skin":
								if m.SoakFrames <= 0 {
									t.Fatal("cast did not apply self buff")
								}
							case "stun":
								if redirected {
									if foe.StunTurnsRemaining <= 0 {
										t.Fatal("shockwave missed summon")
									}
								} else {
									stunned := false
									for _, ch := range g.party.Members {
										if ch.StunTurnsRemaining > 0 {
											stunned = true
										}
									}
									if !stunned {
										t.Fatal("shockwave missed party")
									}
								}
							case "fireball":
								if len(g.magicProjectiles) == 0 {
									t.Fatal("cast did not create a spell projectile")
								}
								p := &g.magicProjectiles[len(g.magicProjectiles)-1]
								owner := ProjectileOwnerMonster
								if redirected {
									owner = ProjectileOwnerMonsterAtBound
								}
								if p.Owner != owner || p.VelX <= 0 || p.VelY != 0 || p.SpellType != spell {
									t.Fatalf("wrong projectile target/owner: %+v", p)
								}
								if redirected {
									before := foe.HitPoints
									g.combat.resolveMonsterProjectileVsMonster(p, "magic_projectile", foe, p.ID)
									if foe.HitPoints >= before {
										t.Fatal("spell did not damage summon at impact")
									}
								}
							}
						})
					}
				}
			}
		}
	}
}
