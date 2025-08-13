package world

import (
	"fmt"
	"math/rand"
	"ugataima/internal/monster"
)

// populateWithMonsters adds monsters to the world based on habitat preferences
func (w *World3D) populateWithMonsters() {
	// Use YAML-based system (required)
	if monster.MonsterConfig == nil {
		panic("Monster configuration not loaded. Cannot populate world with monsters.")
	}

	w.populateWithMonstersFromConfig()
}

// populateWithMonstersFromConfig uses the new YAML-based monster system
func (w *World3D) populateWithMonstersFromConfig() {
	config := monster.MonsterConfig

	// Place regular monsters
	monsterCount := config.Placement.Common.CountMin + rand.Intn(config.Placement.Common.CountMax-config.Placement.Common.CountMin+1)

	for i := 0; i < monsterCount; i++ {
		// Pick a random monster type
		monsterKey := config.GetRandomMonsterKey()
		w.placeMonsterFromConfig(monsterKey)
	}

	// Place special rare monsters
	w.placeSpecialMonstersFromConfig()
}

// Legacy placement functions removed - all monster placement now uses YAML configuration

// placeMonsterFromConfig finds a suitable location for a monster from YAML config
func (w *World3D) placeMonsterFromConfig(monsterKey string) {
	maxAttempts := 50

	for attempt := 0; attempt < maxAttempts; attempt++ {
		x := float64(rand.Intn(w.Width-2) + 1)
		y := float64(rand.Intn(w.Height-2) + 1)

		if w.isSuitableLocationFromConfig(x, y, monsterKey) {
			monster := monster.NewMonster3DFromConfig(x*64, y*64, monsterKey, w.config)
			w.Monsters = append(w.Monsters, monster)
			return
		}
	}
}

// isSuitableLocationFromConfig checks if a location is suitable for a monster from config
func (w *World3D) isSuitableLocationFromConfig(x, y float64, monsterKey string) bool {
	tileX, tileY := int(x), int(y)

	if !w.isValidCoordinate(tileX, tileY) {
		return false
	}

	currentTile := w.Tiles[tileY][tileX]

	// Check if tile is walkable
	if GlobalTileManager == nil || !GlobalTileManager.IsWalkable(currentTile) {
		return false
	}

	def, err := monster.MonsterConfig.GetMonsterByKey(monsterKey)
	if err != nil {
		return true // Default to allowing placement if config error
	}

	// Check habitat preferences
	for _, habitat := range def.HabitatPrefs {
		tileType, err := monster.MonsterConfig.ConvertTileType(habitat)
		if err != nil {
			continue
		}
		if int(currentTile) == tileType {
			return true
		}
	}

	// Check habitat near rules
	for _, rule := range def.HabitatNear {
		requiredTileType, err := monster.MonsterConfig.ConvertTileType(rule.Type)
		if err != nil {
			continue
		}
		if w.isNearTileType(tileX, tileY, TileType3D(requiredTileType), rule.Radius) {
			return true
		}
	}

	// If no habitat preferences defined, allow any walkable tile
	if len(def.HabitatPrefs) == 0 && len(def.HabitatNear) == 0 {
		return true
	}

	return false
}

// placeSpecialMonstersFromConfig adds rare and powerful monsters from config
func (w *World3D) placeSpecialMonstersFromConfig() {
	config := monster.MonsterConfig

	// Chance for a Treant
	if rand.Float64() < config.Placement.Special.TreantChance {
		w.placeMonsterFromConfig("treant")
	}

	// Chance for Pixies
	pixieCount := rand.Intn(config.Placement.Special.PixieCountMax + 1)
	for i := 0; i < pixieCount; i++ {
		w.placeMonsterFromConfig("pixie")
	}

	// Very rare dragon
	if rand.Float64() < config.Placement.Special.DragonChance {
		w.placeMonsterFromConfig("dragon")
	}

	// Rare troll
	if rand.Float64() < config.Placement.Special.TrollChance {
		w.placeMonsterFromConfig("troll")
	}
}

// Legacy placeSpecialMonsters function removed - now uses placeSpecialMonstersFromConfig

// Helper methods

func (w *World3D) isNearTileType(x, y int, tileType TileType3D, radius int) bool {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			checkX, checkY := x+dx, y+dy
			if w.isValidCoordinate(checkX, checkY) {
				if w.Tiles[checkY][checkX] == tileType {
					return true
				}
			}
		}
	}
	return false
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

	// Create monster at specified location
	monster := monster.NewMonster3DFromConfig(x*64, y*64, monsterKey, w.config)
	w.Monsters = append(w.Monsters, monster)

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
