package game

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Cross ground species, foreground RT/TB, and off-screen predator threats.
// Foreground party/predator selection differs; remote worlds have no party.
func TestWildlifeEscapeRoaming(t *testing.T) {
	for _, key := range []string{"fennec", "desert_rabbit", "ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, mode := range []string{"party_RT", "party_TB", "predator_RT", "predator_TB", "remote_RT", "remote_TB"} {
			t.Run(key+"/"+mode, func(t *testing.T) {
				g, wm, tile := ecologyTestGame(t)
				turn := mode == "party_TB" || mode == "predator_TB" || mode == "remote_TB"
				remote := mode == "remote_RT" || mode == "remote_TB"
				g.turnBasedMode = turn
				placePlayerAtTile(g, 9, 10, tile)
				tx, ty := g.camera.X, g.camera.Y
				m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, key, g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				if mode != "party_RT" && mode != "party_TB" {
					predator := monster.NewMonster3DFromConfig(tx, ty, "fennec", g.config)
					predator.Prey = []string{key}
					predator.RootFramesRemaining, predator.RootTurnsRemaining = 100000, 100000
					g.world.Monsters = append(g.world.Monsters, predator)
					placePlayerAtTile(g, 1, 1, tile)
				}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				if remote {
					g.world = newTestWorldSized(g.config, 30, 30)
					wm.LoadedMaps["other"], wm.CurrentMapKey = g.world, "other"
				}
				steps := g.config.GetTPS() * 12
				if turn {
					steps = 50
				}
				safe, walked := false, 0.0
				for i := 0; i < steps; i++ {
					x, y := m.X, m.Y
					if remote {
						g.frameCount++
						g.simulateRemoteEcology(turn, true)
					} else {
						advanceLemurThroughScheduler(g, m, turn)
					}
					d := Distance(m.X, m.Y, tx, ty)
					if safe {
						walked += Distance(x, y, m.X, m.Y)
						if d+1e-6 < m.AlertRadius+tile || m.AIFoe != nil {
							t.Fatal("safe roaming returned inside the threat clearance or started hunting")
						}
					} else if d >= m.AlertRadius+tile {
						safe = true
					}
				}
				if !safe || walked < tile {
					t.Fatalf("wildlife did not continue roaming after escape: safe=%v walked=%.2f tiles", safe, walked/tile)
				}
			})
		}
	}
}

func TestWildlifeThreatMemoryAndMovementHolds(t *testing.T) {
	for _, key := range []string{"fennec", "desert_rabbit", "ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			for _, scenario := range []string{"occlusion", "gone", "root", "slow", "blocked", "approach"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, turn, scenario), func(t *testing.T) {
					g, _, tile := ecologyTestGame(t)
					g.turnBasedMode = turn
					placePlayerAtTile(g, 9, 10, tile)
					m := monster.NewMonster3DFromConfig(15.5*tile, 10.5*tile, key, g.config)
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					advanceLemurThroughScheduler(g, m, turn)
					if !m.AmbientFlee || m.Threat.Seconds <= 0 {
						t.Fatal("threat not remembered")
					}
					x, y := m.X, m.Y
					switch scenario {
					case "occlusion":
						for y := range g.world.Tiles {
							g.world.Tiles[y][11] = world.TileWall
						}
					case "gone":
						placePlayerAtTile(g, 1, 1, tile)
					case "approach":
						placePlayerAtTile(g, 12, 10, tile)
					case "root", "slow":
						placePlayerAtTile(g, 12, 10, tile)
						if scenario == "root" {
							m.RootFramesRemaining, m.RootTurnsRemaining = 100000, 100000
						} else {
							m.ApplySlow(100, 100000, 100000)
						}
					case "blocked":
						placePlayerAtTile(g, 12, 10, tile)
						m.AmbientBounds = &[4]int{15, 10, 16, 11}
					}
					advanceLemurThroughScheduler(g, m, turn)
					if scenario == "approach" {
						// Slow species can spend a TB update accumulating move credit.
						advanceLemurThroughScheduler(g, m, turn)
						if m.X == x && m.Y == y {
							t.Fatal("approaching threat did not restart escape")
						}
						return
					}
					if !m.AmbientFlee {
						t.Fatal("brief occlusion discarded the threat")
					}
					if scenario == "root" || scenario == "slow" || scenario == "blocked" {
						if m.X != x || m.Y != y {
							t.Fatal("held wildlife moved")
						}
						if (&Renderer{game: g}).shouldAnimateMonster(m) {
							t.Fatal("stationary wildlife kept playing its walking cycle")
						}
					}
					if scenario == "occlusion" || scenario == "gone" {
						for i := 0; i < g.config.GetTPS()*4 && m.Threat.Seconds > 0; i++ {
							advanceLemurThroughScheduler(g, m, turn)
						}
						if m.Threat.Seconds != 0 || m.AmbientFlee || m.SpawnX != m.X || m.SpawnY != m.Y {
							t.Fatal("lost threat did not release wildlife at its refuge")
						}
						x, y = m.X, m.Y
						moved := false
						for i := 0; i < 200; i++ {
							advanceLemurThroughScheduler(g, m, turn)
							moved = moved || m.X != x || m.Y != y
						}
						if !moved {
							t.Fatal("wildlife never resumed roaming")
						}
					}
				})
			}
		}
	}
}

