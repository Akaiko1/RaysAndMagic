package game

import (
	"fmt"
	"testing"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestAIRegressionPatrolAnchorSurvivesSave(t *testing.T) {
	for _, key := range []string{"karasu_tengu", "kasa_obake"} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", key, tb), func(t *testing.T) {
				g, _, tile := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 30, 30, tile)
				m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, key, g.config)
				homeX, homeY := m.SpawnX, m.SpawnY
				m.X = 14.5 * tile
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				wm := world.NewWorldManager(g.config)
				wm.CurrentMapKey = "forest"
				wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
				old := world.GlobalWorldManager
				world.GlobalWorldManager = wm
				t.Cleanup(func() { world.GlobalWorldManager = old })
				saved := g.buildSave(wm)
				if err := g.applySave(wm, &saved); err != nil {
					t.Fatal(err)
				}
				got := g.world.Monsters[0]
				if got.SpawnX != homeX || got.SpawnY != homeY {
					t.Fatalf("patrol anchor moved on load: (%.1f,%.1f) -> (%.1f,%.1f) tiles", homeX/tile, homeY/tile, got.SpawnX/tile, got.SpawnY/tile)
				}
			})
		}
	}
}

func TestAIRegressionStationaryBroodMotherBothModes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, gl, tile := tbBehaviorGame(t, 40, 40)
			g.turnBasedMode = tb
			placePlayerAtTile(g, 25, 25, tile)
			m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "dragon_brood_mother", g.config)
			if m.Speed != 0 {
				t.Fatal("fixture must use authored zero speed")
			}
			// Isolate locomotion from random summons, without changing movement data.
			m.SummonChance = 0
			m.SummonFirstGuaranteed = false
			m.WasAttacked = true
			m.BeginPlayerEngagement()
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			x, y := m.X, m.Y
			g.refreshMonsterAIState()
			if tb {
				runOneMonsterTurn(g, gl)
			} else {
				for i := 0; i < 20; i++ {
					wr := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
					wr.Update()
					wr.ApplyCollisionUpdate()
				}
			}
			if m.X != x || m.Y != y {
				t.Fatalf("zero-speed brood mother moved (%.1f,%.1f) -> (%.1f,%.1f) tiles", x/tile, y/tile, m.X/tile, m.Y/tile)
			}
		})
	}
}

func TestAIRegressionFleeRespectsSlowBothModes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, gl, tile := tbBehaviorGame(t, 40, 40)
			g.turnBasedMode = tb
			placePlayerAtTile(g, 10, 10, tile)
			m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "kasa_obake", g.config)
			m.State = monster.StateFleeing
			m.WasAttacked = true
			m.ApplySlow(100, 10*g.config.GetTPS(), 10)
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			x, y := m.X, m.Y
			g.refreshMonsterAIState()
			if tb {
				runOneMonsterTurn(g, gl)
			} else {
				wr := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
				wr.Update()
				wr.ApplyCollisionUpdate()
			}
			if m.X != x || m.Y != y {
				t.Fatalf("100%% slowed fleer still moved (%.1f,%.1f) -> (%.1f,%.1f)", x/tile, y/tile, m.X/tile, m.Y/tile)
			}
		})
	}
}

func TestAIRegressionStunnedFlockMemberHoldsPosition(t *testing.T) {
	g, gl, tile := tbBehaviorGame(t, 40, 40)
	g.turnBasedMode = false
	placePlayerAtTile(g, 30, 30, tile)
	leader := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "karasu_tengu", g.config)
	follower := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, "karasu_tengu", g.config)
	leader.ID, follower.ID = "a-leader", "b-follower"
	g.world.Monsters = []*monster.Monster3D{leader, follower}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	gl.updateMonsterBands()
	if follower.BandLeaderID != leader.ID {
		t.Fatal("fixture did not form a flock")
	}
	g.combat.applyStunDR(follower, 3, 3*g.config.GetTPS(), false)
	leader.MoveTargetState = monster.StatePatrolling
	leader.HasMoveTarget = true
	leader.MoveTargetTileX, leader.MoveTargetTileY = 12, 10
	leader.PathTiles = []monster.TileCoord{{X: 10, Y: 10}, {X: 11, Y: 10}, {X: 12, Y: 10}}
	leader.PathIndex = 1
	leader.PathTargetTileX, leader.PathTargetTileY = 12, 10
	x, y := follower.X, follower.Y
	g.refreshMonsterAIState()
	wrappers := g.ConvertMonstersToWrappers()
	for _, wr := range wrappers {
		wr.Update()
	}
	for _, wr := range wrappers {
		wr.(*MonsterWrapper).ApplyCollisionUpdate()
	}
	gl.updateMonsterBands()
	if follower.StunFramesRemaining <= 0 {
		t.Fatal("stun should remain active")
	}
	if follower.X != x || follower.Y != y {
		t.Fatalf("stunned flock member moved %.3f pixels after serial band reconciliation", Distance(x, y, follower.X, follower.Y))
	}
}

