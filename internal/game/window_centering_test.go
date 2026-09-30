package game

import (
	"fmt"
	"image"
	"testing"
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
			stash := computeStashLayout(w, hh)
			lx, ly, lw, lh, _, _ := levelUpChoiceLayout(levelUp, w, hh)
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
				{"stash", layoutRect{stash.popupX, stash.popupY, stash.popupW, stash.popupH}, frame},
				{"stack split", fromRect(stackSplitPickerRect(w, hh)), frame},
				{"level-up choice", layoutRect{lx, ly, lw, lh}, frame},
				{"npc dialog", layoutRect{npc.x, npc.y, npc.w, npc.h}, frame},
				{"map overlay", computeMapOverlayLayout(w, hh).panel, frame},
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
