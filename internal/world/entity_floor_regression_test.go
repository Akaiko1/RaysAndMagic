package world

import (
	"fmt"
	"slices"
	"testing"

	"ugataima/internal/config"
)

func TestAuthoredEntityGroundSurvivesRebuild(t *testing.T) {
	tm := installTestTileManager(t)
	for _, kind := range []string{"npc", "monster"} {
		for _, key := range []string{"dragon_cliffs_bridge", "water", "dragon_cliffs_chasm_floor"} {
			t.Run(kind+"/"+key, func(t *testing.T) {
				ground, _ := tm.GetTileTypeFromKey(key)
				chasm, _ := tm.GetTileTypeFromKey("dragon_cliffs_chasm_floor")
				md := &MapData{Width: 3, Height: 1, Tiles: [][]TileType3D{{chasm, ground, chasm}}}
				if kind == "npc" {
					md.NPCSpawns = []NPCSpawn{{X: 1, NPCKey: "ground_test"}}
				} else {
					md.MonsterSpawns = []MonsterSpawn{{X: 1, MonsterKey: "ground_test"}}
				}
				for i := 0; i < 2; i++ {
					md.RebuildFloors(tm, "dragon_cliffs")
					if md.Tiles[0][1] != ground {
						t.Fatalf("rebuild %d replaced authored %s", i, key)
					}
				}
				w := &World3D{Tiles: md.Tiles, entityFloors: md.entityFloors}
				w.RebuildInheritedFloors()
				if w.Tiles[0][1] != ground {
					t.Fatal("runtime rebuild replaced authored entity ground")
				}
			})
		}
	}
}

// Every shipped NPC without an authored ground stands on walkable automatic
// floor, split and stitched; a removed travel device takes its ground tile
// with it and leaves walkable floor behind.
func TestShippedEntitiesKeepSafeGroundAndRemovedPortalsLeaveNoTile(t *testing.T) {
	wm, ow := bootOpenWorldTest(t)
	tm := GlobalTileManager
	checked := 0
	for key, mc := range wm.MapConfigs {
		md, err := NewMapLoaderWithBiome(wm.config, mc.Biome).LoadMap("assets/" + mc.File)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		removed := ow.Removals[key].NPCs
		for _, spawn := range md.NPCSpawns {
			if spawn.groundTileKey() != "" {
				continue
			}
			checked++
			if !tm.IsWalkable(md.Tiles[spawn.Y][spawn.X]) {
				t.Errorf("%s/%s: split map inherited blocked entity ground", key, spawn.NPCKey)
			}
			if wm.IsOpenWorldRegion(key) && !slices.Contains(removed, spawn.NPCKey) {
				x, y := wm.ProjectTile(key, spawn.X, spawn.Y)
				if !tm.IsWalkable(wm.OpenWorld.Tiles[y][x]) {
					t.Errorf("%s/%s: stitched map inherited blocked entity ground", key, spawn.NPCKey)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no shipped NPC relies on automatic ground (positive control)")
	}
	for key, removal := range ow.Removals {
		mc := wm.MapConfigs[key]
		md, err := NewMapLoaderWithBiome(wm.config, mc.Biome).LoadMap("assets/" + mc.File)
		if err != nil {
			t.Fatal(err)
		}
		for _, spawn := range md.NPCSpawns {
			if !slices.Contains(removal.NPCs, spawn.NPCKey) || spawn.groundTileKey() == "" {
				continue
			}
			x, y := wm.ProjectTile(key, spawn.X, spawn.Y)
			ground, _ := tm.GetTileTypeFromKey(spawn.groundTileKey())
			if got := wm.OpenWorld.Tiles[y][x]; got == ground || !tm.IsWalkable(got) {
				t.Errorf("removed %s/%s left ground %q, want walkable floor without its ground tile",
					key, spawn.NPCKey, tm.GetTileKey(got))
			}
		}
	}
}

func TestRemovedNPCGroundOverrides(t *testing.T) {
	bootOpenWorldTest(t)
	tm := GlobalTileManager
	for _, override := range []string{"", "deep_water"} {
		for _, donor := range []string{"forest_stream", "water", "dragon_cliffs_chasm_floor", "dragon_cliffs_bridge", "wall"} {
			for _, edited := range []bool{false, true} {
				t.Run(fmt.Sprintf("override=%s/donor=%s/edited=%v", override, donor, edited), func(t *testing.T) {
					tile, ok := tm.GetTileTypeFromKey(donor)
					if !ok {
						t.Fatalf("missing donor %q", donor)
					}
					md := &MapData{Width: 4, Height: 3, Tiles: make([][]TileType3D, 3)}
					for y := range md.Tiles {
						md.Tiles[y] = []TileType3D{tile, tile, tile, tile}
					}
					for _, x := range []int{1, 2} {
						md.NPCSpawns = append(md.NPCSpawns, NPCSpawn{X: x, Y: 1, NPCKey: "portal_gate_highlands", GroundTile: override})
					}
					md.RebuildFloors(tm, "forest")
					want := TileEmpty
					if donor == "forest_stream" {
						want = tile
					}
					if edited {
						want, _ = tm.GetTileTypeFromKey("dragon_cliffs_basalt_floor")
						md.Tiles[1][1], md.Tiles[1][2] = want, want
					}
					if err := applyOpenWorldRemovals(&md.NPCSpawns, &md.SpecialTileSpawns, md,
						config.OpenWorldRemoval{NPCs: []string{"portal_gate_highlands"}}, "forest", TileEmpty); err != nil {
						t.Fatal(err)
					}
					w := &World3D{Tiles: md.Tiles, entityFloors: md.entityFloors}
					for _, stage := range []string{"removed", "runtime rebuild", "map rebuild"} {
						if stage == "runtime rebuild" {
							w.RebuildInheritedFloors()
						} else if stage == "map rebuild" {
							md.RebuildFloors(tm, "forest")
						}
						for _, x := range []int{1, 2} {
							if md.Tiles[1][x] != want || len(md.NPCSpawns) != 0 {
								t.Fatalf("%s cell %d: ground = %s, want %s; NPCs=%d", stage, x, tm.GetTileKey(md.Tiles[1][x]), tm.GetTileKey(want), len(md.NPCSpawns))
							}
						}
					}
				})
			}
		}
	}
}
