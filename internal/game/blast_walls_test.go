package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/threading/entities"
	"ugataima/internal/world"
)

// Every party point blast stops at walls: a monster behind a wall tile is
// shielded however close it is, while a neighbour on the blast's side is hit.
// The same layout without the wall is the positive control. Novas (Inferno,
// Earthquake) keep their own area policy and are not point blasts.
func TestPointBlastsStopAtWalls(t *testing.T) {
	sources := []string{"spell_hit", "spell_wall", "spell_range", "arrow_hit", "melee_splash", "death_burst", "flask", "trap"}
	for _, source := range sources {
		for _, walled := range []bool{true, false} {
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/wall=%v/tb=%v", source, walled, tb), func(t *testing.T) {
					g, ts := summonTileWorld(t)
					g.turnBasedMode = tb
					if walled {
						for y := 6; y <= 14; y++ {
							g.world.Tiles[y][15] = world.TileWall
						}
					}
					g.collisionSystem.UpdateTileChecker(g.world)
					cx, cy := 14.85*ts, 10.5*ts
					mob := func(id string, x, y float64) *monsterPkg.Monster3D {
						m := mkTestMonster(id, 5000)
						m.ID, m.X, m.Y = id, x, y
						return m
					}
					struck := mob("struck", cx, cy)
					open := mob("open", cx, cy+0.8*ts)
					shielded := mob("shielded", 16.2*ts, cy)
					g.world.Monsters = []*monsterPkg.Monster3D{struck, open, shielded}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					hero := g.party.Members[0]
					g.selectedChar = 0
					equip := func(key string) *config.WeaponDefinitionConfig {
						hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
						def, ok := config.GetWeaponDefinition(key)
						if !ok {
							t.Fatalf("weapon %s", key)
						}
						return def
					}
					shoot := func(x, vx float64, life int) {
						shot := MagicProjectile{WorldAim: true, ID: g.GenerateProjectileID("fireball"), X: x, Y: cy, VelX: vx,
							Damage: 30, LifeTime: life, Active: true, SpellType: "fireball", Owner: ProjectileOwnerPlayer, Attacker: hero, AoeTiles: 2}
						g.magicProjectiles = []MagicProjectile{shot}
						updater := entities.NewEntityUpdaterWithWorkers(1)
						defer updater.Stop()
						updater.UpdateProjectilesParallel(g.ConvertProjectilesToWrappers(), g.world.CanProjectileMoveTo)
					}
					switch source {
					case "spell_hit":
						shot := MagicProjectile{WorldAim: true, ID: g.GenerateProjectileID("fireball"), X: cx, Y: cy, VelX: 1,
							Damage: 30, LifeTime: 50, Active: true, SpellType: "fireball", Owner: ProjectileOwnerPlayer, Attacker: hero, AoeTiles: 2}
						g.magicProjectiles = []MagicProjectile{shot}
						g.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, 8, 8, collision.CollisionTypeProjectile, false))
						g.combat.CheckProjectileMonsterCollisions()
						if struck.HitPoints >= 5000 {
							t.Fatal("the shot never struck its target")
						}
					case "spell_wall":
						// Flies into the wall; without one it spends its range on the same spot.
						life := 50
						if !walled {
							life = 1
						}
						shoot(cx+0.05*ts, ts/4, life)
					case "spell_range":
						shoot(cx-0.3*ts, -1, 1)
					case "arrow_hit":
						equip("bow_of_hellfire")
						ar := &Arrow{ID: "arrow", Active: true, LifeTime: 60, Damage: 30, BowKey: "bow_of_hellfire", DamageType: "fire", Owner: ProjectileOwnerPlayer, Attacker: hero}
						g.combat.applyProjectileDamage(ar, "arrow", struck, ar.ID)
					case "melee_splash":
						def := equip("idol_breakers_maul")
						g.combat.ApplyDamageToMonster(struck, 30, def.Name, false)
					case "death_burst":
						def := equip("ember_egg_mace")
						struck.HitPoints = 10
						g.combat.ApplyDamageToMonster(struck, 30, def.Name, false)
					case "flask":
						g.combat.detonateFlask(&MagicProjectile{ID: "flask", FlaskKey: "harm_flask", FlaskRadius: 2 * ts, Damage: 30, LifeTime: 10, Active: true, Attacker: hero}, cx, cy)
					case "trap":
						g.combat.fireTrap(&PlacedTrap{Key: "cleave_trap", X: cx, Y: cy, Owner: hero}, struck)
					}
					if open.HitPoints >= 5000 {
						t.Fatalf("the neighbour on the blast's side was not hit (positive control)")
					}
					if got := shielded.HitPoints < 5000; got != !walled {
						t.Fatalf("monster across the wall line hit=%v, want %v (hp %d)", got, !walled, shielded.HitPoints)
					}
				})
			}
		}
	}
}

