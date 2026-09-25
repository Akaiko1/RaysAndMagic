package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func brushScopeViewer(t *testing.T) *viewer {
	t.Helper()
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	data, err := os.ReadFile("../npcs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var npcs character.NPCConfig
	if err := yaml.Unmarshal(data, &npcs); err != nil {
		t.Fatal(err)
	}
	character.NPCConfigInstance = &npcs
	tm := world.NewTileManager(map[string]float64{})
	writeYAML := func(name string, content any) string {
		data, err := yaml.Marshal(content)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tiles := config.TileConfig{TileData: map[string]config.TileData{
		"ground":       {Letter: ".", Type: "floor", RenderType: "floor", Walkable: true},
		"universal":    {Letter: "T", Type: "wall", RenderType: "wall"},
		"override":     {Letter: "T", Type: "wall", RenderType: "wall", Biomes: []string{"forest"}},
		"scoped":       {Letter: "S", Type: "wall", RenderType: "wall", Biomes: []string{"culverts"}},
		"decor":        {ShortLabel: "decor", Type: "floor", RenderType: "floor", Walkable: true},
		"scoped_decor": {ShortLabel: "scoped_decor", Type: "floor", RenderType: "floor", Walkable: true, Biomes: []string{"culverts"}},
	}}
	if err := tm.LoadTileConfig(writeYAML("tiles.yaml", tiles)); err != nil {
		t.Fatal(err)
	}
	specials := config.SpecialTileConfig{SpecialTileData: map[string]config.TileData{
		"portal": {RenderType: "floor", Walkable: true},
	}}
	if err := tm.LoadSpecialTileConfig(writeYAML("specials.yaml", specials)); err != nil {
		t.Fatal(err)
	}
	return &viewer{tileManager: tm, monsterCfg: &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{
		"universal": {Letter: "a"},
		"override":  {Letter: "a", Biomes: []string{"forest"}},
		"scoped":    {Letter: "b", Biomes: []string{"culverts"}},
		"champion":  {Letter: "c", Champion: "test"},
	}}}
}

// Case table: every brush kind x matching/foreign/universal scope, including
// letter overrides, absent resources and champion exclusion. Each row runs
// through direct painting, map-switch refresh and collapsed-palette refresh.
// Rejected writes preserve the entire cell; accepted writes replace it.
// Brush/gesture state is transient. Persisted gate placement is covered below.
func TestBrushScopeAcrossEntryPoints(t *testing.T) {
	v := brushScopeViewer(t)
	cases := []struct {
		name, biome string
		brush       brush
		allowed     bool
	}{
		{"gate at home", "culverts", brush{kind: brushNPC, npcKey: "culvert_gate"}, true},
		{"gate abroad", "forest", brush{kind: brushNPC, npcKey: "culvert_gate"}, false},
		{"gate unspecified", "", brush{kind: brushNPC, npcKey: "culvert_gate"}, false},
		{"universal npc", "forest", brush{kind: brushNPC, npcKey: "reinforced_door"}, true},
		{"missing npc", "forest", brush{kind: brushNPC, npcKey: "missing"}, false},
		{"scoped tile home", "culverts", brush{kind: brushTile, tileKey: "scoped", letter: "S"}, true},
		{"scoped tile abroad", "forest", brush{kind: brushTile, tileKey: "scoped", letter: "S"}, false},
		{"universal tile", "culverts", brush{kind: brushTile, tileKey: "universal", letter: "T"}, true},
		{"shadowed tile", "forest", brush{kind: brushTile, tileKey: "universal", letter: "T"}, false},
		{"override tile", "forest", brush{kind: brushTile, tileKey: "override", letter: "T"}, true},
		{"wrong tile letter", "forest", brush{kind: brushTile, tileKey: "override", letter: "S"}, false},
		{"scoped decor home", "culverts", brush{kind: brushGeneral, tileKey: "scoped_decor"}, true},
		{"scoped decor abroad", "forest", brush{kind: brushGeneral, tileKey: "scoped_decor"}, false},
		{"universal decor", "forest", brush{kind: brushGeneral, tileKey: "decor"}, true},
		{"missing decor", "forest", brush{kind: brushGeneral, tileKey: "missing"}, false},
		{"scoped monster home", "culverts", brush{kind: brushMonster, monsterKey: "scoped"}, true},
		{"scoped monster abroad", "forest", brush{kind: brushMonster, monsterKey: "scoped"}, false},
		{"universal monster", "culverts", brush{kind: brushMonster, monsterKey: "universal"}, true},
		{"shadowed monster", "forest", brush{kind: brushMonster, monsterKey: "universal"}, false},
		{"override monster", "forest", brush{kind: brushMonster, monsterKey: "override"}, true},
		{"champion", "forest", brush{kind: brushMonster, monsterKey: "champion"}, false},
		{"missing monster", "forest", brush{kind: brushMonster, monsterKey: "missing"}, false},
		{"special", "forest", brush{kind: brushSpecialTile, tileKey: "portal"}, true},
		{"missing special", "forest", brush{kind: brushSpecialTile, tileKey: "missing"}, false},
		{"eraser", "forest", brush{kind: brushEraser}, true},
		{"none", "forest", brush{}, false},
	}
	for _, tc := range cases {
		for _, entry := range []string{"direct", "map switch", "collapsed"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				ground, _ := v.tileManager.GetTileTypeFromKey("ground")
				m := mapInfo{Config: &config.MapConfig{Biome: tc.biome}, Data: &world.MapData{
					Width: 1, Height: 1, Tiles: [][]world.TileType3D{{ground}},
					NPCSpawns:         []world.NPCSpawn{{NPCKey: "existing"}},
					MonsterSpawns:     []world.MonsterSpawn{{MonsterKey: "existing"}},
					SpecialTileSpawns: []world.SpecialTileSpawn{{TileKey: "existing"}},
				}}
				v.maps = []mapInfo{{Config: &config.MapConfig{Biome: "culverts"}}, m}
				v.mapIndex, v.legendCollapsed = 0, nil
				v.refreshLegend()
				v.brush = tc.brush
				if entry != "direct" {
					v.mapIndex = 1
					v.resetMapView()
					v.refreshLegend()
					if entry == "collapsed" {
						for _, row := range append([]legendEntry(nil), v.legendLines...) {
							if row.CollapseID != "" {
								v.toggleLegendCollapse(row.CollapseID)
							}
						}
						v.refreshLegend()
					}
					if tc.allowed && v.brush != tc.brush {
						t.Fatal("available brush lost on refresh")
					}
					if !tc.allowed && v.brush.kind != brushNone {
						t.Fatal("unavailable brush retained on refresh")
					}
				}
				before, _ := json.Marshal(m.Data)
				v.applyBrush(&m, 0, 0)
				after, _ := json.Marshal(m.Data)
				if !tc.allowed {
					if string(before) != string(after) {
						t.Fatal("rejected brush modified the cell")
					}
					return
				}
				wantTile := "ground"
				if tc.brush.kind == brushTile || tc.brush.kind == brushGeneral || tc.brush.kind == brushSpecialTile {
					wantTile = tc.brush.tileKey
				}
				if got := v.tileManager.GetTileKey(m.Data.Tiles[0][0]); got != wantTile {
					t.Fatalf("painted tile %q, want %q", got, wantTile)
				}
				if tc.brush.kind == brushNPC {
					if len(m.Data.NPCSpawns) != 1 || m.Data.NPCSpawns[0].NPCKey != tc.brush.npcKey {
						t.Fatal("NPC placement failed")
					}
				} else if len(m.Data.NPCSpawns) != 0 {
					t.Fatal("old NPC retained")
				}
				if tc.brush.kind == brushMonster {
					if len(m.Data.MonsterSpawns) != 1 || m.Data.MonsterSpawns[0].MonsterKey != tc.brush.monsterKey {
						t.Fatal("monster placement failed")
					}
				} else if len(m.Data.MonsterSpawns) != 0 {
					t.Fatal("old monster retained")
				}
				if tc.brush.kind == brushSpecialTile {
					if len(m.Data.SpecialTileSpawns) != 1 || m.Data.SpecialTileSpawns[0].TileKey != tc.brush.tileKey {
						t.Fatal("special tile placement failed")
					}
				} else if len(m.Data.SpecialTileSpawns) != 0 {
					t.Fatal("old special tile retained")
				}
			})
		}
	}
}