func TestWildlifeThreatSaveRoundTrip(t *testing.T) {
	for _, unified := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("open=%v/legacy=%v", unified, legacy), func(t *testing.T) {
				t.Chdir("../..")
				g, wm, cfg := bootOpenWorldGame(t, unified)
				if err := g.switchToMap("desert"); err != nil {
					t.Fatal(err)
				}
				x, y := wm.ProjectWorldPos("desert", 15.5*cfg.GetTileSize(), 10.5*cfg.GetTileSize())
				tx, ty := wm.ProjectWorldPos("desert", 9.5*cfg.GetTileSize(), 10.5*cfg.GetTileSize())
				m := monster.NewMonster3DFromConfig(x, y, "fennec", cfg)
				g.camera.X, g.camera.Y = tx, ty
				m.RememberAmbientThreat(tx, ty)
				m.Threat.DetourClearance = 2 * cfg.GetTileSize()
				g.world.Monsters = []*monster.Monster3D{m}
				save := g.buildSave(wm)
				if legacy {
					for i := range save.Monsters {
						save.Monsters[i].AmbientThreat = monster.AmbientThreat{}
					}
					for _, list := range save.MapMonsters {
						for i := range list {
							list[i].AmbientThreat = monster.AmbientThreat{}
						}
					}
				} else {
					local := save.MapMonsters["desert"][0].AmbientThreat
					if math.Abs(local.X-9.5*cfg.GetTileSize()) > 1e-6 || math.Abs(local.Y-10.5*cfg.GetTileSize()) > 1e-6 {
						t.Fatal("threat saved in the wrong coordinate space")
					}
				}
				data, err := json.Marshal(save)
				if err != nil {
					t.Fatal(err)
				}
				var loaded GameSave
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatal(err)
				}
				if err := g.applySave(wm, &loaded); err != nil {
					t.Fatal(err)
				}
				got := g.world.Monsters[0]
				if legacy {
					if got.Threat.Seconds != 0 {
						t.Fatal("legacy save invented a remembered threat")
					}
				} else if !got.AmbientFlee || got.Threat != m.Threat {
					t.Fatalf("escape memory changed on reload: %+v -> %+v", m.Threat, got.Threat)
				}
				x, y = got.X, got.Y
				for i := 0; i < g.config.GetTPS(); i++ {
					advanceLemurThroughScheduler(g, got, false)
				}
				if got.X == x && got.Y == y {
					t.Fatal("reloaded wildlife stayed frozen beside the party")
				}
			})
		}
	}
}

