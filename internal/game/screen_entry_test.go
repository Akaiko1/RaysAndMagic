package game

import (
	"testing"
	"time"
)

// Root buttons act on the RELEASE position, only after a press this app saw,
// and only on the root screen. A stale press coordinate (macOS focus settling)
// is dropped with the activation so it cannot leak into the new screen.
func TestConsumeEntryMenuRootRelease(t *testing.T) {
	cfg := loadTestConfig(t)
	layout := makeEntryMenuRootLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	center := func(t *testing.T, key string) (int, int) {
		t.Helper()
		for i, button := range entryButtons() {
			if button.key == key {
				r := layout.button(i)
				return r.x + r.w/2, r.y + r.h/2
			}
		}
		t.Fatalf("root button %q is missing", key)
		return 0, 0
	}
	for _, tc := range []struct {
		name        string
		mode        EntryMenuMode
		armed       bool
		queued      string // button the queued press points at; "" = a stale (0,0)
		release     string
		wantHandled bool
		check       func(t *testing.T, g *MMGame)
	}{
		{"start", EntryMenuRoot, true, "start", "start", true, func(t *testing.T, g *MMGame) {
			if g.appScreen != AppScreenPartyCreate || g.partyCreate == nil {
				t.Fatalf("app screen = %v, party creation set up = %v", g.appScreen, g.partyCreate != nil)
			}
			if len(g.mouseLeftClicks) != 0 {
				t.Fatalf("click queue length = %d, want 0 after handling", len(g.mouseLeftClicks))
			}
		}},
		{"load at the release point", EntryMenuRoot, true, "", "load", true, func(t *testing.T, g *MMGame) {
			if g.entryMenuMode != EntryMenuLoad || g.slotSelection != 0 || g.savePage != 0 {
				t.Fatalf("mode %v slot %d page %d, want Load at slot 0 page 0", g.entryMenuMode, g.slotSelection, g.savePage)
			}
			if len(g.mouseLeftClicks) != 0 {
				t.Fatalf("stale press remained in click queue: %d", len(g.mouseLeftClicks))
			}
		}},
		{"subscreen click left alone", EntryMenuLoad, false, "start", "start", false, func(t *testing.T, g *MMGame) {
			if len(g.mouseLeftClicks) != 1 || g.appScreen != AppScreenMainMenu {
				t.Fatalf("click queue length = %d, app screen %v; want the click untouched", len(g.mouseLeftClicks), g.appScreen)
			}
		}},
		{"unarmed release", EntryMenuRoot, false, "", "quit", false, func(t *testing.T, g *MMGame) {
			if g.exitRequested {
				t.Fatal("release without an observed press requested exit")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &MMGame{
				menuState:               menuState{slotSelection: 4, savePage: 3},
				config:                  cfg,
				appScreen:               AppScreenMainMenu,
				entryMenuMode:           tc.mode,
				entryMenuRootPressArmed: tc.armed,
			}
			qx, qy := 0, 0
			if tc.queued != "" {
				qx, qy = center(t, tc.queued)
			}
			g.mouseLeftClicks = []queuedClick{{x: qx, y: qy, at: time.Now().UnixMilli()}}
			if got := g.consumeEntryMenuRootReleaseAt(center(t, tc.release)); got != tc.wantHandled {
				t.Fatalf("handled = %v, want %v", got, tc.wantHandled)
			}
			tc.check(t, g)
		})
	}
}

func TestEntryMenuRootLayoutFitsShortWindows(t *testing.T) {
	minW, minH := MinimumWindowSize()
	for _, frame := range withInterfaceFrames(t, [][2]int{{minW, minH}, {640, 480}, {800, 600}, {1280, 720}}) {
		size := struct{ w, h int }{frame[0], frame[1]}
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
		bottom := layout.button(len(entryButtons()) - 1).bottom()
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
	bottom := layout.button(len(entryButtons()) - 1).bottom()
	if bottom > h-entryBottomGap {
		t.Fatalf("expanded root menu ends at %d, content limit is %d", bottom, h-entryBottomGap)
	}
	if layout.buttonH < entryButtonMinH {
		t.Fatalf("expanded root menu button height = %d, want at least %d", layout.buttonH, entryButtonMinH)
	}
}
