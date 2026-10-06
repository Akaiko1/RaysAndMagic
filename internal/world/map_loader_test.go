package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
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
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("special tiles: %v", err)
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

func TestMapLoader_SpecialTileByKey(t *testing.T) {
	tm := installTestTileManager(t)

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
	tm := installTestTileManager(t)

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
	md := load(t, ",,,\n,@,  >[npc:merchant]\n,,,\n")
	if got := md.Tiles[1][1]; got != wood {
		t.Fatalf("under-entity tile = %v, want dominant wood %v", got, wood)
	}

	// '@' ringed by the default '.' floor -> unchanged (cobble).
	md = load(t, "...\n.@.  >[npc:merchant]\n...\n")
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want default cobble %v", got, cobble)
	}

	// Automatic entity ground stays walkable; water requires an explicit override.
	md = load(t, "WWW\nW@W  >[npc:merchant]\nWWW\n")
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want walkable fallback %v", got, cobble)
	}
}

func TestResolveFloorsHonorsExcludedUnderFloorTiles(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("special tiles: %v", err)
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
	rock, _ := tm.GetTileTypeFromKey("moss_rock")
	tiles[1][1] = rock
	if got, ok := tm.ResolveFloors(tiles, nil).At(1, 1); !ok || got != stream {
		t.Fatalf("ordinary dominant floor = %v, %t; want forest stream %v, true", got, ok, stream)
	}
	for _, owner := range []TileType3D{tree, ancientTree} {
		tiles[1][1] = owner
		if got, ok := tm.ResolveFloors(tiles, nil).At(1, 1); !ok || got != grass {
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
	prev := GlobalTileManager
	GlobalTileManager = goodTiles
	t.Cleanup(func() { GlobalTileManager = prev })

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
	installTestTileManager(t)

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
	installTestTileManager(t)

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
			assertMapCarriesNPCs(t, tc.file, tc.biome, tc.wantNPCs)
		})
	}
}

// assertMapCarriesNPCs checks PRESENCE only, never cells: maps are hand-edited
// and an NPC may be moved anywhere in its map without breaking anything.
func assertMapCarriesNPCs(t *testing.T, file, biome string, want []string) *MapData {
	t.Helper()
	md, err := NewMapLoaderWithBiome(nil, biome).LoadMap(filepath.Join("..", "..", "assets", file))
	if err != nil {
		t.Fatalf("load map: %v", err)
	}
	present := make(map[string]bool)
	for _, spawn := range md.NPCSpawns {
		present[spawn.NPCKey] = true
	}
	for _, key := range want {
		if !present[key] {
			t.Errorf("%s: NPC %q missing (have %v)", file, key, present)
		}
	}
	return md
}

// Yusra's service gate (shrine_lamps) asks for three lamps, ONE PER PYRAMID
// FLOOR. Lose a floor's lamp and the errand can never be finished, so her Light
// and Dark pages stay shut forever.
func TestPyramidFloorsCarryTheShrineLamps(t *testing.T) {
	installTestTileManager(t)

	previousConfig := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = previousConfig }()

	for floor, lamp := range map[string]string{
		"pyramid_1.map": "shrine_lamp_1",
		"pyramid_2.map": "shrine_lamp_2",
		"pyramid_3.map": "shrine_lamp_3",
	} {
		t.Run(floor, func(t *testing.T) {
			assertMapCarriesNPCs(t, floor, "pyramid", []string{lamp})
		})
	}
}

