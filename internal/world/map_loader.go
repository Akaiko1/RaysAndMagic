package world

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"ugataima/internal/monster"
)

// MapLoader handles loading world maps from files
type MapLoader struct {
	config interface{} // Will be *config.Config
	biome  string      // Current biome for tile resolution
}

// MonsterSpawn represents a monster spawn point from the map
type MonsterSpawn struct {
	X, Y       int
	MonsterKey string // YAML monster key instead of enum
}

// NPCSpawn represents an NPC spawn point from the map
type NPCSpawn struct {
	X, Y   int
	NPCKey string // YAML NPC key
}

// SpecialTileSpawn represents a special tile spawn point from the map
type SpecialTileSpawn struct {
	X, Y     int
	TileKey  string // Special tile key (e.g., "vteleporter", "rteleporter")
	TileType TileType3D
}

// MapData contains the loaded map information
type MapData struct {
	Width             int
	Height            int
	Tiles             [][]TileType3D
	MonsterSpawns     []MonsterSpawn
	NPCSpawns         []NPCSpawn
	SpecialTileSpawns []SpecialTileSpawn
	StartX            int
	StartY            int
}

// NewMapLoader creates a new map loader
func NewMapLoader(config interface{}) *MapLoader {
	return &MapLoader{
		config: config,
		biome:  "forest", // Default biome
	}
}

// NewMapLoaderWithBiome creates a new map loader for a specific biome
func NewMapLoaderWithBiome(config interface{}, biome string) *MapLoader {
	return &MapLoader{
		config: config,
		biome:  biome,
	}
}

// LoadMap loads a map from the specified file path
func (ml *MapLoader) LoadMap(mapPath string) (*MapData, error) {
	// Try to open the file
	file, err := os.Open(mapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open map file %s: %w", mapPath, err)
	}
	defer file.Close()

	var lines []string
	var npcSpawns []NPCSpawn
	var specialTileSpawns []SpecialTileSpawn
	var atCells [][2]int // [x,y] of every '@' placeholder (auto-floored under an entity)
	scanner := bufio.NewScanner(file)

	// Read all non-comment lines and process as tile tokens
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines and comment lines (lines starting with #)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line into tiles and extract NPCs and special tiles
		y := len(lines)
		parsedLine, lineNPCs, lineSpecialTiles, lineAt := ml.parseTileTokens(line, y)
		npcSpawns = append(npcSpawns, lineNPCs...)
		specialTileSpawns = append(specialTileSpawns, lineSpecialTiles...)
		for _, ax := range lineAt {
			atCells = append(atCells, [2]int{ax, y})
		}
		lines = append(lines, parsedLine)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading map file: %w", err)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("map file contains no valid map data")
	}

	// Determine map dimensions
	height := len(lines)
	width := 0
	if height > 0 {
		width = len(lines[0])
	}

	// Validate all lines have the same width
	for i, line := range lines {
		if len(line) != width {
			fmt.Print("The line with error is: ", line, "\n")
			return nil, fmt.Errorf("line %d has inconsistent width: expected %d, got %d", i+1, width, len(line))
		}
	}

	mapData := &MapData{
		Width:             width,
		Height:            height,
		Tiles:             make([][]TileType3D, height),
		MonsterSpawns:     make([]MonsterSpawn, 0),
		NPCSpawns:         npcSpawns,
		SpecialTileSpawns: specialTileSpawns,
		StartX:            -1, // No default start position - must be set explicitly with +
		StartY:            -1,
	}

	// Initialize tile arrays
	for y := 0; y < height; y++ {
		mapData.Tiles[y] = make([]TileType3D, width)
	}

	// Parse the map
	for y, line := range lines {
		for x, char := range line {
			tileType, monsterLetter, isStart := ml.parseMapCharacter(char)

			mapData.Tiles[y][x] = tileType

			if isStart {
				mapData.StartX = x
				mapData.StartY = y
			}

			if monsterLetter != "" {
				// Convert letter to monster key using YAML config
				if monster.MonsterConfig != nil {
					_, monsterKey, err := monster.MonsterConfig.GetMonsterByLetterForBiome(monsterLetter, ml.biome)
					if err == nil {
						spawn := MonsterSpawn{
							X:          x,
							Y:          y,
							MonsterKey: monsterKey,
						}
						mapData.MonsterSpawns = append(mapData.MonsterSpawns, spawn)
					}
				}
			}
		}
	}

	// Resolve the auto-default floor under placed entities (monsters + '@'
	// placeholders) to the dominant nearby floor before special tiles override.
	autoFloored := make(map[[2]int]bool, len(mapData.MonsterSpawns)+len(atCells))
	for _, s := range mapData.MonsterSpawns {
		autoFloored[[2]int{s.X, s.Y}] = true
	}
	for _, c := range atCells {
		autoFloored[c] = true
	}
	ml.resolveUnderEntityFloors(mapData, autoFloored)

	// Apply special tile spawns to the map
	for _, specialTile := range mapData.SpecialTileSpawns {
		if specialTile.Y >= 0 && specialTile.Y < mapData.Height &&
			specialTile.X >= 0 && specialTile.X < mapData.Width {
			mapData.Tiles[specialTile.Y][specialTile.X] = specialTile.TileType
		}
	}

	return mapData, nil
}

