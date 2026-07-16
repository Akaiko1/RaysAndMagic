package world

import (
	"fmt"
	"os"
	"strings"
	"ugataima/internal/config"

	"gopkg.in/yaml.v3"
)

// TileManager handles tile configuration and properties
type TileManager struct {
	tileData     map[string]*config.TileData
	typeToKey    map[TileType3D]string
	keyToType    map[string]TileType3D // Map from key to tile type
	letterToType map[string]TileType3D // Map from letter to tile type
	typeToLetter map[TileType3D]string // Map from tile type to letter
	// shortLabelToType / typeToShortLabel place letterless GENERAL tiles via a
	// >[tile:short_label] map def (see TileData.ShortLabel).
	shortLabelToType map[string]TileType3D
	typeToShortLabel map[TileType3D]string
	nextDynamicType  TileType3D // Next available type for dynamic tiles
	// specialTileKeys is the set of keys loaded from special_tiles.yaml (placed
	// via [stile:key], letterless). Tracked so the map editor can offer them as
	// their own brush category - they're merged into tileData like any tile, so
	// nothing else distinguishes them.
	specialTileKeys map[string]bool
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

// validateTileConfiguration checks for conflicts in tile letters.
// ASCII map contract: lowercase a-z belongs exclusively to monster spawns.
// Lettered terrain/props therefore use uppercase letters (or punctuation and
// digits); free-standing decor uses the letterless [tile:short_label] form.
// validTileRenderTypes is the closed set of render_type values the renderer
// actually dispatches on. An unknown value would load fine and then render
// NOTHING (the legacy "flooring_object" died exactly that way), so it is a
// load-time error, never a silent invisible tile.
var validTileRenderTypes = map[string]bool{
	"floor_only": true, "textured_wall": true, "environment_sprite": true,
	"tree_sprite": true, "landmark": true,
}

func (tm *TileManager) validateTileConfiguration() error {
	for key, data := range tm.tileData {
		if !validTileRenderTypes[data.RenderType] {
			return fmt.Errorf("tile %q has missing or unknown render_type %q (valid: floor_only|textured_wall|environment_sprite|tree_sprite|landmark)", key, data.RenderType)
		}
		// Every authored tile carries an explicit organizational `type` (editor
		// palette grouping). Special tiles (teleporters/traps) have their own
		// palette section and are exempt.
		if tm.specialTileKeys[key] {
			continue
		}
		if !config.ValidTileTypes[data.Type] {
			return fmt.Errorf("tile %q has missing or unknown type %q (valid: floor|water|marker|wall|wall_decor|nature|rock|structure|prop)", key, data.Type)
		}
		if len(data.Letter) == 1 && data.Letter[0] >= 'a' && data.Letter[0] <= 'z' {
			return fmt.Errorf("tile %q uses lowercase map letter %q - a-z is reserved for monster spawns; use uppercase or a letterless [tile:] prop", key, data.Letter)
		}
	}

	// Map to track letter conflicts: letter -> biome -> tile keys
	letterMap := make(map[string]map[string][]string)

	for key, data := range tm.tileData {
		letter := data.Letter
		if letter == "" {
			// Tiles without a letter aren't placed directly in map text, so skip validation.
			continue
		}

		// Initialize letter entry if needed
		if _, exists := letterMap[letter]; !exists {
			letterMap[letter] = make(map[string][]string)
		}

		// Check each biome this tile supports
		if len(data.Biomes) == 0 {
			// Universal tile - check against "universal" scope
			letterMap[letter]["universal"] = append(letterMap[letter]["universal"], key)
		} else {
			// Biome-specific tile
			for _, biome := range data.Biomes {
				letterMap[letter][biome] = append(letterMap[letter][biome], key)
			}
		}
	}

	// Check for conflicts
	var conflicts []string
	for letter, biomeMap := range letterMap {
		for biome, tileKeys := range biomeMap {
			if len(tileKeys) > 1 {
				if biome == "universal" {
					conflicts = append(conflicts, fmt.Sprintf("Letter '%s' has multiple universal tiles: %v", letter, tileKeys))
				} else {
					conflicts = append(conflicts, fmt.Sprintf("Letter '%s' has multiple tiles in biome '%s': %v", letter, biome, tileKeys))
				}
			}
		}

		// Universal tiles are allowed as fallbacks for biome-specific overrides.
		// GetTileTypeFromLetterForBiome resolves biome-specific tiles first.
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("tile configuration conflicts detected:\n%s", strings.Join(conflicts, "\n"))
	}

	return nil
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

	// Validate configuration for conflicts
	if err := tm.validateTileConfiguration(); err != nil {
		return err
	}

	return nil
}

// LoadSpecialTileConfig loads special tile configuration from a YAML file
func (tm *TileManager) LoadSpecialTileConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read special tile config file: %w", err)
	}

	var specialTileConfig config.SpecialTileConfig
	err = yaml.Unmarshal(data, &specialTileConfig)
	if err != nil {
		return fmt.Errorf("failed to parse special tile config: %w", err)
	}

	// Merge special tile data into existing tile data
	if tm.specialTileKeys == nil {
		tm.specialTileKeys = make(map[string]bool)
	}
	for key, tileData := range specialTileConfig.SpecialTileData {
		// Make a copy to avoid pointer issues
		tileCopy := tileData
		tm.tileData[key] = &tileCopy
		tm.specialTileKeys[key] = true
	}

	// Recreate mappings to include new tiles
	tm.createTypeMapping()
	tm.createLetterMappings()

	// Validate configuration for conflicts
	if err := tm.validateTileConfiguration(); err != nil {
		return err
	}

	return nil
}

