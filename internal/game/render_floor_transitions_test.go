package game

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func terrainTestRenderer(t *testing.T, biome string) (*Renderer, *world.WorldManager) {
	t.Helper()
	g, wm, _ := ecologyTestGame(t)
	if err := wm.LoadMapConfigs("../../assets/map_configs.yaml"); err != nil {
		t.Fatal(err)
	}
	wm.CurrentMapKey = "terrain_test"
	wm.MapConfigs[wm.CurrentMapKey] = &config.MapConfig{Biome: biome}
	wm.LoadedMaps[wm.CurrentMapKey] = g.world
	r := &Renderer{game: g, floorTexGroups: map[string]floorTextureGroup{}, floorColorCache: map[[2]int]color.RGBA{}}
	groups := []string{}
	for name := range wm.Biomes[biome].FloorTextureGroups {
		groups = append(groups, name)
	}
	sort.Strings(groups)
	for i, name := range groups {
		r.floorTexGroups[name] = floorTextureGroup{start: i, count: 1}
	}
	for y := 0; y < g.world.Height; y++ {
		for x := 0; x < g.world.Width; x++ {
			r.floorColorCache[[2]int{x, y}] = color.RGBA{100, 110, 120, 255}
		}
	}
	return r, wm
}

func terrainTile(t *testing.T, key string) world.TileType3D {
	t.Helper()
	v, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
	if !ok {
		t.Fatal(key)
	}
	return v
}

func TestTerrainProfilesCoverShippedGroups(t *testing.T) {
	_, wm := terrainTestRenderer(t, "forest")
	for biome, b := range wm.Biomes {
		for group := range b.FloorTextureGroups {
			if _, ok := b.FloorTransitions[group]; !ok {
				t.Errorf("unaudited group %s/%s", biome, group)
			}
		}
	}
}

func TestTerrainResolvedInputs(t *testing.T) {
	for _, tc := range []struct {
		name, biome, tile, ground string
		profile                   byte
		shore                     bool
	}{
		{"bare", "forest", "empty", "empty", floorBlendNatural, true},
		{"inherited prop", "forest", "palm_tree", "empty", floorBlendNatural, true},
		{"spawn marker", "forest", "spawn", "empty", floorBlendNatural, true},
		{"water", "forest", "water", "empty", floorBlendWater, false},
		{"deep water", "forest", "deep_water", "empty", floorBlendWater, false},
		{"stream", "forest", "forest_stream", "empty", floorBlendWater, false},
		{"no beach biome", "highlands", "empty", "empty", floorBlendNatural, false},
		{"authored garden shore", "sakura_garden", "empty", "empty", floorBlendHard, false},
		{"cliff east", "dragon_cliffs", "dragon_cliffs_chasm_edge", "dragon_cliffs_floor", floorBlendCliffEast, false},
		{"cliff west", "dragon_cliffs", "dragon_cliffs_chasm_edge_b", "dragon_cliffs_floor", floorBlendCliffWest, false},
		{"void", "dragon_cliffs", "dragon_cliffs_chasm_floor", "dragon_cliffs_floor", floorBlendVoid, false},
		{"void variant", "dragon_cliffs", "dragon_cliffs_chasm_floor_b", "dragon_cliffs_floor", floorBlendVoid, false},
		{"bridge", "dragon_cliffs", "dragon_cliffs_bridge", "dragon_cliffs_floor", floorBlendHard, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := terrainTestRenderer(t, tc.biome)
			ground, tile := terrainTile(t, tc.ground), terrainTile(t, tc.tile)
			for y := 9; y <= 11; y++ {
				for x := 9; x <= 11; x++ {
					r.game.world.Tiles[y][x] = ground
				}
			}
			r.game.world.Tiles[10][10] = tile
			r.game.world.Tiles[10][12] = terrainTile(t, "water")
			_, indices, shore := r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
			encoded := indices.RGBAAt(10, 10)
			if encoded.B%8 != tc.profile || (encoded.G != 0) != tc.shore || encoded.R == 0 {
				t.Fatalf("material=%v, profile=%d shore=%v", encoded, tc.profile, tc.shore)
			}
			if tc.shore && int(encoded.R) == r.floorTexGroups["beach"].start+1 {
				t.Fatal("beach replaced underlying ground")
			}
			if shore.RGBAAt(12, 10).R != 0 || shore.RGBAAt(7, 10).R == 0 {
				t.Fatal("distance field did not preserve the water edge")
			}
			before := append([]byte(nil), indices.Pix...)
			r.game.world.Monsters = []*monster.Monster3D{{X: 10 * 64, Y: 10 * 64}}
			r.game.world.Monsters[0].X += 64
			_, again, _ := r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
			if !bytes.Equal(before, again.Pix) || r.game.world.Tiles[10][10] != tile {
				t.Fatal("moving actor changed terrain")
			}
		})
	}
}

