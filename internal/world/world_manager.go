package world

import (
	"fmt"
	"math/rand"
	"os"
	"time"
	"ugataima/internal/config"

	"gopkg.in/yaml.v3"
)

// WorldManager handles multiple loaded maps and transitions between them
type WorldManager struct {
	CurrentMapKey        string
	LoadedMaps           map[string]*World3D
	MapConfigs           map[string]*config.MapConfig  
	TransitionInProgress bool
	config               *config.Config
	
	// Global teleporter registry for cross-map teleportation
	GlobalTeleporterRegistry *TeleporterRegistry
}

// Global instance
var GlobalWorldManager *WorldManager

// NewWorldManager creates a new world manager
func NewWorldManager(cfg *config.Config) *WorldManager {
	return &WorldManager{
		CurrentMapKey:    "forest", // Default starting map
		LoadedMaps:       make(map[string]*World3D),
		MapConfigs:       make(map[string]*config.MapConfig),
		TransitionInProgress: false,
		config:           cfg,
		GlobalTeleporterRegistry: &TeleporterRegistry{
			VioletTeleporters: make([]TeleporterLocation, 0),
			RedTeleporters:    make([]TeleporterLocation, 0),
			CooldownPeriod:    5 * time.Second,
		},
	}
}

// LoadMapConfigs loads map configurations from map_configs.yaml
func (wm *WorldManager) LoadMapConfigs(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read map configs file: %w", err)
	}

	var mapConfigs config.MapConfigs
	err = yaml.Unmarshal(data, &mapConfigs)
	if err != nil {
		return fmt.Errorf("failed to parse map configs: %w", err)
	}

	// Store map configs
	wm.MapConfigs = make(map[string]*config.MapConfig)
	for key, mapConfig := range mapConfigs.Maps {
		// Make a copy to avoid pointer issues
		configCopy := mapConfig
		wm.MapConfigs[key] = &configCopy
	}

	fmt.Printf("Loaded %d map configurations\n", len(wm.MapConfigs))
	return nil
}

// LoadAllMaps preloads all maps for instant switching
func (wm *WorldManager) LoadAllMaps() error {
	for mapKey, mapConfig := range wm.MapConfigs {
		fmt.Printf("Loading map: %s (%s)\n", mapConfig.Name, mapConfig.File)
		
		world, err := wm.loadSingleMap(mapKey, mapConfig)
		if err != nil {
			fmt.Printf("Warning: Failed to load map %s: %v\n", mapKey, err)
			continue
		}
		
		wm.LoadedMaps[mapKey] = world
		fmt.Printf("Successfully loaded map: %s\n", mapConfig.Name)
	}

	// Ensure we have at least the default map
	if _, exists := wm.LoadedMaps[wm.CurrentMapKey]; !exists {
		return fmt.Errorf("failed to load default map: %s", wm.CurrentMapKey)
	}

	fmt.Printf("World manager initialized with %d maps\n", len(wm.LoadedMaps))
	return nil
}

// loadSingleMap loads a single map file
func (wm *WorldManager) loadSingleMap(mapKey string, mapConfig *config.MapConfig) (*World3D, error) {
	world := NewWorld3D(wm.config)
	
	// Create map loader with biome information
	mapLoader := NewMapLoaderWithBiome(wm.config, mapConfig.Biome)
	
	// Load the specific map file
	mapData, err := mapLoader.LoadMap("assets/" + mapConfig.File)
	if err != nil {
		return nil, fmt.Errorf("failed to load map file %s: %w", mapConfig.File, err)
	}

	// Apply loaded map data to world
	world.Width = mapData.Width
	world.Height = mapData.Height
	world.StartX = mapData.StartX
	world.StartY = mapData.StartY
	world.Tiles = mapData.Tiles

	// Load NPCs from map data
	world.loadNPCsFromMapData(mapData.NPCSpawns)

	// Register teleporters from map data (local to world)
	world.registerTeleportersFromMapData(mapData.SpecialTileSpawns)

	// Register teleporters globally for cross-map teleportation
	wm.registerTeleportersGlobally(mapKey, mapData.SpecialTileSpawns)

	// Populate with monsters
	world.populateWithMonsters()

	return world, nil
}

// GetCurrentWorld returns the currently active world
func (wm *WorldManager) GetCurrentWorld() *World3D {
	if world, exists := wm.LoadedMaps[wm.CurrentMapKey]; exists {
		return world
	}
	
	// Fallback to first available map if current doesn't exist
	for _, world := range wm.LoadedMaps {
		fmt.Printf("Warning: Using fallback map as current map %s not found\n", wm.CurrentMapKey)
		return world
	}
	
	return nil
}

// GetCurrentMapConfig returns the configuration for the current map
func (wm *WorldManager) GetCurrentMapConfig() *config.MapConfig {
	if config, exists := wm.MapConfigs[wm.CurrentMapKey]; exists {
		return config
	}
	return nil
}