// resolveUnderEntityFloors replaces the auto-default floor under placed entities
// (monster spawns, '@' NPC/placeholder cells) with the DOMINANT floor tile among
// the 8 neighbours, so an entity dropped onto a floor-variant patch blends in
// instead of stamping the biome default '.'. Only explicitly-authored floor tiles
// vote (other auto-floored cells are skipped); orthogonal neighbours count double.
// Falls back to the biome '.' floor when no floor neighbour exists. On uniform
// floors the dominant neighbour IS the biome default, so behaviour is unchanged.
func (ml *MapLoader) resolveUnderEntityFloors(md *MapData, autoFloored map[[2]int]bool) {
	if GlobalTileManager == nil || len(autoFloored) == 0 {
		return
	}
	defFloor, hasDef := GlobalTileManager.GetTileTypeFromLetterForBiome(".", ml.biome)
	// A floor candidate must be BOTH a real ground tile (render_type "floor_only"
	// — excludes walkable "environment_sprite" decorations like fern_patch /
	// clearing / firefly_swarm / mushroom_ring) AND actually steppable (excludes
	// "floor_only" but impassable water / deep_water / chasm-floor tiles). Neither
	// check alone is enough.
	isFloor := func(t TileType3D) bool {
		return GlobalTileManager.GetRenderType(t) == "floor_only" &&
			GlobalTileManager.IsWalkable(t) && !GlobalTileManager.IsSolid(t)
	}
	// Fixed neighbour order (orthogonal first) so tie-breaks are deterministic.
	type off struct{ dx, dy, w int }
	neighbours := []off{
		{0, -1, 2}, {0, 1, 2}, {-1, 0, 2}, {1, 0, 2}, // orthogonal, weight 2
		{-1, -1, 1}, {1, -1, 1}, {-1, 1, 1}, {1, 1, 1}, // diagonal, weight 1
	}
	for cell := range autoFloored {
		x, y := cell[0], cell[1]
		counts := make(map[TileType3D]int)
		best := TileEmpty
		bestScore := 0
		for _, n := range neighbours {
			nx, ny := x+n.dx, y+n.dy
			if nx < 0 || ny < 0 || ny >= md.Height || nx >= md.Width {
				continue
			}
			if autoFloored[[2]int{nx, ny}] {
				continue // ignore other entity cells — vote on authored floor only
			}
			t := md.Tiles[ny][nx]
			if !isFloor(t) {
				continue
			}
			counts[t] += n.w
			if counts[t] > bestScore {
				bestScore = counts[t]
				best = t
			}
		}
		if bestScore == 0 {
			if hasDef {
				md.Tiles[y][x] = defFloor
			}
			continue
		}
		md.Tiles[y][x] = best
	}
}

// parseMapCharacter converts a map character to tile type and optional monster letter
func (ml *MapLoader) parseMapCharacter(char rune) (TileType3D, string, bool) {
	charStr := string(char)
	var tileType TileType3D
	var monsterLetter string
	isStartPosition := false

	// Check if it's the starting position marker
	if char == '+' {
		tileType = TileSpawn
		isStartPosition = true
		return tileType, monsterLetter, isStartPosition
	}

	// Try to get tile type from tile manager (biome-aware)
	if GlobalTileManager != nil {
		if tType, found := GlobalTileManager.GetTileTypeFromLetterForBiome(charStr, ml.biome); found {
			tileType = tType
			return tileType, monsterLetter, isStartPosition
		}
	}

	// Check if it's a monster spawn (lowercase letters)
	// Monster spawns use the biome's normal floor underneath.
	if char >= 'a' && char <= 'z' {
		tileType = TileEmpty
		if GlobalTileManager != nil {
			if floorType, found := GlobalTileManager.GetTileTypeFromLetterForBiome(".", ml.biome); found {
				tileType = floorType
			}
		}
		monsterLetter = charStr
		return tileType, monsterLetter, isStartPosition
	}

	// If tile manager is not available or character not found, default to empty
	// This should not happen in normal operation since tile manager is initialized at startup
	tileType = TileEmpty

	return tileType, monsterLetter, isStartPosition
}

