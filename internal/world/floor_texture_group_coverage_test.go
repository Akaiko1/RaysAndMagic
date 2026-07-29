package world

import (
	"sort"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// bootSplitMapsForFloorTest loads every shipped map as a separate world (no
// open-world stitching) so each grid can be checked against its own biome.
func bootSplitMapsForFloorTest(t *testing.T) *WorldManager {
	t.Helper()
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	prevTM, prevWM, prevMC := GlobalTileManager, GlobalWorldManager, monster.MonsterConfig
	t.Cleanup(func() {
		GlobalTileManager, GlobalWorldManager, monster.MonsterConfig = prevTM, prevWM, prevMC
	})

	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	wm := NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	GlobalWorldManager = wm
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	return wm
}

// Every floor group a map's tiles ask for must be defined by that map's biome,
// otherwise the renderer silently falls back to the tile's flat floor_color.
// This is how water stayed untextured in culverts/highlands/dragon_cliffs: the
// "water" group existed in four biomes and the load-time check only required it
// somewhere. Universal tiles belong in shared_floor_texture_groups.
func TestEveryMapTileFloorGroupIsDefinedByItsBiome(t *testing.T) {
	wm := bootSplitMapsForFloorTest(t)

	type gap struct{ mapKey, biome, tileKey, group string }
	var gaps []gap
	seen := make(map[string]bool)

	for mapKey, world := range wm.LoadedMaps {
		mapConfig, ok := wm.MapConfigs[mapKey]
		if !ok {
			t.Fatalf("loaded map %q has no config", mapKey)
		}
		groups := wm.Biomes[mapConfig.Biome].FloorTextureGroups
		for y := 0; y < world.Height; y++ {
			for x := 0; x < world.Width; x++ {
				data := GlobalTileManager.GetTileData(world.Tiles[y][x])
				if data == nil || data.FloorTextureGroup == "" {
					continue
				}
				if _, defined := groups[data.FloorTextureGroup]; defined {
					continue
				}
				tileKey := GlobalTileManager.GetTileKey(world.Tiles[y][x])
				id := mapKey + "/" + tileKey + "/" + data.FloorTextureGroup
				if seen[id] {
					continue
				}
				seen[id] = true
				gaps = append(gaps, gap{mapKey, mapConfig.Biome, tileKey, data.FloorTextureGroup})
			}
		}
	}

	sort.Slice(gaps, func(i, j int) bool { return gaps[i].mapKey < gaps[j].mapKey })
	for _, g := range gaps {
		t.Errorf("map %q (biome %q): tile %q wants floor group %q, which biome %q does not define - it will render flat color",
			g.mapKey, g.biome, g.tileKey, g.group, g.biome)
	}
}

// Water is universal: it must be textured in every biome from one definition,
// with no biome carrying its own copy of the same art.
func TestWaterFloorGroupIsSharedByEveryBiome(t *testing.T) {
	wm := bootSplitMapsForFloorTest(t)

	for name, biome := range wm.Biomes {
		texs, ok := biome.FloorTextureGroups["water"]
		if !ok {
			t.Errorf("biome %q has no water floor group", name)
			continue
		}
		if len(texs) == 0 {
			t.Errorf("biome %q water group is empty", name)
		}
	}
}

func TestMergeSharedFloorTextureGroups(t *testing.T) {
	shared := map[string][]string{
		"water": {"water_0", "water_1"},
		"lava":  {"lava_0"},
	}
	biome := map[string][]string{
		"default": {"grass_0"},
		"water":   {"swamp_0"}, // biome art wins over the shared list
	}

	merged := mergeSharedFloorTextureGroups(biome, shared)
	if got := merged["water"]; len(got) != 1 || got[0] != "swamp_0" {
		t.Errorf("biome override lost: water = %v", got)
	}
	if got := merged["lava"]; len(got) != 1 || got[0] != "lava_0" {
		t.Errorf("shared group not inherited: lava = %v", got)
	}
	if got := merged["default"]; len(got) != 1 || got[0] != "grass_0" {
		t.Errorf("biome group lost: default = %v", got)
	}
	if got := shared["water"]; len(got) != 2 {
		t.Errorf("merge mutated the shared table: water = %v", got)
	}
	if len(biome) != 2 {
		t.Errorf("merge mutated the biome table: %v", biome)
	}
}
