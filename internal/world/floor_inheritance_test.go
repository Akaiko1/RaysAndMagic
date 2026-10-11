package world

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestEntityFloorEligibilityPreservesWalkability(t *testing.T) {
	tm := installTestTileManager(t)
	source, _ := tm.GetTileTypeFromKey("dragon_cliffs_basalt_floor")
	data := tm.GetTileData(source)
	original := *data
	t.Cleanup(func() { *data = original })
	for _, kind := range []string{"npc", "monster"} {
		for _, water := range []bool{false, true} {
			for _, walkable := range []bool{false, true} {
				for _, solid := range []bool{false, true} {
					for _, excluded := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/water=%v/walkable=%v/solid=%v/excluded=%v", kind, water, walkable, solid, excluded), func(t *testing.T) {
							*data = original
							if water {
								data.Type = "water"
							}
							data.Walkable, data.Solid, data.ExcludeAsUnderFloor = walkable, solid, excluded
							md := &MapData{Width: 2, Height: 1, Tiles: [][]TileType3D{{source, TileEmpty}}}
							if kind == "monster" {
								md.MonsterSpawns = []MonsterSpawn{{X: 1, MonsterKey: "floor_test_mob"}}
							} else {
								md.NPCSpawns = []NPCSpawn{{X: 1, NPCKey: "floor_test_npc"}}
							}
							md.RebuildFloors(tm, "dragon_cliffs")
							chosen, ok := md.Floors.At(1, 0)
							want := walkable && !excluded && (!water || kind == "monster")
							if ok != want || ok && chosen != source {
								t.Fatalf("unsafe automatic floor: got %v/%v", chosen, ok)
							}
							if tm.IsWalkable(source) != walkable || tm.IsSolid(source) != solid {
								t.Fatal("choosing appearance changed the donor's movement flags")
							}
						})
					}
				}
			}
		}
	}
	for _, render := range []string{config.TileRenderFloor, config.TileRenderWall} {
		t.Run("not a source/"+render, func(t *testing.T) {
			*data = original
			data.RenderType = render
			data.InheritFloor = render == config.TileRenderFloor
			md := &MapData{Width: 2, Height: 1, Tiles: [][]TileType3D{{source, TileEmpty}},
				NPCSpawns: []NPCSpawn{{X: 1, NPCKey: "floor_test_npc"}}}
			md.RebuildFloors(tm, "dragon_cliffs")
			if _, ok := md.Floors.At(1, 0); ok {
				t.Fatal("an inheriting marker or non-floor object became an authored source")
			}
		})
	}
}

