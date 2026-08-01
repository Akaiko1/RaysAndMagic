package main

import (
	"errors"
	"log"
	"os"
	"runtime"

	"ugataima/internal/boot"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/sound"
	"ugataima/internal/storage"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {

	// Shared content configs (also loaded by the map editor).
	cfg, _ := boot.LoadGameData()
	minWindowW, minWindowH := game.MinimumWindowSize()

	// Game-only configs.
	config.MustLoadLevelUpConfig("assets/level_up.yaml")

	// Load achievement definitions (optional - stubbed feature, non-fatal).
	if _, err := config.LoadAchievementConfig("assets/achievements.yaml"); err != nil {
		log.Printf("Warning: Failed to load achievements config: %v", err)
	}

	// Load aggro relationships (which party traits enrage passive monsters)
	monster.MustLoadHatesConfig("assets/hates.yaml")

	// Load quest configuration and initialize quest manager
	questConfig, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		log.Fatalf("Failed to load quest config: %v", err)
	}
	quests.GlobalQuestManager = quests.NewQuestManager(questConfig)
	quests.GlobalQuestManager.InitializeStartingQuests()

	// Tavern rumors: the guide-rail hints shown at every tavern.
	if err := game.LoadRumorConfig("assets/rumors.yaml", quests.GlobalQuestManager); err != nil {
		log.Fatalf("Failed to load rumors: %v", err)
	}

	// Initialize and load world manager
	world.GlobalWorldManager = world.NewWorldManager(cfg)
	if err := world.GlobalWorldManager.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		log.Fatalf("Failed to load map configs: %v", err)
	}
	if cfg.OpenWorldEnabled() {
		world.GlobalWorldManager.SetOpenWorldConfig(config.MustLoadOpenWorldConfig("assets/open_world.yaml"))
	}
	if err := world.GlobalWorldManager.LoadAllMaps(); err != nil {
		log.Fatalf("Failed to load maps: %v", err)
	}
	audioManager, err := sound.LoadGlobal("assets/audio.yaml", storage.AppSavePath("audio_settings.json"))
	if err != nil {
		if sound.IsCatalogContractError(err) {
			log.Fatalf("Invalid audio catalog: %v", err)
		}
		log.Printf("Warning: Failed to load audio; continuing without sound: %v", err)
	} else {
		validators := []struct {
			name string
			run  func() error
		}{
			{name: "gameplay sounds", run: func() error { return audioManager.ValidateSoundKeys(game.RequiredSoundKeys()) }},
			{name: "magic schools", run: func() error { return audioManager.ValidateSchoolSounds(game.RequiredSoundSchools()) }},
			{name: "ranged weapon categories", run: func() error { return audioManager.ValidateWeaponCategories(game.RequiredWeaponSoundCategories()) }},
			{name: "music biomes", run: func() error { return audioManager.ValidateMusicBiomes(knownMusicBiomes(world.GlobalWorldManager)) }},
		}
		for _, validator := range validators {
			if err := validator.run(); err != nil {
				audioManager.Close()
				log.Fatalf("Invalid audio catalog contract (%s): %v", validator.name, err)
			}
		}
		defer audioManager.Close()
	}

	// Set window properties from config
	ebiten.SetWindowSizeLimits(minWindowW, minWindowH, -1, -1)
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

func knownMusicBiomes(manager *world.WorldManager) []string {
	if manager == nil {
		return nil
	}
	biomes := make([]string, 0, len(manager.Biomes))
	for biome := range manager.Biomes {
		biomes = append(biomes, biome)
	}
	return biomes
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
