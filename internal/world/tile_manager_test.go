package world

import (
	"os"
	"testing"
	"ugataima/internal/config"
)

func TestTileManager(t *testing.T) {
	// Create a temporary tiles.yaml for testing
	testConfig := `tiles:
  test_wall:
    name: "Test Wall"
    solid: true
    transparent: false
    walkable: false
    height_multiplier: 1.0
    sprite: ""
    render_type: "textured_wall"
    letter: "W"
    biomes: ["universal"]
  test_stream:
    name: "Test Stream"
    solid: false
    transparent: true
    walkable: true
    height_multiplier: 1.0
    sprite: "water"
    render_type: "environment_sprite"
    floor_color: [100, 150, 200]
    letter: "S"
    biomes: ["universal"]
`

	// Write test config to temporary file
	tmpFile, err := os.CreateTemp("", "test_tiles_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	tmpFile.Close()

	// Test tile manager
	tm := NewTileManager()
	err = tm.LoadTileConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load tile config: %v", err)
	}

	// Test getting tile data
	data := tm.tileData["test_wall"]
	if data == nil {
		t.Fatalf("Expected test_wall data to be loaded")
	}

	if !data.Solid {
		t.Errorf("Expected test_wall to be solid")
	}
	if data.Transparent {
		t.Errorf("Expected test_wall to not be transparent")
	}

	streamData := tm.tileData["test_stream"]
	if streamData == nil {
		t.Fatalf("Expected test_stream data to be loaded")
	}

	if streamData.Solid {
		t.Errorf("Expected test_stream to not be solid")
	}
	if !streamData.Transparent {
		t.Errorf("Expected test_stream to be transparent")
	}

	expectedColor := [3]int{100, 150, 200}
	if streamData.FloorColor != expectedColor {
		t.Errorf("Expected floor color %v, got %v", expectedColor, streamData.FloorColor)
	}
}

func TestTileManagerProperties(t *testing.T) {
	// Test with actual tile types
	tm := NewTileManager()

	// Create some default tile data for testing
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Name:             "Forest Tree",
			Solid:            true,
			Transparent:      false,
			Walkable:         false,
			HeightMultiplier: 2.0,
			Sprite:           "tree",
			RenderType:       "tree_sprite",
		},
		"forest_stream": {
			Name:             "Flowing Water",
			Solid:            false,
			Transparent:      true,
			Walkable:         true,
			HeightMultiplier: 1.0,
			Sprite:           "forest_stream",
			RenderType:       "environment_sprite",
		},
	}

	// Initialize with default mapping after setting up tileData
	tm.createTypeMapping()

	// Test tree properties
	if !tm.IsSolid(TileTree) {
		t.Errorf("Expected tree to be solid")
	}
	if tm.IsTransparent(TileTree) {
		t.Errorf("Expected tree to not be transparent")
	}
	if tm.IsWalkable(TileTree) {
		t.Errorf("Expected tree to not be walkable")
	}
	if tm.GetHeightMultiplier(TileTree) != 2.0 {
		t.Errorf("Expected tree height multiplier to be 2.0, got %f", tm.GetHeightMultiplier(TileTree))
	}

	// Test stream properties
	if tm.IsSolid(TileForestStream) {
		t.Errorf("Expected forest stream to not be solid")
	}
	if !tm.IsTransparent(TileForestStream) {
		t.Errorf("Expected forest stream to be transparent")
	}
	if !tm.IsWalkable(TileForestStream) {
		t.Errorf("Expected forest stream to be walkable")
	}

	// Test dynamic property modification
	err := tm.SetTileProperty(TileTree, "walkable", true)
	if err != nil {
		t.Errorf("Failed to set tile property: %v", err)
	}

	if !tm.IsWalkable(TileTree) {
		t.Errorf("Expected tree to be walkable after setting property")
	}

	// Test invalid property
	err = tm.SetTileProperty(TileTree, "invalid_property", true)
	if err == nil {
		t.Errorf("Expected error when setting invalid property")
	}
}

func TestTileManagerFallback(t *testing.T) {
	// Test behavior when tile manager is not initialized
	originalManager := GlobalTileManager
	GlobalTileManager = nil
	defer func() { GlobalTileManager = originalManager }()

	// GetTileHeight should return default value when tile manager is not available
	height := GetTileHeight(TileTree)
	if height != 1.0 {
		t.Errorf("Expected default height to be 1.0 when tile manager not available, got %f", height)
	}

	height = GetTileHeight(TileLowWall)
	if height != 1.0 {
		t.Errorf("Expected default height to be 1.0 when tile manager not available, got %f", height)
	}
}
