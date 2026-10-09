package game

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestSolsticeSlowActionFloorAndRecovery(t *testing.T) {
	g, _ := newSpecialsTestGame(t)
	g.turnBasedMode, g.currentTurn = true, 0
	hero := g.party.Members[0]
	gun, err := items.TryCreateWeaponFromYAML("suppressor_gun")
	if err != nil {
		t.Fatal(err)
	}
	hero.Equipment[items.SlotMainHand] = gun
	for _, member := range g.party.Members {
		member.Speed = -1000
	}
	g.partyHinder.Slow = 1
	g.startPartyTurn()
	if hero.ActionsRemaining < 1 || hero.TBRoundActionFloor > hero.ActionsRemaining {
		t.Fatal("slow left an uncredited equipment floor")
	}
	delete(hero.Equipment, items.SlotMainHand)
	g.reconcileTBRoundActionFloorAfterEquipmentChange(0)
	if hero.ActionsRemaining != 1 || hero.TBRoundActionFloor != 1 {
		t.Fatalf("gear swap charged slow twice: actions=%d floor=%d", hero.ActionsRemaining, hero.TBRoundActionFloor)
	}
	if got := g.quickenRecovery(hero, 60); got != 80 {
		t.Fatalf("slowed recovery=%d", got)
	}
	if got := g.partyConditionLabel(hero); got != "Slowed" {
		t.Fatalf("condition label=%s", got)
	}
}

func TestSolsticeTelegraphCrossfire(t *testing.T) {
	for _, key := range []string{"ember_elemental", "tide_elemental", "jade_elemental", "transfer_automaton"} {
		for _, tb := range []bool{false, true} {
			for _, outcome := range []string{"hit", "escaped", "allegiance-changed", "caster-bound"} {
				t.Run(fmt.Sprintf("%s/TB%v/%s", key, tb, outcome), func(t *testing.T) {
					g, gl := newSpecialsTestGame(t)
					g.gameLoop, g.turnBasedMode = gl, tb
					placePlayerAtTile(g, 2, 2, 64)
					m := spawnSpecialsMonster(g, "solstice_"+key, 8, 6)
					foe := spawnSpecialsMonster(g, "goblin", 6, 6)
					foe.Bound, foe.BoundFramesRemaining = true, 100000
					foe.HitPoints, foe.MaxHitPoints, foe.Speed = 10000, 10000, 0
					foe.AttackCDFrames = 100000
					m.State, m.StateTimer = monster.StateAttacking, 1
					g.refreshMonsterAIState()
					m.AIFoe = foe
					before := partyHPSum(g)
					if tb {
						runTBMonsterTurns(g, gl, 1)
					} else {
						g.combat.HandleMonsterInteractions()
					}
					if m.Telegraph.TargetID != foe.ID || m.Telegraph.Warning == 0 {
						t.Fatal("crossfire warning did not lock its actor")
					}
					switch outcome {
					case "escaped":
						foe.X, foe.Y = 6.5*64, 9.5*64
					case "allegiance-changed":
						foe.Bound, foe.BoundFramesRemaining = false, 0
					case "caster-bound":
						m.Bound, m.BoundFramesRemaining = true, 100000
					}
					g.refreshMonsterAIState()
					a := telegraphDefinition(m)
					g.tickMonsterTelegraphs(a.WarningSeconds, tb)
					if (foe.HitPoints < 10000) != (outcome == "hit") {
						t.Fatalf("foe HP=%d", foe.HitPoints)
					}
					if partyHPSum(g) != before {
						t.Fatal("actor cast retargeted the party")
					}
				})
			}
		}
	}
}

