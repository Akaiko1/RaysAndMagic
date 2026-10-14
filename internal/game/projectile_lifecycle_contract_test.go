package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"
)

func TestArrowImpactStorageAndContinuation(t *testing.T) {
	for _, capacity := range []int{2, 8} {
		for _, rider := range []string{"none", "pierce", "ricochet"} {
			t.Run(fmt.Sprintf("%s/cap_%d", rider, capacity), func(t *testing.T) {
				g := newTestCombatSystemWithConfig(t).game
				m := mkTestMonster("Target", 1000)
				m.ID = "target"
				m.X, m.Y = 64, 0
				next := mkTestMonster("Next", 1000)
				next.ID = "next"
				next.X, next.Y = 128, 64
				g.world.Monsters = []*monsterPkg.Monster3D{m, next}
				for _, mob := range g.world.Monsters {
					g.collisionSystem.RegisterEntity(collision.NewEntity(mob.ID, mob.X, mob.Y, 16, 16, collision.CollisionTypeMonster, false))
				}
				g.arrows = make([]Arrow, 2, capacity)
				for i := range g.arrows {
					id := g.GenerateProjectileID("arrow")
					g.arrows[i] = Arrow{ID: id, Active: true, X: m.X, Y: m.Y, VelX: 8, Damage: 10, LifeTime: 50, BowKey: "arbalest", DamageType: "physical", Owner: ProjectileOwnerPlayer}
					g.collisionSystem.RegisterEntity(collision.NewEntity(id, m.X, m.Y, 8, 8, collision.CollisionTypeProjectile, false))
				}
				if rider == "pierce" {
					g.arrows[0].PierceLeft = 1
				}
				if rider == "ricochet" {
					g.arrows[0].RicochetLeft = 1
					g.arrows[0].BowKey = "nest_arbalest"
				}
				g.turnBasedMode = true
				g.combat.CheckProjectileMonsterCollisions()
				if m.HitPoints != 980 || next.HitPoints != 1000 {
					t.Fatal("impact or new-continuation timing changed")
				}
				for i := 0; i < 2; i++ {
					if g.arrows[i].Active || g.collisionSystem.GetEntityByID(g.arrows[i].ID) != nil {
						t.Fatalf("impacted arrow %d is still live", i)
					}
				}
				want := 2
				if rider != "none" {
					want = 3
				}
				if len(g.arrows) != want {
					t.Fatalf("arrows=%d, want %d", len(g.arrows), want)
				}
				if rider != "none" && (!g.arrows[2].Active || g.arrows[2].SkipMonster != m || g.collisionSystem.GetEntityByID(g.arrows[2].ID) == nil) {
					t.Fatal("continuation lost lifecycle or skip target")
				}
				g.combat.CheckProjectileMonsterCollisions()
				if m.HitPoints != 980 {
					t.Fatal("retired projectile hit primary twice")
				}
			})
		}
	}
}

