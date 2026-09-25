package main

import (
	"path/filepath"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestNPCGroundMoveCopyEraseRoundTrip(t *testing.T) {
	v, _ := dragTestViewer(t)
	for _, ground := range []string{"", "deep_water"} {
		for _, action := range []string{"move", "copy", "erase", "overwrite"} {
			t.Run(ground+"/"+action, func(t *testing.T) {
				md := &world.MapData{Width: 5, Height: 3, StartX: -1, StartY: -1,
					Tiles:     [][]world.TileType3D{{0, 0, 0, 0, 0}, {0, 0, 0, 0, 0}, {0, 0, 0, 0, 0}},
					NPCSpawns: []world.NPCSpawn{{X: 1, Y: 1, NPCKey: "portal_gate_highlands", GroundTile: ground}}}
				v.maps = []mapInfo{{Key: "test", Config: &config.MapConfig{Biome: "forest"}, Data: md}}
				v.mapIndex = 0
				m := &v.maps[0]
				rebuildMapFloors(m, v.tileManager)
				stamp := md.Tiles[1][1]
				switch action {
				case "move", "copy":
					if !v.dropAt(m, grabAt(m, 1, 1), 3, 1, action == "copy") {
						t.Fatal("drop failed")
					}
					if md.Tiles[1][3] != stamp {
						t.Fatal("destination lost NPC ground")
					}
				case "erase":
					v.brush = brush{kind: brushEraser}
					v.applyBrush(m, 1, 1)
				case "overwrite":
					v.brush = brush{kind: brushNPC, npcKey: "campfire"}
					v.applyBrush(m, 1, 1)
				}
				if action == "copy" {
					if md.Tiles[1][1] != stamp {
						t.Fatal("copy erased source ground")
					}
				} else if md.Tiles[1][1] != world.TileEmpty {
					t.Fatal("vacated/replaced NPC left a ground stamp")
				}
				v.savePath = filepath.Join(t.TempDir(), "ground.map")
				if err := v.saveCurrentMap(); err != nil {
					t.Fatal(err)
				}
				reloaded, err := world.NewMapLoaderWithBiome(v.cfg, "forest").LoadMap(v.savePath)
				if err != nil {
					t.Fatal(err)
				}
				compareMapData(t, md, reloaded)
			})
		}
	}
}

func TestBrushStrokeDefersFloorRebuild(t *testing.T) {
	v, m := dragTestViewer(t)
	rebuildMapFloors(m, v.tileManager)
	before := &m.Data.Floors[0][0]
	v.dragPainted = make(map[[2]int]bool)
	v.brush = brush{kind: brushEraser}
	for _, cell := range [][2]int{{1, 1}, {2, 2}, {3, 3}} {
		v.applyBrush(m, cell[0], cell[1])
		if &m.Data.Floors[0][0] != before {
			t.Fatal("held brush rebuilt the whole floor grid per cell")
		}
	}
	v.finishBrushFloors()
	after := &m.Data.Floors[0][0]
	if after == before || v.brushFloorMap != nil {
		t.Fatal("stroke release did not flush derived floors")
	}
	v.finishBrushFloors()
	if &m.Data.Floors[0][0] != after {
		t.Fatal("idle frame rebuilt floors again")
	}
}