func TestSolsticeForcedArrivalSafety(t *testing.T) {
	for _, entry := range []string{"push", "transfer"} {
		for _, hazard := range []string{"clear", "wall", "occupied", "occupation", "trap", "transfer", "lane-idle", "lane-warning", "lane-live", "disabled", "monster-warning", "monster-field"} {
			t.Run(entry+"/"+hazard, func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				g.gameLoop = gl
				world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{}, LoadedMaps: map[string]*world.World3D{"forest": g.world}}
				a := &config.AdventureConfig{}
				world.GlobalWorldManager.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
				placePlayerAtTile(g, 6, 6, 64)
				v := g.adventureVisit("forest")
				allowed := hazard == "clear" || hazard == "lane-idle" || hazard == "disabled"
				if hazard == "wall" {
					g.world.Tiles[6][5] = world.TileWall
				} else if hazard == "occupied" {
					spawnSpecialsMonster(g, "goblin", 5, 6)
				} else if hazard == "monster-warning" || hazard == "monster-field" {
					m := spawnSpecialsMonster(g, "solstice_ember_elemental", 8, 6)
					g.refreshMonsterAIState()
					m.Telegraph.Tiles = [][2]int{{5, 6}}
					if hazard == "monster-warning" {
						m.Telegraph.Warning = 1
					} else {
						m.Telegraph.Remaining = 1
					}
				} else if hazard != "clear" {
					kind := hazard
					if hazard == "disabled" {
						kind = "trap"
					}
					if hazard == "lane-idle" || hazard == "lane-warning" || hazard == "lane-live" {
						kind = "lane"
					}
					a.Effects = append(a.Effects, config.EnvironmentEffect{ID: "hazard", Kind: kind, Rect: [4]int{5, 6, 5, 6}, Destination: [2]int{4, 6}})
					if hazard == "disabled" {
						v.effectState("hazard").Disarmed = true
					}
					if hazard == "lane-warning" {
						v.effectState("hazard").Warning = 1
					}
					if hazard == "lane-live" {
						v.effectState("hazard").Remaining = 1
					}
				}
				if entry == "push" {
					g.pushTelegraphParty([2]int{-1, 0})
				} else {
					a.Effects = append(a.Effects, config.EnvironmentEffect{ID: "source", Kind: "transfer", Rect: [4]int{6, 6, 6, 6}, Destination: [2]int{5, 6}})
					g.applyEnvironmentArrival()
				}
				x, y := g.GetPlayerTilePosition()
				wantX := 6
				if allowed {
					wantX = 5
				}
				if x != wantX || y != 6 {
					t.Fatalf("landed at %d,%d; want %d,6", x, y, wantX)
				}
			})
		}
	}
}

