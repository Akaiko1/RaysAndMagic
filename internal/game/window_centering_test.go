package game

import (
	"fmt"
	"image"
	"testing"

	"ugataima/internal/stats"
)

// Every centred window sits in the middle of its area and wholly inside it on
// every interface frame - each interface size and screen - through the same
// rect functions the draw and input code use. Windows recompute these every
// frame, so a size change recentres them at once. The tabbed menu is centred
// in the gameplay viewport above the party HUD; the rest in the whole frame.
func TestWindowsCenteredOnEveryInterfaceFrame(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	levelUp := &levelUpChoiceRequest{options: make([]levelUpChoiceOption, 12)}
	for _, f := range interfaceFrameSizes(t) {
		w, hh := f[0], f[1]
		t.Run(fmt.Sprintf("%dx%d", w, hh), func(t *testing.T) {
			g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = w, hh
			frame := layoutRect{0, 0, w, hh}
			viewport := layoutRect{0, 0, w, gameplayViewportBottomWithPartyHUD(hh)}
			fromRect := func(r image.Rectangle) layoutRect { return layoutRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()} }
			settingsTitle := makeAudioSettingsPanelLayout(w, hh, true)
			settingsEsc := makeAudioSettingsPanelLayout(w, hh, false)
			stash, _ := computeStashLayout(w, hh)
			levelPanel := makeLevelUpChoicePanel(levelUp, w, hh).panel
			npc := npcDialogLayout(g)
			cx, cy, cw, ch := combatLogPanelLayout(g)
			windows := []struct {
				name    string
				r, area layoutRect
			}{
				{"settings (title)", layoutRect{settingsTitle.px, settingsTitle.py, settingsTitle.panelW, settingsTitle.panelH}, frame},
				{"settings (esc)", layoutRect{settingsEsc.px, settingsEsc.py, settingsEsc.panelW, settingsEsc.panelH}, frame},
				{"entry load", entryLoadPanelRect(w, hh), frame},
				{"save rename", saveRenameDialogRect(w, hh), frame},
				{"stat popup", statPopupRect(w, hh), frame},
				{"member picker 360", memberPickerRect(w, hh, 360, 4, 28), frame},
				{"member picker 380", memberPickerRect(w, hh, 380, 4, 28), frame},
				{"roster", rosterScreenRect(w, hh), frame},
				{"combat log", layoutRect{cx, cy, cw, ch}, frame},
				{"camp", layoutCampConfirmation(w, hh).panel, frame},
				{"profile", profilePanelRect(w, hh), frame},
				{"stash", stash, frame},
				{"stack split", fromRect(stackSplitPickerRect(w, hh)), frame},
				{"level-up choice", levelPanel, frame},
				{"npc dialog", layoutRect{npc.x, npc.y, npc.w, npc.h}, frame},
				{"map overlay", computeMapOverlayLayout(w, viewport.h).panel, viewport},
				{"tabbed menu", computeTabbedMenuLayout(w, viewport.h).panel, viewport},
			}
			for mode := MenuMain; mode <= MenuControlTips; mode++ {
				windows = append(windows, struct {
					name    string
					r, area layoutRect
				}{fmt.Sprintf("esc menu mode %d", mode), mainMenuPanelRect(w, hh, mode), frame})
			}
			for _, win := range windows {
				r, a := win.r, win.area
				if r.w <= 0 || r.h <= 0 || r.x < a.x || r.y < a.y || r.right() > a.right() || r.bottom() > a.bottom() {
					t.Errorf("%s %+v is not inside %+v", win.name, r, a)
					continue
				}
				if left, right := r.x-a.x, a.right()-r.right(); abs(left-right) > 1 {
					t.Errorf("%s is off-centre horizontally: %d left, %d right", win.name, left, right)
				}
				if top, bottom := r.y-a.y, a.bottom()-r.bottom(); abs(top-bottom) > 1 {
					t.Errorf("%s is off-centre vertically: %d above, %d below", win.name, top, bottom)
				}
			}
			entry := makeEntryMenuRootLayout(w, hh)
			for _, span := range [][2]int{{entry.logoX, entry.logoW}, {entry.buttonX, entry.buttonW}} {
				if left, right := span[0], w-span[0]-span[1]; span[0] < 0 || abs(left-right) > 1 {
					t.Errorf("title screen column at x=%d w=%d is off-centre (%d | %d)", span[0], span[1], left, right)
				}
			}
			// The logo and the framed buttons are one block, centred vertically
			// by what is drawn: logo top to the bottom of the frame decor.
			n := len(entryButtons())
			blockTop := min(entry.logoY, entry.buttonStartY-entryButtonDecorPad)
			blockBottom := entry.buttonStartY + n*entry.buttonH + (n-1)*entry.buttonGap + entryButtonDecorPad
			if top, below := blockTop, hh-blockBottom; top < 0 || below < 0 || abs(top-below) > 1 {
				t.Errorf("title screen block is off-centre vertically: %d above, %d below", top, below)
			}
			if entry.logoY+entry.logoH > entry.buttonStartY {
				t.Error("title screen buttons overlap the logo")
			}
		})
	}
}

// The stat and level-up popups are fixed-size, so their text has to fit in
// every font: the points line clears Auto, stat names clear their values,
// the AUTO preview clears the "+", the widest mastery detail leaves room for
// a name, and the picker's count and hints clear the Confirm button and panel.
func TestStatAndLevelUpPopupsFitEveryFont(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		popup := statPopupRect(1280, 720)
		auto := statPopupAutoRect(popup)
		if x := popup.x + 16 + uiTextWidth("120")*2 + 10 + uiTextWidth("points to spend"); x > auto.x-4 {
			t.Errorf("points line ends at %d, Auto starts at %d", x, auto.x)
		}
		for i, key := range stats.Names {
			row, plus := statPopupRowRect(popup, i), statPlusButtonRect(popup, i)
			if end := row.x + 8 + uiTextWidth(stats.Title(key)); end > row.x+statPopupValueRight-uiTextWidth("99")-4 {
				t.Errorf("%s runs into its value", key)
			}
			if end := row.x + statPopupValueRight + 8 + uiTextWidth("+99"); end > plus.x-4 {
				t.Errorf("%s preview runs into its +", key)
			}
		}
		if last := statPopupRowRect(popup, len(stats.Names)-1); last.bottom() > popup.bottom()-8 {
			t.Error("the last stat row sits on the popup rim")
		}

		req := &levelUpChoiceRequest{maxSelections: 2, options: make([]levelUpChoiceOption, 8)}
		p := makeLevelUpChoicePanel(req, 1280, 720)
		row := p.target(0)
		detail := uiTextWidth("Master -> Grandmaster")
		if room := (row.right() - 10 - 22 - detail - 12) - (row.x + 6 + 24 + 10); room < uiTextWidth("Spirit Magic") {
			t.Errorf("a mastery row leaves %dpx for its name", room)
		}
		if end := p.panel.x + 16 + uiTextWidth("2 of 2 chosen"); end > p.confirm.x-8 {
			t.Error("the chosen count runs into Confirm")
		}
		for _, hint := range []string{"Up/Down, Space toggles, Enter confirms", "Up/Down and Enter, or click"} {
			if uiTextWidth(hint) > p.panel.w-32 {
				t.Errorf("hint %q leaves the panel", hint)
			}
		}
		if p.footerY+uiTextCharHeight > p.panel.bottom()-6 || p.confirm.bottom() > p.footerY {
			t.Error("the footer hint collides with Confirm or the rim")
		}
	})
}
