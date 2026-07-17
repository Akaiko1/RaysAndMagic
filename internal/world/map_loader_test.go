package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// hasNPCKey reports whether a map spawned an NPC with the given key. Map tests
// assert key NPCs are PRESENT by name rather than counting spawns, so adding
// service NPCs (taverns, etc.) doesn't break unrelated tests.
func hasNPCKey(spawns []NPCSpawn, key string) bool {
	for _, s := range spawns {
		if s.NPCKey == key {
			return true
		}
	}
	return false
}

func TestMapLoader_SpecialTileByKey(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("load special tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	mapDir := t.TempDir()
	mapPath := filepath.Join(mapDir, "test.map")
	content := "@..  >[stile:spike_trap]\n"
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}

	ml := NewMapLoaderWithBiome(nil, "forest")
	mapData, err := ml.LoadMap(mapPath)
	if err != nil {
		t.Fatalf("load map: %v", err)
	}

	trapType, ok := tm.GetTileTypeFromKey("spike_trap")
	if !ok {
		t.Fatalf("spike_trap tile key not found")
	}
	if got := mapData.Tiles[0][0]; got != trapType {
		t.Fatalf("expected spike_trap at (0,0), got %v", got)
	}
}

// TestMapLoader_UnderEntityFloorDominantNeighbour: the auto-default floor under a
// placed entity ('@') matches the dominant FLOOR variant around it, not the bare
// biome '.' default - and stays the '.' default when neighbours are uniform.
func TestMapLoader_UnderEntityFloorDominantNeighbour(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	// japanese_castle has two walkable floor variants: '.'=cobble (default), ','=wood.
	wood, ok := tm.GetTileTypeFromLetterForBiome(",", "japanese_castle")
	if !ok {
		t.Fatalf("wood floor tile not found")
	}
	cobble, ok := tm.GetTileTypeFromLetterForBiome(".", "japanese_castle")
	if !ok {
		t.Fatalf("cobble floor tile not found")
	}
	if wood == cobble {
		t.Fatalf("test needs two distinct floor variants")
	}

	load := func(t *testing.T, content string) *MapData {
		t.Helper()
		p := filepath.Join(t.TempDir(), "t.map")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write map: %v", err)
		}
		ml := NewMapLoaderWithBiome(nil, "japanese_castle")
		md, err := ml.LoadMap(p)
		if err != nil {
			t.Fatalf("load map: %v", err)
		}
		return md
	}

	// '@' ringed by wood -> under-tile becomes wood (dominant), not the '.' cobble default.
	md := load(t, ",,,\n,@,\n,,,\n")
	if got := md.Tiles[1][1]; got != wood {
		t.Fatalf("under-entity tile = %v, want dominant wood %v", got, wood)
	}

	// '@' ringed by the default '.' floor -> unchanged (cobble).
	md = load(t, "...\n.@.\n...\n")
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want default cobble %v", got, cobble)
	}

	// 'W' (water) is render_type "floor_only" but NOT walkable: it must never be
	// voted as floor. '@' ringed only by water -> no floor neighbour -> biome '.'
	// fallback (cobble), never the impassable water tile.
	water, ok := tm.GetTileTypeFromLetterForBiome("W", "japanese_castle")
	if !ok {
		t.Fatalf("water tile not found")
	}
	md = load(t, "WWW\nW@W\nWWW\n")
	if got := md.Tiles[1][1]; got == water {
		t.Fatalf("under-entity tile became impassable water %v", water)
	}
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want '.' fallback cobble %v (water excluded)", got, cobble)
	}
}

