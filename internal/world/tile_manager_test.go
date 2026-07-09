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
    wall_height_multiplier: 1.0
    sprite: ""
    render_type: "textured_wall"
    letter: "W"
    biomes: ["universal"]
  test_stream:
    name: "Test Stream"
    solid: false
    transparent: true
    walkable: true
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
			Name:        "Forest Tree",
			Solid:       true,
			Transparent: false,
			Walkable:    false,
			SizeTiles:   2.0,
			Sprite:      "tree",
			RenderType:  "tree_sprite",
		},
		"forest_stream": {
			Name:        "Flowing Water",
			Solid:       false,
			Transparent: true,
			Walkable:    true,
			Sprite:      "forest_stream",
			RenderType:  "environment_sprite",
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
	if tm.GetHeightMultiplier(TileTree) != 1.0 {
		t.Errorf("Expected tree height multiplier to default to 1.0, got %f", tm.GetHeightMultiplier(TileTree))
	}
	if tm.GetSizeTiles(TileTree) != 2.0 {
		t.Errorf("Expected tree size multiplier to be 2.0, got %f", tm.GetSizeTiles(TileTree))
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

	err = tm.SetTileProperty(TileTree, "size_tiles", 2.5)
	if err != nil {
		t.Errorf("Failed to set tile size multiplier: %v", err)
	}
	if tm.GetSizeTiles(TileTree) != 2.5 {
		t.Errorf("Expected tree size multiplier to be 2.5, got %f", tm.GetSizeTiles(TileTree))
	}

	// Test invalid property
	err = tm.SetTileProperty(TileTree, "invalid_property", true)
	if err == nil {
		t.Errorf("Expected error when setting invalid property")
	}
}

func TestTileSizeTilesFallback(t *testing.T) {
	tm := NewTileManager()
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Name:             "Legacy Tree",
			HeightMultiplier: 2.25,
			RenderType:       "tree_sprite",
		},
		"wall": {
			Name:                 "Wall",
			WallHeightMultiplier: 1.5,
			RenderType:           "textured_wall",
		},
	}
	tm.createTypeMapping()

	if got := tm.GetSizeTiles(TileTree); got != 2.25 {
		t.Fatalf("expected legacy billboard height_multiplier fallback 2.25, got %f", got)
	}
	if got := tm.GetSizeTiles(TileWall); got != 1.0 {
		t.Fatalf("expected wall size multiplier default 1.0, got %f", got)
	}
	if got := tm.GetHeightMultiplier(TileWall); got != 1.5 {
		t.Fatalf("expected wall height multiplier 1.5, got %f", got)
	}
}

func TestTileWallHeightMultiplierFallback(t *testing.T) {
	tm := NewTileManager()
	tm.tileData = map[string]*config.TileData{
		"wall": {
			Name:             "Legacy Wall",
			HeightMultiplier: 0.75,
			RenderType:       "textured_wall",
		},
	}
	tm.createTypeMapping()

	if got := tm.GetHeightMultiplier(TileWall); got != 0.75 {
		t.Fatalf("expected legacy height_multiplier fallback 0.75, got %f", got)
	}
}

func TestTileOpaqueTreatsSolidSpritesAsSightBlockers(t *testing.T) {
	tm := NewTileManager()
	tm.tileData = map[string]*config.TileData{
		"solid_palm": {
			Name:        "Solid Palm",
			Solid:       true,
			Transparent: true,
			Walkable:    false,
			RenderType:  "environment_sprite",
		},
		"walkable_fern": {
			Name:        "Walkable Fern",
			Solid:       false,
			Transparent: true,
			Walkable:    true,
			RenderType:  "environment_sprite",
		},
		"ground_hazard": {
			Name:        "Ground Hazard",
			Solid:       true,
			Transparent: true,
			Walkable:    false,
			RenderType:  "floor_only",
		},
		"stone_wall": {
			Name:        "Stone Wall",
			Solid:       true,
			Transparent: false,
			Walkable:    false,
			RenderType:  "textured_wall",
		},
	}
	tm.createTypeMapping()

	solidPalm, ok := tm.GetTileTypeFromKey("solid_palm")
	if !ok {
		t.Fatal("solid_palm type missing")
	}
	walkableFern, ok := tm.GetTileTypeFromKey("walkable_fern")
	if !ok {
		t.Fatal("walkable_fern type missing")
	}
	groundHazard, ok := tm.GetTileTypeFromKey("ground_hazard")
	if !ok {
		t.Fatal("ground_hazard type missing")
	}
	stoneWall, ok := tm.GetTileTypeFromKey("stone_wall")
	if !ok {
		t.Fatal("stone_wall type missing")
	}

	if !tm.IsOpaque(solidPalm) {
		t.Fatalf("expected a solid transparent environment sprite to block line of sight")
	}
	if tm.IsOpaque(walkableFern) {
		t.Fatalf("expected a non-solid environment sprite to remain see-through")
	}
	if tm.IsOpaque(groundHazard) {
		t.Fatalf("expected floor-only blockers to stay see-through for line of sight")
	}
	if !tm.IsOpaque(stoneWall) {
		t.Fatalf("expected non-transparent walls to block line of sight")
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
