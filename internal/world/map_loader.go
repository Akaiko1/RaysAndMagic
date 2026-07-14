package world

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"ugataima/internal/monster"
)

// Map-file entity tokens, shared by the loader (parse) and the map editor
// (emit) so the round-trip never drifts. A placeholder char sits on the tile
// row; it is bound to a bracketed def ("[tag:body]") appended after the row.
const (
	MapCellInteractive = '@' // NPC / special-tile cell -> [npc:key] or [stile:key]
	MapCellGeneral     = '$' // general decoration cell -> [tile:short_label]

	MapDefNPC   = "npc"   // [npc:key]
	MapDefStile = "stile" // [stile:key]
	MapDefTile  = "tile"  // [tile:short_label]
)

// FormatMapDef builds a bracketed entity def, e.g. FormatMapDef(MapDefNPC,
// "goblin") -> "[npc:goblin]".
func FormatMapDef(tag, body string) string { return "[" + tag + ":" + body + "]" }

// ParseMapDef splits a bracketed entity def into its tag and body. ok is false
// if s is not a well-formed "[tag:body]".
func ParseMapDef(s string) (tag, body string, ok bool) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return "", "", false
	}
	inner := s[1 : len(s)-1]
	i := strings.IndexByte(inner, ':')
	if i < 0 {
		return "", "", false
	}
	return inner[:i], inner[i+1:], true
}

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
	var generalTileSpawns []SpecialTileSpawn // letterless general tiles ([tile:short_label])
	var atCells [][2]int                     // [x,y] of every '@' placeholder (auto-floored under an entity)
	scanner := bufio.NewScanner(file)

	// Read all non-comment lines and process as tile tokens
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines and comment lines (lines starting with #)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line into tiles and extract NPCs, special tiles, general tiles
		y := len(lines)
		parsedLine, lineNPCs, lineSpecialTiles, lineGeneralTiles, lineAt := ml.parseTileTokens(line, y)
		npcSpawns = append(npcSpawns, lineNPCs...)
		specialTileSpawns = append(specialTileSpawns, lineSpecialTiles...)
		generalTileSpawns = append(generalTileSpawns, lineGeneralTiles...)
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

	// Apply general (letterless) tile spawns to the grid. Not stored on MapData:
	// they live in the grid like any tile, distinguished by their short_label,
	// and the editor re-encodes them by scanning the grid.
	for _, gt := range generalTileSpawns {
		if gt.Y >= 0 && gt.Y < mapData.Height && gt.X >= 0 && gt.X < mapData.Width {
			mapData.Tiles[gt.Y][gt.X] = gt.TileType
		}
	}

	return mapData, nil
}

// resolveUnderEntityFloors replaces the auto-default floor under placed entities
// (monster spawns, '@' NPC/placeholder cells) with the dominant nearby floor, so an
// entity dropped onto a floor-variant patch blends in instead of stamping the biome
// default '.'. Other entity cells are skipped from the vote. Falls back to the biome
// '.' floor when no floor neighbour exists. Uses the same vote as inherit_floor
// markers - see TileManager.DominantNeighbourFloor.
func (ml *MapLoader) resolveUnderEntityFloors(md *MapData, autoFloored map[[2]int]bool) {
	if GlobalTileManager == nil || len(autoFloored) == 0 {
		return
	}
	defFloor, hasDef := GlobalTileManager.GetTileTypeFromLetterForBiome(".", ml.biome)
	skip := func(nx, ny int) bool { return autoFloored[[2]int{nx, ny}] }
	for cell := range autoFloored {
		x, y := cell[0], cell[1]
		if best, ok := GlobalTileManager.DominantNeighbourFloor(md.Tiles, md.Width, md.Height, x, y, skip); ok {
			md.Tiles[y][x] = best
		} else if hasDef {
			md.Tiles[y][x] = defFloor
		}
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
func (ml *MapLoader) parseTileTokens(line string, lineY int) (string, []NPCSpawn, []SpecialTileSpawn, []SpecialTileSpawn, []int) {
	var npcSpawns []NPCSpawn
	var specialTileSpawns []SpecialTileSpawn
	var generalTileSpawns []SpecialTileSpawn

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

	// Placeholders (left to right = ascending X). '@' = INTERACTIVE entities
	// (NPCs, special tiles); '$' = non-interactive GENERAL decoration tiles.
	// Separate symbols so decorations never consume an interactive slot.
	var atPositions, dollarPositions []int
	for pos, char := range tilesPart {
		switch char {
		case MapCellInteractive:
			atPositions = append(atPositions, pos)
		case MapCellGeneral:
			dollarPositions = append(dollarPositions, pos)
		}
	}

	// Match defs to placeholders in order: npc/stile consume '@' (shared counter
	// - a line may mix them), [tile:] consumes '$'. Def lists are emitted in
	// ascending-X order to match the placeholder scan.
	atIndex, dollarIndex := 0, 0
	for _, def := range entityDefs {
		def = strings.TrimSpace(def)
		cleanDef := strings.TrimSpace(strings.TrimPrefix(def, ">"))
		tag, body, ok := ParseMapDef(cleanDef)
		if !ok {
			continue
		}

		switch tag {
		case MapDefNPC:
			if atIndex < len(atPositions) {
				npcSpawns = append(npcSpawns, NPCSpawn{X: atPositions[atIndex], Y: lineY, NPCKey: body})
				atIndex++
			}
		case MapDefStile:
			if GlobalTileManager == nil {
				continue
			}
			tileType, ok := GlobalTileManager.GetTileTypeFromKey(body)
			if !ok {
				continue // unknown special tile key
			}
			if atIndex < len(atPositions) {
				specialTileSpawns = append(specialTileSpawns, SpecialTileSpawn{X: atPositions[atIndex], Y: lineY, TileKey: body, TileType: tileType})
				atIndex++
			}
		case MapDefTile:
			// General (universal, letterless) decoration tile placed by short_label.
			if GlobalTileManager == nil {
				continue
			}
			tileType, ok := GlobalTileManager.GetTileTypeFromShortLabel(body)
			if !ok {
				continue // unknown general tile short_label
			}
			if dollarIndex < len(dollarPositions) {
				generalTileSpawns = append(generalTileSpawns, SpecialTileSpawn{X: dollarPositions[dollarIndex], Y: lineY, TileKey: body, TileType: tileType})
				dollarIndex++
			}
		}
	}

	// Replace both placeholders with '.' (empty walkable) in the tile grid.
	resultTiles := strings.ReplaceAll(tilesPart, string(MapCellInteractive), ".")
	resultTiles = strings.ReplaceAll(resultTiles, string(MapCellGeneral), ".")

	// Return all placeholder X's (interactive + general) so the caller floors
	// the ground beneath every entity cell.
	placeholders := append(append([]int(nil), atPositions...), dollarPositions...)
	return resultTiles, npcSpawns, specialTileSpawns, generalTileSpawns, placeholders
}
