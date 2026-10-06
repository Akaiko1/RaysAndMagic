package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Outdoor nodes refill at dawn only, in either world mode. Crossing regions,
// collecting another node, and save restoration must preserve depleted stock.
func TestSolsticeHarvestDailyPopulation(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	for _, stitched := range []bool{false, true} {
		t.Run(fmt.Sprint(stitched), func(t *testing.T) {
			g, wm, cfg := bootOpenWorldGame(t, stitched)
			if _, err := config.LoadAlchemySpawns("assets/alchemy_spawns.yaml"); err != nil {
				t.Fatal(err)
			}
			g.party.Members[0] = character.CreateCharacter("Alchemist", character.ClassAlchemist, cfg)
			if err := g.transitionToMap(mapTransition{mapKey: "solstice_approach", arrival: mapArrivalEntrance}); err != nil {
				t.Fatal(err)
			}
			g.calendarDay = 1
			g.world.Monsters = nil
			g.gameLoop.closeResourceLoading()
			g.gameLoop.renderer = nil
			finishRareHarvest(t, g)
			const id = "solstice_approach:field"
			if got := len(g.alchemy.Populations[id].Nodes); got != 8 {
				t.Fatalf("initial nodes %d", got)
			}
			for remaining := 7; remaining >= 0; remaining-- {
				node := g.alchemy.Populations[id].Nodes[0]
				var target *character.NPC
				for _, n := range g.world.NPCs {
					if n.Key == node.ID {
						target = n
						break
					}
				}
				if target == nil {
					t.Fatal("missing harvest prop")
				}
				g.setPartyPosition(target.X, target.Y)
				NewInputHandler(g).openNPCInteraction(target)
				for _, quest := range []string{"solstice_survey", "solstice_expedition", "solstice_anchors", "solstice_schedule"} {
					if g.questManager.GetQuest(quest) == nil {
						NewInputHandler(g).handleGiveQuest(quest)
					}
					g.applyCompletedQuestTiles()
				}
				for range 100 {
					if err := g.gameLoop.Update(); err != nil {
						t.Fatal(err)
					}
				}
				seen := map[string]bool{}
				for _, npc := range g.world.NPCs {
					if npc.HarvestOwner != id {
						continue
					}
					if seen[npc.Key] || npc.Visited {
						t.Fatalf("duplicate or collected harvest prop after quest update: %s", npc.Key)
					}
					seen[npc.Key] = true
				}
				if len(seen) != remaining {
					t.Fatalf("visible harvest props: %d want %d", len(seen), remaining)
				}
				if got := len(g.alchemy.Populations[id].Nodes); got != remaining {
					t.Fatalf("same-day refill: got %d want %d", got, remaining)
				}
			}
			for _, dest := range []string{"deep_jungle", "solstice_approach", "solstice_vestibule", "solstice_approach"} {
				if err := g.transitionToMap(mapTransition{mapKey: dest, arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				finishRareHarvest(t, g)
				if len(g.alchemy.Populations[id].Nodes) != 0 {
					t.Fatalf("%s reentry refilled materials", dest)
				}
			}
			save := g.buildSave(wm)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			for range 100 {
				g.updateAlchemyHarvest()
			}
			if len(g.alchemy.Populations[id].Nodes) != 0 {
				t.Fatal("load refilled materials")
			}
			g.advanceCalendarAtDawn()
			finishRareHarvest(t, g)
			if len(g.alchemy.Populations[id].Nodes) != 8 {
				t.Fatal("dawn did not refill materials")
			}
		})
	}
}

// Exercise the displayed material controls and the real brew dispatcher for
// every stock option, including old saved selections from the fixed recipes.
func TestSolsticeAlchemyDisplayedBrewing(t *testing.T) {
	for _, size := range withInterfaceFrames(t, [][2]int{{1280, 720}, {1920, 1080}}) {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			g.party.Members[0] = character.CreateCharacter("Alchemist", character.ClassAlchemist, g.config)
			g.menuOpen, g.showPartyStats, g.currentTab, g.selectedChar = true, true, TabSpellbook, 0
			g.world.Monsters = nil
			l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, true)
			for ri := range config.GlobalAlchemy.Recipes {
				r := &config.GlobalAlchemy.Recipes[ri]
				if !strings.HasPrefix(r.Key, "solstice_") {
					continue
				}
				g.selectedRare, g.alchemyBatches = ri, 1
				a := makeAlchemyMaterialLayout(l, r)
				if len(a.cells) < 2 {
					t.Fatalf("%s: empty selectable stock", r.Key)
				}
				if a.title.w >= 254 && a.cells[0].rect.y != a.cells[1].rect.y {
					t.Fatalf("%v: enough space for two columns, got one", size)
				}
				baseKey := r.Ingredients[1].Alternatives[0].Items[0]
				base, _ := items.TryCreateItemFromYAML(baseKey)
				original := r.Ingredients[0].Alternatives[0].Items[0]
				g.alchemy.Selections = map[string]character.AlchemySelection{r.Key: {{original: true}, {baseKey: true}}}
				g.alchemy = g.alchemy.Clone()
				if !g.alchemySelection(r)[0][original] {
					t.Fatal("old saved stock selection lost")
				}
				for _, cell := range a.cells {
					g.brewAnimation = nil
					g.party.Inventory = nil
					stock, _ := items.TryCreateItemFromYAML(cell.key)
					stock.Quantity = cell.count
					g.party.AddItem(stock)
					h.clicks(false, a.toolbar.right()-82, a.toolbar.y+10, 1)
					if g.brewSelectedRecipe() {
						t.Fatal("brewed without required base")
					}
					g.party.AddItem(base)
					h.clicks(false, a.toolbar.right()-24, a.toolbar.y+10, 1)
					h.clicks(false, a.brew.x+20, a.brew.y+10, 1)
					if g.brewAnimation != nil {
						t.Fatal("None brewed unselected materials")
					}
					h.ui.alchemyScroll[r.Key] = max(0, cell.rect.bottom()-a.viewport.bottom())
					y := cell.rect.y - h.ui.alchemyScroll[r.Key] + 25
					h.clicks(false, cell.rect.x+10, y, 1)
					if !g.alchemySelection(r)[cell.group][cell.key] {
						t.Fatalf("%s/%s: cannot select stock", r.Key, cell.key)
					}
					h.clicks(false, a.brew.x+20, a.brew.y+10, 1)
					if g.brewAnimation == nil {
						t.Fatalf("%s/%s: cannot brew: %s", r.Key, cell.key, g.rareBookMessage)
					}
					output, _ := items.TryCreateItemFromYAML(r.Output)
					if g.party.CountItemsByName(output.Name) < 1 || g.party.CountItemsByName(base.Name) != 0 || g.party.CountItemsByName(stock.Name) != 0 {
						t.Fatal("wrong brew transaction")
					}
				}
			}
		})
	}
}