func TestDominantNeighbourFloorForTile_HonorsExcludedUnderFloorTiles(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	tree, ok := tm.GetTileTypeFromKey("tree")
	if !ok {
		t.Fatal("forest tree tile not found")
	}
	ancientTree, ok := tm.GetTileTypeFromKey("ancient_tree")
	if !ok {
		t.Fatal("ancient tree tile not found")
	}
	stream, ok := tm.GetTileTypeFromKey("forest_stream")
	if !ok {
		t.Fatal("forest stream tile not found")
	}
	grass, ok := tm.GetTileTypeFromKey("empty")
	if !ok {
		t.Fatal("default forest floor tile not found")
	}

	// Stream wins the ordinary weighted vote (four orthogonal neighbours), but
	// both forest tree variants must inherit the lone grass neighbour instead.
	tiles := [][]TileType3D{
		{grass, stream, stream},
		{stream, tree, stream},
		{stream, stream, stream},
	}
	if got, ok := tm.DominantNeighbourFloor(tiles, 3, 3, 1, 1, nil); !ok || got != stream {
		t.Fatalf("ordinary dominant floor = %v, %t; want forest stream %v, true", got, ok, stream)
	}
	for _, owner := range []TileType3D{tree, ancientTree} {
		if got, ok := tm.DominantNeighbourFloorForTile(owner, tiles, 3, 3, 1, 1, nil); !ok || got != grass {
			t.Fatalf("tree %q inherited floor = %v, %t; want grass %v, true", tm.GetTileKey(owner), got, ok, grass)
		}
	}
}

func TestTileConfigurationRejectsUnknownExcludedUnderFloorTile(t *testing.T) {
	tm := NewTileManager()
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Type:                    "nature",
			RenderType:              "tree_sprite",
			ExcludedUnderFloorTiles: []string{"missing_floor"},
		},
	}
	if err := tm.validateTileConfiguration(); err == nil || !strings.Contains(err.Error(), "unknown under-floor tile") {
		t.Fatalf("unknown excluded under-floor tile must fail clearly, got: %v", err)
	}
}

func TestMapContractRejectsWrongLetterCase(t *testing.T) {
	tm := NewTileManager()
	tm.tileData = map[string]*config.TileData{
		"bad_tile": {Letter: "b", Type: "floor", RenderType: "floor_only"},
	}
	if err := tm.validateTileConfiguration(); err == nil || !strings.Contains(err.Error(), "reserved for monster") {
		t.Fatalf("lowercase tile letter must fail clearly, got: %v", err)
	}

	goodTiles := NewTileManager()
	if err := goodTiles.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	GlobalTileManager = goodTiles
	defer func() { GlobalTileManager = nil }()

	mapPath := filepath.Join(t.TempDir(), "bad_marker.map")
	if err := os.WriteFile(mapPath, []byte("B\n"), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	if _, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(mapPath); err == nil || !strings.Contains(err.Error(), "unknown uppercase map marker") {
		t.Fatalf("uppercase monster marker must fail clearly, got: %v", err)
	}
}

// Map content tests pin QUEST NPCs and MERCHANTS only - monster spawns are
// balance-tuned live and must never be pinned by count.
func TestClockTowerMapsCarryQuestAndMerchantNPCs(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	previousConfig := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = previousConfig }()

	cases := []struct {
		file     string
		biome    string
		wantNPCs []string
	}{
		// Stairs/exits are progression contract too: losing one soft-locks a floor.
		{"clock_tower_1.map", "clock_tower_workshop", []string{"clockmaker", "clock_tower_exit", "clock_stairs_to_2"}},
		{"clock_tower_2.map", "clock_tower_gearworks", []string{"clock_stairs_to_1", "clock_stairs_to_3"}},
		{"clock_tower_3.map", "clock_tower_belfry", []string{"clock_stairs_down_to_2"}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			md, err := NewMapLoaderWithBiome(nil, tc.biome).LoadMap(filepath.Join("..", "..", "assets", tc.file))
			if err != nil {
				t.Fatalf("load map: %v", err)
			}
			present := make(map[string]bool)
			for _, spawn := range md.NPCSpawns {
				present[spawn.NPCKey] = true
			}
			for _, key := range tc.wantNPCs {
				if !present[key] {
					t.Errorf("%s: quest/merchant NPC %q missing (have %v)", tc.file, key, present)
				}
			}
		})
	}
}
