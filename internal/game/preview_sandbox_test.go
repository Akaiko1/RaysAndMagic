package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// setupPreviewSandboxTest prepares the globals an editor preview sandbox
// (FxPreview, MobPreview) needs, mirroring real editor conditions: tile
// manager loaded, no world manager (the sandbox installs its own stage), no
// quest manager, and the real ecology config loaded by editor boot. Restores
// globals on cleanup; campaign maps are deliberately absent.
func setupPreviewSandboxTest(t *testing.T) *config.Config {
	t.Helper()
	cfg := loadTestConfig(t)
	prevEcology := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = prevEcology })
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	if world.GlobalTileManager == nil {
		tm := world.NewTileManager(testTileSizeClasses())
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

// Invariant: preview stages need content, not campaign maps or a journal;
// ordinary game startup still rejects missing campaign maps. Case table:
// Mobs/FX first, either tab after the other, with/without a global journal.
// Persistence is N/A: preview worlds and journals are never saved.
func TestPreviewSandboxCampaignIsolation(t *testing.T) {
	for _, order := range []string{"mobs_first", "fx_first"} {
		for _, journal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/journal=%v", order, journal), func(t *testing.T) {
				cfg := setupPreviewSandboxTest(t)
				if journal {
					qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
					if err != nil {
						t.Fatal(err)
					}
					quests.GlobalQuestManager = quests.NewQuestManager(qc)
				}
				originalEcology, originalJournal := config.GlobalEcology, quests.GlobalQuestManager
				var mobs *MobPreview
				var fx *FxPreview
				openMobs := func() {
					var err error
					mobs, err = NewMobPreview(cfg)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(mobs.g.Shutdown)
					mobs.Select("goblin")
					mobs.Step()
				}
				openFX := func() {
					var err error
					fx, err = NewFxPreview(cfg)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(fx.g.Shutdown)
					if len(fx.Items()) == 0 {
						t.Fatal("empty FX catalog")
					}
					fx.Select(fx.Items()[0])
					fx.Step()
				}
				if order == "mobs_first" {
					openMobs()
					openFX()
				} else {
					openFX()
					openMobs()
				}
				mobs.Step()
				fx.Step()
				if mobs.g.questManager != nil || fx.g.questManager != nil {
					t.Fatal("preview adopted the campaign journal")
				}
				if mobs.g.worldClickAllowed() || fx.g.worldClickAllowed() {
					t.Fatal("editor preview accepts game-world pointer selection")
				}
				if config.GlobalEcology != originalEcology || quests.GlobalQuestManager != originalJournal {
					t.Fatal("preview replaced global campaign data")
				}
				// Exercise the public campaign constructor, not just its validator.
				defer func() {
					r := recover()
					if r == nil || !strings.Contains(fmt.Sprint(r), "fish map") {
						t.Fatalf("campaign must reject missing fish maps, got %v", r)
					}
				}()
				NewMMGame(cfg)
			})
		}
	}
}