func TestHarvestRepairPreservesDailyDepletion(t *testing.T) {
	for _, key := range []string{"dawnleaf", "solstice_calendar_jade", "solstice_whisper_silk", "solstice_rain_pearl", "solstice_sun_resin"} {
		for _, opening := range []bool{false, true} {
			for _, restored := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/opening=%v/restored=%v", key, opening, restored), func(t *testing.T) {
					g, _ := rareClassGame(t, character.ClassAlchemist, false)
					previous := config.GlobalAlchemySpawns
					t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
					cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
					if err != nil {
						t.Fatal(err)
					}
					cfg.Populations = []config.HarvestPopulation{{Map: "forest", Key: "ordinary", Count: 8, Yield: 1, Weights: map[string]int{key: 1}}}
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
					setTestWorldManager(t, wm)
					if opening {
						wm.MapConfigs["forest"] = &config.MapConfig{Adventure: &config.AdventureConfig{OpeningOwned: true}}
						g.adventureVisit("forest").Generation = 4
					}
					const id = "forest:ordinary"
					finishRareHarvest(t, g)
					for range 3 {
						var target *character.NPC
						for _, npc := range g.world.NPCs {
							if npc.HarvestOwner == id {
								target = npc
								break
							}
						}
						if target == nil {
							t.Fatal("missing harvest node")
						}
						g.setPartyPosition(target.X, target.Y)
						NewInputHandler(g).openNPCInteraction(target)
						finishRareHarvest(t, g)
					}
					before := g.alchemy.Clone()
					node := before.Populations[id].Nodes[0]
					x, y := TileCenterFromTile(node.X, node.Y, g.config.GetTileSize())
					g.world.NPCs = append(g.world.NPCs, &character.NPC{Key: "test_blocker", X: x, Y: y, RenderCategory: npcCatName[catLandmark]})
					if restored {
						g.restoreSavedTurnState(&GameSave{Alchemy: before})
					} else {
						g.harvestRuntime.dirty = true
					}
					finishRareHarvest(t, g)
					pop := g.alchemy.Populations[id]
					if pop.Day != before.Populations[id].Day || len(pop.Nodes) != 5 {
						t.Fatalf("repair refilled collected stock: day=%d nodes=%d want 5", pop.Day, len(pop.Nodes))
					}
					found := false
					for _, n := range pop.Nodes {
						if n.ID == node.ID {
							found = true
							if n.Key != node.Key || n.Quantity != node.Quantity || (n.X == node.X && n.Y == node.Y) {
								t.Fatal("repair did not relocate the original material")
							}
						}
					}
					if !found {
						t.Fatal("repair replaced the original node identity")
					}
					for range 10 {
						g.harvestRuntime.dirty = true
						finishRareHarvest(t, g)
					}
					if len(g.alchemy.Populations[id].Nodes) != 5 {
						t.Fatal("repeated reconciliation refilled stock")
					}
					g.advanceCalendarAtDawn()
					finishRareHarvest(t, g)
					if opening {
						if len(g.alchemy.Populations[id].Nodes) != 5 {
							t.Fatal("dawn refilled a dungeon opening")
						}
						g.commitAdventureVisit("forest", &AdventureVisit{Generation: 5})
						finishRareHarvest(t, g)
					}
					if len(g.alchemy.Populations[id].Nodes) != 8 {
						t.Fatal("repair blocked dawn refill")
					}
				})
			}
		}
	}
}

