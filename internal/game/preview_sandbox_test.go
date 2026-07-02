package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// setupPreviewSandboxTest prepares the globals an editor preview sandbox
// (FxPreview, MobPreview) needs, mirroring real editor conditions: tile
// manager loaded, no world manager (the sandbox installs its own stage), no
// quest manager (the editor never loads quests — one left behind by another
// test would make NewMMGame validate quest tile-changes against the sandbox's
// single-map world and panic). Restores everything on cleanup.
func setupPreviewSandboxTest(t *testing.T) *config.Config {
	t.Helper()
	cfg := loadTestConfig(t)
	if world.GlobalTileManager == nil {
		tm := world.NewTileManager()
		if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
			t.Fatalf("load tiles: %v", err)
		}
		if err := tm.LoadSpecialTileConfig("../../assets/special_tiles.yaml"); err != nil {
			t.Fatalf("load special tiles: %v", err)
		}
		world.GlobalTileManager = tm
	}
	prevWM := world.GlobalWorldManager
	world.GlobalWorldManager = nil
	t.Cleanup(func() { world.GlobalWorldManager = prevWM })
	prevQM := quests.GlobalQuestManager
	quests.GlobalQuestManager = nil
	t.Cleanup(func() { quests.GlobalQuestManager = prevQM })
	return cfg
}
