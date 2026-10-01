package game

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

func rareInputCast(g *MMGame, ih *InputHandler, tb bool) {
	if tb {
		ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyF })
		ih.keys.BeginFrame()
		ih.handleTurnBasedInput()
	} else {
		ih.performRTCombatAction(rtActCast, true)
	}
}

func TestRareInputCastCycles(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"flask", "veil"} {
			for _, state := range []string{"ready", "recovering", "unconscious", "unavailable"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, kind, state), func(t *testing.T) {
					class := character.ClassAlchemist
					if kind == "veil" {
						class = character.ClassWayfarer
					}
					g, c := rareClassGame(t, class, tb)
					ih := NewInputHandler(g)
					if tb {
						c.ActionsRemaining = 1
					}
					caster := character.CreateCharacter("Caster", character.ClassSorcerer, g.config)
					caster.SpellPoints = 100
					caster.ActionsRemaining = 2
					g.party.Members = []*character.MMCharacter{c, caster}
					if kind == "veil" {
						it, _ := config.TechniqueItem("phase_veil")
						c.Equipment[items.SlotSpell] = it
					} else {
						it, _ := items.TryCreateItemFromYAML("harm_flask")
						it.Quantity = 3
						g.party.AddItem(it)
						g.equipFlask(0, "harm_flask")
					}
					switch state {
					case "recovering":
						c.RTCooldown = 999
						if tb {
							c.ActionsRemaining = 0
						}
					case "unconscious":
						c.HitPoints = 0
						c.AddCondition(character.ConditionUnconscious)
						g.parkSelection = true
					case "unavailable":
						if kind == "veil" {
							g.addCombatBuff(TimedCombatBuff{SpellID: "phase_veil", Frames: 100})
						} else {
							g.party.Inventory = nil
						}
					}
					rareInputCast(g, ih, tb)
					if state == "ready" {
						if kind == "flask" && g.flaskStock(g.party.Members[0], "harm_flask") != 2 {
							t.Fatal("ready flask did not consume one bottle")
						}
						if kind == "veil" && c.SpellPoints != 92 {
							t.Fatal("ready technique did not spend SP")
						}
						if !tb && g.selectedChar != 1 {
							t.Fatal("rare action did not advance RT cast selection")
						}
						g.spellInputCooldown = 0
						rareInputCast(g, ih, tb)
					}
					if caster.SpellPoints >= 100 {
						t.Fatal("F chain never reached the ordinary caster")
					}
					if state != "ready" && c.SpellPoints != 100 {
						t.Fatal("rejected rare action spent SP")
					}
				})
			}
		}
	}
}

func TestRareSpatialCommandsAreManualAndFaceForward(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, entry := range []string{"F", "Space", "held_Space", "mouse", "held_F"} {
			if tb && strings.HasPrefix(entry, "held_") {
				continue
			}
			t.Run(fmt.Sprintf("TB=%v/%s", tb, entry), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				g.appScreen = AppScreenInGame
				it, _ := config.TechniqueItem("fold_step")
				c.Equipment[items.SlotSpell] = it
				ih := NewInputHandler(g)
				m := zoneVictim(t, g)
				ts := float64(g.config.GetTileSize())
				m.X += ts
				m.Y += 3 * ts
				x, y, angle := g.camera.X, g.camera.Y, g.camera.Angle
				undo := g.combat.beginPartyTargetAim(m)
				defer undo()
				if entry == "F" {
					c.ActionsRemaining = 0
					c.RTCooldown = 999
					rareInputCast(g, ih, tb)
				} else if entry == "mouse" {
					ih.performMouseSmartAttack(m)
				} else if tb {
					ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeySpace })
					ih.keys.BeginFrame()
					ih.handleTurnBasedInput()
				} else if entry == "Space" {
					ih.performRTCombatAction(rtActSmart, false)
				} else {
					ih.pendingRepeat = rtActSmart
					if entry == "held_F" {
						ih.pendingRepeat = rtActCast
					}
					ih.performPendingRepeat()
				}
				if entry == "F" {
					if math.Abs(g.camera.X-x-3*ts) > .001 || math.Abs(g.camera.Y-y) > .001 || c.SpellPoints != 92 || c.ActionsRemaining != 0 || c.RTCooldown != 999 {
						t.Fatal("manual step did not preserve facing-axis position/AP/recovery")
					}
					if tb && (math.Mod(g.camera.X, ts) != ts/2 || math.Mod(g.camera.Y, ts) != ts/2) {
						t.Fatal("step left TB grid")
					}
				} else if g.camera.X != x || g.camera.Y != y || c.SpellPoints != 100 || g.spatialReuseFrames != 0 {
					t.Fatal("attack or held command automatically moved the party")
				}
				if g.camera.Angle != angle {
					t.Fatal("step changed facing")
				}
			})
		}
	}
}

