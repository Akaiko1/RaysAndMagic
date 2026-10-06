package game

import (
	"fmt"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/items"

	"ugataima/internal/spells"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestSolsticeConnectionPlayable(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, method := range []string{"jump", "device", "fold0", "fold1", "fold2", "fold3", "return"} {
				t.Run(fmt.Sprintf("stitched=%v/TB=%v/%s", stitched, tb, method), func(t *testing.T) {
					g, wm, cfg := bootOpenWorldGame(t, stitched)
					loadBenchContent(t)
					if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
						t.Fatal(err)
					}
					g.turnBasedMode = tb
					g.currentTurn = 0
					c := character.CreateCharacter("Pilgrim", character.ClassWayfarer, cfg)
					c.Level = 20
					c.ActionsRemaining = 1
					c.SpellPoints = 1000
					c.MaxSpellPoints = 1000
					g.party.Members = []*character.MMCharacter{c}
					g.selectedChar = 0
					if err := g.activateQuest("solstice_connection"); err != nil {
						t.Fatal(err)
					}
					ts := cfg.GetTileSize()
					g.setPartyPosition(7.5*ts, 7.5*ts)
					g.camera.Angle = 0
					key, lx, ly := g.canonicalPosition(g.camera.X, g.camera.Y)
					if key != "solstice_vestibule" || lx != g.camera.X || ly != g.camera.Y {
						t.Errorf("indoor position localized to %s (%v,%v)", key, lx, ly)
					}
					if method == "jump" {
						c.LearnSpell("jump")
						def, _ := spells.GetSpellDefinitionByID("jump")
						if g.combat.tryCastJump(def, c) != castCommitted {
							t.Fatal("Jump cannot cross gap")
						}
					} else if method == "device" {
						g.party.Inventory = []items.Item{items.CreateItemFromYAML("translocator")}
						if !g.useDeviceFromInventory(0, 0) {
							t.Fatal("Translocator cannot cross gap")
						}
					} else if method == "return" {
						c.RareClass.Anchor = character.SpatialAnchor{MapKey: "solstice_vestibule", X: 10.5 * ts, Y: 7.5 * ts, Frames: 1000}
						if !g.useTechnique(0, "return_step", false, false) {
							t.Fatal("Return cannot cross gap")
						}
					} else {
						tier := int(method[len(method)-1] - '0')
						c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.SkillMastery(tier)}
						if !g.useTechnique(0, "fold_step", false, false) {
							t.Fatal("Fold cannot cross gap")
						}
					}
					q := g.questManager.GetQuest("solstice_connection")
					if q.CurrentCount != 1 {
						t.Fatalf("crossing not credited: %+v", q.Activity)
					}
					save := g.buildSave(wm)
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
					q = g.questManager.GetQuest("solstice_connection")
					if q.CurrentCount != 1 {
						t.Fatal("crossing lost on load")
					}
					var receiver *character.NPC
					for _, n := range g.world.NPCs {
						if n.Key == "solstice_connection_receiver" {
							receiver = n
						}
					}
					if receiver == nil {
						t.Fatal("missing receiving anchor")
					}
					g.setPartyPosition(receiver.X-ts, receiver.Y)
					ih := NewInputHandler(g)
					ih.openNPCInteraction(receiver)
					for _, choice := range receiver.DialogueData.Choices {
						if choice.Action == "prop" {
							dialogActions[choice.Action](ih, receiver, choice)
						}
					}
					q = g.questManager.GetQuest("solstice_connection")
					if !q.Completed || q.CurrentCount != 2 {
						t.Fatalf("receiver did not finish quest: %+v", q.Activity)
					}
					save = g.buildSave(wm)
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
					q = g.questManager.GetQuest("solstice_connection")
					if !q.Completed {
						t.Fatal("completion lost on load")
					}
					(&UISystem{game: g}).claimQuestReward("solstice_connection")
					g.currentTurn = 0
					g.spatialStepThisTurn = false
					g.spatialReuseFrames = 0
					g.party.Members[0].SpellPoints = 0
					if method == "device" {
						if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
							t.Fatalf("carried Translocator did not satisfy entry gate: %v", err)
						}
						return
					}
					if err := g.enterAdventureSchedule("solstice_vestibule"); err == nil {
						t.Fatal("entry allowed without traversal SP")
					}
					g.party.Members[0].SpellPoints = 1000
					if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
						t.Fatalf("prepared party blocked after turn-in: %v", err)
					}

				})
			}
		}
	}
}

