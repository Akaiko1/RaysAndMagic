package game

import (
	"fmt"
	"testing"

	"ugataima/internal/items"
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
				{"stack split", layoutRectOf(stackSplitPickerRect(w, hh)), frame},
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

// The save list and the Town Portal chooser hold their text in every font:
// two lines per row, the doubled heading, an empty load-only slot with its
// tag, the portal's hint; and a portal list too long for the screen scrolls
// inside it, its last row reachable and its scrollbar inside the panel.
func TestSaveListAndTownPortalFitEveryFont(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		if two := 2*uiTextCharHeight + 2; two > saveListRowH || two > townPortalRowH {
			t.Errorf("two lines need %dpx; save rows are %d, portal rows %d", two, saveListRowH, townPortalRowH)
		}
		if 2*uiTextCharHeight > saveListTitleH {
			t.Errorf("the doubled heading is %dpx in a %dpx band", 2*uiTextCharHeight, saveListTitleH)
		}
		for _, l := range []saveListLayout{escSaveListLayout(1280, 720), entrySaveListLayout(1280, 720)} {
			row := l.rows[0]
			used := 10 + uiTextWidth(saveRowLabel(quicksaveRow)) + 14 + uiTextWidth("Not written yet") + 12 + uiTextWidth("Load only") + 10
			if used > row.w {
				t.Errorf("an empty load-only row needs %dpx of %d", used, row.w)
			}
		}
		short := makeTownPortalLayout(5, 1280, 720)
		if short.scrolls() || short.row(4, 0).bottom() > short.list.bottom() {
			t.Error("five destinations scroll on Large")
		}
		for _, hint := range []string{townPortalHint, townPortalScrollHint} {
			if uiTextWidth(hint) > short.panel.w-32 {
				t.Errorf("the portal hint %q leaves the panel", hint)
			}
		}
		for _, n := range []int{12, 40} {
			long := makeTownPortalLayout(n, 800, 600)
			end := long.scroll(1 << 20)
			last := long.row(n-1, end)
			if !long.scrolls() || long.panel.y < 0 || long.panel.bottom() > 600 || last.bottom() != long.list.bottom() || long.track().right()+scrollbarGrabRight > long.panel.right() {
				t.Errorf("%d destinations at 800x600: panel %+v, list %+v, last row at the end %+v", n, long.panel, long.list, last)
			}
		}
	})
}

// The High Scores screen holds a full board in every font, on Large and on
// the smallest frame, in game and on the title (Back in its footer): ten
// two-line runs, and a run's detail with its score parts and four heroes
// whose names keep room beside their gear.
func TestHighScoresFitEveryFont(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		for _, tc := range []struct {
			size     [2]int
			withBack bool
		}{{[2]int{1280, 720}, false}, {[2]int{800, 600}, false}, {[2]int{1280, 720}, true}, {[2]int{800, 600}, true}} {
			size := tc.size
			l := makeHighScoresLayout(size[0], size[1], tc.withBack)
			if last := l.row(9); last.bottom() > l.list.bottom() {
				t.Errorf("%v: the tenth run ends at %d, the list at %d", size, last.bottom(), l.list.bottom())
			}
			if 2*uiTextCharHeight+2 > highScoresRowH {
				t.Errorf("two lines need %dpx of a %dpx row", 2*uiTextCharHeight+2, highScoresRowH)
			}
			detail := 2*uiTextCharHeight + 16 + 8 + 8 + 8 + 4*(uiTextCharHeight+4) + 6 + 8 + 4*highScoresHeroH
			if detail > l.detail.h {
				t.Errorf("%v: the detail needs %dpx of %d", size, detail, l.detail.h)
			}
			if rows := 2; highScoreGearCols*rows < len(items.DisplayEquipSlots) || rows*(highScoresGearSz+2) > highScoresHeroH {
				t.Errorf("%d gear cells in two rows cannot hold %d worn slots in a %dpx hero row", highScoreGearCols*rows, len(items.DisplayEquipSlots), highScoresHeroH)
			}
			nameRoom := l.detail.w - highScoreGearCols*(highScoresGearSz+2) - 8 - 50
			if nameRoom < uiTextWidth("Battle Mage 45") {
				t.Errorf("%v: a hero's class line gets %dpx", size, nameRoom)
			}
			if !tc.withBack {
				if uiTextWidth(highScoresHint+highScoresEscTip) > l.title.w {
					t.Errorf("%v: the hint leaves the panel", size)
				}
				continue
			}
			if l.back.x < l.panel.x || l.back.bottom() > l.panel.bottom() || l.back.y < l.list.bottom() || l.back.y < l.detail.bottom() {
				t.Errorf("%v: Back %+v leaves the footer of panel %+v (list ends %d)", size, l.back, l.panel, l.list.bottom())
			}
			if uiTextWidth(highScoresHint) > l.title.right()-l.back.right()-16 {
				t.Errorf("%v: the hint runs into Back", size)
			}
		}
	})
}