func TestCrossfireSplashIndependentOfLethality(t *testing.T) {
	for _, kind := range []string{"magic", "arrow", "suppressed_arrow", "dodged", "disintegrate"} {
		for _, owner := range []ProjectileOwner{ProjectileOwnerPartyAlly, ProjectileOwnerMonsterAtActor} {
			for _, hp := range []int{1, 1000} {
				t.Run(fmt.Sprintf("%s/owner_%d/hp_%d", kind, owner, hp), func(t *testing.T) {
					g := newTestCombatSystemWithConfig(t).game
					source := mkTestMonster("source", 1000)
					source.ID = "source"
					source.Bound = owner == ProjectileOwnerPartyAlly
					target := mkTestMonster("direct", hp)
					target.ID = "direct"
					target.X, target.Y = 64, 64
					target.Bound = owner == ProjectileOwnerMonsterAtActor
					near := mkTestMonster("near", 1000)
					near.ID = "near"
					near.X, near.Y = 100, 64
					near.Bound = target.Bound
					g.world.Monsters = []*monsterPkg.Monster3D{source, target, near}
					for _, m := range g.world.Monsters {
						g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
					}
					const id = "impact"
					if kind == "arrow" || kind == "suppressed_arrow" {
						g.arrows = []Arrow{{ID: id, X: 64, Y: 64, Damage: 10, Active: true, LifeTime: 50, Owner: owner, SourceMonster: source, BowKey: "bow_of_hellfire", DamageType: "fire", SuppressAoE: kind == "suppressed_arrow"}}
					} else {
						g.magicProjectiles = []MagicProjectile{{ID: id, X: 64, Y: 64, Damage: 10, Active: true, LifeTime: 50, Owner: owner, SourceMonster: source, SpellType: "fireball"}}
						if kind == "dodged" {
							target.PerfectDodge = 100
						}
						if kind == "disintegrate" {
							g.magicProjectiles[0].DisintegrateChance = 1
						}
					}
					g.collisionSystem.RegisterEntity(collision.NewEntity(id, 64, 64, 8, 8, collision.CollisionTypeProjectile, false))
					g.combat.CheckProjectileMonsterCollisions()
					wantSplash := kind == "magic" || kind == "arrow"
					if (near.HitPoints < 1000) != wantSplash {
						t.Fatalf("near HP=%d, want splash=%v", near.HitPoints, wantSplash)
					}
					if source.HitPoints != 1000 {
						t.Fatal("crossfire splashed its source faction")
					}
					before := near.HitPoints
					g.combat.CheckProjectileMonsterCollisions()
					if near.HitPoints != before {
						t.Fatal("impact exploded twice")
					}
				})
			}
		}
	}
}