func TestSolsticeTelegraphSchedulers(t *testing.T) {
	keys := []string{"ember_elemental", "cinder_hound", "kiln_automaton", "tide_elemental", "cistern_eel", "flow_automaton", "neressa", "jade_elemental", "shardback_burrower", "anchor_automaton", "gale_elemental", "storm_kite", "transfer_automaton", "sunscarab", "rain_serpent", "rootbound_sentinel", "dusk_moth"}
	for _, key := range keys {
		for _, tb := range []bool{false, true} {
			for _, outcome := range []string{"hit", "escaped", "interrupted"} {
				t.Run(fmt.Sprintf("%s/TB%v/%s", key, tb, outcome), func(t *testing.T) {
					g, gl := newSpecialsTestGame(t)
					g.gameLoop = gl
					g.turnBasedMode = tb
					placePlayerAtTile(g, 6, 6, 64)
					for _, c := range g.party.Members {
						c.HitPoints = 10000
						c.MaxHitPoints = 10000
					}
					m := spawnSpecialsMonster(g, "solstice_"+key, 8, 6)
					m.HitPoints = m.MaxHitPoints / 5
					a := telegraphDefinition(m)
					if a == nil {
						t.Fatal("authored special missing")
					}
					var ally *monster.Monster3D
					if a.Kind == "shield" {
						m.DamageMin, m.DamageMax = 60, 80
						ally = spawnSpecialsMonster(g, "goblin", 8, 7)
						ally.AttackCDFrames = 99999
						ally.Speed = 0
					}
					m.State, m.StateTimer = monster.StateAttacking, 1
					g.refreshMonsterAIState()
					before := partyHPSum(g)
					if tb {
						runTBMonsterTurns(g, gl, 1)
					} else {
						g.combat.HandleMonsterInteractions()
					}
					if m.Telegraph.Warning != 1 || len(m.Telegraph.Tiles) == 0 {
						t.Fatalf("scheduler did not announce: %+v", m.Telegraph)
					}
					if partyHPSum(g) != before {
						t.Fatal("wind-up also dealt an ordinary hit")
					}
					original := m.Telegraph.Clone()
					if outcome == "escaped" {
						if ally != nil {
							ally.Pacified = true
						} else {
							placePlayerAtTile(g, 6, 9, 64)
						}
					}
					if outcome == "interrupted" {
						m.StunFramesRemaining, m.StunTurnsRemaining = 999, 9
					}
					if tb {
						runTBMonsterTurns(g, gl, 1)
					} else {
						for range int(math.Ceil(a.WarningSeconds*float64(g.config.GetTPS()))) + 1 {
							g.tickMonsterTelegraphs(1/float64(g.config.GetTPS()), false)
							// One resolution frame is enough: the following frame belongs to normal AI.
							if m.Telegraph.Released || outcome == "interrupted" {
								g.combat.HandleMonsterInteractions()
								break
							}
						}
					}
					if m.Telegraph.Warning > 0 {
						t.Fatal("warning did not expire")
					}
					if outcome == "interrupted" {
						if partyHPSum(g) != before {
							t.Fatal("interrupted cast hurt party")
						}
						return
					}
					if !reflect.DeepEqual(original.Tiles, m.Telegraph.Tiles) {
						t.Fatal("warning retargeted")
					}
					if ally != nil {
						if (ally.SoakDamage > 0) != (outcome == "hit") {
							t.Fatalf("shield=%d", ally.SoakDamage)
						}
						if outcome == "hit" && ally.SoakDamage != 35 {
							t.Fatalf("shield absorption = %d, want 35 from staged attack stats", ally.SoakDamage)
						}
					} else {
						hit := partyHPSum(g) < before
						if hit != (outcome == "hit") {
							t.Fatalf("damage=%v, before=%d after=%d", hit, before, partyHPSum(g))
						}
						if outcome == "hit" {
							switch a.Kind {
							case "root":
								if !g.partyRooted() {
									t.Fatal("root absent")
								}
							case "slow":
								if g.partyHinder.Slow <= 0 {
									t.Fatal("slow absent")
								}
							case "push":
								x, _ := g.GetPlayerTilePosition()
								if x != 5 {
									t.Fatal("push did not land legally")
								}
							case "charge":
								if m.X != 7.5*64 {
									t.Fatalf("charge crossed target or failed: %v", m.X)
								}
							case "retreat":
								if m.X <= 8.5*64 {
									t.Fatal("retreat did not move away")
								}
							}
						}
					}
				})
			}
		}
	}
}

