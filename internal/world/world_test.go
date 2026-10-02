package world

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestInitialMonsterKeysPersistAfterMapMonstersAreRemoved(t *testing.T) {
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	if _, err := monster.LoadMonsterConfig("../../assets/monsters.yaml"); err != nil {
		t.Fatalf("load monster config: %v", err)
	}

	w := NewWorld3D(createTestWorldConfig())
	w.loadMonstersFromMapData([]MonsterSpawn{
		{X: 1, Y: 1, MonsterKey: "mummy"},
		{X: 2, Y: 1, MonsterKey: "mummy"},
		{X: 3, Y: 1, MonsterKey: "minotaur"},
	}, "test_map")
	w.Monsters = nil // Simulate the party clearing the map.

	if len(w.InitialMonsterKeys) != 2 {
		t.Fatalf("initial monster kinds = %v, want mummy and minotaur", w.InitialMonsterKeys)
	}
	if _, ok := w.InitialMonsterKeys["mummy"]; !ok {
		t.Error("mummy missing from initial monster pool")
	}
	if _, ok := w.InitialMonsterKeys["minotaur"]; !ok {
		t.Error("minotaur missing from initial monster pool")
	}
}

// Every shipped map loads through the world manager's own path: the grid
// matches its declared size and an authored '+' start is a walkable tile.
func TestShippedMapsLoadWithWalkableStarts(t *testing.T) {
	wm, _ := bootWorldManagerTest(t, false)
	if len(wm.FailedMaps) > 0 {
		t.Fatalf("maps failed to load: %v", wm.FailedMaps)
	}
	if len(wm.LoadedMaps) != len(wm.MapConfigs) {
		t.Fatalf("loaded %d of %d configured maps", len(wm.LoadedMaps), len(wm.MapConfigs))
	}
	for key, w := range wm.LoadedMaps {
		if len(w.Tiles) != w.Height {
			t.Errorf("%s: %d tile rows, want height %d", key, len(w.Tiles), w.Height)
		}
		for y, row := range w.Tiles {
			if len(row) != w.Width {
				t.Errorf("%s: row %d has %d tiles, want width %d", key, y, len(row), w.Width)
			}
		}
		if w.StartX < 0 || w.StartY < 0 {
			continue // entered only through stairs or teleporters
		}
		if x, y := w.GetStartingPosition(); !w.CanMoveTo(x, y) {
			t.Errorf("%s: start tile (%d,%d) is not walkable", key, w.StartX, w.StartY)
		}
	}
}

func TestIsTileBlockingForMonster(t *testing.T) {
	prevTileManager := GlobalTileManager
	t.Cleanup(func() {
		GlobalTileManager = prevTileManager
	})

	GlobalTileManager = NewTileManager(testTileSizeClasses())
	if err := GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("Failed to load tile config: %v", err)
	}
	if err := GlobalTileManager.LoadSpecialTileConfig("../../assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}

	var blockedKey string
	var blockedType TileType3D
	for key, data := range GlobalTileManager.ListTiles() {
		if data == nil || data.Walkable {
			continue
		}
		if tileType, ok := GlobalTileManager.GetTileTypeFromKey(key); ok {
			blockedKey = key
			blockedType = tileType
			break
		}
	}

	if blockedKey == "" {
		t.Skip("No non-walkable tile key found in tiles config")
	}

	world := &World3D{
		Width:  1,
		Height: 1,
		Tiles:  [][]TileType3D{{blockedType}},
	}

	if !world.IsTileBlockingForMonster(0, 0, nil, false) {
		t.Fatalf("Expected tile %q to block without tile overrides", blockedKey)
	}

	if !world.IsTileBlockingForMonster(0, 0, []string{"__unmatched_tile__"}, false) {
		t.Fatalf("Expected tile %q to block for unmatched tile overrides", blockedKey)
	}

	if world.IsTileBlockingForMonster(0, 0, []string{blockedKey}, false) {
		t.Fatalf("Expected tile %q to be walkable for matching tile overrides", blockedKey)
	}

	chasmType, ok := GlobalTileManager.GetTileTypeFromKey("dragon_cliffs_chasm_floor")
	if !ok {
		t.Fatal("dragon_cliffs_chasm_floor is missing")
	}
	world.Tiles[0][0] = chasmType
	if !world.IsTileBlockingForMonster(0, 0, nil, false) {
		t.Fatal("chasm must block a ground monster")
	}
	if world.IsTileBlockingForMonster(0, 0, nil, true) {
		t.Fatal("authored fly_over chasm must admit a flying monster")
	}

	wallType, ok := GlobalTileManager.GetTileTypeFromKey("wall")
	if !ok {
		t.Fatal("wall tile is missing")
	}
	world.Tiles[0][0] = wallType
	if !world.IsTileBlockingForMonster(0, 0, nil, true) {
		t.Fatal("flying monster must not pass through an opaque wall")
	}
}

// Helper function to create minimal test configuration
func createTestWorldConfig() *config.Config {
	// Use the actual config loading if available, otherwise minimal config
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		// Fallback minimal config
		return &config.Config{
			World: config.WorldConfig{
				TileSize:  64,
				MapWidth:  50,
				MapHeight: 50,
			},
			Display: config.DisplayConfig{
				ScreenWidth:  800,
				ScreenHeight: 600,
			},
		}
	}
	return cfg
}
