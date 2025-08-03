package world

import (
	"fmt"
	"os"
	"ugataima/internal/config"

	"gopkg.in/yaml.v3"
)

// TileManager handles tile configuration and properties
type TileManager struct {
	tileData        map[string]*config.TileData
	typeToKey       map[TileType3D]string
	keyToType       map[string]TileType3D // Map from key to tile type
	letterToType    map[string]TileType3D // Map from letter to tile type
	typeToLetter    map[TileType3D]string // Map from tile type to letter
	nextDynamicType TileType3D            // Next available type for dynamic tiles
}

// NewTileManager creates a new tile manager
func NewTileManager() *TileManager {
	return &TileManager{
		tileData:        make(map[string]*config.TileData),
		typeToKey:       make(map[TileType3D]string),
		keyToType:       make(map[string]TileType3D),
		letterToType:    make(map[string]TileType3D),
		typeToLetter:    make(map[TileType3D]string),
		nextDynamicType: 1000, // Start dynamic types at 1000 to avoid conflicts
	}
}

// LoadTileConfig loads tile configuration from a YAML file
func (tm *TileManager) LoadTileConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read tile config file: %w", err)
	}

	var tileConfig config.TileConfig
	err = yaml.Unmarshal(data, &tileConfig)
	if err != nil {
		return fmt.Errorf("failed to parse tile config: %w", err)
	}

	// Store tile data and create mapping
	tm.tileData = make(map[string]*config.TileData)
	for key, tileData := range tileConfig.TileData {
		// Make a copy to avoid pointer issues
		tileCopy := tileData
		tm.tileData[key] = &tileCopy
	}

	// Create mapping from TileType3D to config keys
	tm.createTypeMapping()

	// Create letter mappings
	tm.createLetterMappings()

	return nil
}

// createTypeMapping creates a mapping from TileType3D constants to config keys
func (tm *TileManager) createTypeMapping() {
	// Core tile mappings - these must match the TileType3D constants
	coreMapping := map[TileType3D]string{
		TileEmpty:        "empty",
		TileWall:         "wall",
		TileWater:        "water",
		TileDoor:         "door",
		TileStairs:       "stairs",
		TileTree:         "tree",
		TileAncientTree:  "ancient_tree",
		TileThicket:      "thicket",
		TileMossRock:     "moss_rock",
		TileMushroomRing: "mushroom_ring",
		TileForestStream: "forest_stream",
		TileFernPatch:    "fern_patch",
		TileFireflySwarm: "firefly_swarm",
		TileClearing:     "clearing",
		TileSpawn:        "spawn",
		TileLowWall:      "low_wall",
		TileHighWall:     "high_wall",
	}

	// Initialize mappings
	tm.typeToKey = make(map[TileType3D]string)
	tm.keyToType = make(map[string]TileType3D)

	// First, map all core tiles that exist in the config
	for tileType, key := range coreMapping {
		if _, exists := tm.tileData[key]; exists {
			tm.typeToKey[tileType] = key
			tm.keyToType[key] = tileType
		}
	}

	// Then, assign dynamic TileType3D values to any tiles in YAML that don't have constants
	for key := range tm.tileData {
		if _, alreadyMapped := tm.keyToType[key]; !alreadyMapped {
			// This is a new tile from YAML - assign it a dynamic type
			tm.typeToKey[tm.nextDynamicType] = key
			tm.keyToType[key] = tm.nextDynamicType
			tm.nextDynamicType++
		}
	}
}

// createLetterMappings creates bidirectional mappings between letters and tile types
func (tm *TileManager) createLetterMappings() {
	tm.letterToType = make(map[string]TileType3D)
	tm.typeToLetter = make(map[TileType3D]string)

	// Map all tiles (both core and dynamic) that have letters
	for tileType, key := range tm.typeToKey {
		if data, ok := tm.tileData[key]; ok && data.Letter != "" {
			tm.letterToType[data.Letter] = tileType
			tm.typeToLetter[tileType] = data.Letter
		}
	}
}

// GetTileData returns the configuration data for a tile type
func (tm *TileManager) GetTileData(tileType TileType3D) *config.TileData {
	key, ok := tm.typeToKey[tileType]
	if !ok {
		return nil
	}
	return tm.tileData[key]
}

// GetTileDataByKey returns the configuration data for a tile by its string key
// This allows access to tiles that don't have corresponding TileType3D constants
func (tm *TileManager) GetTileDataByKey(key string) *config.TileData {
	return tm.tileData[key]
}

// GetTileTypeFromKey returns the TileType3D for a given string key
// This works for both core tiles and dynamically assigned tiles
func (tm *TileManager) GetTileTypeFromKey(key string) (TileType3D, bool) {
	tileType, ok := tm.keyToType[key]
	return tileType, ok
}

// GetAllTileKeys returns all available tile keys from the loaded configuration
func (tm *TileManager) GetAllTileKeys() []string {
	keys := make([]string, 0, len(tm.tileData))
	for key := range tm.tileData {
		keys = append(keys, key)
	}
	return keys
}

// HasTileKey checks if a tile key exists in the loaded configuration
func (tm *TileManager) HasTileKey(key string) bool {
	_, exists := tm.tileData[key]
	return exists
}

// IsSolid returns whether a tile type is solid (blocks movement)
func (tm *TileManager) IsSolid(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	if data == nil {
		return false // Default to non-solid for unknown tiles
	}
	return data.Solid
}

// IsTransparent returns whether a tile type is transparent (allows ray to continue)
func (tm *TileManager) IsTransparent(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	if data == nil {
		return true // Default to transparent for unknown tiles
	}
	return data.Transparent
}