func TestWildlifeEscapeRevalidation(t *testing.T) {
	for _, key := range []string{"fennec", "desert_rabbit", "ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			for _, scenario := range []string{"blocked_goal", "threat_ahead"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, turn, scenario), func(t *testing.T) {
					g, _, tile := ecologyTestGame(t)
					g.turnBasedMode = turn
					placePlayerAtTile(g, 9, 10, tile)
					m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, key, g.config)
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					for i := 0; i < 3 && !m.HasMoveTarget; i++ {
						advanceLemurThroughScheduler(g, m, turn)
					}
					if !m.HasMoveTarget {
						t.Fatal("no initial escape goal")
					}
					gx, gy := m.MoveTargetTileX, m.MoveTargetTileY
					if scenario == "blocked_goal" {
						g.collisionSystem.RegisterEntity(collision.NewEntity("escape-blocker", (float64(gx)+.5)*tile, (float64(gy)+.5)*tile, tile*.8, tile*.8, collision.CollisionTypeNPC, true))
					} else {
						nx, ny, ok := m.NextPathStepTileToAny(g.collisionSystem, []monster.TileCoord{{X: gx, Y: gy}}, nil)
						if !ok {
							t.Fatal("no initial escape route")
						}
						placePlayerAtTile(g, nx, ny, tile)
					}
					x, y := m.X, m.Y
					before := Distance(x, y, g.camera.X, g.camera.Y)
					steps := 10
					if !turn {
						steps = g.config.GetTPS()
					}
					for i := 0; i < steps; i++ {
						advanceLemurThroughScheduler(g, m, turn)
						if scenario == "threat_ahead" && Distance(m.X, m.Y, g.camera.X, g.camera.Y)+1e-6 < before {
							t.Fatal("cached route moved toward the approaching threat")
						}
					}
					if Distance(x, y, m.X, m.Y) < tile*.1 {
						t.Fatal("wildlife failed to choose an available escape route")
					}
				})
			}
		}
	}
}

// A reachable exit must not disappear merely because its first step is inward.
func TestWildlifeEscapeDetourLifecycle(t *testing.T) {
	for _, key := range []string{"fennec", "desert_rabbit", "ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, mode := range []string{"party_RT", "party_TB", "predator_RT", "predator_TB", "remote_RT", "remote_TB"} {
			for _, gap := range []int{3, 6} {
				t.Run(fmt.Sprintf("%s/%s/gap=%d", key, mode, gap), func(t *testing.T) {
					g, wm, tile := ecologyTestGame(t)
					turn := mode == "party_TB" || mode == "predator_TB" || mode == "remote_TB"
					remote := mode == "remote_RT" || mode == "remote_TB"
					g.turnBasedMode = turn
					placePlayerAtTile(g, 12-gap, 10, tile)
					tx, ty := g.camera.X, g.camera.Y
					m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, key, g.config)
					g.world.Monsters = []*monster.Monster3D{m}
					if mode != "party_RT" && mode != "party_TB" {
						predator := monster.NewMonster3DFromConfig(tx, ty, "fennec", g.config)
						predator.Prey = []string{key}
						predator.RootFramesRemaining, predator.RootTurnsRemaining = 100000, 100000
						g.world.Monsters = append(g.world.Monsters, predator)
						placePlayerAtTile(g, 1, 1, tile)
					}
					g.world.Tiles[10][13], g.world.Tiles[9][12], g.world.Tiles[11][12] = world.TileWall, world.TileWall, world.TileWall
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					if !m.HasPathToTile(g.collisionSystem, 16, 8) {
						t.Fatal("fixture has no escape route")
					}
					if remote {
						g.world = newTestWorldSized(g.config, 30, 30)
						wm.LoadedMaps["other"], wm.CurrentMapKey = g.world, "other"
					}
					detoured, escaped := false, false
					steps := g.config.GetTPS() * 8
					if turn {
						steps = 25
					}
					for i := 0; i < steps; i++ {
						if remote {
							g.frameCount++
							g.simulateRemoteEcology(turn, true)
						} else {
							advanceLemurThroughScheduler(g, m, turn)
						}
						d := Distance(m.X, m.Y, tx, ty)
						detoured = detoured || d < float64(gap)*tile-.01
						escaped = escaped || (detoured && d >= m.AlertRadius+tile)
						if m.Threat.Seconds > 0 && d < float64(gap-1)*tile-1e-6 {
							t.Fatal("escape spent more than its initial one-tile detour budget")
						}
					}
					if !detoured || !escaped {
						t.Fatalf("reachable pocket exit not completed: detoured=%v escaped=%v", detoured, escaped)
					}
				})
			}
		}
	}
}