func TestRarePendingRepeatHonorsRunningWeapon(t *testing.T) {
	for _, kind := range []rtActionKind{rtActWeapon, rtActSmart} {
		for _, wing := range []bool{false, true} {
			for _, down := range []bool{false, true} {
				t.Run(fmt.Sprintf("kind=%d/wing=%v/down=%v", kind, wing, down), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, false)
					ih := NewInputHandler(g)
					ih.heldKeys = func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft }
					holder := g.party.Members[1]
					if wing {
						holder.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("wyrmspine_bow")
					}
					if down {
						holder.HitPoints = 0
					}
					delete(c.Equipment, items.SlotSpell)
					c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
					ih.pendingRepeat = kind
					ih.performPendingRepeat()
					want := wing && !down
					if (len(g.arrows) > 0) != want {
						t.Fatalf("running repeat fired=%v want %v", len(g.arrows) > 0, want)
					}
					if ih.pendingRepeat != rtActNone {
						t.Fatal("repeat intent survived dispatch")
					}
				})
			}
		}
	}
}

func TestRareChestCatalogAndEncounterGuards(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	for _, source := range []string{"rarity", "consumable", "encounter_runtime", "encounter_single", "encounter_list"} {
		for _, key := range []string{"health_potion", "brewed_health_potion", "harm_flask", "brewed_revival_potion"} {
			t.Run(source+"/"+key, func(t *testing.T) {
				oldItems, oldWeapons, oldNPC := config.GlobalItems, config.GlobalWeapons, character.NPCConfigInstance
				t.Cleanup(func() {
					config.GlobalItems = oldItems
					config.GlobalWeapons = oldWeapons
					character.NPCConfigInstance = oldNPC
				})
				d, _ := config.GetItemDefinition(key)
				if source == "rarity" || source == "consumable" {
					config.GlobalItems = &config.ItemSystemConfig{Items: map[string]*config.ItemDefinitionConfig{key: d}}
					config.GlobalWeapons = nil
					var ok bool
					if source == "rarity" {
						_, ok = rollCatalogItem("any", exactRarity(d.Rarity))
					} else {
						_, ok = rollCatalogItem("consumable", config.RarityRange{Min: 0, Max: config.RarityTier("unique")})
					}
					if ok == d.CraftedOnly {
						t.Fatalf("catalog eligibility=%v crafted=%v", ok, d.CraftedOnly)
					}
				} else if source == "encounter_runtime" {
					got := fixedItemRewards([]string{key})
					if (len(got) > 0) == d.CraftedOnly {
						t.Fatal("encounter reward crossed crafted-only boundary")
					}
				} else {
					name, shape := "treasure_chest", "{items: ["+key+"]}"
					if source == "encounter_list" {
						name = "treasure_chests"
						shape = "[" + shape + "]"
					}
					p := filepath.Join(t.TempDir(), "npcs.yaml")
					body := fmt.Sprintf("npcs:\n  test:\n    name: Test\n    type: merchant\n    render_category: standee\n    encounter:\n      rewards:\n        %s: %s\n", name, shape)
					if err := os.WriteFile(p, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
					err := character.LoadNPCConfig(p)
					if d.CraftedOnly {
						if err == nil || !strings.Contains(err.Error(), "crafted-only") {
							t.Fatalf("missing encounter guard: %v", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestRareBuffRecoveryAndUnconsciousAnchor(t *testing.T) {
	for _, key := range []string{"phase_veil", "quickening", "purify"} {
		t.Run(key, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, false)
			g.party.Members[1].AddCondition(character.ConditionPoisoned)
			if !g.useTechnique(0, key, false, false) {
				t.Fatal("cast refused")
			}
			if (c.RTCooldown == 0) != (key != "purify") {
				t.Fatalf("recovery=%d", c.RTCooldown)
			}
		})
	}
	for _, life := range []character.Condition{character.ConditionUnconscious, character.ConditionDead, character.ConditionEradicated} {
		t.Run(life.String(), func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, false)
			c.RareClass.Anchor = character.SpatialAnchor{MapKey: "forest", Frames: 100}
			c.HitPoints = 0
			c.AddCondition(life)
			g.tickRareClassClocks(1)
			if life == character.ConditionUnconscious {
				if c.RareClass.Anchor.Frames != 99 {
					t.Fatal("unconscious hero lost anchor")
				}
			} else if c.RareClass.Anchor.Frames != 0 {
				t.Fatal("dead hero retained anchor")
			}
		})
	}
}

func TestRareHarvestBoundedAndEventDriven(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	prev := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = prev })
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	world.GlobalWorldManager = wm
	if _, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml"); err != nil {
		t.Fatal(err)
	}
	g.updateAlchemyHarvest()
	if job := g.harvestRuntime.search; job == nil || job.cursor > harvestSearchTilesPerUpdate {
		t.Fatal("population flood was not bounded")
	}
	finishRareHarvest(t, g)
	if len(g.world.NPCs) == 0 {
		t.Fatal("incremental search produced no herbs")
	}
	marker := &character.NPC{Name: "Unrelated prop"}
	g.world.NPCs = append(g.world.NPCs, marker)
	original := append([]*character.NPC(nil), g.world.NPCs...)
	for _, active := range []bool{true, false} {
		if !active {
			g.party.Members[0] = character.CreateCharacter("Other", character.ClassKnight, g.config)
		} else {
			g.party.Members[0] = c
		}
		for i := 0; i < 20; i++ {
			g.updateAlchemyHarvest()
		}
		if !reflect.DeepEqual(original, g.world.NPCs) || g.harvestRuntime.search != nil {
			t.Fatal("unchanged frame rebuilt props or searched terrain")
		}
	}
	g.party.Members[0] = c
	g.advanceCalendarAtDawn()
	g.updateAlchemyHarvest()
	if g.harvestRuntime.search != nil {
		t.Fatal("full population searched terrain at dawn")
	}
}

func TestRareBookSafetyAndMaximumBatches(t *testing.T) {
	for _, safe := range []bool{false, true} {
		for _, route := range []string{"open", "switch"} {
			t.Run(fmt.Sprintf("safe=%v/%s", safe, route), func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, false)
				ih := NewInputHandler(g)
				if !safe {
					zoneVictim(t, g)
				}
				if route == "open" {
					ih.openTabbedMenu(TabSpellbook)
				} else {
					g.menuOpen = true
					g.currentTab = TabInventory
					ih.toggleTabbedMenu(TabSpellbook)
				}
				if (g.menuOpen && g.currentTab == TabSpellbook) != safe {
					t.Fatal("unsafe book entry")
				}
			})
		}
	}
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	g.party.Inventory = nil
	recipe := config.AlchemyRecipeByKey("health_potion")
	if recipe == nil {
		recipe = &config.GlobalAlchemy.Recipes[0]
	}
	for _, group := range recipe.Ingredients {
		alt := group.Alternatives[0]
		it, _ := items.TryCreateItemFromYAML(alt.Items[0])
		it.Quantity = alt.Count * 7
		g.party.AddItem(it)
	}
	before := append([]items.Item(nil), g.party.Inventory...)
	if got := g.party.MaxAlchemyBatches(recipe, nil); got != 7 {
		t.Fatalf("max=%d want 7", got)
	}
	if !reflect.DeepEqual(before, g.party.Inventory) {
		t.Fatal("maximum preview consumed ingredients")
	}
	if _, _, err := g.party.Brew(c, recipe, nil, 7); err != nil {
		t.Fatal(err)
	}
	if got := g.party.MaxAlchemyBatches(recipe, nil); got != 0 {
		t.Fatalf("max after brew=%d", got)
	}
}

func TestRareDisplayedWorkbenchMaxAndSafety(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
			g.selectedChar = 0
			g.menuOpen = true
			g.currentTab = TabInventory
			layout := computeTabbedMenuLayout(size[0], gameplayViewportBottom(g))
			var tab layoutRect
			for i, d := range tabbedMenuTabs {
				if d.tab == TabSpellbook {
					tab = layout.tabs[i]
				}
			}
			m := zoneVictim(t, g)
			h.clicks(false, tab.x+10, tab.y+10, 1)
			if g.currentTab == TabSpellbook {
				t.Fatal("unsafe displayed tab opened")
			}
			m.HitPoints = 0
			h.clicks(false, tab.x+10, tab.y+10, 1)
			if g.currentTab != TabSpellbook {
				t.Fatal("safe displayed tab refused")
			}
			g.selectedRare = 0
			r := &config.GlobalAlchemy.Recipes[0]
			g.party.Inventory = nil
			for _, group := range r.Ingredients {
				a := group.Alternatives[0]
				it, _ := items.TryCreateItemFromYAML(a.Items[0])
				it.Quantity = a.Count * 7
				g.party.AddItem(it)
			}
			l := computeRareBookLayout(layout.content, true)
			a := makeAlchemyMaterialLayout(l, r)
			maxButton := a.maximum
			h.clicks(false, maxButton.x+10, maxButton.y+10, 1)
			if g.alchemyBatches != 7 {
				t.Fatalf("Max selected %d batches", g.alchemyBatches)
			}
			h.clicks(false, a.brew.x+40, a.brew.y+15, 1)
			if got := g.party.MaxAlchemyBatches(r, nil); got != 0 {
				t.Fatalf("displayed Brew left %d batches", got)
			}
		})
	}
}