// IsWalkable returns whether a tile type is walkable
func (tm *TileManager) IsWalkable(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	if data == nil {
		return true // Default to walkable for unknown tiles
	}
	return data.Walkable
}

// GetHeightMultiplier returns the height multiplier for a tile type
func (tm *TileManager) GetHeightMultiplier(tileType TileType3D) float64 {
	data := tm.GetTileData(tileType)
	if data == nil {
		return 1.0 // Default height
	}
	return data.HeightMultiplier
}

// GetSprite returns the sprite name for a tile type
func (tm *TileManager) GetSprite(tileType TileType3D) string {
	data := tm.GetTileData(tileType)
	if data == nil {
		return ""
	}
	return data.Sprite
}

// GetRenderType returns the render type for a tile type
func (tm *TileManager) GetRenderType(tileType TileType3D) string {
	data := tm.GetTileData(tileType)
	if data == nil {
		return "textured_wall" // Default render type
	}
	return data.RenderType
}

// GetFloorColor returns the floor color for a tile type
func (tm *TileManager) GetFloorColor(tileType TileType3D) [3]int {
	data := tm.GetTileData(tileType)
	if data == nil {
		return [3]int{60, 180, 60} // Default green
	}
	// Check if floor color is set (non-zero)
	if data.FloorColor[0] != 0 || data.FloorColor[1] != 0 || data.FloorColor[2] != 0 {
		return data.FloorColor
	}
	return [3]int{60, 180, 60} // Default green
}

// GetFloorNearColor returns the floor color to use near this tile type
func (tm *TileManager) GetFloorNearColor(tileType TileType3D) [3]int {
	data := tm.GetTileData(tileType)
	if data == nil {
		return [3]int{0, 0, 0} // No special color
	}
	return data.FloorNearColor
}

// GetWallColor returns the wall color for this tile type
func (tm *TileManager) GetWallColor(tileType TileType3D) [3]int {
	data := tm.GetTileData(tileType)
	if data == nil {
		return [3]int{101, 67, 33} // Default brown
	}

	// Check if wall color is set (non-zero)
	if data.WallColor[0] != 0 || data.WallColor[1] != 0 || data.WallColor[2] != 0 {
		return data.WallColor
	}
	return [3]int{101, 67, 33} // Default brown
}

// HasFloorNearColor returns whether this tile type affects nearby floor colors
func (tm *TileManager) HasFloorNearColor(tileType TileType3D) bool {
	color := tm.GetFloorNearColor(tileType)
	return color[0] != 0 || color[1] != 0 || color[2] != 0
}

// SetTileProperty allows dynamic modification of tile properties at runtime
func (tm *TileManager) SetTileProperty(tileType TileType3D, property string, value interface{}) error {
	key, ok := tm.typeToKey[tileType]
	if !ok {
		return fmt.Errorf("unknown tile type: %d", tileType)
	}

	data := tm.tileData[key]
	if data == nil {
		return fmt.Errorf("no data found for tile type: %d", tileType)
	}

	switch property {
	case "solid":
		if val, ok := value.(bool); ok {
			data.Solid = val
		} else {
			return fmt.Errorf("solid property requires boolean value")
		}
	case "transparent":
		if val, ok := value.(bool); ok {
			data.Transparent = val
		} else {
			return fmt.Errorf("transparent property requires boolean value")
		}
	case "walkable":
		if val, ok := value.(bool); ok {
			data.Walkable = val
		} else {
			return fmt.Errorf("walkable property requires boolean value")
		}
	case "height_multiplier":
		if val, ok := value.(float64); ok {
			data.HeightMultiplier = val
		} else {
			return fmt.Errorf("height_multiplier property requires float64 value")
		}
	case "sprite":
		if val, ok := value.(string); ok {
			data.Sprite = val
		} else {
			return fmt.Errorf("sprite property requires string value")
		}
	case "render_type":
		if val, ok := value.(string); ok {
			data.RenderType = val
		} else {
			return fmt.Errorf("render_type property requires string value")
		}
	default:
		return fmt.Errorf("unknown property: %s", property)
	}

	return nil
}

// GetTileKey returns the configuration key for a tile type
func (tm *TileManager) GetTileKey(tileType TileType3D) string {
	return tm.typeToKey[tileType]
}

// ListTiles returns all available tile types and their keys
func (tm *TileManager) ListTiles() map[string]*config.TileData {
	result := make(map[string]*config.TileData)
	for key, data := range tm.tileData {
		// Make a copy to prevent external modification
		dataCopy := *data
		result[key] = &dataCopy
	}
	return result
}

// GetTileTypeFromLetter returns the tile type for a given letter
func (tm *TileManager) GetTileTypeFromLetter(letter string) (TileType3D, bool) {
	tileType, ok := tm.letterToType[letter]
	return tileType, ok
}

// GetTileKeyFromLetter returns the tile key for a given letter
// This works for all tiles, including dynamically assigned ones
func (tm *TileManager) GetTileKeyFromLetter(letter string) (string, bool) {
	if tileType, ok := tm.letterToType[letter]; ok {
		return tm.typeToKey[tileType], true
	}
	return "", false
}

// GetLetterFromTileType returns the letter for a given tile type
func (tm *TileManager) GetLetterFromTileType(tileType TileType3D) string {
	return tm.typeToLetter[tileType]
}

// GetLetterFromTileKey returns the letter for a given tile key
func (tm *TileManager) GetLetterFromTileKey(key string) string {
	if data, ok := tm.tileData[key]; ok {
		return data.Letter
	}
	return ""
}

// GetAllLetterMappings returns all letter to tile type mappings
func (tm *TileManager) GetAllLetterMappings() map[string]TileType3D {
	result := make(map[string]TileType3D)
	for letter, tileType := range tm.letterToType {
		result[letter] = tileType
	}
	return result
}