func TestMapSwitchCancelsSourceGestures(t *testing.T) {
	for _, kind := range []dragKind{dragMonster, dragNPC, dragSpecial, dragTile} {
		v := viewer{grab: dragState{kind: kind}, pendingGrab: dragState{kind: kind}, dragPainted: map[[2]int]bool{{0, 0}: true}}
		v.resetMapView()
		if v.grab.active() || v.pendingGrab.active() || v.dragPainted != nil {
			t.Fatalf("map switch retained gesture kind %v", kind)
		}
	}
}

func TestBrushOutsideMapPreservesData(t *testing.T) {
	v := brushScopeViewer(t)
	v.brush = brush{kind: brushNPC, npcKey: "culvert_gate"}
	m := mapInfo{Config: &config.MapConfig{Biome: "culverts"}, Data: &world.MapData{Tiles: [][]world.TileType3D{{world.TileEmpty}, {}}}}
	before, _ := json.Marshal(m.Data)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 2}, {0, 1}} {
		v.applyBrush(&m, p[0], p[1])
	}
	after, _ := json.Marshal(m.Data)
	if string(before) != string(after) {
		t.Fatal("out-of-bounds brush changed map data")
	}
}

func TestCulvertGateBrushSaveRoundTrip(t *testing.T) {
	v, _ := dragTestViewer(t)
	for _, biome := range []string{"culverts", "forest"} {
		t.Run(biome, func(t *testing.T) {
			floor, ok := v.tileManager.GetTileTypeFromLetterForBiome(".", biome)
			if !ok {
				t.Fatal("missing floor")
			}
			v.maps = []mapInfo{{Config: &config.MapConfig{Biome: biome}, Data: &world.MapData{
				Width: 2, Height: 2, Tiles: [][]world.TileType3D{{floor, floor}, {floor, floor}},
			}}}
			v.brush = brush{kind: brushNPC, npcKey: "culvert_gate"}
			v.applyBrush(&v.maps[0], 1, 1)
			v.savePath = filepath.Join(t.TempDir(), "gate.map")
			if err := v.saveCurrentMap(); err != nil {
				t.Fatal(err)
			}
			got, err := world.NewMapLoaderWithBiome(v.cfg, biome).LoadMap(v.savePath)
			if err != nil {
				t.Fatal(err)
			}
			if biome == "culverts" {
				if len(got.NPCSpawns) != 1 || got.NPCSpawns[0].NPCKey != "culvert_gate" || got.NPCSpawns[0].X != 1 || got.NPCSpawns[0].Y != 1 {
					t.Fatalf("gate lost on reload: %+v", got.NPCSpawns)
				}
			} else if len(got.NPCSpawns) != 0 {
				t.Fatalf("rejected gate persisted: %+v", got.NPCSpawns)
			}
		})
	}
}
