package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
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
		for _, owner := range []ProjectileOwner{ProjectileOwnerBoundUndead, ProjectileOwnerMonsterAtBound} {
			for _, hp := range []int{1, 1000} {
				t.Run(fmt.Sprintf("%s/owner_%d/hp_%d", kind, owner, hp), func(t *testing.T) {
					g := newTestCombatSystemWithConfig(t).game
					source := mkTestMonster("source", 1000)
					source.ID = "source"
					source.Bound = owner == ProjectileOwnerBoundUndead
					target := mkTestMonster("direct", hp)
					target.ID = "direct"
					target.X, target.Y = 64, 64
					target.Bound = owner == ProjectileOwnerMonsterAtBound
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
