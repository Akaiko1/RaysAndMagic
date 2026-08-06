package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// installTestTileManager loads the shipped tiles into a fresh manager, installs
// it as the process global, and RESTORES the previous one - never nil, which
// would hand the next test a broken global under -shuffle.
func installTestTileManager(t *testing.T) *TileManager {
	t.Helper()
	tm := NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	prev := GlobalTileManager
	GlobalTileManager = tm
	t.Cleanup(func() { GlobalTileManager = prev })
	return tm
}

// restoreContentGlobals pins the content globals a test is about to overwrite
// (spell and NPC catalogs) back to their previous values afterwards.
func restoreContentGlobals(t *testing.T) {
	t.Helper()
	prevSpells, prevNPC := config.GlobalSpells, character.NPCConfigInstance
	t.Cleanup(func() {
		config.GlobalSpells, character.NPCConfigInstance = prevSpells, prevNPC
	})
}

// The isolation helper is shared infrastructure: it must RESTORE the previous
// global, never leave nil behind, or a shuffled run hands the next test a
// broken tile manager.
func TestInstallTestTileManagerRestoresThePrevious(t *testing.T) {
	outer := GlobalTileManager
	sentinel := NewTileManager(testTileSizeClasses())
	GlobalTileManager = sentinel
	t.Cleanup(func() { GlobalTileManager = outer })

	t.Run("inner", func(t *testing.T) {
		installTestTileManager(t)
		if GlobalTileManager == sentinel {
			t.Fatal("the helper did not install its own manager")
		}
	})
	if GlobalTileManager != sentinel {
		t.Fatalf("after the helper's cleanup the global is %p, want the previous %p", GlobalTileManager, sentinel)
	}
}

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
	tm := NewTileManager(testTileSizeClasses())
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
	tm := NewTileManager(testTileSizeClasses())
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

	// 'W' (water) is render_type "floor" but NOT walkable: it must never be
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
	tm := NewTileManager(testTileSizeClasses())
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
	tm := NewTileManager(testTileSizeClasses())
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Type:                    "nature",
			RenderType:              "crossed_standee",
			SizeClass:               "tree",
			Sprite:                  "tree",
			ExcludedUnderFloorTiles: []string{"missing_floor"},
		},
	}
	if err := tm.validateTileConfiguration(); err == nil || !strings.Contains(err.Error(), "unknown under-floor tile") {
		t.Fatalf("unknown excluded under-floor tile must fail clearly, got: %v", err)
	}
}

func TestMapContractRejectsWrongLetterCase(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	tm.tileData = map[string]*config.TileData{
		"bad_tile": {Letter: "b", Type: "floor", RenderType: "floor"},
	}
	if err := tm.validateTileConfiguration(); err == nil || !strings.Contains(err.Error(), "reserved for monster") {
		t.Fatalf("lowercase tile letter must fail clearly, got: %v", err)
	}

	goodTiles := NewTileManager(testTileSizeClasses())
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
	tm := NewTileManager(testTileSizeClasses())
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

// Silverbough and Dunehold are service towns behind a one-way landmark: drop
// the overworld gate and the town is unreachable, drop the inside gate and the
// party is stuck in it. Both ends must exist, along with the trades that are the
// only reason to walk there (bows, potions, armour, the two archives and the
// drill master).
func TestOutlandTownsCarryServiceNPCsAndBothGates(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	previousConfig := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = previousConfig }()

	// Presence only, never cells: maps are hand-edited and a gate may be moved
	// anywhere in its region without breaking anything.
	cases := []struct {
		file     string
		biome    string
		wantNPCs []string
	}{
		{file: "elf_city.map", biome: "elf_city", wantNPCs: []string{
			"elf_city_exit", "elf_city_archive", "elf_city_apothecary", "elf_city_bowyer"}},
		{file: "nomad_city.map", biome: "nomad_city", wantNPCs: []string{
			"nomad_city_exit", "nomad_city_caravan", "nomad_city_trainer", "nomad_city_spells"}},
		// The way in, from the region each town stands in. The elves live in
		// the highland pinewood, not the forest that carries their name.
		{file: "highlands.map", biome: "highlands", wantNPCs: []string{"silverbough_gate"}},
		{file: "desert.map", biome: "desert", wantNPCs: []string{"dunehold_gate"}},
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
					t.Errorf("%s: town NPC %q missing (have %v)", tc.file, key, present)
				}
			}
		})
	}
}