func TestMapLoaderFloorPropagation(t *testing.T) {
	tm := installTestTileManager(t)
	previous := character.NPCConfigInstance
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"grounded": {GroundTile: "dragon_cliffs_chasm_edge_b"},
	}}
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	previousMonsters := monster.MonsterConfig
	monster.MonsterConfig = &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{
		"floor_test_mob": {Letter: "a", Biomes: []string{"dragon_cliffs"}},
	}}
	t.Cleanup(func() { monster.MonsterConfig = previousMonsters })
	for _, tc := range []struct {
		name, content, want string
		x                   int
		resolved            bool
	}{
		{"direct ground beside edge", "V@E  >[npc:chest]\n", "dragon_cliffs_basalt_floor", 1, true},
		{"chest waits for rocks", "VRRR@E  >[npc:chest]\n", "dragon_cliffs_basalt_floor", 4, true},
		{"monster waits for rocks", "VRRRaE\n", "dragon_cliffs_basalt_floor", 4, true},
		{"mixed monster and NPC chain", "VaR@aE  >[npc:chest]\n", "dragon_cliffs_basalt_floor", 4, true},
		{"entity chain is not a default seed", "V@@R@E  >[npc:chest], [npc:chest], [npc:chest]\n", "dragon_cliffs_basalt_floor", 4, true},
		{"book over chasm bottom", "C@E  >[npc:spell_lectern]\n", "dragon_cliffs_floor", 1, false},
		{"other chasm bottom through rocks", "FRRR@E  >[npc:chest]\n", "dragon_cliffs_floor", 4, false},
		{"monster over chasm bottom", "CRRRaE\n", "dragon_cliffs_floor", 4, false},
		{"water through entity chain", "WaR@aE  >[npc:chest]\n", "dragon_cliffs_floor", 4, false},
		{"stream through entity chain", "SR@E  >[npc:chest]\n", "dragon_cliffs_floor", 2, false},
		{"monster inherits stream", "SaE\n", "dragon_cliffs_stream", 1, true},
		{"keyed monster inherits stream", "S@E  >[monster:floor_test_mob]\n", "dragon_cliffs_stream", 1, true},
		{"stream through monster chain", "SaaaE\n", "dragon_cliffs_stream", 3, true},
		{"stream stops at NPC", "Sa@E  >[npc:chest]\n", "dragon_cliffs_floor", 2, false},
		{"stream cannot cross NPC", "Sa@aE  >[npc:chest]\n", "dragon_cliffs_floor", 3, false},
		{"stream cannot cross scenery", "SRaE\n", "dragon_cliffs_floor", 2, false},
		{"monster rejects blocked water", "WaE\n", "dragon_cliffs_floor", 1, false},
		{"monster rejects deep water", "QaE\n", "dragon_cliffs_floor", 1, false},
		{"explicit monster water stays authored", "V@E  >[monster:floor_test_mob@deep_water]\n", "deep_water", 1, true},
		{"explicit stream ground", "V@E  >[npc:chest@forest_stream]\n", "forest_stream", 1, true},
		{"deep water through marker", "QR@@E  >[stile:vteleporter], [npc:chest]\n", "dragon_cliffs_floor", 3, false},
		{"no source fallback", "ER@RE  >[npc:chest]\n", "dragon_cliffs_floor", 2, false},
		{"edge cannot carry floor", "VER@  >[npc:chest]\n", "dragon_cliffs_floor", 3, false},
		{"special marker is resolved before chest", "VR@@E  >[stile:vteleporter], [npc:chest]\n", "dragon_cliffs_basalt_floor", 3, true},
		{"general prop stays a prop", "VR$@E  >[tile:planks], [npc:chest]\n", "dragon_cliffs_basalt_floor", 3, true},
		{"explicit placement ground", "V@E  >[npc:chest@dragon_cliffs_chasm_edge_b]\n", "dragon_cliffs_chasm_edge_b", 1, false},
		{"explicit catalog ground", "V@E  >[npc:grounded]\n", "dragon_cliffs_chasm_edge_b", 1, false},
		{"explicit placement wins catalog", "V@E  >[npc:grounded@dragon_cliffs_basalt_floor]\n", "dragon_cliffs_basalt_floor", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "floor.map")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			md, err := NewMapLoaderWithBiome(nil, "dragon_cliffs").LoadMap(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := tm.GetTileKey(md.Tiles[0][tc.x]); got != tc.want {
				t.Fatalf("under-entity floor = %s, want %s", got, tc.want)
			}
			if _, ok := md.Floors.At(tc.x, 0); ok != tc.resolved {
				t.Fatalf("authored floor reached entity = %v, want %v", ok, tc.resolved)
			}
			for x, char := range tc.content {
				if char == ' ' || char == '\n' {
					break
				}
				if char == '$' && tm.GetShortLabelFromType(md.Tiles[0][x]) != "planks" {
					t.Fatal("ground resolution replaced the general prop")
				}
				if char == 'E' && tm.GetTileKey(md.Tiles[0][x]) != "dragon_cliffs_chasm_edge" {
					t.Fatal("ground resolution replaced an authored edge")
				}
			}
			w := &World3D{Tiles: md.Tiles, entityFloors: md.entityFloors}
			w.RebuildInheritedFloors()
			if got := tm.GetTileKey(w.Tiles[0][tc.x]); got != tc.want {
				t.Fatalf("world rebuild changed chosen ground to %s", got)
			}
			md = md.Clone()
			md.RebuildFloors(tm, "dragon_cliffs")
			if got := tm.GetTileKey(md.Tiles[0][tc.x]); got != tc.want {
				t.Fatalf("editor rebuild changed chosen ground to %s", got)
			}
		})
	}
}