// createTypeMapping creates a mapping from TileType3D constants to config keys
func (tm *TileManager) createTypeMapping() {
	// Core tile mappings - these must match the TileType3D constants
	coreMapping := map[TileType3D]string{
		TileEmpty:            "empty",
		TileWall:             "wall",
		TileWater:            "water",
		TileDeepWater:        "deep_water",
		TileDoor:             "door",
		TileTree:             "tree",
		TileAncientTree:      "ancient_tree",
		TileThicket:          "thicket",
		TileMossRock:         "moss_rock",
		TileMushroomRing:     "mushroom_ring",
		TileForestStream:     "forest_stream",
		TileFernPatch:        "fern_patch",
		TileFireflySwarm:     "firefly_swarm",
		TileClearing:         "clearing",
		TileSpawn:            "spawn",
		TileVioletTeleporter: "vteleporter",
		TileRedTeleporter:    "rteleporter",
		TileLowWall:          "low_wall",
		TileHighWall:         "high_wall",
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
	tm.shortLabelToType = make(map[string]TileType3D)
	tm.typeToShortLabel = make(map[TileType3D]string)

	// Map all tiles (both core and dynamic) that have letters or short labels.
	for tileType, key := range tm.typeToKey {
		data, ok := tm.tileData[key]
		if !ok {
			continue
		}
		if data.Letter != "" {
			tm.letterToType[data.Letter] = tileType
			tm.typeToLetter[tileType] = data.Letter
		}
		if data.ShortLabel != "" {
			tm.shortLabelToType[data.ShortLabel] = tileType
			tm.typeToShortLabel[tileType] = data.ShortLabel
		}
	}
}

// GetTileTypeFromShortLabel resolves a general tile's [tile:short_label] token.
func (tm *TileManager) GetTileTypeFromShortLabel(label string) (TileType3D, bool) {
	t, ok := tm.shortLabelToType[label]
	return t, ok
}

// GetShortLabelFromType returns a tile's placement short_label ("" if none).
func (tm *TileManager) GetShortLabelFromType(tileType TileType3D) string {
	return tm.typeToShortLabel[tileType]
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

// IsOpaque returns whether a tile type blocks sight
func (tm *TileManager) IsOpaque(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	if data == nil {
		return false // Default to transparent (not opaque) for unknown tiles
	}
	if data.Solid {
		switch data.RenderType {
		case "tree_sprite", "environment_sprite", "landmark":
			return true
		}
	}
	// Opaque means NOT transparent
	return !data.Transparent
}

// GetHeightMultiplier returns the height multiplier for a tile type
func (tm *TileManager) GetHeightMultiplier(tileType TileType3D) float64 {
	data := tm.GetTileData(tileType)
	if data == nil {
		return 1.0 // Default height
	}
	if data.WallHeightMultiplier > 0 {
		return data.WallHeightMultiplier
	}
	if data.HeightMultiplier <= 0 {
		return 1.0
	}
	return data.HeightMultiplier
}

// GetSizeTiles returns the visual sprite scale for billboard-style tiles.
// size_tiles is the canonical content key. height_multiplier remains as a
// fallback for older tile YAML where billboard scale and wall height shared one
// field.
func (tm *TileManager) GetSizeTiles(tileType TileType3D) float64 {
	data := tm.GetTileData(tileType)
	if data == nil {
		return 1.0
	}
	if data.SizeTiles > 0 {
		return data.SizeTiles
	}
	switch data.RenderType {
	case "tree_sprite", "environment_sprite", "landmark":
		if data.HeightMultiplier > 0 {
			return data.HeightMultiplier
		}
	}
	return 1.0
}

// IsWallMounted reports whether a tile is a wall-mounted decoration standee
// (config wall_mounted). Rendered stuck to the nearest solid neighbour.
func (tm *TileManager) IsWallMounted(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	return data != nil && data.WallMounted
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

// GetFloorColor returns the floor color for a tile type, or [0,0,0] when no
// floor_color is configured. Callers use the zero sentinel to mean "unset" and
// fall back to the current map's default_floor_color, so do NOT bake a green
// (or any other) default here - that would override the map's biome colour.
func (tm *TileManager) GetFloorColor(tileType TileType3D) [3]int {
	data := tm.GetTileData(tileType)
	if data == nil {
		return [3]int{0, 0, 0}
	}
	return data.FloorColor
}

// InheritsFloor reports whether a tile should take the surrounding biome floor
// (colour + texture) rather than painting its own floor_color - see
// config.TileData.InheritFloor. Marker tiles (spawn, teleporters) set this so
// they blend into the ground like a mob-spawn cell.
func (tm *TileManager) InheritsFloor(tileType TileType3D) bool {
	data := tm.GetTileData(tileType)
	return data != nil && data.InheritFloor
}

// floorVoteNeighbours are the 8 neighbours that vote on an inherited floor,
// orthogonal first (weight 2) then diagonal (weight 1). Fixed order makes
// tie-breaks deterministic.
var floorVoteNeighbours = []struct{ dx, dy, w int }{
	{0, -1, 2}, {0, 1, 2}, {-1, 0, 2}, {1, 0, 2},
	{-1, -1, 1}, {1, -1, 1}, {-1, 1, 1}, {1, 1, 1},
}

// DominantNeighbourFloor returns the dominant authored, steppable floor tile among
// the 8 neighbours of (x,y) - orthogonal neighbours weighted double, with a
// deterministic tie-break by neighbour order. Only real ground votes: render_type
// "floor_only" AND walkable, non-solid, and not itself an inherit_floor marker
// (so spawn/teleporters never stamp their own square under an entity). Cells where
// skip(nx,ny) is true are ignored (e.g. other entity-placeholder cells). ok is
// false when no floor neighbour exists, leaving the fallback to the caller. Single
// source for both under-entity floors (map load) and inherit_floor markers (render).
func (tm *TileManager) DominantNeighbourFloor(tiles [][]TileType3D, width, height, x, y int, skip func(nx, ny int) bool) (TileType3D, bool) {
	isFloor := func(t TileType3D) bool {
		return tm.GetRenderType(t) == "floor_only" &&
			tm.IsWalkable(t) && !tm.IsSolid(t) && !tm.InheritsFloor(t)
	}
	counts := make(map[TileType3D]int)
	best := TileEmpty
	bestScore := 0
	for _, n := range floorVoteNeighbours {
		nx, ny := x+n.dx, y+n.dy
		if nx < 0 || ny < 0 || ny >= height || nx >= width {
			continue
		}
		if skip != nil && skip(nx, ny) {
			continue
		}
		t := tiles[ny][nx]
		if !isFloor(t) {
			continue
		}
		counts[t] += n.w
		if counts[t] > bestScore {
			bestScore = counts[t]
			best = t
		}
	}
	return best, bestScore > 0
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
	case "wall_height_multiplier":
		if val, ok := value.(float64); ok {
			data.WallHeightMultiplier = val
		} else {
			return fmt.Errorf("wall_height_multiplier property requires float64 value")
		}
	case "size_tiles":
		if val, ok := value.(float64); ok {
			data.SizeTiles = val
		} else {
			return fmt.Errorf("size_tiles property requires float64 value")
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

// ListSpecialTiles returns the special tiles (from special_tiles.yaml, placed
// via [stile:key] and letterless) so the map editor can offer them as their
// own brush category. Copies, like ListTiles.
func (tm *TileManager) ListSpecialTiles() map[string]*config.TileData {
	result := make(map[string]*config.TileData)
	for key := range tm.specialTileKeys {
		if data := tm.tileData[key]; data != nil {
			dataCopy := *data
			result[key] = &dataCopy
		}
	}
	return result
}

// IsSpecialTile reports whether a key came from special_tiles.yaml.
func (tm *TileManager) IsSpecialTile(key string) bool {
	return tm.specialTileKeys[key]
}

// GetTileTypeFromLetter returns the tile type for a given letter
func (tm *TileManager) GetTileTypeFromLetter(letter string) (TileType3D, bool) {
	tileType, ok := tm.letterToType[letter]
	return tileType, ok
}

// GetTileTypeFromLetterForBiome returns the tile type for a given letter in a specific biome
func (tm *TileManager) GetTileTypeFromLetterForBiome(letter string, biome string) (TileType3D, bool) {
	// First try to find a biome-specific tile
	for tileType, key := range tm.typeToKey {
		if data, ok := tm.tileData[key]; ok && data.Letter == letter {
			// Check if this tile supports the requested biome
			if len(data.Biomes) > 0 && tm.tileSupportsbiome(data, biome) {
				return tileType, true
			}
		}
	}

	// Fallback to any tile with this letter (biome-agnostic tiles)
	for tileType, key := range tm.typeToKey {
		if data, ok := tm.tileData[key]; ok && data.Letter == letter {
			// If no biomes specified, tile works in any biome
			if len(data.Biomes) == 0 {
				return tileType, true
			}
		}
	}

	return TileEmpty, false
}

// tileSupportsbiome checks if a tile's biome list includes the requested biome.
// Callers must verify len(tileData.Biomes) > 0 before calling; universal tiles
// (empty Biomes) are handled by the fallback loop in GetTileTypeFromLetterForBiome.
func (tm *TileManager) tileSupportsbiome(tileData *config.TileData, biome string) bool {
	for _, supportedBiome := range tileData.Biomes {
		if supportedBiome == biome {
			return true
		}
	}
	return false
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