func TestTerrainMonsterMarkerResolvesBeforeTransitions(t *testing.T) {
	r, _ := terrainTestRenderer(t, "forest")
	// Reuse a real forest monster marker, selected from the loaded catalog.
	marker := ""
	for _, key := range []string{"g", "w", "r"} {
		if def, _, err := monster.MonsterConfig.GetMonsterByLetterForBiome(key, "forest"); err == nil && def.Disposition != monster.DispositionFish {
			marker = key
			break
		}
	}
	if marker == "" {
		t.Fatal("missing forest monster fixture")
	}
	file := filepath.Join(t.TempDir(), "terrain.map")
	if err := os.WriteFile(file, []byte(".....\n.....\n.."+marker+"..\n.....\n..+..\n"), 0600); err != nil {
		t.Fatal(err)
	}
	md, err := world.NewMapLoaderWithBiome(r.game.config, "forest").LoadMap(file)
	if err != nil {
		t.Fatal(err)
	}
	r.game.world.Tiles = md.Tiles
	r.game.world.Width = md.Width
	r.game.world.Height = md.Height
	_, indices, _ := r.prepareFloorMaps(md.Width, md.Height)
	if len(md.MonsterSpawns) != 1 || indices.RGBAAt(2, 2).B%8 != floorBlendNatural || indices.RGBAAt(2, 2).R != indices.RGBAAt(1, 2).R {
		t.Fatal("monster marker became a different floor material")
	}
}

func TestTerrainOpenWorldRestoreAndQuestRebuild(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	r := g.gameLoop.renderer
	_, before, shore := r.prepareFloorMaps(g.world.Width, g.world.Height)
	seen := map[string]bool{}
	for _, region := range wm.OpenWorldRegions {
		for y := region.OffsetY; y < region.OffsetY+region.Height; y++ {
			for x := region.OffsetX; x < region.OffsetX+region.Width; x++ {
				m := r.resolvedFloorMaterial(x, y, g.world.Tiles[y][x])
				if m.atlas == 0 {
					continue
				}
				seen[region.MapKey] = true
				if wm.Biomes[r.floorBiomeKeyAt(x, y)].FloorTransitions == nil {
					t.Fatalf("region %s lost its floor policy", region.MapKey)
				}
				if before.RGBAAt(x, y).B&0x38 != 0 {
					t.Fatalf("region %s rotated authored floor UVs", region.MapKey)
				}
			}
		}
	}
	for _, key := range []string{"forest", "desert", "highlands", "dragon_cliffs", "deep_jungle", "japanese_castle", "sakura_garden"} {
		if !seen[key] {
			t.Errorf("region %s has no resolved textures", key)
		}
	}
	md, err := world.NewMapLoaderWithBiome(g.config, "forest").LoadMap("assets/forest.map")
	if err != nil {
		t.Fatal(err)
	}
	overrides := 0
	for _, npc := range md.NPCSpawns {
		if npc.GroundTile != "deep_water" {
			continue
		}
		x, y := wm.ProjectTile("forest", npc.X, npc.Y)
		if got := before.RGBAAt(x, y); got.B%8 != floorBlendWater || got.G != 0 {
			t.Fatalf("NPC ground override lost in transition map: %v", got)
		}
		overrides++
	}
	if overrides == 0 {
		t.Fatal("missing authored underwater chest fixture")
	}
	save := g.buildSave(wm)
	r.floorShoreMap.Deallocate()
	r.floorShoreMap = nil
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if r.floorShoreMap == nil {
		t.Fatal("save restore skipped derived shoreline rebuild")
	}
	_, after, afterShore := r.prepareFloorMaps(g.world.Width, g.world.Height)
	if !bytes.Equal(before.Pix, after.Pix) || !bytes.Equal(shore.Pix, afterShore.Pix) {
		t.Fatal("save round trip changed derived terrain materials")
	}
	if err := g.questManager.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatal(err)
	}
	g.questManager.MarkCompleted("forest_wolf_cull")
	r.floorShoreMap.Deallocate()
	r.floorShoreMap = nil
	g.applyCompletedQuestTiles()
	if r.floorShoreMap == nil {
		t.Fatal("quest tile update skipped shoreline rebuild")
	}
	for _, change := range g.questManager.Definitions()["forest_wolf_cull"].OnCompleteTiles {
		x, y := wm.ProjectTile("forest", change.X, change.Y)
		if m := r.resolvedFloorMaterial(x, y, g.world.Tiles[y][x]); m.profile != floorBlendHard || m.shore != 0 {
			t.Fatalf("quest bridge still receives a water/shore blend: %+v", m)
		}
	}
}