// LoadForestMap loads the default forest map
func (ml *MapLoader) LoadForestMap() (*MapData, error) {
	// Try multiple possible paths for the assets directory
	possiblePaths := []string{
		filepath.Join("assets", "forest.map"),        // From project root
		filepath.Join(".", "assets", "forest.map"),   // Current directory
		filepath.Join("..", "assets", "forest.map"),  // One directory up
		filepath.Join("...", "assets", "forest.map"), // Two directories up (for tests)
		filepath.Join("../../assets", "forest.map"),  // Two directories up (explicit)
	}

	for _, mapPath := range possiblePaths {
		if _, err := os.Stat(mapPath); err == nil {
			return ml.LoadMap(mapPath)
		}
	}

	// If no path worked, return an error
	return nil, fmt.Errorf("forest.map not found in any of the expected locations")
}

// parseTileTokens parses a line into tiles, handling both NPCs and special tiles:
// Map tiles use single characters, definitions are at line end with >[npc:key] or >[stile:key] format
func (ml *MapLoader) parseTileTokens(line string, lineY int) (string, []NPCSpawn, []SpecialTileSpawn, []int) {
	var npcSpawns []NPCSpawn
	var specialTileSpawns []SpecialTileSpawn

	// Split line into tile data and entity definitions
	// Look for the first '>]' which indicates start of entity definitions
	tilesPart := line
	entityDefinitions := ""

	if sepIndex := strings.Index(line, "  >"); sepIndex != -1 {
		tilesPart = line[:sepIndex]
		entityDefinitions = line[sepIndex+2:] // Skip the "  >" part
	}

	// Parse entity definitions from the end of the line
	entityDefs := strings.Split(entityDefinitions, ", ")
	npcIndex := 0
	specialTileIndex := 0

	// Find all '@' positions first
	var atPositions []int
	for pos, char := range tilesPart {
		if char == '@' {
			atPositions = append(atPositions, pos)
		}
	}

	// Match each entity definition to an '@' position
	for _, def := range entityDefs {
		def = strings.TrimSpace(def)

		// Remove leading '>' if present
		cleanDef := strings.TrimPrefix(def, ">")
		cleanDef = strings.TrimSpace(cleanDef)

		if strings.HasPrefix(cleanDef, "[npc:") && strings.HasSuffix(cleanDef, "]") {
			// Handle NPC placement
			npcKey := strings.TrimSuffix(strings.TrimPrefix(cleanDef, "[npc:"), "]")

			// Place NPC at the next available '@' position
			if npcIndex < len(atPositions) {
				npcSpawn := NPCSpawn{
					X:      atPositions[npcIndex],
					Y:      lineY,
					NPCKey: npcKey,
				}
				npcSpawns = append(npcSpawns, npcSpawn)
				npcIndex++
			}
		} else if strings.HasPrefix(cleanDef, "[stile:") && strings.HasSuffix(cleanDef, "]") {
			// Handle special tile placement (data-driven)
			tileKey := strings.TrimSuffix(strings.TrimPrefix(cleanDef, "[stile:"), "]")
			if GlobalTileManager == nil {
				continue
			}
			tileType, ok := GlobalTileManager.GetTileTypeFromKey(tileKey)
			if !ok {
				continue // Skip unknown special tile keys
			}

			// Place special tile at the next available '@' position
			if specialTileIndex < len(atPositions) {
				specialTileSpawn := SpecialTileSpawn{
					X:        atPositions[specialTileIndex],
					Y:        lineY,
					TileKey:  tileKey,
					TileType: tileType,
				}
				specialTileSpawns = append(specialTileSpawns, specialTileSpawn)
				specialTileIndex++
			}
		}
	}

	// Replace '@' characters with '.' (empty walkable tiles) in the result
	resultTiles := strings.ReplaceAll(tilesPart, "@", ".")

	return resultTiles, npcSpawns, specialTileSpawns, atPositions
}
