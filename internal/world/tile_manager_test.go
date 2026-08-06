package world

import (
	"os"
	"strings"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/testutil"
)

func testTileSizeClasses() map[string]float64 {
	return testutil.UniformVisualSizeClasses(1)
}

func TestNewTileManagerRequiresExplicitSizeClasses(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewTileManager(nil) did not panic")
		}
	}()
	NewTileManager(nil)
}

func TestTileManager(t *testing.T) {
	// Create a temporary tiles.yaml for testing
	testConfig := `tiles:
  test_wall:
    name: "Test Wall"
    type: "wall"
    solid: true
    transparent: false
    walkable: false
    wall_height_multiplier: 1.0
    sprite: ""
    render_type: "wall"
    letter: "W"
    biomes: ["universal"]
  test_stream:
    name: "Test Stream"
    type: "water"
    solid: false
    transparent: true
    walkable: true
    sprite: "water"
    render_type: "standee"
    size_class: full_tile
    floor_color: [100, 150, 200]
    letter: "S"
    biomes: ["universal"]
  test_gap:
    name: "Test Gap"
    type: "floor"
    solid: false
    transparent: true
    walkable: false
    fly_over: true
    sprite: ""
    render_type: "floor"
    letter: "G"
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
	tm := NewTileManager(map[string]float64{"full_tile": 1, "tree": 2})
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

	gapType, ok := tm.GetTileTypeFromKey("test_gap")
	if !ok || !tm.CanFlyOver(gapType) {
		t.Fatal("explicit fly_over gap must be traversable by flying monsters")
	}
	wallType, ok := tm.GetTileTypeFromKey("test_wall")
	if !ok || tm.CanFlyOver(wallType) {
		t.Fatal("opaque wall must not be traversable by flying monsters")
	}
}

func TestTileManagerRejectsFlyOverOnWall(t *testing.T) {
	tm := NewTileManager(map[string]float64{"full_tile": 1, "tree": 2})
	tm.tileData = map[string]*config.TileData{
		"bad_wall": {
			Name:        "Bad Wall",
			Type:        "wall",
			Solid:       true,
			Transparent: false,
			Walkable:    false,
			FlyOver:     true,
			RenderType:  config.TileRenderWall,
		},
	}
	if err := tm.validateTileConfiguration(); err == nil || !strings.Contains(err.Error(), "fly_over") {
		t.Fatalf("fly_over wall must fail clearly, got %v", err)
	}
}

func TestTileManagerProperties(t *testing.T) {
	// Test with actual tile types
	tm := NewTileManager(map[string]float64{"full_tile": 1, "tall_prop": 1.25, "tree": 2})

	// Create some default tile data for testing
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Name:        "Forest Tree",
			Solid:       true,
			Transparent: false,
			Walkable:    false,
			Sprite:      "tree",
			RenderType:  "crossed_standee",
			SizeClass:   "tree",
		},
		"forest_stream": {
			Name:        "Flowing Water",
			Solid:       false,
			Transparent: true,
			Walkable:    true,
			Sprite:      "forest_stream",
			RenderType:  "standee",
			SizeClass:   "full_tile",
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

}

func TestTileSizeClassesResolveWithoutLegacyFallback(t *testing.T) {
	tm := NewTileManager(map[string]float64{"tree": 2, "full_tile": 1})
	tm.tileData = map[string]*config.TileData{
		"tree": {
			Name:       "Tree",
			RenderType: "crossed_standee",
			SizeClass:  "tree",
		},
		"prop": {
			Name:       "Prop",
			RenderType: "standee",
			SizeClass:  "full_tile",
		},
		"wall": {
			Name:                 "Wall",
			WallHeightMultiplier: 1.5,
			RenderType:           "wall",
		},
	}
	tm.createTypeMapping()

	if got := tm.GetSizeTiles(TileTree); got != 2 {
		t.Fatalf("tree shared size = %f, want 2", got)
	}
	tm.tileData["tree"].SizeClass = "full_tile"
	if got := tm.GetSizeTiles(TileTree); got != 1 {
		t.Fatalf("tree alternate size class = %f, want 1", got)
	}
	if got := tm.GetSizeTiles(TileWall); got != 1.0 {
		t.Fatalf("expected wall size multiplier default 1.0, got %f", got)
	}
	if got := tm.GetHeightMultiplier(TileWall); got != 1.5 {
		t.Fatalf("expected wall height multiplier 1.5, got %f", got)
	}
}

func TestTileWallHeightMultiplierFallback(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	tm.tileData = map[string]*config.TileData{
		"wall": {
			Name:             "Legacy Wall",
			HeightMultiplier: 0.75,
			RenderType:       "wall",
		},
	}
	tm.createTypeMapping()

	if got := tm.GetHeightMultiplier(TileWall); got != 0.75 {
		t.Fatalf("expected legacy height_multiplier fallback 0.75, got %f", got)
	}
}

func TestTileVisualSizeValidation(t *testing.T) {
	classes := map[string]float64{"person": 0.6, "small_prop": 0.5, "tree": 2}
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "known prop class",
			body: "size_class: small_prop\n    sprite: prop\n    render_type: standee",
		},
		{
			name:    "missing prop class",
			body:    "sprite: prop\n    render_type: standee",
			wantErr: "requires size_class",
		},
		{
			name:    "missing sprite",
			body:    "size_class: small_prop\n    sprite: \"\"\n    render_type: standee",
			wantErr: "requires a sprite",
		},
		{
			name:    "unknown prop class",
			body:    "size_class: typo\n    sprite: prop\n    render_type: standee",
			wantErr: "unknown size_class",
		},
		{
			name:    "removed raw size",
			body:    "size_tiles: 0.5\n    sprite: prop\n    render_type: standee",
			wantErr: "removed size_tiles",
		},
		{
			name: "tree shared size",
			body: "size_class: tree\n    sprite: tree\n    render_type: crossed_standee",
		},
		{
			name:    "tree missing class",
			body:    "sprite: tree\n    render_type: crossed_standee",
			wantErr: "requires size_class",
		},
		{
			name: "tree alternate width class",
			body: "size_class: small_prop\n    sprite: tree\n    render_type: crossed_standee",
		},
		{
			name:    "actor class on tree",
			body:    "size_class: person\n    sprite: tree\n    render_type: crossed_standee",
			wantErr: "cannot use size_class",
		},
		{
			name:    "actor class on prop",
			body:    "size_class: person\n    sprite: prop\n    render_type: standee",
			wantErr: "cannot use size_class",
		},
		{
			name:    "floor class",
			body:    "size_class: small_prop\n    sprite: \"\"\n    render_type: floor",
			wantErr: "must not set size_class",
		},
		{
			name:    "retired render class",
			body:    "size_class: small_prop\n    sprite: prop\n    render_type: environment_sprite",
			wantErr: "unknown render_type",
		},
		{
			name:    "night motes require crossed standee",
			body:    "size_class: small_prop\n    sprite: prop\n    render_type: standee\n    night_motes:\n      glow_color: [1, 2, 3]\n      core_color: [4, 5, 6]",
			wantErr: "want crossed_standee",
		},
		{
			name:    "night motes require both colors",
			body:    "size_class: tree\n    sprite: tree\n    render_type: crossed_standee\n    night_motes:\n      glow_color: [1, 2, 3]",
			wantErr: "require non-zero glow_color and core_color",
		},
		{
			name:    "unknown procedural effect",
			body:    "size_class: small_prop\n    sprite: prop\n    render_type: standee\n    procedural_effect: typo",
			wantErr: "unknown procedural_effect",
		},
		{
			name:    "swarm effect requires standee",
			body:    "size_class: tree\n    sprite: tree\n    render_type: crossed_standee\n    procedural_effect: firefly_swarm",
			wantErr: "requires render_type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents := "tiles:\n  subject:\n    name: Subject\n    type: prop\n    solid: false\n    transparent: true\n    walkable: true\n    " + tt.body + "\n"
			file, err := os.CreateTemp("", "visual_size_*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file.Name())
			if _, err := file.WriteString(contents); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			tm := NewTileManager(classes)
			err = tm.LoadTileConfig(file.Name())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("valid tile rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestTileOpaqueTreatsSolidSpritesAsSightBlockers(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	tm.tileData = map[string]*config.TileData{
		"solid_palm": {
			Name:        "Solid Palm",
			Solid:       true,
			Transparent: true,
			Walkable:    false,
			RenderType:  "standee",
		},
		"walkable_fern": {
			Name:        "Walkable Fern",
			Solid:       false,
			Transparent: true,
			Walkable:    true,
			RenderType:  "standee",
		},
		"ground_hazard": {
			Name:        "Ground Hazard",
			Solid:       true,
			Transparent: true,
			Walkable:    false,
			RenderType:  "floor",
		},
		"stone_wall": {
			Name:        "Stone Wall",
			Solid:       true,
			Transparent: false,
			Walkable:    false,
			RenderType:  "wall",
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

// The three field-contract rules: a camera-facing standee cannot block
// movement, wall_mounted belongs to the flat standee alone, and no_spin only
// means something where there is a spin.
func TestTileFieldContractValidation(t *testing.T) {
	classes := map[string]float64{"small_prop": 0.5, "full_tile": 1.0, "tree": 2.0, "person": 0.6}
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "blocking ordinary standee",
			body:    "size_class: small_prop\n    sprite: prop\n    render_type: standee",
			wantErr: "camera-facing standee that blocks movement",
		},
		{
			name: "walkable ordinary standee",
			body: "walkable: true\n    size_class: small_prop\n    sprite: prop\n    render_type: standee",
		},
		{
			name: "blocking crossed prop",
			body: "size_class: small_prop\n    sprite: prop\n    render_type: crossed_prop",
		},
		{
			name: "narrow crossed tree",
			body: "size_class: full_tile\n    sprite: tree\n    render_type: crossed_standee",
		},
		{
			name:    "actor class on crossed prop",
			body:    "size_class: person\n    sprite: prop\n    render_type: crossed_prop",
			wantErr: "cannot use size_class",
		},
		{
			name:    "wall_mounted on a cross",
			body:    "walkable: true\n    size_class: small_prop\n    sprite: prop\n    render_type: crossed_standee\n    wall_mounted: true",
			wantErr: "uses wall_mounted but render_type",
		},
		{
			name:    "no_spin on a cross",
			body:    "size_class: tree\n    sprite: tree\n    render_type: crossed_standee\n    no_spin: true",
			wantErr: "never spins",
		},
		{
			name:    "no_spin on a crossed prop",
			body:    "size_class: small_prop\n    sprite: prop\n    render_type: crossed_prop\n    no_spin: true",
			wantErr: "never spins",
		},
		{
			name: "no_spin on a landmark",
			body: "size_class: small_prop\n    sprite: prop\n    render_type: landmark_standee\n    no_spin: true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents := "tiles:\n  subject:\n    name: Subject\n    type: prop\n    solid: false\n    transparent: true\n    " + tt.body + "\n"
			file, err := os.CreateTemp("", "tile_contract_*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file.Name())
			if _, err := file.WriteString(contents); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			err = NewTileManager(classes).LoadTileConfig(file.Name())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("valid tile rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
