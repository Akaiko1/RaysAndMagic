package game

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// All physical reward kinds share terrain policy, including the target cell.
// Water is exempt without a buff; only Fly crosses a chasm. Test both pickup
// dispatch and a stale selection that reaches the consumption handler directly.
func TestWorldRewardReach(t *testing.T) {
	cfg := crateTestGame(t).config
	config.GlobalSpells.Spells["spirit_walk_fixture"] = &config.SpellDefinitionConfig{TerrainPassage: true}
	spellConfig := config.GlobalSpells
	t.Cleanup(func() { delete(spellConfig.Spells, "spirit_walk_fixture") })
	setTestWorldManager(t, nil)
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	tm := world.NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	world.GlobalTileManager = tm
	spriteManager := graphics.NewSpriteManager()
	ts := cfg.GetTileSize()
	for _, terrain := range []struct {
		key             string
		walking, flying bool
	}{
		{"empty", true, true},
		{"dragon_cliffs_chasm_floor", false, true},
		{"dragon_cliffs_chasm_floor_b", false, true},
		{"water", true, true},
		{"deep_water", true, true},
		{"wall", false, false},
		{"tree", false, false},
		{"dragon_cliffs_boulder", false, false},
	} {
		for _, placement := range []string{"target", "between", "start", "corner", "same_cell"} {
			for _, ability := range []string{"none", "water_magic", "fly", "spirit_walk"} {
				for _, kind := range []string{"spell_lectern", "chest_wooden", "bag", "encounter_chest"} {
					for _, entry := range []string{"selection", "stale_selection"} {
						t.Run(fmt.Sprintf("%s/%s/%s/%s/%s", terrain.key, placement, ability, kind, entry), func(t *testing.T) {
							g := newTestGame(cfg, newTestWorldSized(cfg, 10, 10))
							g.sprites = spriteManager
							g.renderHelper = NewRenderingHelper(g)
							g.camera.FOV = squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight())
							g.camera.ViewDist = 5000
							px, py, rx, ry, bx, by := 3.5, 3.5, 4.5, 3.5, 4, 3
							switch placement {
							case "between":
								px, rx, bx = 3.9, 5.1, 4
							case "start":
								bx = 3
							case "corner":
								ry = 4.5
							case "same_cell":
								rx, bx = 3.7, 3
							}
							g.camera.X, g.camera.Y = px*ts, py*ts
							g.camera.Angle = math.Atan2(ry-py, rx-px)
							g.world.Tiles[by][bx] = terrainTile(t, terrain.key)
							g.flyActive = ability == "fly"
							spiritActive, spiritDuration := ability == "spirit_walk", 600
							g.gameLoop = &GameLoop{game: g}
							g.gameLoop.timedBuffRegistry = append(g.buildTimedBuffs(), timedBuff{id: "spirit_walk_fixture", active: &spiritActive, duration: &spiritDuration})
							// Deliberately unsynchronized: reach must use live party Fly.
							g.world.SetTerrainPassageActive(!g.flyActive)
							g.walkOnWaterActive = ability == "water_magic"
							g.waterBreathingActive = ability == "water_magic"
							want := terrain.walking || (g.flyActive || spiritActive) && terrain.flying
							gold, inventory := g.party.Gold, len(g.party.Inventory)
							if kind == "bag" || kind == "encounter_chest" {
								containerKind := ContainerKindLootBag
								if kind == "encounter_chest" {
									containerKind = ContainerKindTreasureChest
								}
								g.groundContainers = []GroundContainer{{Kind: containerKind, X: rx * ts, Y: ry * ts, Gold: 7, Sprite: "missing_reach_fixture"}}
								if entry == "selection" {
									// Mouse and Space share the terrain filter. At a normal
									// projected distance verify the actual mouse hit-test too.
									if placement != "same_cell" {
										info := g.groundContainerRenderInfo(&g.groundContainers[0], -1)
										if !info.Visible {
											t.Fatal("reward fixture not visible")
										}
										idx := g.findGroundContainerIndexAtScreen(info.ScreenX, info.ScreenY+info.SpriteSize/2, 2*ts)
										if (idx == 0) != want {
											t.Fatalf("mouse selection = %d, reachable = %v", idx, want)
										}
									}
									if got := g.tryPickupNearestGroundContainer(2 * ts); got != want {
										t.Fatalf("nearest pickup = %v, want %v", got, want)
									}
								} else {
									g.pickupGroundContainerAt(0)
								}
								if (len(g.groundContainers) == 0) != want {
									t.Fatalf("container consumed = %v, want %v", len(g.groundContainers) == 0, want)
								}
								if want && g.party.Gold != gold+7 {
									t.Fatal("reachable container did not grant gold")
								}
							} else {
								for _, member := range g.party.Members {
									member.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
								}
								reader := g.party.Members[0]
								reader.MagicSchools[character.MagicSchoolAir] = &character.MagicSkill{}
								npc := spawnCrate(t, g, kind, rx*ts, ry*ts)
								input := NewInputHandler(g)
								if entry == "selection" {
									g.updateFocusedNPC()
									visible := g.collisionSystem.CheckLineOfSight(g.camera.X, g.camera.Y, npc.X, npc.Y)
									if (g.focusedNPC == npc) != visible {
										t.Fatalf("NPC focus = %v, visible = %v", g.focusedNPC == npc, visible)
									}
									if visible {
										input.tryFocusedNPCInteraction()
									} else {
										input.openNPCInteraction(npc)
									}
								} else {
									g.focusedNPC = npc
									input.tryFocusedNPCInteraction()
								}
								if npc.Visited != want {
									t.Fatalf("NPC consumed = %v, want %v", npc.Visited, want)
								}
								if !want {
									messages := g.GetCombatMessages()
									if len(messages) == 0 || !strings.Contains(messages[len(messages)-1], "can't reach") {
										t.Fatal("unreachable reward attempt gave no feedback")
									}
								}
								if kind == "spell_lectern" {
									learned := false
									for _, id := range npc.Lectern.Pool {
										learned = learned || reader.KnowsSpell(spells.SpellID(id))
									}
									if learned != want {
										t.Fatalf("spell granted = %v, want %v", learned, want)
									}
								}
							}
							if !want && (g.party.Gold != gold || len(g.party.Inventory) != inventory) {
								t.Fatal("blocked pickup granted a reward")
							}
						})
					}
				}
			}
		}
	}
}

