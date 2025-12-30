package world

import (
	"fmt"
	"math/rand"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// TeleporterRegistry manages teleporter locations and cooldowns
// TeleporterRegistry manages all teleporters globally, across all worlds
type TeleporterRegistry struct {
	Teleporters    []TeleporterLocation // All teleporters, regardless of type or world
	LastUsedTime   time.Time
	CooldownPeriod time.Duration
}

// TeleporterLocation represents a teleporter's position and properties
// TeleporterLocation represents a teleporter's position and properties
type TeleporterLocation struct {
	X, Y     int
	TileType TileType3D
	Label    string // Unique label for this teleporter
	MapKey   string // Which map/world this teleporter is in
	Type     string // e.g., "violet", "red", etc.
}

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
	walkOnWaterActive    bool
	waterBreathingActive bool
}

func NewWorld3D(cfg *config.Config) *World3D {
	world := &World3D{
		Monsters: make([]*monster.Monster3D, 0),
		NPCs:     make([]*character.NPC, 0),
		Items:    make([]*character.WorldItem, 0),
		Teachers: make([]*character.SkillTeacher, 0),
		config:   cfg,
	}

	// Note: Map loading is now handled by WorldManager
	// No longer auto-loading forest.map here to avoid conflicts

	// Place skill teachers in appropriate locations
	world.placeSkillTeachers()

	return world
}

// loadFromMapFile loads the world from the forest.map file (legacy, used by tests)
func (w *World3D) loadFromMapFile() {
	// Create map loader
	mapLoader := NewMapLoader(w.config)

	// Load the forest map
	mapData, err := mapLoader.LoadForestMap()
	if err != nil {
		panic(fmt.Sprintf("Failed to load map file: %v", err))
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

	// Load monsters from map data (fixed placements only)
	w.loadMonstersFromMapData(mapData.MonsterSpawns)
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
	case TileWater, TileDeepWater:
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
	if w.StartX == -1 || w.StartY == -1 {
		panic("Map has no starting position defined! Maps must have a '+' symbol to be used as starting maps.")
	}
	tileSize := w.config.GetTileSize()
	return float64(w.StartX) * tileSize, float64(w.StartY) * tileSize
}

// Registers all teleporter tiles in the map into the global registry with unique labels and type
// This should be called by WorldManager, passing the global registry
func RegisterTeleportersFromMapData(specialTileSpawns []SpecialTileSpawn, mapKey string, registry *TeleporterRegistry, tiles [][]TileType3D) {
	count := 0
	for y, row := range tiles {
		for x, tile := range row {
			teleType := ""
			switch tile {
			case TileVioletTeleporter:
				teleType = "violet"
			case TileRedTeleporter:
				teleType = "red"
			}
			if teleType != "" {
				label := fmt.Sprintf("%s_%s_%d_%d", mapKey, teleType, x, y)
				teleporter := TeleporterLocation{
					X:        x,
					Y:        y,
					TileType: tile,
					Label:    label,
					MapKey:   mapKey,
					Type:     teleType,
				}
				// Ensure uniqueness by label
				found := false
				for _, t := range registry.Teleporters {
					if t.Label == teleporter.Label {
						found = true
						break
					}
				}
				if !found {
					registry.Teleporters = append(registry.Teleporters, teleporter)
					count++
				}
			}
		}
	}
	fmt.Printf("Registered %d teleporters in world '%s' (global registry)\n", count, mapKey)
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

// IsTileBlocking implements the collision.TileChecker interface
func (w *World3D) IsTileBlocking(tileX, tileY int) bool {
	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return true // Treat out-of-bounds as blocking
	}

	tile := w.Tiles[tileY][tileX]

	// Use tile manager to check if tile blocks movement
	if GlobalTileManager != nil {
		isWalkable := GlobalTileManager.IsWalkable(tile)

		// Special case: if walk on water OR water breathing is active and this is a water tile, allow movement
		if !isWalkable && (w.walkOnWaterActive || w.waterBreathingActive) && (tile == TileWater || tile == TileDeepWater) {
			return false // Allow movement on water
		}

		return !isWalkable
	}

	// Fallback logic if tile manager not available
	switch tile {
	case TileWall, TileTree, TileAncientTree, TileThicket, TileMossRock:
		return true
	case TileWater, TileDeepWater:
		// Allow movement on water if walk on water is active
		return !w.walkOnWaterActive
	case TileForestStream:
		return false // Forest Stream is passable
	default:
		return false
	}
}

// IsTileBlockingForHabitat checks if a tile blocks movement for a monster with given habitat preferences
// Monsters can walk on tiles that are in their habitat preferences even if normally blocked
func (w *World3D) IsTileBlockingForHabitat(tileX, tileY int, habitatPrefs []string) bool {
	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return true // Treat out-of-bounds as blocking
	}

	tile := w.Tiles[tileY][tileX]

	// Use tile manager to check if tile blocks movement
	if GlobalTileManager != nil {
		isWalkable := GlobalTileManager.IsWalkable(tile)

		// If already walkable, return false (not blocking)
		if isWalkable {
			return false
		}

		// Check if this tile type is in the monster's habitat preferences
		if len(habitatPrefs) > 0 {
			tileKey := GlobalTileManager.GetTileKey(tile)
			for _, habitat := range habitatPrefs {
				if tileKey == habitat {
					return false // Monster can walk on its habitat tiles
				}
			}
		}

		return true // Tile is not walkable and not in habitat preferences
	}

	// Fallback to standard blocking check if tile manager not available
	return w.IsTileBlocking(tileX, tileY)
}

