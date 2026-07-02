package main

import (
	"errors"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"

	"ugataima/internal/boot"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// Seed RNG for combat rolls (crit, loot, etc.)
	rand.Seed(time.Now().UnixNano())

	// Shared content configs (also loaded by the map editor).
	cfg, _ := boot.LoadGameData()

	// Game-only configs.
	config.MustLoadLootTables("assets/loots.yaml")
	config.MustLoadLevelUpConfig("assets/level_up.yaml")

	// Load achievement definitions (optional — stubbed feature, non-fatal).
	if _, err := config.LoadAchievementConfig("assets/achievements.yaml"); err != nil {
		log.Printf("Warning: Failed to load achievements config: %v", err)
	}

	// Load aggro relationships (which party traits enrage passive monsters)
	monster.MustLoadHatesConfig("assets/hates.yaml")

	// Load quest configuration and initialize quest manager
	questConfig, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		log.Printf("Warning: Failed to load quest config: %v", err)
	} else {
		quests.GlobalQuestManager = quests.NewQuestManager(questConfig)
		quests.GlobalQuestManager.InitializeStartingQuests()
	}

	// Initialize and load world manager
	world.GlobalWorldManager = world.NewWorldManager(cfg)
	if err := world.GlobalWorldManager.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		log.Fatalf("Failed to load map configs: %v", err)
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
	if cfg.Display.Fullscreen {
		ebiten.SetFullscreen(true)
	}
	disableVsync := runtime.GOOS == "darwin" && cfg.Display.DisableVsyncOnMac
	if disableVsync {
		ebiten.SetVsyncEnabled(false)
	}
	tps := cfg.GetTPS()
	if disableVsync {
		tps = 120
	}
	ebiten.SetTPS(tps)

	g := game.NewMMGame(cfg)
	defer g.Shutdown()

	// --test-arena: fast-forward the party to a mid-game state for testing.
	if hasFlag("--test-arena") {
		g.ApplyTestArena()
	}
	if err := ebiten.RunGame(g); err != nil {
		if errors.Is(err, game.ErrExit) {
			// Clean exit requested from game
			return
		}
		log.Fatal(err)
	}
}

// hasFlag reports whether the given command-line flag was passed.
func hasFlag(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}
