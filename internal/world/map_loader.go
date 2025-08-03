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

// MapData contains the loaded map information
type MapData struct {
	Width         int
	Height        int
	Tiles         [][]TileType3D
	MonsterSpawns []MonsterSpawn
	NPCSpawns     []NPCSpawn
	StartX        int
	StartY        int
}

// NewMapLoader creates a new map loader
func NewMapLoader(config interface{}) *MapLoader {
	return &MapLoader{
		config: config,
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
	scanner := bufio.NewScanner(file)

	// Read all non-comment lines and process as tile tokens
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines and comment lines (lines starting with #)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line into tiles and extract NPCs
		parsedLine, lineNPCs := ml.parseTileTokens(line, len(lines))
		npcSpawns = append(npcSpawns, lineNPCs...)
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
			return nil, fmt.Errorf("line %d has inconsistent width: expected %d, got %d", i+1, width, len(line))
		}
	}

	mapData := &MapData{
		Width:         width,
		Height:        height,
		Tiles:         make([][]TileType3D, height),
		MonsterSpawns: make([]MonsterSpawn, 0),
		NPCSpawns:     npcSpawns,
		StartX:        width / 2, // Default start position
		StartY:        height / 2,
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
					_, monsterKey, err := monster.MonsterConfig.GetMonsterByLetter(monsterLetter)
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

	return mapData, nil
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

	// Try to get tile type from tile manager
	if GlobalTileManager != nil {
		if tType, found := GlobalTileManager.GetTileTypeFromLetter(charStr); found {
			tileType = tType
			return tileType, monsterLetter, isStartPosition
		}
	}

	// Check if it's a monster spawn (lowercase letters)
	// Monster spawns set the underlying tile to empty
	if char >= 'a' && char <= 'z' {
		tileType = TileEmpty
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
		filepath.Join("assets", "forest.map"),           // From project root
		filepath.Join(".", "assets", "forest.map"),      // Current directory
		filepath.Join("..", "assets", "forest.map"),     // One directory up
		filepath.Join("...", "assets", "forest.map"),    // Two directories up (for tests)
		filepath.Join("../../assets", "forest.map"),     // Two directories up (explicit)
	}

	for _, mapPath := range possiblePaths {
		if _, err := os.Stat(mapPath); err == nil {
			return ml.LoadMap(mapPath)
		}
	}

	// If no path worked, return an error
	return nil, fmt.Errorf("forest.map not found in any of the expected locations")
}

// parseTileTokens parses a line into tiles, handling the new format:
// Map tiles use single characters, NPC definitions are at line end with >[npc:key] format
func (ml *MapLoader) parseTileTokens(line string, lineY int) (string, []NPCSpawn) {
	var npcSpawns []NPCSpawn
	
	// Split line into tile data and NPC definitions
	// Look for the first '>]' which indicates start of NPC definitions
	tilesPart := line
	npcDefinitions := ""
	
	if sepIndex := strings.Index(line, "  >"); sepIndex != -1 {
		tilesPart = line[:sepIndex]
		npcDefinitions = line[sepIndex+2:] // Skip the "  >" part
	}
	
	// Parse NPC definitions from the end of the line
	npcDefs := strings.Split(npcDefinitions, ", ")
	npcIndex := 0
	
	// Find all '@' positions first
	var atPositions []int
	for pos, char := range tilesPart {
		if char == '@' {
			atPositions = append(atPositions, pos)
		}
	}
	
	// Match each NPC definition to an '@' position
	for _, def := range npcDefs {
		def = strings.TrimSpace(def)
		
		// Remove leading '>' if present, then check for [npc:key] format
		cleanDef := strings.TrimPrefix(def, ">")
		cleanDef = strings.TrimSpace(cleanDef)
		
		if strings.HasPrefix(cleanDef, "[npc:") && strings.HasSuffix(cleanDef, "]") {
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
		}
	}
	
	// Replace '@' characters with '.' (empty walkable tiles) in the result
	resultTiles := strings.ReplaceAll(tilesPart, "@", ".")
	
	return resultTiles, npcSpawns
}