func TestSolsticeTelegraphWallAndFactionGates(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, gate := range []string{"wall", "bound", "charm", "dead", "cooldown"} {
			t.Run(fmt.Sprintf("TB%v/%s", tb, gate), func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				g.gameLoop = gl
				g.turnBasedMode = tb
				placePlayerAtTile(g, 6, 6, 64)
				m := spawnSpecialsMonster(g, "solstice_transfer_automaton", 8, 6)
				switch gate {
				case "wall":
					g.world.Tiles[6][7] = world.TileWall
				case "bound":
					m.Bound = true
				case "charm":
					m.Pacified = true
				case "dead":
					m.HitPoints = 0
				case "cooldown":
					m.Telegraph.Cooldown = 1
				}
				g.refreshMonsterAIState()
				if tb {
					runTBMonsterTurns(g, gl, 1)
				} else {
					g.combat.HandleMonsterInteractions()
				}
				if m.Telegraph.Warning > 0 {
					t.Fatal("gate allowed a special")
				}
			})
		}
	}
}
func TestSolsticeHinderRecoveryWindow(t *testing.T) {
	for _, kind := range []string{"slow", "root"} {
		t.Run(kind, func(t *testing.T) {
			g, _ := newSpecialsTestGame(t)
			g.applyTelegraphHinder(kind)
			for range g.config.GetTPS() {
				g.tickPartyRoot(false)
			}
			remaining := g.partyHinder
			g.applyTelegraphHinder(kind)
			if g.partyHinder != remaining {
				t.Fatal("recast extended hinder")
			}
			g.tickPartyRoot(true)
			if math.Abs(g.partyHinder.Recovery-1.25) > .00001 {
				t.Fatalf("mode switch reset duration: %+v", g.partyHinder)
			}
			for range 3 * g.config.GetTPS() {
				g.tickPartyRoot(false)
			}
			if g.partyRooted() || g.partyHinder.Slow > 0 {
				t.Fatal("hinder outlived duration")
			}
			prior := g.partyHinder
			g.applyTelegraphHinder(kind)
			if prior != g.partyHinder {
				t.Fatal("recovery window allowed chain")
			}
		})
	}
}
func TestSolsticeTelegraphSaveRestore(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	if err := g.transitionToMap(mapTransition{mapKey: "solstice_fire", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	m := g.world.Monsters[0]
	id := m.ID
	m.Telegraph = monster.TelegraphState{Warning: .75, Cooldown: .875, Remaining: .5, Tiles: [][2]int{{3, 4}, {3, 5}}, Origin: [2]int{3, 3}, Direction: [2]int{0, 1}, TargetID: "original-target", Carry: .4}
	want := m.Telegraph.Clone()
	g.partyHinder = PartyHinderState{Slow: .625, Recovery: 1.625}
	hinder := g.partyHinder
	raw, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	if err = g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	for _, restored := range g.world.Monsters {
		if restored.ID == id {
			if !reflect.DeepEqual(restored.Telegraph, want) || g.partyHinder != hinder {
				t.Fatal("save lost telegraph or hinder")
			}
			return
		}
	}
	t.Fatal("saved caster missing")
}

// Support casts need a fight even when a friendly target is already in range.
func TestFlowShieldRequiresCombat(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, engagement := range []string{"idle", "party", "foe"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, engagement), func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				g.gameLoop, g.turnBasedMode = gl, tb
				placePlayerAtTile(g, 1, 1, 64)
				m := spawnSpecialsMonster(g, "solstice_flow_automaton", 9, 9)
				ally := spawnSpecialsMonster(g, "goblin", 9, 10)
				m.IsEngagingPlayer, m.WasAttacked = false, false
				ally.IsEngagingPlayer, ally.WasAttacked = false, false
				m.State, ally.State = monster.StateIdle, monster.StateIdle
				ally.Speed, ally.AttackCDFrames = 0, 100000
				g.refreshMonsterAIState()
				switch engagement {
				case "party":
					m.WasAttacked = true
					m.BeginPlayerEngagement()
				case "foe":
					foe := spawnSpecialsMonster(g, "goblin", 8, 9)
					foe.Bound, foe.BoundFramesRemaining, foe.Speed = true, 100000, 0
					foe.AttackCDFrames = 100000
					m.AIFoe = foe
				}
				before := countCombatLog(g, "prepares")
				if tb {
					runTBMonsterTurns(g, gl, 1)
				} else {
					g.combat.HandleMonsterInteractions()
				}
				if engagement == "idle" {
					if m.Telegraph.Warning != 0 || ally.SoakDamage != 0 || countCombatLog(g, "prepares") != before {
						t.Fatal("idle shield started combat or logged a cast")
					}
				} else {
					if m.Telegraph.Warning == 0 || m.Telegraph.TargetID != ally.ID {
						t.Fatal("engaged shield failed to target its ally")
					}
					g.tickMonsterTelegraphs(telegraphDefinition(m).WarningSeconds, tb)
					if ally.SoakDamage == 0 {
						t.Fatal("engaged shield did not resolve")
					}
				}
			})
		}
	}
}