// SwitchToMap transitions to a different map
func (wm *WorldManager) SwitchToMap(mapKey string) error {
	if wm.TransitionInProgress {
		return fmt.Errorf("map transition already in progress")
	}

	if _, exists := wm.LoadedMaps[mapKey]; !exists {
		return fmt.Errorf("map not loaded: %s", mapKey)
	}

	wm.TransitionInProgress = true
	oldMap := wm.CurrentMapKey
	wm.CurrentMapKey = mapKey
	wm.TransitionInProgress = false

	fmt.Printf("Switched from map %s to %s\n", oldMap, mapKey)
	return nil
}

// GetAvailableMaps returns list of all available map keys
func (wm *WorldManager) GetAvailableMaps() []string {
	var maps []string
	for mapKey := range wm.LoadedMaps {
		maps = append(maps, mapKey)
	}
	return maps
}

// GetMapConfig returns configuration for a specific map
func (wm *WorldManager) GetMapConfig(mapKey string) *config.MapConfig {
	if config, exists := wm.MapConfigs[mapKey]; exists {
		return config
	}
	return nil
}

// IsValidMap checks if a map key exists
func (wm *WorldManager) IsValidMap(mapKey string) bool {
	_, exists := wm.LoadedMaps[mapKey]
	return exists
}

// registerTeleportersGlobally registers teleporters from a map into the global registry
func (wm *WorldManager) registerTeleportersGlobally(mapKey string, specialTileSpawns []SpecialTileSpawn) {
	for _, spawn := range specialTileSpawns {
		teleporter := TeleporterLocation{
			X:        spawn.X,
			Y:        spawn.Y,
			TileType: spawn.TileType,
			Key:      spawn.TileKey,
			MapKey:   mapKey,
		}

		switch spawn.TileType {
		case TileVioletTeleporter:
			wm.GlobalTeleporterRegistry.VioletTeleporters = append(wm.GlobalTeleporterRegistry.VioletTeleporters, teleporter)
		case TileRedTeleporter:
			wm.GlobalTeleporterRegistry.RedTeleporters = append(wm.GlobalTeleporterRegistry.RedTeleporters, teleporter)
		}
	}

	fmt.Printf("Global registry now has %d violet and %d red teleporters across all maps\n",
		len(wm.GlobalTeleporterRegistry.VioletTeleporters),
		len(wm.GlobalTeleporterRegistry.RedTeleporters))
}

// TryGlobalTeleport attempts cross-map teleportation
func (wm *WorldManager) TryGlobalTeleport(currentX, currentY float64) (targetMapKey string, newX, newY float64, teleported bool) {
	// Check cooldown
	if time.Since(wm.GlobalTeleporterRegistry.LastUsedTime) < wm.GlobalTeleporterRegistry.CooldownPeriod {
		return wm.CurrentMapKey, currentX, currentY, false
	}

	// Get current world and tile position
	currentWorld := wm.GetCurrentWorld()
	if currentWorld == nil {
		return wm.CurrentMapKey, currentX, currentY, false
	}

	tileSize := wm.config.GetTileSize()
	tileX := int(currentX / tileSize)
	tileY := int(currentY / tileSize)

	// Check if standing on a teleporter
	if tileX < 0 || tileX >= currentWorld.Width || tileY < 0 || tileY >= currentWorld.Height {
		return wm.CurrentMapKey, currentX, currentY, false
	}

	currentTile := currentWorld.Tiles[tileY][tileX]
	var targetTeleporters []TeleporterLocation

	switch currentTile {
	case TileVioletTeleporter:
		targetTeleporters = wm.GlobalTeleporterRegistry.VioletTeleporters
	case TileRedTeleporter:
		targetTeleporters = wm.GlobalTeleporterRegistry.RedTeleporters
	default:
		return wm.CurrentMapKey, currentX, currentY, false
	}

	// Need at least 2 teleporters to teleport
	if len(targetTeleporters) < 2 {
		return wm.CurrentMapKey, currentX, currentY, false
	}

	// Create list of possible destinations (exclude current teleporter)
	var destinations []TeleporterLocation
	for _, tel := range targetTeleporters {
		// Exclude current teleporter (same position and map)
		if tel.MapKey != wm.CurrentMapKey || tel.X != tileX || tel.Y != tileY {
			destinations = append(destinations, tel)
		}
	}

	if len(destinations) == 0 {
		return wm.CurrentMapKey, currentX, currentY, false
	}

	// Pick a random destination
	destination := destinations[rand.Intn(len(destinations))]

	// Update cooldown
	wm.GlobalTeleporterRegistry.LastUsedTime = time.Now()

	// Calculate new position (center of tile)
	newX = float64(destination.X)*tileSize + tileSize/2
	newY = float64(destination.Y)*tileSize + tileSize/2

	fmt.Printf("Global teleported from %s (%d,%d) to %s (%d,%d)\n", 
		wm.CurrentMapKey, tileX, tileY, destination.MapKey, destination.X, destination.Y)

	return destination.MapKey, newX, newY, true
}