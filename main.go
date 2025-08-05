package main

import (
	"log"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// Load configuration
	cfg := config.MustLoadConfig("config.yaml")

	// Initialize dynamic spell system with configuration
	spells.SetGlobalConfig(cfg)

	// Load and initialize tile manager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		log.Printf("Warning: Failed to load tile config: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		log.Printf("Warning: Failed to load special tile config: %v", err)
	}

	// Load monster configuration (needed before world loading)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	// Load NPC configuration (needed before world loading)
	character.MustLoadNPCConfig("assets/npcs.yaml")

	// Initialize and load world manager
	world.GlobalWorldManager = world.NewWorldManager(cfg)
	if err := world.GlobalWorldManager.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		log.Printf("Warning: Failed to load map configs: %v", err)
	}
	if err := world.GlobalWorldManager.LoadAllMaps(); err != nil {
		log.Fatalf("Failed to load maps: %v", err)
	}

	// Set window properties from config
	ebiten.SetWindowSize(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	ebiten.SetWindowTitle(cfg.Display.WindowTitle)
	if cfg.Display.Resizable {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	}

	g := game.NewMMGame(cfg)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