// Monster blasts obey the same wall rule: an AoE bolt fired by a bound ally,
// by an enemy at bound allies, or by a champion whose splash reaches the party
// never hits what stands behind a wall. Without the wall the same victim is hit
// (positive control), and a neighbour on the blast's side is always hit.
func TestMonsterBlastsStopAtWalls(t *testing.T) {
	for _, source := range []string{"ally_bolt", "enemy_bolt", "champion_party"} {
		for _, walled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/wall=%v", source, walled), func(t *testing.T) {
				g, ts := summonTileWorld(t)
				if walled {
					for y := 6; y <= 14; y++ {
						g.world.Tiles[y][15] = world.TileWall
					}
				}
				g.collisionSystem.UpdateTileChecker(g.world)
				cx, cy := 14.85*ts, 10.5*ts
				mob := func(id string, x, y float64, bound bool) *monsterPkg.Monster3D {
					m := mkTestMonster(id, 5000)
					m.ID, m.X, m.Y, m.Bound = id, x, y, bound
					m.PerfectDodge = 0
					return m
				}
				alliesHit := source != "ally_bolt" // enemies fire at bound allies
				shooter := mob("shooter", 5.5*ts, cy, !alliesHit)
				if source == "champion_party" {
					shooter.ChampionKey = "test_champion"
				}
				target := mob("target", cx, cy, alliesHit)
				open := mob("open", cx, cy+0.8*ts, alliesHit)
				shielded := mob("shielded", 16.2*ts, cy, alliesHit)
				g.world.Monsters = []*monsterPkg.Monster3D{shooter, target, open, shielded}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				owner := ProjectileOwnerMonsterAtBound
				if source == "ally_bolt" {
					owner = ProjectileOwnerBoundUndead
				}
				partyHP := 0
				if source == "champion_party" {
					// The party stands behind the wall, the splash victim the champion aims at.
					placePlayerAtTile(g, 16, 10, ts)
					g.camera.X, g.camera.Y = 16.2*ts, cy
					for _, m := range g.party.Members {
						m.HitPoints, m.MaxHitPoints = 5000, 5000
						partyHP += m.HitPoints
					}
				}
				bolt := &MagicProjectile{ID: "bolt", Active: true, LifeTime: 30, Damage: 30, SpellType: "fireball",
					Owner: owner, SourceMonster: shooter, SourceName: "Shooter", IgnoresDodge: true}
				g.combat.resolveMonsterProjectileVsMonster(bolt, "magic_projectile", target, bolt.ID)
				if target.HitPoints >= 5000 || open.HitPoints >= 5000 {
					t.Fatalf("the bolt or the open-side splash never landed (target %d, open %d)", target.HitPoints, open.HitPoints)
				}
				if source == "champion_party" {
					after := 0
					for _, m := range g.party.Members {
						after += m.HitPoints
					}
					if got := after < partyHP; got != !walled {
						t.Fatalf("party behind the wall line hit=%v, want %v", got, !walled)
					}
					return
				}
				if got := shielded.HitPoints < 5000; got != !walled {
					t.Fatalf("monster across the wall line hit=%v, want %v (hp %d)", got, !walled, shielded.HitPoints)
				}
			})
		}
	}
}
