package main

import (
	"log"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// Load configuration
	cfg := config.MustLoadConfig("config.yaml")

	// Load and initialize tile manager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		log.Printf("Warning: Failed to load tile config: %v", err)
	}

	// Load monster configuration
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	// Load NPC configuration
	character.MustLoadNPCConfig("assets/npcs.yaml")

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
