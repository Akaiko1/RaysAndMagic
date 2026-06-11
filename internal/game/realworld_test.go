package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// loadRealWorldForTest boots the REAL tile/map/quest data (chdir to repo root),
// switches to the given map and returns the loaded managers. Globals are
// restored on cleanup. Shared by every test that exercises shipped content.
func loadRealWorldForTest(t *testing.T, cfg *config.Config, mapKey string) (*world.WorldManager, *quests.QuestManager) {
	t.Helper()

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	t.Cleanup(func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	})
	t.Chdir("../..")

	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	if mapKey != "" {
		if err := wm.SwitchToMap(mapKey); err != nil {
			t.Fatalf("switch: %v", err)
		}
	}
	world.GlobalWorldManager = wm

	questCfg, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	return wm, quests.NewQuestManager(questCfg)
}