func TestTerrainInteriorKeepsSingleMaterialPath(t *testing.T) {
	r, _ := terrainTestRenderer(t, "forest")
	ground := terrainTile(t, "empty")
	for y := range r.game.world.Tiles {
		for x := range r.game.world.Tiles[y] {
			r.game.world.Tiles[y][x] = ground
		}
	}
	_, indices, _ := r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
	if got := indices.RGBAAt(10, 10); got.B != floorBlendNatural || got.G != 0 {
		t.Fatalf("uniform ground enabled expensive boundary/shore paths: %v", got)
	}
	r.game.world.Tiles[10][12] = terrainTile(t, "water")
	_, indices, _ = r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
	if got := indices.RGBAAt(11, 10); got.B&floorBlendBoundary == 0 || got.B&floorBlendShore == 0 || got.G == 0 {
		t.Fatalf("shore did not enable transition paths: %v", got)
	}
	if got := indices.RGBAAt(6, 10); got.B != floorBlendNatural || got.G != 0 {
		t.Fatalf("local water change affected remote ground: %v", got)
	}
}

func TestTerrainVariantSelectionAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, biome, tile, group string
		count                    int
		blend, shore             bool
	}{
		{"single sand", "desert", "empty", "default", 1, true, false},
		{"two sand", "desert", "empty", "default", 2, true, false},
		{"three sand", "desert", "empty", "default", 3, true, false},
		{"four sand", "desert", "empty", "default", 4, true, false},
		{"water", "forest", "water", "water", 2, true, false},
		{"void", "dragon_cliffs", "dragon_cliffs_chasm_floor", "chasm_floor_0", 2, true, false},
		{"hard", "japanese_castle", "japanese_castle_wood", "wood", 2, false, false},
		{"beach overlay", "forest", "empty", "beach", 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := terrainTestRenderer(t, tc.biome)
			if _, ok := r.floorTexGroups[tc.group]; !ok {
				t.Fatalf("missing fixture group %s", tc.group)
			}
			// Isolate this group's variants without overlapping another atlas entry.
			r.floorTexGroups[tc.group] = floorTextureGroup{start: 100, count: tc.count}
			for y := range r.game.world.Tiles {
				for x := range r.game.world.Tiles[y] {
					r.game.world.Tiles[y][x] = terrainTile(t, tc.tile)
					if tc.shore && x%4 == 0 {
						r.game.world.Tiles[y][x] = terrainTile(t, "water")
					}
				}
			}
			w, h := r.game.world.Width, r.game.world.Height
			_, indices, _ := r.prepareFloorMaps(w, h)
			_, rebuilt, _ := r.prepareFloorMaps(w, h)
			if !bytes.Equal(indices.Pix, rebuilt.Pix) {
				t.Fatal("variant selection changed on cache rebuild")
			}
			counts := make([]int, tc.count)
			pairs, same, changed := 0, 0, 0
			for y := 1; y < h-1; y++ {
				for x := 1; x < w-1; x++ {
					a := indices.RGBAAt(x, y)
					v := a.R
					if tc.shore {
						v = a.G
					}
					if v < 101 || int(v) >= 101+tc.count {
						continue
					}
					counts[int(v)-101]++
					for _, step := range [][2]int{{1, 0}, {0, 1}} {
						b := indices.RGBAAt(x+step[0], y+step[1])
						n := b.R
						if tc.shore {
							n = b.G
						}
						if n < 101 || int(n) >= 101+tc.count {
							continue
						}
						pairs++
						if n == v {
							same++
							continue
						}
						changed++
						if (a.B&floorBlendBoundary != 0) != tc.blend || (b.B&floorBlendBoundary != 0) != tc.blend {
							t.Fatalf("variant boundary must opt in on both sides: %v %v", a, b)
						}
					}
				}
			}
			if pairs == 0 {
				t.Fatal("fixture exercised no variant neighbors")
			}
			if tc.count > 1 {
				rate := float64(same) / float64(pairs)
				expected := 1.0 / float64(tc.count)
				if changed == 0 || rate < expected*0.6 || rate > expected*1.4 {
					t.Fatalf("patterned variants: equal-neighbor fraction %.3f, want near %.3f", rate, expected)
				}
			}
			for i, n := range counts {
				if n == 0 {
					t.Fatalf("variant %d never selected", i)
				}
			}
		})
	}
}
