package game

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
	"ugataima/internal/storage"
)

// Case table: sequence/forage x split/stitched x journal/NPC turn-in.
// Each cell traverses offered, active, partial, ready, claimed, old-save load,
// failed-save load and new-game reset through the production entry points.
func TestQuestPropsLifecycle(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, unified := range []bool{false, true} {
		for _, id := range []string{"unbroken_desert", "unbroken_jungle"} {
			for _, npcTurnIn := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stitched=%v/npc=%v", id, unified, npcTurnIn), func(t *testing.T) {
					g, wm, cfg := bootOpenWorldGame(t, unified)
					def := g.questManager.Definitions()[id]
					forage := len(def.Activity.Forage) > 0
					initial, partial, ready := 3, 3, 3
					if forage {
						initial, partial, ready = 5, 4, 0
					}
					terrain := map[string]map[[2]int]int{}
					for _, p := range g.activeQuestPropPlacements(id) {
						x, y := projectTileToCurrentWorld(p.Map, p.X, p.Y)
						if terrain[p.Map] == nil {
							terrain[p.Map] = map[[2]int]int{}
						}
						terrain[p.Map][[2]int{x, y}] = int(wm.WorldByKey(p.Map).Tiles[y][x])
					}
					check := func(label string, want int) {
						t.Helper()
						seen := map[string]bool{}
						for _, npc := range g.allLoadedNPCs() {
							owner, _ := activityProp(npc)
							if owner != id {
								continue
							}
							if seen[npc.Key] || npc.QuestPropOwner != id || g.npcAbsent(npc) {
								t.Fatalf("%s: duplicate, unowned or absent physical prop %s", label, npc.Key)
							}
							seen[npc.Key] = true
							for _, p := range g.activeQuestPropPlacements(id) {
								if p.NPC != npc.Key {
									continue
								}
								x, y := projectTileToCurrentWorld(p.Map, p.X, p.Y)
								px, py := TileCenterFromTile(x, y, cfg.GetTileSize())
								if npc.X != px || npc.Y != py {
									t.Fatalf("%s: wrong projected position", label)
								}
							}
						}
						if len(seen) != want {
							t.Fatalf("%s: props=%d want %d", label, len(seen), want)
						}
						for m, tiles := range terrain {
							for p, tile := range tiles {
								if int(wm.WorldByKey(m).Tiles[p[1]][p[0]]) != tile {
									t.Fatalf("%s: changed underlying terrain", label)
								}
							}
						}
					}
					capture := func() GameSave {
						t.Helper()
						raw, err := json.Marshal(g.buildSave(wm))
						if err != nil {
							t.Fatal(err)
						}
						var save GameSave
						if err := json.Unmarshal(raw, &save); err != nil {
							t.Fatal(err)
						}
						for _, npc := range save.NPCStates {
							if npc.Name == "Sunlotus" || npc.Name == "Moonbell" || npc.Name == "Spring Sluice" || npc.Name == "Travelers Sluice" || npc.Name == "Monastery Sluice" {
								t.Fatal("derived prop persisted as NPC state")
							}
						}
						return save
					}
					load := func(save GameSave) {
						t.Helper()
						if err := g.applySave(wm, &save); err != nil {
							t.Fatal(err)
						}
					}
					check("offered", 0)
					before := capture()
					(&InputHandler{game: g}).handleGiveQuest(id)
					check("accepted", initial)
					q := g.questManager.GetQuest(id)
					tokens := append([]string(nil), def.Activity.Sequence...)
					if forage {
						tokens = append([]string(nil), q.Activity.Selected...)
					}
					interact := func(token string) {
						t.Helper()
						for _, npc := range g.allLoadedNPCs() {
							owner, p := activityProp(npc)
							if owner == id && p.Token == token {
								g.dialogNPC = npc
								g.dayNightIsNight = def.Activity.TokenPhase(token) == "night"
								(&InputHandler{game: g}).handleQuestPropInteract(id, p)
								return
							}
						}
						t.Fatal("missing token", token)
					}
					interact(tokens[0])
					check("partial", partial)
					part := capture()
					for i := 0; i < 2; i++ {
						if err := g.switchToMap(g.activeQuestPropPlacements(id)[0].Map); err != nil {
							t.Fatal(err)
						}
						check("map arrival", partial)
					}
					for _, token := range tokens[1:] {
						interact(token)
					}
					check("ready", ready)
					completed := capture()
					if npcTurnIn {
						for _, npc := range g.allLoadedNPCs() {
							if npc.Key == "sister_mira" {
								g.dialogNPC = npc
								break
							}
						}
						(&InputHandler{game: g}).handleTurnInQuest(id)
					} else {
						(&UISystem{game: g}).claimQuestReward(id)
					}
					if !g.questManager.GetQuest(id).RewardsClaimed {
						t.Fatal("turn-in failed")
					}
					check("claimed", 0)
					claimed := capture()
					for _, state := range []struct {
						name string
						save GameSave
						want int
					}{
						{"reload partial", part, partial}, {"reload ready", completed, ready}, {"reload offered", before, 0}, {"reload claimed", claimed, 0},
					} {
						load(state.save)
						check(state.name, state.want)
					}
					load(part)
					q = g.questManager.GetQuest(id)
					if forage && !reflect.DeepEqual(q.Activity.Selected, tokens) {
						t.Fatal("flowers rerolled")
					}
					q.Status = quests.QuestStatusFailed
					failed := capture()
					load(failed)
					check("failed", 0)
					load(part)
					g.startNewGameWithParty(character.NewParty(cfg))
					check("new game", 0)
				})
			}
		}
	}
}