// An aimed shot hits the monster it was aimed at, even inside a stack drawn
// fanned apart. If that target dies in flight, the shot ends on the spot it
// was aimed at: it never flies on to hit what stands behind, a monster still
// on that spot takes it, and a blast goes off there. A target that steps off
// the line lets the shot fly on along its line, never rising, until it meets
// the floor. A live target that stays is the control.
func TestAimedShotHitsOrLands(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name              string
			spell, bolt, dies bool
			aimPartner        bool
			moves             bool
			partner           bool
			targetHit         bool
			partnerHit        bool
			neighbourHit      bool
		}{
			{name: "arrow_alive", targetHit: true},
			{name: "arrow_dead", dies: true},
			{name: "arrow_dead_partner_on_spot", dies: true, partner: true, partnerHit: true},
			{name: "arrow_moved", moves: true},
			{name: "arrow_aimed_at_stacked_partner", partner: true, aimPartner: true, partnerHit: true},
			{name: "bolt_aimed_at_stacked_partner", spell: true, bolt: true, partner: true, aimPartner: true, partnerHit: true},
			{name: "blast_alive", spell: true, targetHit: true, neighbourHit: true},
			{name: "blast_dead", spell: true, dies: true, neighbourHit: true},
		} {
			t.Run(fmt.Sprintf("%s/tb=%v", tc.name, tb), func(t *testing.T) {
				g, ts := summonTileWorld(t)
				g.turnBasedMode = tb
				g.camera.Angle = 0
				mob := func(id string, x, y float64) *monsterPkg.Monster3D {
					m := mkTestMonster("Goblin", 5000) // a medium body: its line meets the floor
					m.ID, m.X, m.Y = id, x, y
					return m
				}
				spotX, spotY := g.camera.X+4*ts, g.camera.Y
				target := mob("target", spotX, spotY)
				partner := mob("partner", spotX, spotY)
				neighbour := mob("neighbour", spotX, spotY+1.5*ts)
				behind := mob("behind", spotX+5*ts, spotY)
				g.world.Monsters = []*monsterPkg.Monster3D{target, neighbour, behind}
				if tc.partner {
					partner.ID = "zz-partner" // auto-aim and an ID tie-break pick the target
					g.world.Monsters = append(g.world.Monsters, partner)
				}
				if tc.aimPartner {
					g.combat.partyAimTarget = partner
				}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				hero := g.party.Members[0]
				g.selectedChar = 0
				hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
				var aimDistance float64
				var launch projectileLaunch
				if tc.spell {
					launch = g.combat.partyProjectileLaunch(0, 10*ts, false)
					aimDistance = launch.aimDistance
					spell, aoe := "fireball", 2.0
					if tc.bolt {
						spell, aoe = "firebolt", 0
					}
					shot := MagicProjectile{ID: g.GenerateProjectileID(spell), X: launch.x, Y: launch.y, VelX: ts / 8,
						Damage: 30, LifeTime: 200, Active: true, SpellType: spell, Owner: ProjectileOwnerPlayer, Attacker: hero, AoeTiles: aoe, Launch: launch}
					g.magicProjectiles = []MagicProjectile{shot}
					g.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, 8, 8, collision.CollisionTypeProjectile, false))
				} else {
					if !g.combat.createArrowAttackAimed(30, items.SlotMainHand, "", 0, false) || len(g.arrows) == 0 {
						t.Fatal("the bow did not shoot")
					}
					launch = g.arrows[0].Launch
					aimDistance = launch.aimDistance
				}
				if aimDistance != 4*ts {
					t.Fatalf("the shot was not aimed at the target: aim distance %g", aimDistance)
				}
				g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
				defer g.threading.Shutdown()
				gl := &GameLoop{game: g}
				farthest := 0.0
				for tick := 0; tick < 600 && g.projectileCount() > 0; tick++ {
					if tick == 2 && tc.dies {
						target.HitPoints = 0 // killed by a faster shot
					}
					if tick == 2 && tc.moves {
						target.Y -= 1.5 * ts // steps off the line
						g.collisionSystem.UpdateEntity(target.ID, target.X, target.Y)
					}
					gl.updateProjectilesAndImpacts()
					for _, a := range g.arrows {
						farthest = max(farthest, a.X-g.camera.X)
					}
					for _, p := range g.magicProjectiles {
						farthest = max(farthest, p.X-g.camera.X)
					}
				}
				hurt := func(m *monsterPkg.Monster3D) bool { return m.HitPoints < m.MaxHitPoints && m.HitPoints > 0 }
				switch {
				case tc.dies:
					if farthest > aimDistance+ts/2 {
						t.Fatalf("the shot flew on %.2f tiles past its fallen target's spot", (farthest-aimDistance)/ts)
					}
				case tc.moves:
					floor := launch.floorDistance
					if floor <= aimDistance+ts || math.Abs(farthest-floor) > ts/2 || hurt(target) {
						t.Fatalf("a missed shot should fly on to the floor at %.2f tiles, flew %.2f (aim %.2f)", floor/ts, farthest/ts, aimDistance/ts)
					}
					g.renderHelper = NewRenderingHelper(g)
					last := 0.0
					for travel := aimDistance; travel <= floor; travel += ts / 4 {
						drop := launch.eyeDrop(g, launch.x+launch.dx*travel, launch.y+launch.dy*travel)
						if drop < last-1e-9 {
							t.Fatalf("the shot rose past its aim point: drop %.3f after %.3f at %.2f tiles", drop/ts, last/ts, travel/ts)
						}
						last = drop
					}
					if eye := launch.eyeDrop(g, launch.x+launch.dx*floor, launch.y+launch.dy*floor); math.Abs(eye-eyeHeightTiles*ts) > 1e-6 {
						t.Fatalf("the shot is drawn %.3f tiles below the eye where it lands, not on the floor", eye/ts)
					}
				case hurt(target) != tc.targetHit:
					t.Fatalf("target hit=%v, want %v", hurt(target), tc.targetHit)
				}
				if hurt(behind) {
					t.Fatal("the shot hit the monster behind its target's spot")
				}
				if tc.partner && hurt(partner) != tc.partnerHit {
					t.Fatalf("partner on the spot hit=%v, want %v", hurt(partner), tc.partnerHit)
				}
				if hurt(neighbour) != tc.neighbourHit {
					t.Fatalf("neighbour hit=%v, want %v", hurt(neighbour), tc.neighbourHit)
				}
			})
		}
	}
}