func TestRareHarvestPlacementAndYield(t *testing.T) {
	for _, rule := range []string{"allowed", "terrain", "excluded", "legacy_yield"} {
		t.Run(rule, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			world.GlobalWorldManager = wm
			prev := config.GlobalAlchemySpawns
			t.Cleanup(func() { config.GlobalAlchemySpawns = prev })
			cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cfg.Populations = []config.HarvestPopulation{{Map: "forest", Key: "ordinary", Count: 1, Yield: 3, Weights: map[string]int{"dawnleaf": 1}}}
			if rule == "terrain" {
				cfg.Placement.TerrainTypes = []string{"wall"}
			}
			if rule == "excluded" {
				cfg.Placement.ExcludeTiles = []string{world.GlobalTileManager.GetTileKey(world.TileEmpty)}
			}
			finishRareHarvest(t, g)
			if rule == "terrain" || rule == "excluded" {
				if len(g.world.NPCs) != 0 {
					t.Fatal("placement ignored YAML terrain or exclusion")
				}
				return
			}
			if len(g.world.NPCs) != 1 {
				t.Fatal("allowed population did not spawn")
			}
			p := g.alchemy.Populations["forest:ordinary"]
			if p.Nodes[0].Quantity != 3 {
				t.Fatal("authored yield was not saved")
			}
			want := 3
			if rule == "legacy_yield" {
				p.Nodes[0].Quantity = 0
				g.alchemy.Populations["forest:ordinary"] = p
				want = 1
			}
			n := g.world.NPCs[0]
			ts := float64(g.config.GetTileSize())
			placePlayerAtTile(g, TileIndex(n.X, ts)-1, TileIndex(n.Y, ts), ts)
			g.party.Inventory = nil
			if !g.gatherAlchemyReagent(n) || g.party.CountItemsByName("Dawnleaf") != want {
				t.Fatal("gather yield mismatch")
			}
		})
	}
}

func TestRareRegionalReagentWeights(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	prev := config.GlobalAlchemySpawns
	t.Cleanup(func() { config.GlobalAlchemySpawns = prev })
	cfg, err := config.LoadAlchemySpawns("../../assets/alchemy_spawns.yaml")
	if err != nil {
		t.Fatal(err)
	}
	favored := map[string][]string{
		"forest": {"dawnleaf", "bitterroot"}, "highlands": {"mooncap", "bitterroot"},
		"dragon_cliffs": {"embercap", "mooncap"}, "desert": {"embercap", "bitterroot"},
		"sakura_garden": {"dawnleaf", "mooncap"}, "deep_jungle": {"dawnleaf", "mooncap"},
	}
	for _, p := range cfg.Populations {
		if p.Key != "ordinary" {
			continue
		}
		t.Run(p.Map, func(t *testing.T) {
			for _, key := range []string{"dawnleaf", "mooncap", "bitterroot", "embercap"} {
				want := 1
				for _, favorite := range favored[p.Map] {
					if favorite == key {
						want = 3
					}
				}
				if p.Weights[key] != want {
					t.Fatalf("%s weight=%d want %d", key, p.Weights[key], want)
				}
			}
		})
	}
}
