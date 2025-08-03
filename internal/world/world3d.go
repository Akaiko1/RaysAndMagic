package world

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

type World3D struct {
	Width    int
	Height   int
	Tiles    [][]TileType3D
	Monsters []*monster.Monster3D
	NPCs     []*character.NPC
	Items    []*character.WorldItem
	Teachers []*character.SkillTeacher
	config   *config.Config
	// Starting position from map file
	StartX int
	StartY int
	// Magic effects
	walkOnWaterActive bool
}

func NewWorld3D(cfg *config.Config) *World3D {
	world := &World3D{
		Monsters: make([]*monster.Monster3D, 0),
		NPCs:     make([]*character.NPC, 0),
		Items:    make([]*character.WorldItem, 0),
		Teachers: make([]*character.SkillTeacher, 0),
		config:   cfg,
	}

	// Load the world from map file
	world.loadFromMapFile()

	// Place skill teachers in appropriate locations
	world.placeSkillTeachers()

	return world
}

// loadFromMapFile loads the world from the forest.map file
func (w *World3D) loadFromMapFile() {
	// Create map loader
	mapLoader := NewMapLoader(w.config)

	// Load the forest map
	mapData, err := mapLoader.LoadForestMap()
	if err != nil {
		// Fallback to procedural generation if map loading fails
		fmt.Printf("Warning: Failed to load map file, falling back to procedural generation: %v\n", err)
		w.Width = w.config.GetMapWidth()
		w.Height = w.config.GetMapHeight()
		w.StartX = w.Width / 2
		w.StartY = w.Height / 2
		w.Tiles = make([][]TileType3D, w.Height)
		for y := 0; y < w.Height; y++ {
			w.Tiles[y] = make([]TileType3D, w.Width)
		}
		w.generateElvishForest()
		w.populateWithMonsters()
		return
	}

	// Use loaded map data
	w.Width = mapData.Width
	w.Height = mapData.Height
	w.StartX = mapData.StartX
	w.StartY = mapData.StartY

	// Copy loaded tiles directly (already converted to TileType3D)
	w.Tiles = mapData.Tiles

	// Load NPCs from map data
	w.loadNPCsFromMapData(mapData.NPCSpawns)

	// Populate with monsters
	w.populateWithMonsters()
}

// CanMoveTo checks if the player can move to the specified position
func (w *World3D) CanMoveTo(x, y float64) bool {
	tileX := int(x / 64)
	tileY := int(y / 64)

	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return false
	}

	tile := w.Tiles[tileY][tileX]

	// Check if tile blocks movement using tile manager
	if GlobalTileManager != nil {
		return GlobalTileManager.IsWalkable(tile)
	}

	// Fallback logic if tile manager not available
	switch tile {
	case TileWall, TileTree, TileAncientTree, TileThicket, TileMossRock:
		return false
	case TileWater:
		return false // Water blocks movement (could be changed for swimming)
	case TileForestStream:
		return true // Forest Stream is passable
	default:
		return true
	}
}

// GetTileAt returns the tile type at the given world coordinates
func (w *World3D) GetTileAt(x, y float64) TileType3D {
	tileSize := w.config.GetTileSize()
	tileX := int(x / tileSize)
	tileY := int(y / tileSize)

	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return TileWall // Treat out-of-bounds as walls
	}

	return w.Tiles[tileY][tileX]
}

// GetStartingPosition returns the starting position for the player
func (w *World3D) GetStartingPosition() (float64, float64) {
	tileSize := w.config.GetTileSize()
	return float64(w.StartX) * tileSize, float64(w.StartY) * tileSize
}

// RegisterMonstersWithCollisionSystem registers all monsters with the collision system
func (w *World3D) RegisterMonstersWithCollisionSystem(collisionSystem *collision.CollisionSystem) {
	for _, monster := range w.Monsters {
		// Use the monster's unique ID instead of array index

		// Get monster size from YAML config
		width, height := monster.GetSize()

		entity := collision.NewEntity(monster.ID, monster.X, monster.Y, width, height, collision.CollisionTypeMonster, true)
		collisionSystem.RegisterEntity(entity)
	}
}

// getMonsterSize function removed - monsters now use GetSize() method
// which reads size from YAML configuration

// IsTileBlocking implements the collision.TileChecker interface
func (w *World3D) IsTileBlocking(tileX, tileY int) bool {
	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return true // Treat out-of-bounds as blocking
	}

	tile := w.Tiles[tileY][tileX]

	// Use tile manager to check if tile blocks movement
	if GlobalTileManager != nil {
		isWalkable := GlobalTileManager.IsWalkable(tile)

		// Special case: if walk on water is active and this is a water tile, allow movement
		if !isWalkable && w.walkOnWaterActive && tile == TileWater {
			return false // Allow movement on water
		}

		return !isWalkable
	}

	// Fallback logic if tile manager not available
	switch tile {
	case TileWall, TileTree, TileAncientTree, TileThicket, TileMossRock:
		return true
	case TileWater:
		// Allow movement on water if walk on water is active
		return !w.walkOnWaterActive
	case TileForestStream:
		return false // Forest Stream is passable
	default:
		return false
	}
}

// GetWorldBounds implements the collision.TileChecker interface
func (w *World3D) GetWorldBounds() (width, height int) {
	return w.Width, w.Height
}

// SetWalkOnWaterActive sets the walk on water state for the world
func (w *World3D) SetWalkOnWaterActive(active bool) {
	w.walkOnWaterActive = active
}

// loadNPCsFromMapData loads NPCs from map spawn data
func (w *World3D) loadNPCsFromMapData(npcSpawns []NPCSpawn) {
	for _, spawn := range npcSpawns {
		// Convert tile coordinates to world coordinates
		worldX := float64(spawn.X * 64) // 64 is tile size
		worldY := float64(spawn.Y * 64)

		// Create NPC from configuration
		npc, err := character.CreateNPCFromConfig(spawn.NPCKey, worldX, worldY)
		if err != nil {
			fmt.Printf("Warning: Failed to create NPC %s: %v\n", spawn.NPCKey, err)
			continue
		}

		w.NPCs = append(w.NPCs, npc)
	}
}

// Helper function for absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
