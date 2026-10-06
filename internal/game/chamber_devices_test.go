package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/storage"
)

func TestChamberDevicesPlayable(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, element := range []string{"fire", "water", "earth", "air"} {
				t.Run(fmt.Sprintf("%s/stitched=%v/TB=%v", element, stitched, tb), func(t *testing.T) {
					g, wm, cfg := bootOpenWorldGame(t, stitched)
					loadBenchContent(t)
					key := "solstice_" + element
					if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					g.turnBasedMode = tb
					g.currentTurn = 0
					for _, c := range g.party.Members {
						c.HitPoints = 10000
						c.MaxHitPoints = 10000
						delete(c.Skills, character.SkillDisarmTrap)
					}
					a := g.adventureConfig(key)
					v := g.adventureVisit(key)
					boss := g.adventureBoss(a)
					ts := cfg.GetTileSize()
					place := func(x, y int) { g.setPartyPosition((float64(x)+.5)*ts, (float64(y)+.5)*ts) }
					tick := func(seconds int) {
						if tb {
							for i := 0; i < seconds; i += 3 {
								g.tickEnvironment(3, true)
							}
						} else {
							for range seconds {
								g.tickEnvironment(1, false)
							}
						}
					}
					operate := func(id, action string) {
						t.Helper()
						for _, npc := range g.world.NPCs {
							if npc.DialogueData == nil {
								continue
							}
							for _, c := range npc.DialogueData.Choices {
								if c.Control == id && c.Action == action {
									g.currentTurn = 0
									ih := NewInputHandler(g)
									ih.openNPCInteraction(npc)
									dialogActions[c.Action](ih, npc, c)
									return
								}
							}
						}
						t.Fatalf("missing displayed %s action for %s", action, id)
					}
					enabled := func(id string) bool { return g.effectEnabled(a, g.adventureVisit(key), *a.Effect(id)) }
					transfer := func(id string, want bool) {
						t.Helper()
						e := a.Effect(id)
						place(e.Rect[0], e.Rect[1])
						g.applyEnvironmentArrival()
						x, y := g.GetPlayerTilePosition()
						arrived := x == e.Destination[0] && y == e.Destination[1]
						if arrived != want {
							t.Fatalf("%s arrived=%v at %d,%d", id, arrived, x, y)
						}
					}
					switch element {
					case "water":
						transfer("cistern_current", false)
						boss.IsEngagingPlayer = true
						boss.HitPoints = boss.MaxHitPoints / 2
						hp := boss.HitPoints
						tick(6)
						if boss.HitPoints <= hp {
							t.Fatal("open intake does not heal")
						}
						operate("return_sluice", "adventure_control")
						// A current cannot land on occupied or blocked tiles.
						e := a.Effect("cistern_current")
						dx, dy := e.Destination[0], e.Destination[1]
						old := g.world.Tiles[dy][dx]
						g.world.Tiles[dy][dx] = g.world.Tiles[0][0]
						transfer("cistern_current", false)
						g.world.Tiles[dy][dx] = old
						transfer("cistern_current", true)
						if dx != 4 || dy != 5 {
							t.Fatal("return current no longer reaches entrance")
						}
						hp = boss.HitPoints
						tick(6)
						if boss.HitPoints <= hp {
							t.Fatal("return control incorrectly stops healing")
						}
						operate("intake_sluice", "adventure_control")
						hp = boss.HitPoints
						tick(6)
						if boss.HitPoints != hp {
							t.Fatal("intake alone did not stop healing")
						}
					case "fire":
						place(6, 8)
						hp := g.party.Members[0].HitPoints
						tick(3)
						if g.party.Members[0].HitPoints >= hp {
							t.Fatal("archive route has no heat cost")
						}
						operate("coolant_west", "adventure_control")
						if enabled("archive_heat") || enabled("kiln_lava") {
							t.Fatal("west valve did not cool the archive and kiln")
						}
						hp = g.party.Members[0].HitPoints
						tick(3)
						if g.party.Members[0].HitPoints != hp {
							t.Fatal("cooled archive still burns")
						}
						operate("coolant_east", "adventure_control")
						if enabled("regulator_lane_1") {
							t.Fatal("east valve did not suppress lower firing row")
						}
					case "earth":
						for _, npc := range g.world.NPCs {
							if npc.Key == "solstice_earth_security_bypass" {
								for _, c := range npc.DialogueData.Choices {
									if c.Action == "adventure_control" {
										t.Fatal("skill-free bypass makes disarm redundant")
									}
								}
							}
						}
						operate("plate_row_0", "disarm_environment")
						if v.effectState("plate_row_0").Disarmed {
							t.Fatal("circuit disarmed without skill")
						}
						c := g.party.Members[0]
						c.Skills[character.SkillDisarmTrap] = &character.Skill{Mastery: character.MasteryGrandMaster}
						c.RTCooldown = 0
						operate("plate_row_0", "disarm_environment")
						if enabled("plate_row_0") {
							t.Fatal("Grandmaster did not disarm circuit")
						}
						ac := boss.ArmorClass
						operate("brace_one", "adventure_control")
						if boss.ArmorClass != max(a.Boss.ArmorFloor, ac-18) {
							t.Fatal("load brace did not weaken armor")
						}
						ac = boss.ArmorClass
						operate("brace_one", "adventure_control")
						if boss.ArmorClass != ac {
							t.Fatal("repeat activation reduced armor twice")
						}
						operate("brace_two", "adventure_control")
						if enabled("plate_row_2") || enabled("regulator_lane_1") || !enabled("ground_shock") {
							t.Fatal("north brace affects wrong mechanisms")
						}
						operate("brace_three", "adventure_control")
						if enabled("plate_row_3") || enabled("ground_shock") {
							t.Fatal("south brace did not secure final phase")
						}
					case "air":
						transfer("transfer_jet", false)
						operate("gallery_vane", "adventure_control")
						transfer("transfer_jet", true)
						transfer("return_jet", false)
						for _, id := range []string{"arena_jet_0", "arena_jet_1"} {
							transfer(id, false)
						}
						operate("hub_vane", "adventure_control")
						for _, id := range []string{"return_jet", "arena_jet_0", "arena_jet_1"} {
							transfer(id, true)
						}
					}
					// Old saves can contain removed effect/control keys. They must remain
					// harmless while current controls and safe return routes survive reload.
					v.Controls["legacy_removed_control"] = true
					v.effectState("legacy_removed_effect").Warning = 1
					snapshot := g.buildSave(wm)
					if err := g.applySave(wm, &snapshot); err != nil {
						t.Fatal(err)
					}
					v = g.adventureVisit(key)
					for _, c := range a.Controls {
						if !v.Controls[c.ID] {
							t.Fatalf("lost control %s on load", c.ID)
						}
					}
					switch element {
					case "water":
						transfer("cistern_current", true)
					case "air":
						transfer("return_jet", true)
						transfer("arena_jet_0", true)
					case "earth":
						if enabled("ground_shock") || enabled("plate_row_0") {
							t.Fatal("reload restored secured earth hazards")
						}
					case "fire":
						if enabled("archive_heat") || enabled("regulator_lane_1") {
							t.Fatal("reload restored cooled fire hazards")
						}
					}
				})
			}
		}
	}
}