func TestQuestPropsWorldValidation(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, true)
	d := g.questManager.Definitions()["unbroken_desert"]
	original := append([]quests.QuestProp(nil), d.PropLayouts()[0].Props...)
	d.ActivePropLayouts = nil
	d.ActiveProps = append([]quests.QuestProp(nil), original...)
	npcData := character.NPCConfigInstance.NPCs[original[0].NPC]
	for _, tc := range []struct {
		name   string
		change func()
		want   string
	}{
		{"unknown npc", func() { d.ActiveProps[0].NPC = "missing" }, "NPC data not found"},
		{"unknown map", func() { d.ActiveProps[0].Map = "missing" }, "unknown map"},
		{"outside local region", func() { d.ActiveProps[0].X = wm.OpenWorldRegionByKey("desert").LocalWidth }, "in-bounds walkable"},
		{"blocked tile", func() { d.ActiveProps[0].X = 0; d.ActiveProps[0].Y = 0 }, "in-bounds walkable"},
		{"duplicate npc", func() { d.ActiveProps = append(d.ActiveProps, d.ActiveProps[0]) }, "share active_props"},
		{"overlapping props", func() { d.ActiveProps[1].X = d.ActiveProps[0].X; d.ActiveProps[1].Y = d.ActiveProps[0].Y }, "share position"},
		{"wrong owner", func() { d.ActiveProps[0].NPC = "pilgrim_sun1" }, "its own activity"},
		{"ground painting", func() { npcData.GroundTile = "desert_sand" }, "without ground_tile"},
		{"blocking render type", func() { npcData.RenderCategory = "landmark" }, "activity scenery"},
		{"missing candidate", func() { d.ActiveProps = d.ActiveProps[1:] }, "has no placed NPC"},
		{"also map authored", func() {
			x, y := projectTileToCurrentWorld("desert", 31, 18)
			px, py := TileCenterFromTile(x, y, g.config.GetTileSize())
			npc, err := character.CreateNPCFromConfig(original[0].NPC, px, py)
			if err != nil {
				t.Fatal(err)
			}
			w := wm.WorldByKey("desert")
			w.NPCs = append(w.NPCs, npc)
		}, "also placed directly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d.ActiveProps = append([]quests.QuestProp(nil), original...)
			ground, category := npcData.GroundTile, npcData.RenderCategory
			w := wm.WorldByKey("desert")
			npcs := append([]*character.NPC(nil), w.NPCs...)
			t.Cleanup(func() {
				d.ActiveProps = append([]quests.QuestProp(nil), original...)
				npcData.GroundTile = ground
				npcData.RenderCategory = category
				w.NPCs = npcs
			})
			tc.change()
			err := ValidateInteractTagProducers(g.questManager)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// All authored routes must survive saves made before and after acceptance.
func TestQuestPropLayoutsPersistBeforeAcceptance(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, unified := range []bool{false, true} {
		g, wm, cfg := bootOpenWorldGame(t, unified)
		if len(g.questPropLayouts) != 2 {
			t.Fatal("new game did not choose offered layouts")
		}
		for _, id := range []string{"unbroken_desert", "unbroken_jungle"} {
			d := g.questManager.Definitions()[id]
			if len(d.PropLayouts()) != 3 {
				t.Fatal("expected three authored routes")
			}
			for _, layout := range d.PropLayouts() {
				t.Run(fmt.Sprintf("%v/%s/%s", unified, id, layout.ID), func(t *testing.T) {
					g.questManager.Reset()
					g.questPropLayouts[id] = layout.ID
					g.syncQuestProps()
					save := g.buildSave(wm)
					g.questPropLayouts[id] = "invalid"
					if save.QuestPropLayouts[id] != layout.ID {
						t.Fatal("save aliases live choices")
					}
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
					if g.questPropLayouts[id] != layout.ID {
						t.Fatal("offered layout rerolled on load")
					}
					(&InputHandler{game: g}).handleGiveQuest(id)
					check := func() {
						t.Helper()
						if g.questPropLayouts[id] != layout.ID {
							t.Fatal("acceptance or load rerolled layout")
						}
						count := 0
						for _, n := range g.allLoadedNPCs() {
							if n.QuestPropOwner != id {
								continue
							}
							count++
							found := false
							for _, p := range layout.Props {
								if p.NPC == n.Key {
									x, y := projectTileToCurrentWorld(p.Map, p.X, p.Y)
									px, py := TileCenterFromTile(x, y, cfg.GetTileSize())
									found = n.X == px && n.Y == py
								}
							}
							if !found {
								t.Fatalf("%s in wrong layout", n.Key)
							}
						}
						if count != len(layout.Props) {
							t.Fatalf("props=%d want %d", count, len(layout.Props))
						}
					}
					check()
					active := g.buildSave(wm)
					if err := g.applySave(wm, &active); err != nil {
						t.Fatal(err)
					}
					check()
				})
			}
		}
		// Legacy saves receive one valid choice, retained by their next save.
		legacy := g.buildSave(wm)
		legacy.QuestPropLayouts = nil
		if err := g.applySave(wm, &legacy); err != nil {
			t.Fatal(err)
		}
		migrated := g.buildSave(wm)
		if err := g.applySave(wm, &migrated); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(g.questPropLayouts, migrated.QuestPropLayouts) {
			t.Fatal("legacy layout not persisted")
		}
	}
}

func TestQuestForageLegacySaveKeepsCredit(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	(&InputHandler{game: g}).handleGiveQuest("unbroken_jungle")
	q := g.questManager.GetQuest("unbroken_jungle")
	q.Activity = quests.ActivityState{Selected: []string{"sun4", "sun1", "sun6", "moon3", "moon4"}, Collected: []string{"sun4", "moon4"}}
	q.CurrentCount = 2
	save := g.buildSave(wm)
	save.QuestPropLayouts = nil
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	q = g.questManager.GetQuest("unbroken_jungle")
	if q.CurrentCount != 2 || len(q.Activity.Collected) != 2 {
		t.Fatal("legacy collection credit lost")
	}
	for _, n := range append([]*character.NPC(nil), g.allLoadedNPCs()...) {
		if n.QuestPropOwner != q.ID {
			continue
		}
		_, p := activityProp(n)
		g.dayNightIsNight = q.Definition.Activity.TokenPhase(p.Token) == "night"
		g.dialogNPC = n
		(&InputHandler{game: g}).handleQuestPropInteract(q.ID, p)
	}
	if !q.Completed || q.CurrentCount != 5 {
		t.Fatal("legacy forage save cannot complete")
	}
}
