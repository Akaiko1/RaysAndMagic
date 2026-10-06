package game

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// Generated herbs must satisfy the same renderer contract as authored NPCs.
// Cases: every harvest_sprite reagent, fresh/restored/dawn population,
// visible/hidden roster.
// Saved node identity is authoritative; renderer categories are always derived.
func TestRareHarvestRuntimeRenderContract(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	var reagents []string
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		if config.GlobalItems.Items[key].HarvestSprite != "" {
			reagents = append(reagents, key)
		}
	}
	if len(reagents) == 0 {
		t.Fatal("no harvest_sprite reagents in the catalog")
	}
	for _, lifecycle := range []string{"fresh", "restored", "dawn", "hidden"} {
		t.Run(lifecycle, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Members[1] = character.CreateCharacter("Pilgrim", character.ClassWayfarer, g.config)
			previous := config.GlobalAlchemySpawns
			t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
			cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cfg.Populations = nil
			for _, key := range reagents {
				cfg.Populations = append(cfg.Populations, config.HarvestPopulation{Map: "forest", Key: key, Count: 1, Yield: 1, Weights: map[string]int{key: 1}})
			}
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			world.GlobalWorldManager = wm
			finishRareHarvest(t, g)
			switch lifecycle {
			case "restored":
				raw, err := json.Marshal(g.alchemy)
				if err != nil {
					t.Fatal(err)
				}
				var state AlchemyState
				if err = json.Unmarshal(raw, &state); err != nil {
					t.Fatal(err)
				}
				g.restoreSavedTurnState(&GameSave{Alchemy: state})
				g.world.NPCs = nil
				finishRareHarvest(t, g)
			case "dawn":
				g.alchemy.Populations = map[string]HarvestPopulationState{}
				g.harvestRuntime.dirty = true
				g.advanceCalendarAtDawn()
				finishRareHarvest(t, g)
			case "hidden":
				g.party.Members[0] = character.CreateCharacter("Knight", character.ClassKnight, g.config)
				g.updateAlchemyHarvest()
			}
			t.Chdir("../..")
			g.sprites = graphics.NewSpriteManager()
			assertRareHerbsRender(t, g, len(reagents), lifecycle == "hidden")
		})
	}
}

func assertRareHerbsRender(t *testing.T, g *MMGame, want int, hidden bool) {
	t.Helper()
	r := &Renderer{game: g}
	// Production resource discovery classifies NPCs even when they are hidden
	// or outside the camera frustum, before the actual billboard draw.
	plan := r.collectMapRenderPrewarmPlan(currentMapKey())
	rh := NewRenderingHelper(g)
	count := 0
	for _, n := range g.world.NPCs {
		if n == nil || n.HarvestOwner == "" {
			continue
		}
		count++
		if g.npcAbsent(n) != hidden {
			t.Fatalf("%s visibility disagrees with roster", n.HarvestItem)
		}
		if npcIsPerson(n) {
			t.Fatalf("%s uses person interaction", n.HarvestItem)
		}
		rh.NPCSpriteMetricsF(n, n.X, n.Y, 64)
		if !containsNPCResource(plan.npcSprites, mapNPCPrewarmResource{name: n.Sprite, prefix: "npc", warmVisibleBounds: true}) {
			t.Fatalf("%s missing from render resources", n.HarvestItem)
		}
	}
	if count != want {
		t.Fatalf("herbs=%d want %d", count, want)
	}
}

// Begin Adventure, on the shipped split and stitched worlds, must accept
// ordinary parties, either rare hero alone, and both rare heroes together.
func TestRarePartyBeginAdventure(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	previous := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = previous })
	for _, unified := range []bool{false, true} {
		t.Run(fmt.Sprintf("unified=%v", unified), func(t *testing.T) {
			g, _, cfg := bootOpenWorldGame(t, unified)
			if _, err := config.LoadAlchemySpawns("assets/alchemy_spawns.yaml"); err != nil {
				t.Fatal(err)
			}
			for _, classes := range [][]character.CharacterClass{nil, {character.ClassWayfarer}, {character.ClassAlchemist}, {character.ClassAlchemist, character.ClassWayfarer}} {
				t.Run(fmt.Sprint(classes), func(t *testing.T) {
					pc := newPartyCreateState(cfg)
					for slot, class := range classes {
						found := false
						for i, h := range pc.pool {
							if h.char.Class != class {
								continue
							}
							pc.pool[i], pc.slots[slot] = pc.slots[slot], h
							found = true
							break
						}
						if !found {
							t.Fatalf("class %v missing from party creation", class)
						}
					}
					g.beginAdventure(pc)
					if g.appScreen != AppScreenInGame {
						t.Fatal("Begin Adventure did not enter gameplay")
					}
					finishRareHarvest(t, g)
					want := 0
					if g.hasHarvestAlchemist() {
						region, _, _ := g.canonicalPosition(g.camera.X, g.camera.Y)
						if want = harvestPopulationTotal(region); want == 0 {
							t.Fatalf("start region %q has no reagent population to render", region)
						}
					}
					assertRareHerbsRender(t, g, want, false)
				})
			}
		})
	}
}
