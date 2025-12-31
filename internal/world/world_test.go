package world

import (
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestWorldGeneration(t *testing.T) {
	// Load tile manager configuration for world tests
	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Logf("Warning: Failed to load tile config: %v", err)
	}

	// Load monster configuration for world tests
	_, err := monster.LoadMonsterConfig("../../assets/monsters.yaml")
	if err != nil {
		t.Fatalf("Failed to load monster config: %v", err)
	}

	// Load NPC configuration for world tests
	err = character.LoadNPCConfig("../../assets/npcs.yaml")
	if err != nil {
		t.Fatalf("Failed to load NPC config: %v", err)
	}

	cfg := createTestWorldConfig()

	t.Run("World Creation", func(t *testing.T) {
		world := NewWorld3D(cfg)

		// Initialize the world with map loading or generation
		world.loadFromMapFile()

		if world == nil {
			t.Fatal("World should not be nil")
		}

		// Test world dimensions
		if world.Width <= 0 || world.Height <= 0 {
			t.Error("World should have positive dimensions")
		}

		// World should have proper tile array dimensions
		if len(world.Tiles) != world.Height {
			t.Errorf("Expected %d tile rows, got %d", world.Height, len(world.Tiles))
		}

		if len(world.Tiles) > 0 && len(world.Tiles[0]) != world.Width {
			t.Errorf("Expected %d tile columns, got %d", world.Width, len(world.Tiles[0]))
		}
	})

	t.Run("Starting Position", func(t *testing.T) {
		world := NewWorld3D(cfg)

		// Initialize the world with map loading or generation
		world.loadFromMapFile()
		startX, startY := world.GetStartingPosition()

		// Starting position should be non-negative
		if startX < 0 || startY < 0 {
			t.Errorf("Starting position should be non-negative, got (%.2f, %.2f)", startX, startY)
		}

		// Test tile access at starting position
		tileType := world.GetTileAt(startX, startY)

		// Tile type should be valid
		if tileType < TileEmpty || tileType > TileHighWall {
			t.Errorf("Invalid tile type %d at starting position", tileType)
		}

		// Test movement capability at starting position
		canMove := world.CanMoveTo(startX, startY)
		if !canMove {
			t.Error("Should be able to move to starting position")
		}
	})

	t.Run("Tile Types", func(t *testing.T) {
		world := NewWorld3D(cfg)

		// Initialize the world with map loading or generation
		world.loadFromMapFile()

		// Sample various positions and check for valid tile types
		testPositions := []struct {
			x, y int
		}{
			{0, 0}, {world.Width / 2, world.Height / 2}, {world.Width - 1, world.Height - 1},
			{10, 10}, {world.Width - 10, world.Height - 10},
		}

		for _, pos := range testPositions {
			if pos.x >= 0 && pos.x < world.Width && pos.y >= 0 && pos.y < world.Height {
				tileType := world.Tiles[pos.y][pos.x]

				// Tile types should be within valid range
				if tileType < TileEmpty || tileType > TileHighWall {
					t.Errorf("Invalid tile type %d at position (%d, %d)", tileType, pos.x, pos.y)
				}

				// Test GetTileAt method
				worldX := float64(pos.x * 64)
				worldY := float64(pos.y * 64)
				retrievedTile := world.GetTileAt(worldX, worldY)

				if retrievedTile != tileType {
					t.Errorf("GetTileAt mismatch at (%d, %d): expected %d, got %d", pos.x, pos.y, tileType, retrievedTile)
				}
			}
		}
	})
}

func TestWorldMovement(t *testing.T) {
	// Load tile manager configuration for world tests
	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Logf("Warning: Failed to load tile config: %v", err)
	}

	// Load monster configuration for world tests
	_, err := monster.LoadMonsterConfig("../../assets/monsters.yaml")
	if err != nil {
		t.Fatalf("Failed to load monster config: %v", err)
	}

	// Load NPC configuration for world tests
	err = character.LoadNPCConfig("../../assets/npcs.yaml")
	if err != nil {
		t.Fatalf("Failed to load NPC config: %v", err)
	}

	cfg := createTestWorldConfig()
	world := NewWorld3D(cfg)
	startX, startY := world.GetStartingPosition()

	t.Run("Movement API", func(t *testing.T) {
		// Test that movement functions don't panic
		_ = world.CanMoveTo(startX, startY)
		_ = world.GetTileAt(startX, startY)

		// Test nearby positions
		positions := []struct{ x, y float64 }{
			{startX + 64, startY},
			{startX - 64, startY},
			{startX, startY + 64},
			{startX, startY - 64},
		}

		for _, pos := range positions {
			_ = world.CanMoveTo(pos.x, pos.y)
			_ = world.GetTileAt(pos.x, pos.y)
		}
	})

	t.Run("Boundary Handling", func(t *testing.T) {
		// Test extreme coordinates don't cause panics
		_ = world.CanMoveTo(-1000, -1000)
		_ = world.CanMoveTo(10000, 10000)
		_ = world.GetTileAt(-1000, -1000)
		_ = world.GetTileAt(10000, 10000)
	})
}

func TestWorldMonsters(t *testing.T) {
	// Load tile manager configuration for world tests
	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Logf("Warning: Failed to load tile config: %v", err)
	}

	// Load monster configuration for world tests
	_, err := monster.LoadMonsterConfig("../../assets/monsters.yaml")
	if err != nil {
		t.Fatalf("Failed to load monster config: %v", err)
	}

	// Load NPC configuration for world tests
	err = character.LoadNPCConfig("../../assets/npcs.yaml")
	if err != nil {
		t.Fatalf("Failed to load NPC config: %v", err)
	}

	cfg := createTestWorldConfig()
	world := NewWorld3D(cfg)

	t.Run("Monster Placement", func(t *testing.T) {
		// Check that monsters exist and have valid properties
		for i, monster := range world.Monsters {
			if monster == nil {
				t.Errorf("Monster %d should not be nil", i)
				continue
			}

			// Basic position validation
			if monster.X < 0 || monster.Y < 0 {
				t.Errorf("Monster %d has invalid position: (%.2f, %.2f)", i, monster.X, monster.Y)
			}

			// Monster should be alive when spawned
			if !monster.IsAlive() {
				t.Errorf("Monster %d should be alive when spawned", i)
			}
		}
	})
}

func TestIsTileBlockingForHabitat(t *testing.T) {
	prevTileManager := GlobalTileManager
	t.Cleanup(func() {
		GlobalTileManager = prevTileManager
	})

	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("Failed to load tile config: %v", err)
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

	if !world.IsTileBlockingForHabitat(0, 0, nil, false) {
		t.Fatalf("Expected tile %q to block without habitat prefs", blockedKey)
	}

	if !world.IsTileBlockingForHabitat(0, 0, []string{"__non_habitat__"}, false) {
		t.Fatalf("Expected tile %q to block for non-habitat prefs", blockedKey)
	}

	if world.IsTileBlockingForHabitat(0, 0, []string{blockedKey}, false) {
		t.Fatalf("Expected tile %q to be walkable for matching habitat prefs", blockedKey)
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