func TestAIRegressionChampionOpenerWhenFightingSummon(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, gl, tile := tbBehaviorGame(t, 40, 40)
			g.turnBasedMode = tb
			primeTestChampions(t, g)
			placePlayerAtTile(g, 30, 30, tile)
			m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "wild_druid", g.config)
			m.ChampionTier = "normal"
			m.WasAttacked = true
			m.BeginPlayerEngagement()
			foe := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "skeleton", g.config)
			foe.Bound = true
			foe.HitPoints = 100000
			foe.MaxHitPoints = 100000
			foe.StunTurnsRemaining = 10
			foe.StunFramesRemaining = 10 * g.config.GetTPS()
			g.world.Monsters = []*monster.Monster3D{m, foe}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			g.refreshMonsterAIState()
			if m.AIFoe != foe {
				t.Fatal("summon must be selected foe")
			}
			if tb {
				runOneMonsterTurn(g, gl)
			} else {
				m.State = monster.StateAttacking
				m.StateTimer = 1
				gl.reconcileMonsterAttackPosts()
				g.combat.HandleMonsterInteractions()
			}
			if !m.OpeningSpellDone || m.SoakFrames <= 0 {
				t.Fatalf("champion skipped guaranteed Stone Skin opener against summon; opener=%v soak=%d projectiles=%d/%d", m.OpeningSpellDone, m.SoakFrames, len(g.magicProjectiles), len(g.arrows))
			}
		})
	}
}

func TestAIRegressionHealerKeepsSpecialAgainstSummon(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, redirected := range []bool{false, true} {
			for _, distance := range []int{1, 2} {
				t.Run(fmt.Sprintf("TB=%v/redirected=%v/distance=%d", tb, redirected, distance), func(t *testing.T) {
					g, gl, tile := tbBehaviorGame(t, 40, 40)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 10+distance, 10, tile)
					m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "kitsune_onmyoji", g.config)
					m.AllyHealChance = 1
					m.HitPoints = m.MaxHitPoints - 200
					m.WasAttacked = true
					m.BeginPlayerEngagement()
					g.world.Monsters = []*monster.Monster3D{m}
					if redirected {
						placePlayerAtTile(g, 30, 30, tile)
						foe := monster.NewMonster3DFromConfig((10.5+float64(distance))*tile, 10.5*tile, "skeleton", g.config)
						foe.Bound = true
						foe.StunTurnsRemaining = 10
						foe.StunFramesRemaining = 10 * g.config.GetTPS()
						g.world.Monsters = append(g.world.Monsters, foe)
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					g.refreshMonsterAIState()
					hp := m.HitPoints
					if tb {
						runOneMonsterTurn(g, gl)
					} else {
						m.State = monster.StateAttacking
						m.StateTimer = 1
						gl.reconcileMonsterAttackPosts()
						g.combat.HandleMonsterInteractions()
					}
					if m.HitPoints <= hp {
						t.Fatalf("healer ignored guaranteed eligible self-heal when targeting summon: HP=%d -> %d", hp, m.HitPoints)
					}
				})
			}
		}
	}
}

// Rebased patrol homes are ordinary saved anchors in both mode snapshots.
// TB intentionally does not run ordinary patrol: test that it preserves the
// old home until RT resumes, then persists the fallback home across a reload.
func TestAIRegressionUnreachablePatrolHomeSave(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, wm, tile := travelFixture(t)
			g.turnBasedMode = tb
			placePlayerAtTile(g, 35, 35, tile)
			m := monster.NewMonster3DFromConfig(5.5*tile, 5.5*tile, "bandit", g.config)
			m.SpawnX, m.SpawnY = 20.5*tile, 5.5*tile
			m.TetherRadius = 2 * tile
			m.State = monster.StatePatrolling
			g.world.Tiles[5][20] = world.TileWall
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			if tb {
				runOneMonsterTurn(g, &GameLoop{game: g})
				if m.SpawnX != 20.5*tile {
					t.Fatal("TB unexpectedly ran idle patrol fallback")
				}
				g.turnBasedMode = false
			}
			wr := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
			wr.Update()
			wr.ApplyCollisionUpdate()
			if m.SpawnX != m.X || m.SpawnY != m.Y {
				t.Fatal("RT failed to recover the unreachable patrol home")
			}
			x, y := m.SpawnX, m.SpawnY
			g.turnBasedMode = tb
			saved := auditSaveJSON(t, g.buildSave(wm))
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			m = g.world.Monsters[0]
			if m.SpawnX != x || m.SpawnY != y || !m.IsWithinTetherRadius() {
				t.Fatal("reload restored the unreachable old home")
			}
		})
	}
}
