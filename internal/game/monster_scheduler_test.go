package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

func TestMonsterWorkerPublishesCapturedFrameOnce(t *testing.T) {
	for _, attacking := range []bool{false, true} {
		t.Run(fmt.Sprintf("attacking=%v", attacking), func(t *testing.T) {
			g, _, tile := tbBehaviorGame(t, 20, 20)
			g.turnBasedMode = false
			placePlayerAtTile(g, 10, 10, tile)
			x := 13
			if attacking {
				x = 11
			}
			m := hostileMonsterAt(g, x, 10, tile)
			m.State = monster.StatePursuing
			if attacking {
				m.State = monster.StateAttacking
			}
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			g.refreshMonsterAIState()
			// Start without a published post; only serial application can publish it.
			g.releaseMonsterAttackPost(m)
			if attacking {
				m.State = monster.StateAttacking
			}
			g.frameCount = 71
			wrapper := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
			oldX, oldY := m.X, m.Y
			oldEntity := *g.collisionSystem.GetEntityByID(m.ID)
			// A worker must use the captured observation, not this later camera/tick.
			g.camera.X, g.camera.Y, g.frameCount = tile, tile, 999
			wrapper.Update()
			if m.X != oldX || m.Y != oldY || m.AttackPost {
				t.Fatal("worker published movement or post before the barrier")
			}
			if got := g.collisionSystem.GetEntityByID(m.ID); *got != oldEntity {
				t.Fatal("worker changed the live collision entity")
			}
			wrapper.ApplyCollisionUpdate()
			if attacking {
				if !m.AttackPost || m.AttackPostSince != 71 || m.AttackPostTargetID != partyAttackTargetID {
					t.Fatalf("post did not use captured camera/tick: %+v", m.AttackPostSince)
				}
			} else if m.X >= oldX {
				t.Fatalf("captured target pursuit did not move toward the party: %.1f -> %.1f", oldX, m.X)
			}
			m.X, m.Y = 7*tile, 7*tile
			wrapper.ApplyCollisionUpdate()
			if m.X != 7*tile || m.Y != 7*tile {
				t.Fatal("a second apply replayed stale movement")
			}
		})
	}
}

func TestMonsterSchedulerBarrierPostArbitration(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			g, gl, tile := tbBehaviorGame(t, 20, 20)
			g.turnBasedMode = false
			placePlayerAtTile(g, 10, 10, tile)
			for i := 0; i < 12; i++ {
				m := hostileMonsterAt(g, 11+i%2, 10, tile)
				m.ID = fmt.Sprintf("actor-%02d", i)
				m.State = monster.StatePursuing
				g.world.Monsters = append(g.world.Monsters, m)
			}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			updater := entities.NewEntityUpdaterWithWorkers(workers)
			defer updater.Stop()
			for frame := 0; frame < 30; frame++ {
				g.frameCount++
				g.refreshMonsterAIState()
				gl.reconcileMonsterAttackPosts()
				updater.UpdateMonstersParallel(g.ConvertMonstersToWrappers())
				gl.reconcileMonsterAttackPosts()
				posts := map[[2]int]bool{}
				for _, m := range g.world.Monsters {
					e := g.collisionSystem.GetEntityByID(m.ID)
					if e == nil || e.Solid {
						t.Fatal("missing or physically solid monster")
					}
					if m.AttackPost {
						key := [2]int{int(m.X / tile), int(m.Y / tile)}
						if posts[key] || e.CollisionType != collision.CollisionTypeMonsterEngaged {
							t.Fatalf("conflicting or unpublished post at %v", key)
						}
						posts[key] = true
					}
				}
				if len(posts) == 0 {
					t.Fatal("no contender received an attack post")
				}
			}
		})
	}
}