func TestFloorPropagationPolicies(t *testing.T) {
	tm := installTestTileManager(t)
	tile := func(key string) TileType3D {
		t.Helper()
		v, ok := tm.GetTileTypeFromKey(key)
		if !ok {
			t.Fatal(key)
		}
		return v
	}
	grass, basalt := tile("empty"), tile("dragon_cliffs_basalt_floor")
	rock, tree, stream := tile("dragon_cliffs_boulder"), tile("tree"), tile("forest_stream")
	edge := tile("dragon_cliffs_chasm_edge")
	for _, tc := range []struct {
		name     string
		tiles    [][]TileType3D
		entities map[[2]int]FloorRecipient
		x, y     int
		want     TileType3D
		ok       bool
	}{
		{"water cannot propagate", [][]TileType3D{{tile("water"), rock, rock}}, nil, 2, 0, TileEmpty, false},
		{"stream cannot propagate", [][]TileType3D{{stream, rock, rock}}, nil, 2, 0, TileEmpty, false},
		{"stream from monster cannot enter scenery", [][]TileType3D{{stream, grass, rock}}, map[[2]int]FloorRecipient{{1, 0}: FloorMonster}, 2, 0, TileEmpty, false},
		{"scenery still finds land beside wet monster", [][]TileType3D{{stream, grass, rock, basalt}}, map[[2]int]FloorRecipient{{1, 0}: FloorMonster}, 2, 0, basalt, true},
		{"land beside stream wins", [][]TileType3D{{stream, rock, rock, grass}}, nil, 1, 0, grass, true},
		{"deep water cannot propagate", [][]TileType3D{{tile("deep_water"), rock, rock}}, nil, 2, 0, TileEmpty, false},
		{"land beside water wins", [][]TileType3D{{tile("water"), rock, rock, grass}}, nil, 1, 0, grass, true},
		{"land beside deep water wins", [][]TileType3D{{tile("deep_water"), rock, rock, grass}}, nil, 1, 0, grass, true},
		{"propagated owner exclusion", [][]TileType3D{{stream, rock, tree, rock, grass}}, nil, 2, 0, grass, true},
		{"excluded source cannot pass tree", [][]TileType3D{{stream, rock, tree, rock}}, nil, 3, 0, TileEmpty, false},
		{"same-wave tie stays stable", [][]TileType3D{{grass, rock, rock, rock, basalt}}, nil, 2, 0, grass, true},
		{"nearer source resolves first", [][]TileType3D{{grass, rock, rock, rock, rock, basalt}}, nil, 3, 0, basalt, true},
		{"monster placeholder waits", [][]TileType3D{{basalt, rock, grass, rock}}, map[[2]int]FloorRecipient{{2, 0}: FloorMonster}, 3, 0, basalt, true},
		{"eight-neighbour propagation", [][]TileType3D{{basalt, edge, edge}, {edge, rock, edge}, {edge, edge, rock}}, nil, 2, 2, basalt, true},
		{"orthogonal votes outweigh diagonal", [][]TileType3D{{basalt, grass, basalt}, {edge, rock, edge}, {edge, grass, edge}}, nil, 1, 1, grass, true},
		{"no fallback seeds", [][]TileType3D{{edge, grass, rock}}, map[[2]int]FloorRecipient{{1, 0}: FloorNPC}, 2, 0, TileEmpty, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tm.ResolveFloors(tc.tiles, tc.entities).At(tc.x, tc.y)
			if ok != tc.ok || ok && got != tc.want {
				t.Fatalf("resolved floor = %s/%v, want %s/%v", tm.GetTileKey(got), ok, tm.GetTileKey(tc.want), tc.ok)
			}
		})
	}
	// All water and authored exclusions are skipped, regardless of walkability.
	excluded := 0
	for key, data := range tm.ListTiles() {
		if data == nil || !(data.ExcludeAsUnderFloor || data.Type == "water") {
			continue
		}
		excluded++
		t.Run(key, func(t *testing.T) {
			edge := tile(key)
			if got, ok := tm.ResolveFloors([][]TileType3D{{edge, rock, grass}}, nil).At(1, 0); !ok || got != grass {
				t.Fatal("directional floor entered propagation")
			}
			if _, ok := tm.ResolveFloors([][]TileType3D{{edge, rock}}, nil).At(1, 0); ok {
				t.Fatal("resolution accepted directional floor")
			}
		})
	}
	if excluded == 0 {
		t.Fatal("no tile is authored exclude_as_under_floor (positive control)")
	}
}

func TestFloorPropagationStitchedEntities(t *testing.T) {
	wm, _ := bootOpenWorldTest(t)
	w := wm.OpenWorld
	tm := GlobalTileManager
	w.RebuildInheritedFloors()
	checked := 0
	for _, region := range wm.OpenWorldRegions {
		md, err := NewMapLoaderWithBiome(wm.config, wm.MapConfigs[region.MapKey].Biome).LoadMap("assets/" + wm.MapConfigs[region.MapKey].File)
		if err != nil {
			t.Fatal(err)
		}
		for cell, floor := range md.entityFloors {
			x, y := wm.ProjectTile(region.MapKey, cell[0], cell[1])
			if _, exists := w.entityFloors[[2]int{x, y}]; !exists {
				continue // Authored gate NPCs are removed when stitching.
			}
			checked++
			tile := w.Tiles[y][x]
			if tm.GetTileData(tile).ExcludeAsUnderFloor || !tm.IsWalkable(tile) || (tm.IsWater(tile) && floor.Recipient != FloorMonster) {
				t.Errorf("%s auto entity at %v inherited ineligible ground %s", region.MapKey, cell, tm.GetTileKey(tile))
			}
		}
	}
	if checked == 0 {
		t.Fatal("stitched world lost automatic entity ground metadata")
	}
}