// Parse a neutral authored tile to prove both YAML override directions reach
// the production pickup path without any water/chasm tile-name special case.
func TestWorldRewardReachYAMLOverrides(t *testing.T) {
	cfg := crateTestGame(t).config
	setTestWorldManager(t, nil)
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	for _, tc := range []struct {
		name              string
		walkable, flyOver bool
		property          string
		walking, flying   bool
	}{
		{"walkable_default", true, false, "", true, true},
		{"unwalkable_default", false, false, "", false, false},
		{"walkable_override", true, false, "    blocks_pickup: true\n", false, false},
		{"unwalkable_override", false, false, "    blocks_pickup: false\n", true, true},
		{"open_airspace", false, true, "    blocks_pickup: true\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tiles.yaml")
			source := fmt.Sprintf("tiles:\n  reward_terrain:\n    name: Reach terrain\n    type: floor\n    render_type: floor\n    transparent: true\n    walkable: %v\n    fly_over: %v\n%s", tc.walkable, tc.flyOver, tc.property)
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			tm := world.NewTileManager(testTileSizeClasses())
			if err := tm.LoadTileConfig(path); err != nil {
				t.Fatal(err)
			}
			world.GlobalTileManager = tm
			for _, flying := range []bool{false, true} {
				g := newTestGame(cfg, newTestWorldSized(cfg, 10, 10))
				ts := cfg.GetTileSize()
				placePlayerAtTile(g, 3, 3, ts)
				g.world.Tiles[3][4] = terrainTile(t, "reward_terrain")
				g.flyActive = flying
				g.groundContainers = []GroundContainer{{X: 4.5 * ts, Y: 3.5 * ts, Gold: 7}}
				g.pickupGroundContainerAt(0)
				want := tc.walking
				if flying {
					want = tc.flying
				}
				if (len(g.groundContainers) == 0) != want {
					t.Fatalf("Fly=%v: pickup = %v, want %v", flying, len(g.groundContainers) == 0, want)
				}
			}
		})
	}
}