func TestMonsterCommitRevalidatesBehaviorBothModes(t *testing.T) {
	for _, cadence := range []monsterAttackCadence{monsterAttackRealtime, monsterAttackTurn} {
		for _, tc := range []struct {
			name    string
			apply   func(*monster.Monster3D)
			allowed bool
		}{
			{"inert", func(m *monster.Monster3D) { m.WarlordIdol = true }, false},
			{"pacified", func(m *monster.Monster3D) { m.Pacified = true }, false},
			{"evasive", func(m *monster.Monster3D) { m.BossEvasive = true }, false},
			{"bound_without_foe", func(m *monster.Monster3D) { m.Bound = true }, false},
			{"fleeing", func(m *monster.Monster3D) { m.State = monster.StateFleeing }, false},
			{"passive", func(m *monster.Monster3D) { m.PassiveUntilAttacked = true; m.WasAttacked = false }, false},
			{"fight_foe_cannot_hit_party", func(m *monster.Monster3D) { m.AIFoe = &monster.Monster3D{HitPoints: 1} }, false},
			{"relentless", func(m *monster.Monster3D) { m.Relentless = true }, true},
			{"pursuit", func(m *monster.Monster3D) {}, true},
			{"dead", func(m *monster.Monster3D) { m.HitPoints = 0 }, false},
			{"stunned", func(m *monster.Monster3D) { m.StunFramesRemaining = 3; m.StunTurnsRemaining = 1 }, false},
			{"same_tile", func(m *monster.Monster3D) { m.X -= 64 }, false},
		} {
			t.Run(fmt.Sprintf("%d/%s", cadence, tc.name), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 20, 20)
				g.turnBasedMode = cadence == monsterAttackTurn
				placePlayerAtTile(g, 10, 10, tile)
				m := hostileMonsterAt(g, 11, 10, tile)
				m.State, m.StateTimer = monster.StateAttacking, 1
				tc.apply(m)
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				before := partyHPSum(g)
				if cadence == monsterAttackTurn {
					gl.monsterAttackTurnBased(m)
				} else {
					g.combat.HandleMonsterInteractions()
				}
				if (m.AttackCDFrames > 0) != tc.allowed {
					t.Fatalf("attack spent=%v, want %v", m.AttackCDFrames > 0, tc.allowed)
				}
				if !tc.allowed && partyHPSum(g) != before {
					t.Fatal("rejected action damaged the party")
				}
			})
		}
	}
}

func TestMonsterCommitRevalidatesFoeBothModes(t *testing.T) {
	for _, cadence := range []monsterAttackCadence{monsterAttackRealtime, monsterAttackTurn} {
		for _, bound := range []bool{false, true} {
			for _, state := range []string{"live", "dead", "changed_identity", "changed_control", "out_of_reach", "occupied_post"} {
				t.Run(fmt.Sprintf("%d/bound=%v/%s", cadence, bound, state), func(t *testing.T) {
					g, gl, tile := tbBehaviorGame(t, 20, 20)
					g.turnBasedMode = cadence == monsterAttackTurn
					placePlayerAtTile(g, 2, 2, tile)
					m, foe := hostileMonsterAt(g, 10, 10, tile), hostileMonsterAt(g, 11, 10, tile)
					m.Bound, foe.Bound = bound, !bound
					m.AIFoe, m.AITargetX, m.AITargetY = foe, foe.X, foe.Y
					m.State, m.StateTimer = monster.StateAttacking, 1
					foe.HitPoints, foe.MaxHitPoints = 5000, 5000
					g.world.Monsters = []*monster.Monster3D{m, foe}
					switch state {
					case "dead":
						foe.HitPoints = 0
					case "changed_identity":
						m.AIFoe = &monster.Monster3D{HitPoints: 1}
					case "changed_control":
						foe.Bound = bound
					case "out_of_reach":
						foe.X += 5 * tile
					case "occupied_post":
						holder := hostileMonsterAt(g, 10, 10, tile)
						holder.AttackPost, holder.AttackPostTargetID = true, foe.ID
						holder.State = monster.StateAttacking
						g.world.Monsters = append(g.world.Monsters, holder)
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					if state == "occupied_post" {
						g.applyMonsterCollisionType(g.world.Monsters[2].ID, collision.CollisionTypeMonsterEngaged)
					}
					before := foe.HitPoints
					spent := false
					if cadence == monsterAttackTurn {
						spent = gl.tryMonsterAttackFoeTurnBased(m, foe)
					} else {
						spent = g.combat.commitMonsterAttack(m, monsterAttackDestination{foe: foe}, cadence)
					}
					if spent != (state == "live") {
						t.Fatalf("spent=%v for %s", spent, state)
					}
					if state != "live" && (m.AttackCDFrames != 0 || foe.HitPoints != before) {
						t.Fatal("rejected foe action spent cooldown or dealt damage")
					}
				})
			}
		}
	}
}
