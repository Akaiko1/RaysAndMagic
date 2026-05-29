package world

import (
	"fmt"
	"os"
	"sort"
	"time"
	"ugataima/internal/config"
	"ugataima/internal/monster"

	"gopkg.in/yaml.v3"
)

// WorldManager handles multiple loaded maps and transitions between them
type WorldManager struct {
	CurrentMapKey        string
	LoadedMaps           map[string]*World3D
	MapConfigs           map[string]*config.MapConfig
	Biomes               map[string]config.BiomeConfig
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
		CurrentMapKey:        "forest", // Default starting map
		LoadedMaps:           make(map[string]*World3D),
		MapConfigs:           make(map[string]*config.MapConfig),
		Biomes:               make(map[string]config.BiomeConfig),
		TransitionInProgress: false,
		config:               cfg,
		GlobalTeleporterRegistry: &TeleporterRegistry{
			Teleporters:     make([]TeleporterLocation, 0),
			LastUsedByGroup: make(map[string]time.Time),
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

	// Store biome definitions (floor texture groups etc.) shared by maps.
	wm.Biomes = make(map[string]config.BiomeConfig, len(mapConfigs.Biomes))
	for name, biome := range mapConfigs.Biomes {
		wm.Biomes[name] = biome
	}

	// Fail fast: every map's biome must have a definition so floors render.
	for key, mapConfig := range wm.MapConfigs {
		if _, ok := wm.Biomes[mapConfig.Biome]; !ok {
			return fmt.Errorf("map %q references biome %q with no definition in biomes:", key, mapConfig.Biome)
		}
	}

	// Fail fast: catch typos in tiles.yaml floor_texture_group. A tile's
	// named group must be defined by at least one biome (we don't require
	// every biome to define it — universal tiles like water legitimately
	// fall back to base color in biomes that omit the group). The dynamic
	// "beach"/"default" fallbacks are resolved in the renderer, not from a
	// tile field, so they need no entry here.
	if err := wm.validateTileFloorTextureGroups(); err != nil {
		return err
	}

	fmt.Printf("Loaded %d map configurations, %d biomes\n", len(wm.MapConfigs), len(wm.Biomes))
	return nil
}

// validateTileFloorTextureGroups ensures every floor_texture_group named by a
// tile in tiles.yaml is defined in at least one biome, catching typos at load
// time instead of silently rendering the tile's base color.
func (wm *WorldManager) validateTileFloorTextureGroups() error {
	if GlobalTileManager == nil {
		return nil // tiles not loaded (e.g. a context that skips them) — nothing to check
	}
	definedGroups := make(map[string]bool)
	for _, biome := range wm.Biomes {
		for groupName := range biome.FloorTextureGroups {
			definedGroups[groupName] = true
		}
	}
	for tileKey, data := range GlobalTileManager.ListTiles() {
		group := data.FloorTextureGroup
		if group == "" {
			continue // empty = renderer falls back to the biome "default" group
		}
		if !definedGroups[group] {
			return fmt.Errorf("tile %q references floor_texture_group %q not defined in any biome", tileKey, group)
		}
	}
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

	return nil
}

// Reset reloads all maps and clears global teleporters for a fresh game state.
func (wm *WorldManager) Reset() error {
	// Clear loaded maps and teleporter registry
	wm.LoadedMaps = make(map[string]*World3D)
	wm.GlobalTeleporterRegistry = &TeleporterRegistry{
		Teleporters:     make([]TeleporterLocation, 0),
		LastUsedByGroup: make(map[string]time.Time),
	}

	// Pick a sane starting map based on configs
	startKey := "forest"
	if _, ok := wm.MapConfigs[startKey]; !ok {
		startKey = ""
		for key := range wm.MapConfigs {
			startKey = key
			break
		}
	}
	if startKey == "" {
		return fmt.Errorf("no maps configured")
	}
	wm.CurrentMapKey = startKey

	return wm.LoadAllMaps()
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

	// Register teleporters globally for cross-map teleportation (scan all tiles)
	RegisterTeleportersFromMapData(mapData.SpecialTileSpawns, mapKey, wm.GlobalTeleporterRegistry, mapData.Tiles)

	// Load fixed monsters from map data (converts MonsterSpawn entries to Monster3D objects)
	world.loadMonstersFromMapData(mapData.MonsterSpawns)
	wm.attachMapClearEncounter(world, mapKey, mapConfig)

	// Do NOT add random/procedural monsters on premade (.map) worlds.
	// Only monsters explicitly placed in the map should be present.

	return world, nil
}

func (wm *WorldManager) attachMapClearEncounter(world *World3D, mapKey string, mapConfig *config.MapConfig) {
	if world == nil || mapConfig == nil || len(world.Monsters) == 0 {
		return
	}

	// Multiple independent encounters. Each encounter data-drives its own
	// roster via `monsters: [{type, count}]`; the engine binds the `count`
	// pre-placed monsters of each type nearest to that encounter's chest.
	// The runtime keys reward completion by the EncounterRewards pointer, so
	// each group's chest fires when its bound monsters are all dead.
	if len(mapConfig.ClearEncounters) > 0 {
		tileSize := wm.config.GetTileSize()
		assigned := make([]bool, len(world.Monsters))
		for i := range mapConfig.ClearEncounters {
			ec := &mapConfig.ClearEncounters[i]
			if ec.Rewards == nil {
				continue
			}
			ax, ay := 0.0, 0.0
			if ec.Rewards.TreasureChest != nil {
				ax, ay = tileCenterFromTile(ec.Rewards.TreasureChest.TileX, ec.Rewards.TreasureChest.TileY, tileSize)
			}
			rewards := buildEncounterRewards(ec, mapKey)
			for _, req := range ec.Monsters {
				// Rank unassigned monsters of this type by distance to the chest.
				type candidate struct {
					idx  int
					dist float64
				}
				var cands []candidate
				for mi, mon := range world.Monsters {
					if assigned[mi] || mon.Key != req.Type {
						continue
					}
					dx, dy := mon.X-ax, mon.Y-ay
					cands = append(cands, candidate{mi, dx*dx + dy*dy})
				}
				sort.Slice(cands, func(a, b int) bool { return cands[a].dist < cands[b].dist })
				n := req.Count
				if n > len(cands) {
					fmt.Printf("[WARN] map %q encounter chest %q wants %d %q but only %d available; binding %d\n",
						mapKey, encounterChestID(ec), req.Count, req.Type, len(cands), len(cands))
					n = len(cands)
				}
				for k := 0; k < n; k++ {
					mi := cands[k].idx
					assigned[mi] = true
					world.Monsters[mi].IsEncounterMonster = true
					world.Monsters[mi].EncounterRewards = rewards
				}
			}
		}
		return
	}

	// Single map-wide encounter: every monster shares it; reward fires when
	// the last one dies.
	if mapConfig.ClearEncounter == nil || mapConfig.ClearEncounter.Rewards == nil {
		return
	}
	rewards := buildEncounterRewards(mapConfig.ClearEncounter, mapKey)
	for _, mon := range world.Monsters {
		mon.IsEncounterMonster = true
		mon.EncounterRewards = rewards
	}
}

// encounterChestID returns the chest ID of an encounter for log messages,
// or "<no chest>" when the encounter declares none.
func encounterChestID(ec *config.MapClearEncounterConfig) string {
	if ec.Rewards != nil && ec.Rewards.TreasureChest != nil && ec.Rewards.TreasureChest.ID != "" {
		return ec.Rewards.TreasureChest.ID
	}
	return "<no chest>"
}

// buildEncounterRewards converts a YAML clear-encounter config into the
// runtime reward struct, defaulting the chest's map to the owning map.
func buildEncounterRewards(ec *config.MapClearEncounterConfig, mapKey string) *monster.EncounterRewards {
	cfg := ec.Rewards
	rewards := &monster.EncounterRewards{
		Gold:              cfg.Gold,
		Experience:        cfg.Experience,
		CompletionMessage: cfg.CompletionMessage,
	}
	if cfg.TreasureChest != nil {
		chestCfg := cfg.TreasureChest
		chestMap := chestCfg.Map
		if chestMap == "" {
			chestMap = mapKey
		}
		rewards.TreasureChest = &monster.TreasureChestReward{
			ID:                chestCfg.ID,
			Map:               chestMap,
			TileX:             chestCfg.TileX,
			TileY:             chestCfg.TileY,
			Sprite:            chestCfg.Sprite,
			SizeMultiplier:    chestCfg.SizeMultiplier,
			RandomWeaponCount: chestCfg.RandomWeaponCount,
			Gold:              chestCfg.Gold,
			CompletionMessage: chestCfg.CompletionMessage,
		}
	}
	return rewards
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

// GetCurrentBiomeFloorTextureGroups returns the floor-texture groups defined
// by the current map's biome, or nil if the map/biome is unknown. The
// renderer builds its floor atlas from these groups (keyed by group name).
func (wm *WorldManager) GetCurrentBiomeFloorTextureGroups() map[string][]string {
	mapConfig := wm.GetCurrentMapConfig()
	if mapConfig == nil {
		return nil
	}
	if biome, ok := wm.Biomes[mapConfig.Biome]; ok {
		return biome.FloorTextureGroups
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

// IsValidMap checks if a map key exists
func (wm *WorldManager) IsValidMap(mapKey string) bool {
	_, exists := wm.LoadedMaps[mapKey]
	return exists
}