func TestWildlifeDetourSaveAndThreatChange(t *testing.T) {
	for _, turn := range []bool{false, true} {
		for _, scenario := range []string{"reload", "moving_threat"} {
			t.Run(fmt.Sprintf("TB=%v/%s", turn, scenario), func(t *testing.T) {
				g, _, tile := ecologyTestGame(t)
				g.turnBasedMode = turn
				placePlayerAtTile(g, 9, 10, tile)
				m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "fennec", g.config)
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.Tiles[10][13], g.world.Tiles[9][12], g.world.Tiles[11][12] = world.TileWall, world.TileWall, world.TileWall
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				advanceLemurThroughScheduler(g, m, turn)
				if m.Threat.DetourClearance == 0 {
					t.Fatal("fixture did not start a detour")
				}
				if scenario == "reload" {
					threat := m.Threat
					m = restoreLemurForTest(t, g)
					if m.Threat != threat {
						t.Fatal("reload changed the original escape budget")
					}
				} else {
					placePlayerAtTile(g, 10, 11, tile)
					g.prepareAmbientTarget(m)
					if m.Threat.DetourClearance != 0 || m.HasMoveTarget {
						t.Fatal("moving threat kept the old detour route and clearance")
					}
				}
				x, y := m.X, m.Y
				steps := g.config.GetTPS() * 4
				if turn {
					steps = 12
				}
				for i := 0; i < steps; i++ {
					advanceLemurThroughScheduler(g, m, turn)
				}
				if Distance(x, y, m.X, m.Y) < tile {
					t.Fatal("detour did not resume after lifecycle transition")
				}
			})
		}
	}
}

func TestLemurEscapeRefugeAfterPatrol(t *testing.T) {
	for _, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			for _, reload := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/TB=%v/reload=%v", key, turn, reload), func(t *testing.T) {
					g, m, tile := lemurTestGame(t, key)
					g.turnBasedMode = turn
					placePlayerAtTile(g, 10, 10, tile)
					g.camera.X -= tile * .1
					// A valid pending ground patrol predates this canopy escape.
					m.State, m.MoveTargetState = monster.StatePatrolling, monster.StatePatrolling
					m.HasMoveTarget = true
					m.MoveTargetTileX, m.MoveTargetTileY = 12, 11
					landed, walked := false, 0.0
					steps := g.config.GetTPS() * 15
					if turn {
						steps = 50
					}
					for i := 0; i < steps; i++ {
						x, y := m.X, m.Y
						advanceLemurThroughScheduler(g, m, turn)
						if landed {
							walked += Distance(x, y, m.X, m.Y)
						}
						if !landed && m.Arbor.Phase == "" && Distance(m.X, m.Y, g.camera.X, g.camera.Y) >= m.AlertRadius+tile {
							landed = true
							// Keep the party visible beside the tree after landing. The old
							// patrol home is now unsafe and outside the lemur's tether.
							g.camera.X, g.camera.Y = 13.5*tile, 16*tile
							if !g.collisionSystem.CheckLineOfSight(m.X, m.Y, g.camera.X, g.camera.Y) {
								t.Fatal("landing threat is occluded")
							}
							if reload {
								m = restoreLemurForTest(t, g)
							}
						}
					}
					if !landed || walked < tile {
						t.Fatalf("lemur failed to resume roaming at its refuge: landed=%v walked=%.3f", landed, walked/tile)
					}
				})
			}
		}
	}
}
