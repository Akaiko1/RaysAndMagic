package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
)

// This checks traversal, not combat balance: defeatable monsters are removed,
// and mana/recovery are replenished between independent route probes. NPC and
// terrain collision stay live. No authored JumpLinks are injected as edges.
func TestChamberTraversalCapabilities(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		// One boot per world mode. Each cell enters a fresh copy of its chamber
		// with an empty visit, exactly as a new opening would.
		g, wm, cfg := bootOpenWorldGame(t, stitched)
		loadBenchContent(t)
		for _, tb := range []bool{false, true} {
			for _, element := range []string{"fire", "water", "earth", "air"} {
				for _, tier := range []int{-1, 0, 1, 2, 3} {
					method := "Jump"
					if tier >= 0 {
						method = fmt.Sprintf("Fold%d", tier)
					}
					t.Run(fmt.Sprintf("%s/%s/stitched=%v/TB=%v", element, method, stitched, tb), func(t *testing.T) {
						key := "solstice_" + element
						fresh, err := wm.FreshAdventureMap(key)
						if err != nil {
							t.Fatal(err)
						}
						g.turnBasedMode = false
						if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance, adventureWorld: fresh, adventureVisit: &AdventureVisit{}}); err != nil {
							t.Fatal(err)
						}
						g.turnBasedMode = tb
						c := character.CreateCharacter("Route probe", character.ClassWayfarer, cfg)
						c.Level = 20
						c.MaxHitPoints, c.MaxSpellPoints = 10000, 10000
						if tier >= 0 {
							c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.SkillMastery(tier)}
						} else {
							delete(c.Skills, character.SkillTranslocation)
							c.LearnSpell("jump")
						}
						g.party.Members = []*character.MMCharacter{c}
						g.selectedChar = 0
						a := g.adventureConfig(key)
						boss := g.adventureBoss(a)
						ts := cfg.GetTileSize()
						bossTile := [2]int{TileIndex(boss.X, ts), TileIndex(boss.Y, ts)}
						for _, m := range g.world.Monsters {
							m.HitPoints = 0
							g.collisionSystem.UnregisterEntity(m.ID)
						}
						sx, sy := g.GetPlayerTilePosition()
						start := [2]int{sx, sy}
						routeStart := start
						place := func(p [2]int) {
							g.setPartyPosition((float64(p[0])+.5)*ts, (float64(p[1])+.5)*ts)
							g.currentTurn, g.spatialReuseFrames = 0, 0
							g.spatialStepThisTurn = false
							c.HitPoints, c.SpellPoints = c.MaxHitPoints, c.MaxSpellPoints
							c.RTCooldown = 0
							for _, s := range g.adventureVisit(key).Effects {
								s.WasInside, s.Suppressed = false, false
							}
						}
						jump, _ := spells.GetSpellDefinitionByID("jump")
						// Rebuild the graph after each reachable control changes the world.
						// Thus the air crossing cannot grant access to its own control.
						var reached map[[2]int]bool
						var reverse map[[2]int][][2]int
						for pass := 0; ; pass++ {
							if pass > len(a.Controls) {
								t.Fatal("controls did not converge")
							}
							reached = map[[2]int]bool{routeStart: true}
							reverse = map[[2]int][][2]int{}
							queue := [][2]int{routeStart}
							for i := 0; i < len(queue); i++ {
								p := queue[i]
								for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
									for _, ability := range []bool{false, true} {
										place(p)
										g.camera.Angle = math.Atan2(float64(d[1]), float64(d[0]))
										if ability {
											if tier < 0 {
												if g.combat.tryCastJump(jump, c) != castCommitted {
													continue
												}
											} else if !g.useTechnique(0, "fold_step", false, false) {
												continue
											}
										} else {
											clear := true
											for step := 1; step <= 4; step++ {
												x := (float64(p[0]) + .5 + float64(d[0]*step)/4) * ts
												y := (float64(p[1]) + .5 + float64(d[1]*step)/4) * ts
												if !g.collisionSystem.CanMoveTo("player", x, y) {
													clear = false
													break
												}
											}
											if !clear {
												continue
											}
											g.setPartyPosition((float64(p[0]+d[0])+.5)*ts, (float64(p[1]+d[1])+.5)*ts)
										}
										g.applyEnvironmentArrival()
										x, y := g.GetPlayerTilePosition()
										q := [2]int{x, y}
										reverse[q] = append(reverse[q], p)
										if !reached[q] {
											reached[q] = true
											queue = append(queue, q)
										}
									}
								}
							}
							changed := false
						activateNext:
							for _, n := range g.world.NPCs {
								if n.DialogueData == nil {
									continue
								}
								p, ok := chamberInteractionApproach(g, reached, n)
								if !ok {
									continue
								}
								for _, choice := range n.DialogueData.Choices {
									if choice.Action != "adventure_control" || g.adventureVisit(key).Controls[choice.Control] {
										continue
									}
									place(p)
									ih := NewInputHandler(g)
									ih.openNPCInteraction(n)
									dialogActions[choice.Action](ih, n, choice)
									if !g.adventureVisit(key).Controls[choice.Control] {
										t.Fatalf("reachable control %s did not activate", choice.Control)
									}
									changed = true
									// Continue from this console, not a free return to spawn.
									routeStart = p
									break activateNext
								}
							}
							if !changed {
								break
							}
						}
						for _, control := range a.Controls {
							if !g.adventureVisit(key).Controls[control.ID] {
								t.Errorf("unreachable control: %s", control.ID)
							}
						}
						if !reached[start] {
							t.Error("cannot return to entrance after operating controls")
						}
						if !reached[bossTile] {
							t.Errorf("unreachable boss arena at %v", bossTile)
						}
						for _, n := range g.world.NPCs {
							if _, ok := chamberInteractionApproach(g, reached, n); !ok {
								t.Errorf("unreachable NPC/reward/exit: %s", n.Key)
							}
						}
						for _, encounter := range wm.MapConfigs[key].ClearEncounters {
							chest := encounter.Rewards.TreasureChest
							if chest != nil && !reached[[2]int{chest.TileX, chest.TileY}] {
								t.Errorf("unreachable boss reward: %s", chest.ID)
							}
						}
						// Every reachable landing, including archive alcoves and transport
						// receivers, must have a directed route back to the entrance.
						back := map[[2]int]bool{start: true}
						queue := [][2]int{start}
						for i := 0; i < len(queue); i++ {
							for _, p := range reverse[queue[i]] {
								if !back[p] {
									back[p] = true
									queue = append(queue, p)
								}
							}
						}
						for p := range reached {
							if !back[p] {
								t.Errorf("no return route from %v", p)
							}
						}
					})
				}
			}
		}
	}
}

func chamberInteractionApproach(g *MMGame, reached map[[2]int]bool, n *character.NPC) ([2]int, bool) {
	ts := g.config.GetTileSize()
	x, y := TileIndex(n.X, ts), TileIndex(n.Y, ts)
	for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {0, 0}} {
		p := [2]int{x + d[0], y + d[1]}
		if reached[p] && g.collisionSystem.CheckLineOfSight((float64(p[0])+.5)*ts, (float64(p[1])+.5)*ts, n.X, n.Y) {
			return p, true
		}
	}
	return [2]int{}, false
}
