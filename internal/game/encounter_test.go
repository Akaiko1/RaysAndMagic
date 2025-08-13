package game

import (
	"math"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestBanditSpawning(t *testing.T) {
	// Load configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Initialize the global tile manager for the test
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Logf("Warning: Failed to load tile config: %v", err)
	}

	// Create a test world
	testWorld := world.NewWorld3D(cfg)

	// Initialize tiles array manually (since NewWorld3D doesn't do this)
	testWorld.Width = 50
	testWorld.Height = 50
	testWorld.Tiles = make([][]world.TileType3D, testWorld.Height)
	for i := range testWorld.Tiles {
		testWorld.Tiles[i] = make([]world.TileType3D, testWorld.Width)
	}

	// Fill world with walkable tiles
	for y := 0; y < testWorld.Height; y++ {
		for x := 0; x < testWorld.Width; x++ {
			testWorld.Tiles[y][x] = world.TileEmpty // Walkable tile
		}
	}

	// Create a minimal game instance for testing
	game := &MMGame{
		config: cfg,
		world:  testWorld,
	}

	// Create input handler
	inputHandler := NewInputHandler(game)

	// Verify the game can access the world properly
	if game.GetCurrentWorld() == nil {
		t.Fatal("Game.GetCurrentWorld() returned nil")
	}

	t.Run("Position Walkability Check", func(t *testing.T) {
		// Test that our fixed isPositionWalkable function works
		tileSize := float64(cfg.GetTileSize())

		// Debug: Check if tile manager is working
		if world.GlobalTileManager == nil {
			t.Fatal("GlobalTileManager is nil")
		}

		// Debug: Check if TileEmpty is walkable according to tile manager
		isWalkable := world.GlobalTileManager.IsWalkable(world.TileEmpty)
		if !isWalkable {
			t.Errorf("TileEmpty should be walkable according to tile manager, but got false")
		}

		// Debug: Check what tile is actually at position (0,0)
		tileAt00 := testWorld.Tiles[0][0]
		t.Logf("Tile at (0,0): %v, expected TileEmpty: %v", tileAt00, world.TileEmpty)

		// Test center of tile (0,0) - should be walkable (TileEmpty)
		result := inputHandler.isPositionWalkable(tileSize/2, tileSize/2)
		if !result {
			t.Errorf("Expected position (%.1f, %.1f) to be walkable", tileSize/2, tileSize/2)
		}

		// Test center of tile (1,1) - should be walkable
		result = inputHandler.isPositionWalkable(tileSize*1.5, tileSize*1.5)
		if !result {
			t.Errorf("Expected position (%.1f, %.1f) to be walkable", tileSize*1.5, tileSize*1.5)
		}

		// Test out of bounds - should not be walkable
		result = inputHandler.isPositionWalkable(-10, -10)
		if result {
			t.Error("Expected out of bounds position to not be walkable")
		}

		// Test way out of bounds
		result = inputHandler.isPositionWalkable(100000, 100000)
		if result {
			t.Error("Expected far out of bounds position to not be walkable")
		}
	})

	t.Run("Encounter Spawn Location Finding", func(t *testing.T) {
		// Test finding spawn locations around a central point
		tileSize := float64(cfg.GetTileSize())
		npcX := tileSize * 25 // Center of world
		npcY := tileSize * 25

		found := 0
		attempts := 10

		for i := 0; i < attempts; i++ {
			spawnX, spawnY := inputHandler.findEncounterSpawnLocation(npcX, npcY)

			if spawnX != 0 || spawnY != 0 { // Valid spawn location found
				found++

				// Verify the spawn location is actually walkable
				if !inputHandler.isPositionWalkable(spawnX, spawnY) {
					t.Errorf("findEncounterSpawnLocation returned non-walkable position (%.1f, %.1f)", spawnX, spawnY)
				}

				// Verify it's within reasonable distance (3-5 tiles + some tolerance)
				dx := spawnX - npcX
				dy := spawnY - npcY
				distance := math.Sqrt(dx*dx + dy*dy)
				minDist := 3.0 * tileSize
				maxDist := 6.0 * tileSize // 5 tiles + tolerance

				if distance < minDist || distance > maxDist {
					t.Errorf("Spawn location distance %.1f is outside expected range [%.1f, %.1f]",
						distance, minDist, maxDist)
				}
			}
		}

		if found == 0 {
			t.Error("findEncounterSpawnLocation failed to find any valid spawn locations")
		}

		t.Logf("Successfully found %d/%d valid spawn locations", found, attempts)
	})

	t.Run("Non-Walkable Area Test", func(t *testing.T) {
		// Create a world with walls to test the function handles non-walkable areas
		for y := 0; y < testWorld.Height; y++ {
			for x := 0; x < testWorld.Width; x++ {
				testWorld.Tiles[y][x] = world.TileWall // Non-walkable
			}
		}

		tileSize := float64(cfg.GetTileSize())

		// Test that wall tiles are not walkable
		result := inputHandler.isPositionWalkable(tileSize/2, tileSize/2)
		if result {
			t.Error("Expected wall tile to not be walkable")
		}

		// Test that spawn location finding fails in all-wall world
		npcX := tileSize * 25
		npcY := tileSize * 25
		spawnX, spawnY := inputHandler.findEncounterSpawnLocation(npcX, npcY)

		if spawnX != 0 || spawnY != 0 {
			t.Error("Expected findEncounterSpawnLocation to fail in all-wall world")
		}
	})
}
