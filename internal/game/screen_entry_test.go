package game

import (
	"testing"
	"time"

	"ugataima/internal/config"
)

func entryMenuTestConfig() *config.Config {
	return &config.Config{
		Display: config.DisplayConfig{
			ScreenWidth:  1280,
			ScreenHeight: 720,
		},
	}
}

func TestConsumeEntryMenuRootReleaseHandlesStart(t *testing.T) {
	cfg := loadTestConfig(t)
	layout := makeEntryMenuRootLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g := &MMGame{
		config:                  cfg,
		appScreen:               AppScreenMainMenu,
		entryMenuMode:           EntryMenuRoot,
		entryMenuRootPressArmed: true,
		mouseLeftClicks: []queuedClick{{
			x:  layout.buttonX + entryButtonW/2,
			y:  layout.buttonStartY + entryButtonH/2,
			at: time.Now().UnixMilli(),
		}},
	}

	if !g.consumeEntryMenuRootReleaseAt(layout.buttonX+entryButtonW/2, layout.buttonStartY+entryButtonH/2) {
		t.Fatal("release on Start was not handled")
	}
	if g.appScreen != AppScreenPartyCreate {
		t.Fatalf("app screen = %v, want party creation", g.appScreen)
	}
	if g.partyCreate == nil {
		t.Fatal("Start did not initialize party creation")
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("click queue length = %d, want 0 after handling", len(g.mouseLeftClicks))
	}
}

func TestConsumeEntryMenuRootReleaseUsesCurrentCursorPosition(t *testing.T) {
	cfg := entryMenuTestConfig()
	g := &MMGame{
		config:                  cfg,
		entryMenuMode:           EntryMenuRoot,
		slotSelection:           4,
		savePage:                3,
		entryMenuRootPressArmed: true,
		mouseLeftClicks: []queuedClick{{
			x:  0,
			y:  0,
			at: time.Now().UnixMilli(),
		}},
	}
	layout := makeEntryMenuRootLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	const loadButtonIndex = 1
	loadY := layout.buttonStartY + loadButtonIndex*(entryButtonH+entryButtonGap)

	if !g.consumeEntryMenuRootReleaseAt(layout.buttonX+entryButtonW/2, loadY+entryButtonH/2) {
		t.Fatal("release at the current Load position was not handled")
	}
	if g.entryMenuMode != EntryMenuLoad {
		t.Fatalf("entry menu mode = %v, want Load", g.entryMenuMode)
	}
	if g.slotSelection != 0 || g.savePage != 0 {
		t.Fatalf("load selection = slot %d page %d, want slot 0 page 0", g.slotSelection, g.savePage)
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("stale press remained in click queue: %d", len(g.mouseLeftClicks))
	}
}

func TestConsumeEntryMenuRootReleaseLeavesSubscreenClicksAlone(t *testing.T) {
	cfg := entryMenuTestConfig()
	layout := makeEntryMenuRootLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g := &MMGame{
		config:        cfg,
		entryMenuMode: EntryMenuLoad,
		mouseLeftClicks: []queuedClick{{
			x:  layout.buttonX + entryButtonW/2,
			y:  layout.buttonStartY + entryButtonH/2,
			at: time.Now().UnixMilli(),
		}},
	}

	if g.consumeEntryMenuRootReleaseAt(layout.buttonX+entryButtonW/2, layout.buttonStartY+entryButtonH/2) {
		t.Fatal("root handler consumed a Load subscreen click")
	}
	if len(g.mouseLeftClicks) != 1 {
		t.Fatalf("click queue length = %d, want untouched click", len(g.mouseLeftClicks))
	}
}

func TestConsumeEntryMenuRootReleaseRequiresObservedPress(t *testing.T) {
	cfg := entryMenuTestConfig()
	layout := makeEntryMenuRootLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g := &MMGame{
		config:        cfg,
		entryMenuMode: EntryMenuRoot,
	}

	const quitButtonIndex = 4
	quitY := layout.buttonStartY + quitButtonIndex*(entryButtonH+entryButtonGap)
	if g.consumeEntryMenuRootReleaseAt(layout.buttonX+entryButtonW/2, quitY+entryButtonH/2) {
		t.Fatal("unarmed release activated Quit")
	}
	if g.exitRequested {
		t.Fatal("release without an observed press requested exit")
	}
}
