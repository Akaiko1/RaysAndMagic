package game

import (
	"strings"
	"testing"
)

// On every interface frame and in every font the tips stay inside their
// columns, never overlap, clear the Back button, and scroll just far enough
// to show the last row.
func TestControlTipsLayoutFitsEveryFrame(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		for _, size := range withInterfaceFrames(t, [][2]int{{800, 600}, {1280, 720}, {1920, 1080}}) {
			panel := mainMenuPanelRect(size[0], size[1], MenuControlTips)
			if panel.x < 0 || panel.y < 0 || panel.right() > size[0] || panel.bottom() > size[1] {
				t.Errorf("%v: panel %+v leaves the screen", size, panel)
				continue
			}
			l := computeControlTipsLayout(panel)
			if backY := panel.bottom() - 46; l.body.bottom() > backY {
				t.Errorf("%v: tips run under the Back button", size)
			}
			if l.track.right()+scrollbarGrabRight > panel.right() {
				t.Errorf("%v: scrollbar leaves the panel", size)
			}
			for i, r := range l.rows {
				for _, line := range r.effect {
					if r.x+l.keyW+controlTipsKeyGap+uiTextWidth(line) > l.body.right() {
						t.Errorf("%v: %q runs past the list", size, line)
					}
				}
				for _, q := range l.rows[:i] {
					if q.x == r.x && r.y < q.y+q.h && q.y < r.y+r.h {
						t.Errorf("%v: rows %q and %q overlap", size, q.keys, r.keys)
					}
				}
				if r.y+r.h > l.contentH {
					t.Errorf("%v: %q lies below the scroll range", size, r.keys)
				}
			}
			if got := l.maxScroll(); got != max(0, l.contentH-l.body.h) {
				t.Errorf("%v: max scroll %d", size, got)
			}
		}
	})
}

// The page lists the combos and keys players asked about, each once.
func TestControlTipsCoverTheBindings(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range controlTipGroups {
		for _, tip := range g.tips {
			key := g.title + "/" + tip.keys
			if seen[key] {
				t.Errorf("duplicate tip %s", key)
			}
			seen[key] = true
			for _, s := range []string{tip.keys, tip.effect} {
				for _, r := range s {
					if r > 126 {
						t.Errorf("non-ASCII in tip %q", s)
					}
				}
			}
		}
	}
	for _, want := range []string{"Point at card + F", "Point at card + C", "F5", "Shift + F4", "V / Shift + V", "Shift + right-click card", "I / P / M / J / K"} {
		found := false
		for key := range seen {
			found = found || strings.HasSuffix(key, "/"+want)
		}
		if !found {
			t.Errorf("no tip for %q", want)
		}
	}
}

// The wheel scrolls the page on a small screen, clamped at both ends, and
// reopening the page starts at the top.
func TestControlTipsScrollByWheel(t *testing.T) {
	h := newDisplayedModalHarness(t, 800, 680)
	g := h.g
	g.openMainMenu()
	for i, o := range mainMenuOptions {
		if o.key == "control_tips" {
			g.mainMenuSelection = i
		}
	}
	(&InputHandler{game: g}).activateMainMenuSelection()
	panel := mainMenuPanelRect(800, 680, MenuControlTips)
	l := computeControlTipsLayout(panel)
	if l.maxScroll() == 0 {
		t.Fatal("fixture: the tips fit an 800x680 page without scrolling")
	}
	prevWheel, prevPos := pointerWheel, pointerPosition
	t.Cleanup(func() { pointerWheel, pointerPosition = prevWheel, prevPos })
	pointerPosition = func() (int, int) { return l.body.x + 20, l.body.y + 20 }
	wheel := 0.0
	pointerWheel = func() (float64, float64) { return 0, wheel }
	step := func(w float64) {
		wheel = w
		h.ui.Draw(h.screen)
		if err := h.loop.Update(); err != nil {
			t.Fatal(err)
		}
	}
	step(-1)
	if g.controlTipsScroll <= 0 {
		t.Fatal("the wheel did not scroll the tips")
	}
	for range 200 {
		step(-1)
	}
	if g.controlTipsScroll != l.maxScroll() {
		t.Fatalf("scroll %d, want the bottom %d", g.controlTipsScroll, l.maxScroll())
	}
	for range 200 {
		step(1)
	}
	if g.controlTipsScroll != 0 {
		t.Fatalf("scroll %d, want the top", g.controlTipsScroll)
	}
	step(-1)
	g.mainMenuMode = MenuMain
	(&InputHandler{game: g}).activateMainMenuSelection()
	if g.controlTipsScroll != 0 {
		t.Fatal("reopened tips kept the old scroll")
	}
}