func TestHarvestPendingRepairSurvivesLoadWithoutBusySearch(t *testing.T) {
	g, _ := rareClassGame(t, character.ClassAlchemist, false)
	previous := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
	cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Populations = []config.HarvestPopulation{{Map: "forest", Key: "ordinary", Count: 8, Yield: 1, Weights: map[string]int{"solstice_calendar_jade": 1}}}
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	setTestWorldManager(t, wm)
	const id = "forest:ordinary"
	node := HarvestNode{ID: "repair", Key: "solstice_calendar_jade", Region: "forest", Quantity: 2, X: 13, Y: 10, Relocate: true}
	g.alchemy.Populations = map[string]HarvestPopulationState{id: {Day: g.currentCalendarDay(), Nodes: []HarvestNode{node}}}
	terrain := cfg.Placement.TerrainTypes
	cfg.Placement.TerrainTypes = nil
	finishRareHarvest(t, g)
	for range 100 {
		g.updateAlchemyHarvest()
		if g.harvestRuntime.search != nil {
			t.Fatal("no-space repair continuously rescans")
		}
	}
	raw, err := json.Marshal(g.alchemy)
	if err != nil {
		t.Fatal(err)
	}
	var state AlchemyState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Populations[id].Nodes) != 1 || !state.Populations[id].Nodes[0].Relocate {
		t.Fatal("pending repair not persisted")
	}
	cfg.Placement.TerrainTypes = terrain
	g.restoreSavedTurnState(&GameSave{Alchemy: state})
	finishRareHarvest(t, g)
	got := g.alchemy.Populations[id]
	if len(got.Nodes) != 1 || got.Nodes[0].Relocate || got.Nodes[0].ID != node.ID || got.Nodes[0].Quantity != 2 {
		t.Fatalf("load lost or refilled pending repair: %+v", got)
	}
}
