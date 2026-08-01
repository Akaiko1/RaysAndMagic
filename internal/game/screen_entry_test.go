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
			x:  layout.buttonX + layout.buttonW/2,
			y:  layout.buttonStartY + layout.buttonH/2,
			at: time.Now().UnixMilli(),
		}},
	}

	if !g.consumeEntryMenuRootReleaseAt(layout.buttonX+layout.buttonW/2, layout.buttonStartY+layout.buttonH/2) {
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
	loadY := layout.buttonStartY + loadButtonIndex*(layout.buttonH+layout.buttonGap)

	if !g.consumeEntryMenuRootReleaseAt(layout.buttonX+layout.buttonW/2, loadY+layout.buttonH/2) {
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
			x:  layout.buttonX + layout.buttonW/2,
			y:  layout.buttonStartY + layout.buttonH/2,
			at: time.Now().UnixMilli(),
		}},
	}

	if g.consumeEntryMenuRootReleaseAt(layout.buttonX+layout.buttonW/2, layout.buttonStartY+layout.buttonH/2) {
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

	quitButtonIndex := -1
	for i, button := range entryButtons() {
		if button.key == "quit" {
			quitButtonIndex = i
			break
		}
	}
	if quitButtonIndex < 0 {
		t.Fatal("Quit entry button is missing")
	}
	quitY := layout.buttonStartY + quitButtonIndex*(layout.buttonH+layout.buttonGap)
	if g.consumeEntryMenuRootReleaseAt(layout.buttonX+layout.buttonW/2, quitY+layout.buttonH/2) {
		t.Fatal("unarmed release activated Quit")
	}
	if g.exitRequested {
		t.Fatal("release without an observed press requested exit")
	}
}

func TestEntryMenuRootLayoutFitsShortWindows(t *testing.T) {
	minW, minH := MinimumWindowSize()
	for _, size := range []struct{ w, h int }{{minW, minH}, {640, 480}, {800, 600}, {1280, 720}} {
		layout := makeEntryMenuRootLayout(size.w, size.h)
		aspectError := layout.logoW*entryLogoH - layout.logoH*entryLogoW
		if aspectError < 0 {
			aspectError = -aspectError
		}
		if aspectError > entryLogoH {
			t.Errorf("%dx%d: logo aspect changed to %dx%d", size.w, size.h, layout.logoW, layout.logoH)
		}
		if layout.buttonH < entryButtonMinH {
			t.Errorf("%dx%d: button height = %d, want at least %d", size.w, size.h, layout.buttonH, entryButtonMinH)
		}
		if layout.buttonStartY < layout.logoY+layout.logoH {
			t.Errorf("%dx%d: buttons start at %d over logo ending at %d", size.w, size.h, layout.buttonStartY, layout.logoY+layout.logoH)
		}
		bottom := layout.buttonStartY + len(entryButtons())*layout.buttonH + (len(entryButtons())-1)*layout.buttonGap
		if bottom > size.h-entryBottomGap {
			t.Errorf("%dx%d: buttons end at %d, content limit is %d", size.w, size.h, bottom, size.h-entryBottomGap)
		}
	}
}

func TestMinimumWindowSizeGrowsWithRootButtons(t *testing.T) {
	original := entryButtonDefs
	defer func() { entryButtonDefs = original }()
	_, originalH := MinimumWindowSize()
	entryButtonDefs = append([]entryButton(nil), original...)
	grew := false
	for i := 0; i < 32; i++ {
		entryButtonDefs = append(entryButtonDefs, entryButton{key: "extra", label: "Extra"})
		_, expandedH := MinimumWindowSize()
		if expandedH > originalH {
			grew = true
			break
		}
	}
	if !grew {
		t.Fatal("minimum window height did not grow after adding 32 root buttons")
	}
	w, h := MinimumWindowSize()
	if h <= originalH {
		t.Fatalf("expanded minimum height = %d, want greater than original %d", h, originalH)
	}
	layout := makeEntryMenuRootLayout(w, h)
	bottom := layout.buttonStartY + len(entryButtons())*layout.buttonH + (len(entryButtons())-1)*layout.buttonGap
	if bottom > h-entryBottomGap {
		t.Fatalf("expanded root menu ends at %d, content limit is %d", bottom, h-entryBottomGap)
	}
	if layout.buttonH < entryButtonMinH {
		t.Fatalf("expanded root menu button height = %d, want at least %d", layout.buttonH, entryButtonMinH)
	}
}