func TestSolsticeCrossingRejectsInvalidMoves(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, state := range []string{"not accepted", "walk", "wrong corridor", "wall", "blocked landing", "rooted", "no SP"} {
		t.Run(state, func(t *testing.T) {
			g, wm, cfg := bootOpenWorldGame(t, true)
			loadBenchContent(t)
			if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			c := character.CreateCharacter("Pilgrim", character.ClassWayfarer, cfg)
			c.Level = 20
			c.SpellPoints = 100
			g.party.Members = []*character.MMCharacter{c}
			g.selectedChar = 0
			if state != "not accepted" {
				if err := g.activateQuest("solstice_connection"); err != nil {
					t.Fatal(err)
				}
			}
			ts := cfg.GetTileSize()
			g.setPartyPosition(7.5*ts, 7.5*ts)
			g.camera.Angle = 0
			x, y := g.camera.X, g.camera.Y
			wall, _ := world.GlobalTileManager.GetTileTypeFromKey("wall")
			switch state {
			case "wall":
				g.world.Tiles[7][8] = wall
			case "blocked landing":
				for xx := 9; xx < g.world.Width; xx++ {
					g.world.Tiles[7][xx] = wall
				}
			case "rooted":
				g.partyRoot = PartyRootState{Frames: 100, Turns: 2}
			case "no SP":
				c.SpellPoints = 0
			}
			sp := c.SpellPoints
			switch state {
			case "walk":
				g.setPartyPosition(9.5*ts, 7.5*ts)
			case "wrong corridor":
				g.setPartyPosition(7.5*ts, 6.5*ts)
				def, _ := spells.GetSpellDefinitionByID("jump")
				g.combat.tryCastJump(def, c)
			default:
				committed := g.useTechnique(0, "fold_step", false, false)
				if committed != (state == "not accepted") {
					t.Fatalf("unexpected movement: %v", committed)
				}
				if !committed && (c.SpellPoints != sp || g.camera.X != x || g.camera.Y != y) {
					t.Fatal("failed traversal spent resources or moved")
				}
			}
			save := g.buildSave(wm)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			q := g.questManager.GetQuest("solstice_connection")
			if q != nil && q.CurrentCount != 0 {
				t.Fatalf("invalid movement or load credited quest: %+v", q.Activity)
			}
		})
	}
}

func TestSolsticeTraversalGateEligibility(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, state := range []string{"Pilgrim", "Jump only", "unclaimed", "no skill", "wrong class", "low level", "unconscious", "rooted", "no SP", "shared device", "other owner device", "reserve device", "unconfigured device", "device rooted"} {
		t.Run(state, func(t *testing.T) {
			g, _, cfg := bootOpenWorldGame(t, true)
			loadBenchContent(t)
			if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			g.questManager.ActivateQuest("solstice_connection")
			g.questManager.MarkCompleted("solstice_connection")
			if state != "unclaimed" {
				g.questManager.ClaimRewards("solstice_connection")
			}
			c := character.CreateCharacter("Pilgrim", character.ClassWayfarer, cfg)
			c.Level, c.SpellPoints = 20, 1000
			g.party.Members = []*character.MMCharacter{c}
			switch state {
			case "Jump only":
				c.Class = character.ClassKnight
				c.LearnSpell("jump")
			case "no skill":
				delete(c.Skills, character.SkillTranslocation)
			case "wrong class":
				c.Class = character.ClassKnight
			case "low level":
				c.Level = 0
			case "unconscious":
				c.AddCondition(character.ConditionUnconscious)
			case "rooted":
				g.partyRoot = PartyRootState{Frames: 100, Turns: 2}
			case "no SP":
				c.SpellPoints = 0
			case "shared device", "other owner device", "reserve device", "unconfigured device", "device rooted":
				c.Class, c.SpellPoints = character.ClassKnight, 0
				it := items.CreateItemFromYAML("translocator")
				if state == "unconfigured device" {
					it.UseAction = ""
				}
				if state == "other owner device" || state == "reserve device" {
					owner := character.CreateCharacter("Owner", character.ClassKnight, cfg)
					owner.HitPoints = 0
					owner.Inventory = []items.Item{it}
					if state == "other owner device" {
						g.party.Members = append(g.party.Members, owner)
					} else {
						g.party.Reserve = append(g.party.Reserve, owner)
					}
				} else {
					g.party.Inventory = []items.Item{it}
				}
				if state == "device rooted" {
					g.partyRoot = PartyRootState{Frames: 100, Turns: 2}
				}
			}
			before := g.world
			visits := len(g.adventure.Visits)
			err := g.enterAdventureSchedule("solstice_vestibule")
			want := state == "Pilgrim" || state == "Jump only" || state == "shared device" || state == "other owner device"
			if (err == nil) != want {
				t.Fatalf("gate error = %v, want entry = %v", err, want)
			}
			if !want && (g.world != before || len(g.adventure.Visits) != visits) {
				t.Fatal("refused entry created an opening or changed the map")
			}
		})
	}
}