// IsTileOpaque implements the collision.TileChecker interface
func (w *World3D) IsTileOpaque(tileX, tileY int) bool {
	if tileX < 0 || tileX >= w.Width || tileY < 0 || tileY >= w.Height {
		return true // Treat out-of-bounds as opaque
	}

	tile := w.Tiles[tileY][tileX]

	// Use tile manager to check if tile blocks sight
	if GlobalTileManager != nil {
		return GlobalTileManager.IsOpaque(tile)
	}

	// Fallback logic if tile manager not available
	// For now, fall back to blocking logic (opaque = blocks movement)
	switch tile {
	case TileWall, TileTree, TileAncientTree, TileThicket:
		return true
	case TileWater, TileDeepWater, TileForestStream, TileMossRock:
		return false // Water and moss rocks don't block sight
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

// SetWaterBreathingActive sets the water breathing state for the world
func (w *World3D) SetWaterBreathingActive(active bool) {
	w.waterBreathingActive = active
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

// loadMonstersFromMapData loads monsters from map spawn data
func (w *World3D) loadMonstersFromMapData(monsterSpawns []MonsterSpawn) {
	for _, spawn := range monsterSpawns {
		// Convert tile coordinates to world coordinates (spawn at tile center)
		worldX := float64(spawn.X*64) + 32 // 64 is tile size, +32 for center
		worldY := float64(spawn.Y*64) + 32

		// Create monster from YAML configuration
		newMonster := monster.NewMonster3DFromConfig(worldX, worldY, spawn.MonsterKey, w.config)
		w.Monsters = append(w.Monsters, newMonster)
	}
}

// --- Teleporter System Refactored Functions ---

// GetRandomDestinationTeleporter selects a random teleporter of the same type, not the source, from the global registry
func (reg *TeleporterRegistry) GetRandomDestinationTeleporter(source TeleporterLocation) (TeleporterLocation, bool) {
	var candidates []TeleporterLocation
	for _, t := range reg.Teleporters {
		if t.Type == source.Type && !(t.MapKey == source.MapKey && t.X == source.X && t.Y == source.Y) {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return TeleporterLocation{}, false
	}
	idx := rand.Intn(len(candidates))
	return candidates[idx], true
}

// TeleportParty handles teleportation logic: selects a random destination and moves the party, switching worlds if needed
func (wm *WorldManager) TeleportParty(source TeleporterLocation) bool {
	reg := wm.GlobalTeleporterRegistry
	dest, ok := reg.GetRandomDestinationTeleporter(source)
	if !ok {
		fmt.Println("No valid destination teleporter found.")
		return false
	}
	if dest.MapKey != wm.CurrentMapKey {
		// Switch world
		if err := wm.SwitchToMap(dest.MapKey); err != nil {
			fmt.Printf("Failed to switch world: %v\n", err)
			return false
		}
	}
	world := wm.GetCurrentWorld()
	if world == nil {
		fmt.Println("No current world loaded after teleport.")
		return false
	}
	// Move party to destination teleporter coordinates
	world.StartX = dest.X
	world.StartY = dest.Y
	// Optionally, set player position directly if needed
	fmt.Printf("Teleported to %s at (%d, %d) in world '%s'\n", dest.Label, dest.X, dest.Y, dest.MapKey)
	return true
}

// --- Teleporter System Refactored Functions ---
// Place these after all other code, after the last closing brace

// --- Teleporter System Refactored Functions ---
// Place these at the end of the file after all type and method definitions