// Zone maps carry their quest givers, exits and split-world travel tiles, the
// boss the zone is built around, and at least target_count authored targets
// for every quest they host - read from quests.yaml, never pinned here.
func TestZoneMapsCarryQuestNPCsAndQuestTargets(t *testing.T) {
	installTestTileManager(t)
	restoreContentGlobals(t)
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := character.LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	previousConfig := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	t.Cleanup(func() { monster.MonsterConfig = previousConfig })
	questCfg, err := quests.LoadQuestConfig(filepath.Join("..", "..", "assets", "quests.yaml"))
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}

	cases := []struct {
		file, biome  string
		npcs         []string
		specialTiles []string
		bosses       []string
		quests       []string
	}{
		{file: "culverts.map", biome: "culverts",
			npcs: []string{"culverts_exit", "culverts_oldman"}, bosses: []string{"golden_thief_bug"}, quests: []string{"culverts_valves"}},
		{file: "japanese_castle.map", biome: "japanese_castle",
			npcs: []string{"japanese_castle_exit", "castle_oldman"}, bosses: []string{"old_samurai"}, quests: []string{"castle_armory"}},
		{file: "deep_jungle.map", biome: "jungle",
			npcs: []string{"deep_jungle_exit"}, bosses: []string{"orc_hero_boss"}},
		{file: "dragon_cliffs.map", biome: "dragon_cliffs",
			npcs:         []string{"dragon_cliffs_ranger", "dragon_cliffs_bone_hermit", "dragon_cliffs_ember_lair", "dragon_cliffs_bone_lair"},
			specialTiles: []string{"vteleporter"}, quests: []string{"dragon_cliffs_troll_cull", "dragon_cliffs_ember_rites"}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			md := assertMapCarriesNPCs(t, tc.file, tc.biome, tc.npcs)
			if md.StartX < 0 || md.StartY < 0 {
				t.Errorf("no start position parsed (%d,%d)", md.StartX, md.StartY)
			}
			for _, key := range tc.specialTiles {
				if !hasSpecialTileKey(md.SpecialTileSpawns, key) {
					t.Errorf("special tile %q missing", key)
				}
			}
			for _, key := range tc.bosses {
				if countMatchingMonsterSpawns(md, func(k string) bool { return k == key }) == 0 {
					t.Errorf("boss %q missing", key)
				}
			}
			for _, id := range tc.quests {
				def := questCfg.Quests[id]
				if def == nil || def.TargetCount <= 0 {
					t.Fatalf("quest %q is not authored with a target_count", id)
				}
				var have int
				switch def.Type {
				case quests.QuestTypeKill:
					have = countMatchingMonsterSpawns(md, def.MatchesTarget)
				case quests.QuestTypeInteract:
					have = countQuestPropSpawns(md, id, def)
				default:
					t.Fatalf("quest %q type %q has no map target to count", id, def.Type)
				}
				if have < def.TargetCount {
					t.Errorf("quest %q needs %d targets, the map authors %d", id, def.TargetCount, have)
				}
			}
		})
	}
}

func hasSpecialTileKey(spawns []SpecialTileSpawn, key string) bool {
	for _, s := range spawns {
		if s.TileKey == key {
			return true
		}
	}
	return false
}

func countMatchingMonsterSpawns(md *MapData, match func(string) bool) int {
	n := 0
	for _, s := range md.MonsterSpawns {
		if match(s.MonsterKey) {
			n++
		}
	}
	return n
}

// countQuestPropSpawns counts the placed NPCs whose authored prop choice
// credits this quest's interact tag.
func countQuestPropSpawns(md *MapData, questID string, def *quests.QuestDefinition) int {
	n := 0
	for _, s := range md.NPCSpawns {
		npc := character.NPCConfigInstance.NPCs[s.NPCKey]
		if npc == nil {
			continue
		}
		credits := false
		_ = npc.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			credits = credits || (c.Prop != nil && c.QuestID == questID && def.MatchesTarget(c.Prop.Tag))
			return nil
		})
		if credits {
			n++
		}
	}
	return n
}

// Every monster must fit a 1-wide gap: a body >= the tile spans a 2x2
// footprint (half-open bounds) and wedges in corridors, doorways and foliage
// gaps. The spawned body is its size_class entry of world.monster_bodies.
func TestMonsterBodiesFitOneTileGaps(t *testing.T) {
	cfg, err := config.LoadConfig(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tile := cfg.GetTileSize()
	previousConfig := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	t.Cleanup(func() { monster.MonsterConfig = previousConfig })

	for key, def := range monster.MonsterConfig.Monsters {
		body, ok := cfg.MonsterBodyTiles(def.SizeClass)
		if !ok {
			t.Fatalf("%s: size_class %q has no body", key, def.SizeClass)
		}
		w, h := monster.NewMonster3DFromConfig(0, 0, key, cfg).GetSize()
		if w != body*tile || h != body*tile || w > config.MaxMonsterBodyTiles*tile || w >= tile {
			t.Errorf("%s (%s) body %gx%g, want %g and under the tile", key, def.SizeClass, w, h, body*tile)
		}
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

func TestMapLoaderRejectsAuthoredFish(t *testing.T) {
	installTestTileManager(t)
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	for _, disposition := range []string{"", "wildlife", monster.DispositionFish} {
		t.Run("disposition="+disposition, func(t *testing.T) {
			monster.MonsterConfig = &monster.MonsterYAMLConfig{Monsters: map[string]monster.MonsterDefinition{
				"test_actor": {Letter: "a", Disposition: disposition},
			}}
			path := filepath.Join(t.TempDir(), "actor.map")
			if err := os.WriteFile(path, []byte("+a.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			data, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(path)
			if disposition == monster.DispositionFish {
				if err == nil || !strings.Contains(err.Error(), "test_actor") || !strings.Contains(err.Error(), "ecology") {
					t.Fatalf("missing load-time fish error: %v", err)
				}
			} else if err != nil || len(data.MonsterSpawns) != 1 {
				t.Fatalf("ordinary authored spawn rejected: %v", err)
			}
		})
	}
}
