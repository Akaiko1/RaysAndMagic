package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestEditorFloorPropagation(t *testing.T) {
	v, _ := dragTestViewer(t)
	tm := v.tileManager
	for _, tc := range []struct {
		name, content, want string
	}{
		{"chest through rocks", "VRRR@E  >[npc:chest_wooden]\n", "dragon_cliffs_basalt_floor"},
		{"book over chasm", "CRRR@E  >[npc:spell_lectern]\n", "dragon_cliffs_floor"},
		{"water through rocks", "WRRR@E  >[npc:chest_wooden]\n", "dragon_cliffs_floor"},
		{"deep water through rocks", "QRRR@E  >[npc:chest_wooden]\n", "dragon_cliffs_floor"},
		{"edge only fallback", "ERRR@E  >[npc:chest_wooden]\n", "dragon_cliffs_floor"},
		{"explicit edge stays authored", "VRRR@E  >[npc:chest_wooden@dragon_cliffs_chasm_edge_b]\n", "dragon_cliffs_chasm_edge_b"},
		{"special marker", "VR@R@E  >[stile:vteleporter], [npc:chest_wooden]\n", "dragon_cliffs_basalt_floor"},
		{"general object", "VR$R@E  >[tile:planks], [npc:chest_wooden]\n", "dragon_cliffs_basalt_floor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "floor.map")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			loader := world.NewMapLoaderWithBiome(v.cfg, "dragon_cliffs")
			md, err := loader.LoadMap(path)
			if err != nil {
				t.Fatal(err)
			}
			v.maps = []mapInfo{{Key: "floor", Config: &config.MapConfig{Biome: "dragon_cliffs"}, Data: md}}
			v.mapIndex, v.savePath = 0, path
			m := &v.maps[0]
			rebuildMapFloors(m, tm)
			if got := tm.GetTileKey(md.Tiles[0][4]); got != tc.want {
				t.Fatalf("editor chose %s, want %s", got, tc.want)
			}
			if tc.want != "dragon_cliffs_chasm_edge_b" && tc.want != "dragon_cliffs_floor" {
				floor, _ := tm.GetTileTypeFromKey(tc.want)
				data := tm.GetTileData(floor)
				byKey := map[string]*config.TileData{tc.want: data}
				if got := floorUnderObjectColor(*m, tm, byKey, 3, 0, color.RGBA{}); got != colorFromRGB(data.FloorColor) {
					t.Fatalf("editor object preview disagrees with entity ground: %v", got)
				}
			}
			if err := v.saveCurrentMap(); err != nil {
				t.Fatal(err)
			}
			reloaded, err := loader.LoadMap(path)
			if err != nil {
				t.Fatal(err)
			}
			compareMapData(t, md, reloaded)
		})
	}
}

func TestEditorFloorPropagationAfterPaintAndDrag(t *testing.T) {
	v, _ := dragTestViewer(t)
	tm := v.tileManager
	rock, _ := tm.GetTileTypeFromKey("dragon_cliffs_boulder")
	edge, _ := tm.GetTileTypeFromKey("dragon_cliffs_chasm_edge")
	basalt, _ := tm.GetTileTypeFromKey("dragon_cliffs_basalt_floor")
	for _, action := range []string{"paint", "move", "copy"} {
		t.Run(action, func(t *testing.T) {
			v.maps = []mapInfo{{Config: &config.MapConfig{Biome: "dragon_cliffs"}, Data: &world.MapData{
				Width: 6, Height: 1, StartX: -1, StartY: -1,
				Tiles:     [][]world.TileType3D{{basalt, rock, rock, edge, world.TileEmpty, edge}},
				NPCSpawns: []world.NPCSpawn{{X: 4, Y: 0, NPCKey: "chest_wooden"}},
			}}}
			m := &v.maps[0]
			rebuildMapFloors(m, tm)
			if tm.GetTileKey(m.Data.Tiles[0][4]) != "dragon_cliffs_floor" {
				t.Fatal("disconnected chest did not start with fallback")
			}
			if action == "paint" {
				v.brush = brush{kind: brushTile, letter: "R", tileKey: "dragon_cliffs_boulder"}
				v.applyBrush(m, 3, 0)
			} else {
				if !v.dropAt(m, grabAt(m, 1, 0), 3, 0, action == "copy") {
					t.Fatal("drag failed")
				}
			}
			want := basalt
			if action == "move" {
				// Moving the rock leaves the editor's deliberately painted default
				// ground at its old position, now the nearest authored source.
				want, _ = tm.GetTileTypeFromKey("dragon_cliffs_floor")
			}
			if got := m.Data.Tiles[0][4]; got != want {
				t.Fatalf("%s left stale ground %s, want %s", action, tm.GetTileKey(got), tm.GetTileKey(want))
			}
			if _, ok := m.Data.Floors.At(3, 0); !ok {
				t.Fatal("editor did not rebuild the moved/painted prop")
			}
		})
	}
}