// NPCSpawnDefBody and ParseNPCSpawnDefBody are the ONE encode/decode pair for
// [npc:...] bodies - the loader and the editor's save path both use them.
func TestNPCSpawnDefBodyRoundTrip(t *testing.T) {
	cases := []NPCSpawn{
		{NPCKey: "chest_iron"},
		{NPCKey: "chest_iron", GroundTile: "deep_water"},
	}
	for _, want := range cases {
		body := NPCSpawnDefBody(want)
		key, ground, ok := ParseNPCSpawnDefBody(body)
		if !ok || key != want.NPCKey || ground != want.GroundTile {
			t.Fatalf("round trip %q -> (%q, %q, ok=%v), want (%q, %q, ok=true)", body, key, ground, ok, want.NPCKey, want.GroundTile)
		}
	}

	// Malformed bodies are rejected, not silently read as "no override".
	for _, bad := range []string{"", "chest_iron@", "@deep_water", "@"} {
		if key, ground, ok := ParseNPCSpawnDefBody(bad); ok {
			t.Errorf("malformed def %q parsed as (%q, %q)", bad, key, ground)
		}
	}
}

// A per-placement ground override rides the [npc:key@tile] def through the map
// loader; a plain def leaves it empty.
func TestMapLoader_NPCGroundOverride(t *testing.T) {
	installTestTileManager(t)

	mapPath := filepath.Join(t.TempDir(), "test.map")
	content := "@.@  >[npc:chest_iron@deep_water], [npc:campfire]\n"
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	mapData, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(mapPath)
	if err != nil {
		t.Fatalf("load map: %v", err)
	}
	if len(mapData.NPCSpawns) != 2 {
		t.Fatalf("spawn count = %d, want 2", len(mapData.NPCSpawns))
	}
	if s := mapData.NPCSpawns[0]; s.NPCKey != "chest_iron" || s.GroundTile != "deep_water" {
		t.Fatalf("override spawn = %+v, want chest_iron on deep_water", s)
	}
	if s := mapData.NPCSpawns[1]; s.NPCKey != "campfire" || s.GroundTile != "" {
		t.Fatalf("plain spawn = %+v, want campfire with no override", s)
	}
}

// The placement's [npc:key@tile] override outranks the NPC definition's own
// ground_tile - specific over general (world3d.loadNPCsFromMapData).
func TestPerPlacementGroundOverridesNPCDefinition(t *testing.T) {
	tm := installTestTileManager(t)
	restoreContentGlobals(t)
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := character.LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	deepWater, ok := tm.GetTileTypeFromKey("deep_water")
	if !ok {
		t.Fatal("deep_water tile key not found")
	}
	w := NewWorld3D(createTestWorldConfig())
	w.Width, w.Height = 4, 4
	w.Tiles = make([][]TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]TileType3D, w.Width)
	}

	w.loadNPCsFromMapData([]NPCSpawn{
		// portal_gate_highlands authors its own ground_tile; the placement
		// override must win over it.
		{X: 1, Y: 1, NPCKey: "portal_gate_highlands", GroundTile: "deep_water"},
		// chest_iron has no authored ground_tile; the override alone paints.
		{X: 2, Y: 2, NPCKey: "chest_iron", GroundTile: "deep_water"},
	})

	if got := w.Tiles[1][1]; got != deepWater {
		t.Fatalf("portal tile = %v, want the placement override to beat the NPC ground_tile", got)
	}
	if got := w.Tiles[2][2]; got != deepWater {
		t.Fatalf("chest tile = %v, want deep water", got)
	}
}

// A ground override is authored content: an unknown tile key must fail the map
// load, not degrade to ordinary floor behind a console warning.
func TestMapLoader_UnknownGroundOverrideFailsLoad(t *testing.T) {
	installTestTileManager(t)

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.map")
	if err := os.WriteFile(bad, []byte("@..  >[npc:chest_iron@dep_water]\n"), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	_, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(bad)
	if err == nil {
		t.Fatal("a typo in a ground override loaded silently")
	}
	for _, want := range []string{"chest_iron", "dep_water", "(0,0)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// Control: the correct key loads.
	good := filepath.Join(dir, "good.map")
	if err := os.WriteFile(good, []byte("@..  >[npc:chest_iron@deep_water]\n"), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	if _, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(good); err != nil {
		t.Fatalf("control failed: a valid override was rejected: %v", err)
	}
}

// An empty ground suffix reads as "no override" and would slip past the
// ground-tile validation, so the PARSER rejects it at load time.
func TestMapLoader_EmptyGroundOverrideFailsLoad(t *testing.T) {
	installTestTileManager(t)

	dir := t.TempDir()
	for _, tc := range []struct{ name, content string }{
		{"empty ground suffix", "@..  >[npc:chest_iron@]\n"},
		{"empty npc key", "@..  >[npc:@deep_water]\n"},
		{"empty body", "@..  >[npc:]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".map")
			if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write map: %v", err)
			}
			if _, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(p); err == nil {
				t.Fatal("a malformed npc def loaded silently")
			} else if !strings.Contains(err.Error(), "malformed npc def") {
				t.Fatalf("error %q does not name the malformed def", err)
			}
		})
	}
}
