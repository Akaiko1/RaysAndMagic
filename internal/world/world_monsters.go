package world

import (
	"fmt"
	"ugataima/internal/monster"
)

// isValidCoordinate checks if coordinates are within world bounds
func (w *World3D) isValidCoordinate(x, y int) bool {
	return x >= 0 && x < w.Width && y >= 0 && y < w.Height
}

// PlaceMonsterByLetter places a monster at specific coordinates using letter marker
func (w *World3D) PlaceMonsterByLetter(x, y float64, letter string) error {
	if monster.MonsterConfig == nil {
		return fmt.Errorf("monster config not loaded")
	}

	_, monsterKey, err := monster.MonsterConfig.GetMonsterByLetter(letter)
	if err != nil {
		return fmt.Errorf("monster with letter '%s' not found: %w", letter, err)
	}

	// Check if location is valid
	tileX, tileY := int(x), int(y)
	if !w.isValidCoordinate(tileX, tileY) {
		return fmt.Errorf("invalid coordinates: %f, %f", x, y)
	}

	// Check if tile is walkable (optional - might want to force placement)
	currentTile := w.Tiles[tileY][tileX]
	if GlobalTileManager == nil || !GlobalTileManager.IsWalkable(currentTile) {
		return fmt.Errorf("tile at %f, %f is not walkable", x, y)
	}

	// Create monster at tile center to avoid getting stuck
	spawnX := x*64 + 32
	spawnY := y*64 + 32
	m := monster.NewMonster3DFromConfig(spawnX, spawnY, monsterKey, w.config)
	w.Monsters = append(w.Monsters, m)

	return nil
}

// LoadMonstersFromMap loads monsters from a 2D character array (useful for map files)
func (w *World3D) LoadMonstersFromMap(mapData [][]rune) error {
	if monster.MonsterConfig == nil {
		return fmt.Errorf("monster config not loaded")
	}

	for y, row := range mapData {
		for x, char := range row {
			if char == ' ' || char == '.' || char == '#' {
				continue // Skip empty spaces, floors, and walls
			}

			letter := string(char)
			if err := w.PlaceMonsterByLetter(float64(x), float64(y), letter); err != nil {
				// Log warning but continue processing
				// Could be a non-monster character like terrain
				continue
			}
		}
	}

	return nil
}
