package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Placement is synchronous; no scatter reservation survives a tick or save.
// Cross both modes, member orders, movement holds and each scatter caller.
func TestScatterReservesMembersThatStayInPlace(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, trigger := range []string{"hit", "sight", "death", "guard_hit", "guard_sight"} {
			for _, hold := range []string{"root", "stun", "stationary", "slow", "fighting"} {
				if hold == "fighting" && trigger != "death" {
					continue // Only death scatter excludes already-fighting survivors.
				}
				for _, pinnedFirst := range []bool{false, true} {
					t.Run(fmt.Sprintf("TB=%v/%s/%s/pinnedFirst=%v", tb, trigger, hold, pinnedFirst), func(t *testing.T) {
						g, gl, tile := tbBehaviorGame(t, 40, 40)
						g.turnBasedMode, g.gameLoop = tb, gl
						placePlayerAtTile(g, 30, 30, tile)
						a := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "karasu_tengu", g.config)
						guard := strings.HasPrefix(trigger, "guard_")
						key := "karasu_tengu"
						if guard {
							key = "kasa_obake"
						}
						b := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, key, g.config)
						a.ID, b.ID = "a", "b"
						g.world.Monsters = []*monster.Monster3D{a, b}
						var victim *monster.Monster3D
						if trigger == "death" {
							victim = monster.NewMonster3DFromConfig(10.7*tile, 10.5*tile, "karasu_tengu", g.config)
							victim.ID = "victim"
							g.world.Monsters = append(g.world.Monsters, victim)
						}
						g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
						if guard {
							a.LootGuarding, b.LootGuarding = true, true
							gl.stackMonsterBand(1, g.world.Monsters)
						} else {
							gl.updateMonsterBands()
						}
						if a.BandID == 0 || a.BandID != b.BandID {
							t.Fatal("setup: expected a stacked band")
						}
						if guard {
							if !lootGuardBandIsStacked(g.world.Monsters) {
								t.Fatal("setup: expected a stacked guard pair")
							}
						}
						pinned, mobile := b, a
						if pinnedFirst {
							pinned, mobile = a, b
						}
						switch hold {
						case "root":
							g.combat.applyMonsterRoot(pinned, 3, 3*g.config.GetTPS())
						case "stun":
							g.combat.applyStunDR(pinned, 3, 3*g.config.GetTPS(), false)
						case "stationary":
							pinned.Speed = 0
						case "slow":
							pinned.ApplySlow(100, 3*g.config.GetTPS(), 3)
						case "fighting":
							pinned.BeginPlayerEngagement()
						}
						x, y := pinned.X, pinned.Y
						placePlayerAtTile(g, 11, 10, tile)
						switch trigger {
						case "hit", "guard_hit":
							mobile.WasAttacked = true
							mobile.BeginPlayerEngagement()
						case "sight", "guard_sight":
							mobile.BeginPlayerEngagement()
						}
						switch trigger {
						case "death":
							victim.HitPoints = 0
							g.collisionSystem.UnregisterEntity(victim.ID)
							g.combat.scatterBandOnMemberDeath(victim)
						case "guard_hit", "guard_sight":
							gl.scatterLootGuardBand(g.world.Monsters, false)
						default:
							gl.updateMonsterBands()
						}
						if pinned.X != x || pinned.Y != y {
							t.Fatal("scatter moved the member that must stay in place")
						}
						if tileOf(pinned, tile) == tileOf(mobile, tile) {
							t.Fatal("mobile member took the reserved tile")
						}
						if mobile.BandID != 0 || mobile.LootGuarding || !mobile.IsEngagingPlayer {
							t.Fatal("scatter lost band dissolution or engagement")
						}
						wantHit := trigger == "hit" || trigger == "guard_hit" || trigger == "death"
						if mobile.WasAttacked != wantHit {
							t.Fatal("scatter changed hit versus sight semantics")
						}
						if hold == "root" {
							for _, m := range []*monster.Monster3D{a, b} {
								m.State, m.StateTimer = monster.StateAttacking, 1
							}
							g.refreshMonsterAIState()
							gl.reconcileMonsterAttackPosts()
							cadence := monsterAttackRealtime
							if tb {
								cadence = monsterAttackTurn
							}
							if !g.combat.commitMonsterAttack(pinned, monsterAttackDestination{}, cadence) {
								t.Fatal("rooted member lost the ability to attack from its reserved tile")
							}
						}
					})
				}
			}
		}
	}
}

func TestGuardDeathScatterDoesNotReserveDeadMemberTile(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, deadFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/deadFirst=%v", tb, deadFirst), func(t *testing.T) {
				g, gl, tile, _, a, b := setupLootGuardPair(t, tb)
				for _, m := range []*monster.Monster3D{a, b} {
					m.X, m.Y = TileCenterFromTile(a.LootGuardMoveTileX, a.LootGuardMoveTileY, tile)
					g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
				}
				gl.reconcileLootPropGuardBands()
				victim, survivor := b, a
				if deadFirst {
					victim, survivor = a, b
				}
				x, y := survivor.X, survivor.Y
				victim.HitPoints = 0
				g.collisionSystem.UnregisterEntity(victim.ID)
				g.combat.scatterBandOnMemberDeath(victim)
				if survivor.X != x || survivor.Y != y {
					t.Fatal("dead guard unnecessarily reserved the survivor's tile")
				}
				if survivor.LootGuarding || survivor.BandID != 0 || !survivor.WasAttacked {
					t.Fatal("surviving guard did not leave its post and engage")
				}
			})
		}
	}
}

func TestScatterWithoutFreeTileKeepsPinnedMemberInPlace(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, pinnedFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/pinnedFirst=%v", tb, pinnedFirst), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 30, 30, tile)
				a := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "karasu_tengu", g.config)
				b := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, "karasu_tengu", g.config)
				a.ID, b.ID = "a", "b"
				g.world.Monsters = []*monster.Monster3D{a, b}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				gl.updateMonsterBands()
				pinned, mobile := b, a
				if pinnedFirst {
					pinned, mobile = a, b
				}
				x, y := pinned.X, pinned.Y
				g.combat.applyMonsterRoot(pinned, 3, 3*g.config.GetTPS())
				for _, d := range bandScatterRing[1:] {
					g.world.Tiles[10+d[1]][10+d[0]] = world.TileWall
				}
				mobile.WasAttacked = true
				mobile.BeginPlayerEngagement()
				gl.updateMonsterBands()
				if pinned.X != x || pinned.Y != y || mobile.X != x || mobile.Y != y {
					t.Fatal("scatter without a free tile forced a relocation")
				}
				if pinned.BandID != 0 || mobile.BandID != 0 || !pinned.WasAttacked {
					t.Fatal("blocked placement prevented band dissolution or hit propagation")
				}
			})
		}
	}
}
