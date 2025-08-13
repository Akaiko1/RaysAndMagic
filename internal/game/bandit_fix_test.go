package game

import (
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

// TestBanditSpawningFix tests that the DRY walkability check fix works
func TestBanditSpawningFix(t *testing.T) {
	// Load configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Initialize the global tile manager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Logf("Warning: Failed to load tile config: %v", err)
	}

	// Create a simple world with walkable tiles
	testWorld := world.NewWorld3D(cfg)
	testWorld.Width = 10
	testWorld.Height = 10
	testWorld.Tiles = make([][]world.TileType3D, testWorld.Height)
	for i := range testWorld.Tiles {
		testWorld.Tiles[i] = make([]world.TileType3D, testWorld.Width)
		for j := range testWorld.Tiles[i] {
			testWorld.Tiles[i][j] = world.TileEmpty // All walkable
		}
	}

	// Create game and input handler
	game := &MMGame{
		config: cfg,
		world:  testWorld,
	}
	inputHandler := NewInputHandler(game)

	// Test that the fixed isPositionWalkable function works correctly
	tileSize := float64(cfg.GetTileSize())

	// Test that center positions are walkable
	center := tileSize / 2
	if !inputHandler.isPositionWalkable(center, center) {
		t.Error("Center of tile should be walkable")
	}

	// Test that findEncounterSpawnLocation can find valid locations
	npcX := 5 * tileSize // Center-ish of our small world
	npcY := 5 * tileSize

	successCount := 0
	for i := 0; i < 5; i++ {
		spawnX, spawnY := inputHandler.findEncounterSpawnLocation(npcX, npcY)
		if spawnX != 0 || spawnY != 0 {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("findEncounterSpawnLocation should be able to find valid spawn locations")
	}

	t.Logf("Successfully found spawn locations in %d/5 attempts", successCount)
}
