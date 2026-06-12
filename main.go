package main

import (
	"errors"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ensureRuntimeCWD()
	// Seed RNG for combat rolls (crit, loot, etc.)
	rand.Seed(time.Now().UnixNano())
	// Load configuration
	cfg := config.MustLoadConfig("config.yaml")

	// Load unified spell configuration
	config.MustLoadSpellConfig("assets/spells.yaml")

	// Load weapon configuration
	config.MustLoadWeaponConfig("assets/weapons.yaml")

	// Load non-weapon item configuration
	config.MustLoadItemConfig("assets/items.yaml")

	// Load loot tables
	config.MustLoadLootTables("assets/loots.yaml")

	// Load level-up choices
	config.MustLoadLevelUpConfig("assets/level_up.yaml")

	// Setup bridges to avoid circular imports
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

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

	// Load aggro relationships (which party traits enrage passive monsters)
	monster.MustLoadHatesConfig("assets/hates.yaml")

	// Load NPC configuration (needed before world loading)
	character.MustLoadNPCConfig("assets/npcs.yaml")

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

func ensureRuntimeCWD() {
	if _, err := os.Stat("config.yaml"); err == nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(exe)
	if runtimeDir, ok := findRuntimeCWD(execDir, os.Stat); ok {
		_ = os.Chdir(runtimeDir)
	}
}

func findRuntimeCWD(execDir string, stat func(string) (os.FileInfo, error)) (string, bool) {
	candidates := []string{
		execDir,
		filepath.Join(execDir, ".."),
		// macOS .app bundle: Resources is sibling of MacOS.
		filepath.Join(execDir, "..", "Resources"),
	}
	for _, candidate := range candidates {
		if _, err := stat(filepath.Join(candidate, "config.yaml")); err == nil {
			return filepath.Clean(candidate), true
		}
	}
	return "", false
}
