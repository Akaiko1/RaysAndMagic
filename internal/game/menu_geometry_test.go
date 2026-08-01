package game

import "testing"

// TestMenuPanelSizePerMode locks the per-mode panel dimensions to a single
// source (menuPanelSize), used by both the draw code and the input hit-testing.
func TestMenuPanelSizePerMode(t *testing.T) {
	if w, h := menuPanelSize(MenuMain); w != mainMenuPanelW || h != mainMenuPanelH {
		t.Errorf("MenuMain panel = %dx%d, want %dx%d", w, h, mainMenuPanelW, mainMenuPanelH)
	}
	for _, mode := range []MainMenuMode{MenuSaveSelect, MenuLoadSelect} {
		if w, h := menuPanelSize(mode); w != saveMenuPanelW || h != saveMenuPanelH {
			t.Errorf("mode %d panel = %dx%d, want save %dx%d", mode, w, h, saveMenuPanelW, saveMenuPanelH)
		}
	}
	if w, h := menuPanelSize(MenuSettings); w != settingsMenuPanelW || h != settingsMenuPanelH {
		t.Errorf("MenuSettings panel = %dx%d, want %dx%d", w, h, settingsMenuPanelW, settingsMenuPanelH)
	}
}

func TestMainMenuOptionsOwnTheirActions(t *testing.T) {
	seenKeys := make(map[string]bool, len(mainMenuOptions))
	seenLabels := make(map[string]bool, len(mainMenuOptions))
	settingsIndex := -1
	for i, option := range mainMenuOptions {
		if option.key == "" || option.label == "" || option.action == nil {
			t.Fatalf("option %d is incomplete: %+v", i, option)
		}
		if seenKeys[option.key] || seenLabels[option.label] {
			t.Fatalf("option %d duplicates key %q or label %q", i, option.key, option.label)
		}
		seenKeys[option.key] = true
		seenLabels[option.label] = true
		if option.key == "settings" {
			settingsIndex = i
		}
	}
	if settingsIndex < 0 {
		t.Fatal("Settings option is missing")
	}
	g := &MMGame{mainMenuSelection: settingsIndex, audioSliderDrag: 2}
	(&InputHandler{game: g}).activateMainMenuSelection()
	if g.mainMenuMode != MenuSettings || g.audioSliderDrag != -1 {
		t.Fatalf("Settings action produced mode %d and drag %d", g.mainMenuMode, g.audioSliderDrag)
	}
}

func TestMainMenuControlTipsFitPanel(t *testing.T) {
	bottom := mainMenuTipsTopY() + len(mainMenuControlTips)*debugTextCharHeight
	if bottom > mainMenuPanelH-2 {
		t.Fatalf("control tips end at y=%d, panel content ends at y=%d", bottom, mainMenuPanelH-2)
	}
	for _, tip := range mainMenuControlTips {
		if width := debugTextWidth(tip); width > mainMenuPanelW-32 {
			t.Errorf("control tip width = %d, content width = %d: %q", width, mainMenuPanelW-32, tip)
		}
	}
}

func TestAudioSettingsGeometryFollowsPanelWidth(t *testing.T) {
	const px, py = 100, 50
	for _, panelW := range []int{400, settingsMenuPanelW, 560} {
		r := audioSliderRect(px, py, panelW, 0)
		if r.x2 != px+panelW-audioSliderRightPad {
			t.Errorf("panel width %d: slider right = %d, want %d", panelW, r.x2, px+panelW-audioSliderRightPad)
		}
		for _, contentInset := range []int{menuFrameInset, audioMenuContentInset} {
			columnRight := -1
			for _, label := range []string{"0%", "50%", "100%"} {
				percentX := audioPercentX(px, panelW, contentInset, label)
				if percentX < r.x2+audioPercentGap {
					t.Errorf("panel width %d inset %d: %q starts at %d before percentage column %d", panelW, contentInset, label, percentX, r.x2+audioPercentGap)
				}
				right := percentX + debugTextWidth(label)
				if right > px+panelW-contentInset {
					t.Errorf("panel width %d inset %d: %q ends at %d past content edge %d", panelW, contentInset, label, right, px+panelW-contentInset)
				}
				if columnRight >= 0 && right != columnRight {
					t.Errorf("panel width %d inset %d: %q right edge = %d, want %d", panelW, contentInset, label, right, columnRight)
				}
				columnRight = right
			}
		}
	}
}

func TestAudioSettingsOrnateContentClearsFrame(t *testing.T) {
	const px, py = 100, 50
	selection := audioSelectionRect(px, py, settingsMenuPanelW, menuFrameInset, 0)
	if selection.x1 < px+menuFrameInset || selection.x2 > px+settingsMenuPanelW-menuFrameInset {
		t.Fatalf("selection bounds [%d,%d] enter ornate frame content band [%d,%d]", selection.x1, selection.x2, px+menuFrameInset, px+settingsMenuPanelW-menuFrameInset)
	}
	back := audioBackRect(px, py, settingsMenuPanelH, menuFrameInset)
	if back.x2-back.x1 != menuBackButtonW || back.y2-back.y1 != menuBackButtonH {
		t.Fatalf("back rect = %dx%d, want shared button size %dx%d", back.x2-back.x1, back.y2-back.y1, menuBackButtonW, menuBackButtonH)
	}
	_, _, hintY := audioHintPosition(px, py, settingsMenuPanelW, settingsMenuPanelH, menuFrameInset)
	contentBottom := py + settingsMenuPanelH - menuFrameInset
	if hintY < py+menuFrameInset || hintY+debugTextCharHeight > contentBottom {
		t.Fatalf("hint y-range [%d,%d] leaves ornate content range [%d,%d]", hintY, hintY+debugTextCharHeight, py+menuFrameInset, contentBottom)
	}
}

func TestAudioSettingsPanelLayoutVariants(t *testing.T) {
	const screenW, screenH = 1000, 800
	for _, test := range []struct {
		name      string
		ornate    bool
		wantInset int
	}{
		{name: "entry ornate", ornate: true, wantInset: menuFrameInset},
		{name: "ESC plain", ornate: false, wantInset: audioMenuContentInset},
	} {
		t.Run(test.name, func(t *testing.T) {
			layout := makeAudioSettingsPanelLayout(screenW, screenH, test.ornate)
			if layout.px != (screenW-settingsMenuPanelW)/2 || layout.py != (screenH-settingsMenuPanelH)/2 {
				t.Fatalf("panel origin = (%d,%d), want centered", layout.px, layout.py)
			}
			if layout.panelW != settingsMenuPanelW || layout.panelH != settingsMenuPanelH {
				t.Fatalf("panel size = %dx%d, want %dx%d", layout.panelW, layout.panelH, settingsMenuPanelW, settingsMenuPanelH)
			}
			if layout.contentInset != test.wantInset {
				t.Fatalf("content inset = %d, want %d", layout.contentInset, test.wantInset)
			}
		})
	}
}

func TestMinimumWindowContainsFixedMenuPanels(t *testing.T) {
	w, h := MinimumWindowSize()
	for name, panelW := range map[string]int{
		"entry load":  entryLoadPanelW,
		"main menu":   mainMenuPanelW,
		"settings":    settingsMenuPanelW,
		"tabbed menu": tabbedMenuPanelW,
		"tavern":      tavernDialogWidth,
	} {
		if panelW+2*entryWindowSideGap > w {
			t.Errorf("%s width %d plus side gaps exceeds minimum window width %d", name, panelW, w)
		}
	}
	for name, panelH := range map[string]int{
		"entry load":  entryLoadPanelH,
		"main menu":   mainMenuPanelH,
		"settings":    settingsMenuPanelH,
		"tabbed menu": tabbedMenuPanelH,
		"tavern":      tavernDialogHeight,
	} {
		if panelH+2*entryWindowSideGap > h {
			t.Errorf("%s height %d plus side gaps exceeds minimum window height %d", name, panelH, h)
		}
	}
}

// TestMenuRowRectContract pins the shared row geometry: rows step by exactly
// `pitch`, keep the constant height, share x-bounds, and the text baseline sits
// inside the box. This is the single source the draw highlight, hover tooltip,
// hover-select and right-click rename all consume, so a drift like the old
// hard-coded pitch in hover-select can't return.
func TestMenuRowRectContract(t *testing.T) {
	const px, py, panelW, startY, pitch = 100, 50, saveMenuPanelW, saveMenuListTopY, saveMenuRowPitch

	var prev pagerRect
	for i := 0; i < saveRowsPerPage; i++ {
		box, tx, ty := menuRowRect(px, py, panelW, startY, pitch, i)
		if got := box.y2 - box.y1; got != menuRowHeight {
			t.Errorf("row %d height = %d, want %d", i, got, menuRowHeight)
		}
		if box.x1 != px+16 || box.x2 != px+panelW-16 {
			t.Errorf("row %d x-bounds = [%d,%d], want [%d,%d]", i, box.x1, box.x2, px+16, px+panelW-16)
		}
		if ty < box.y1 || ty > box.y2 || tx < box.x1 {
			t.Errorf("row %d text baseline (%d,%d) outside box %+v", i, tx, ty, box)
		}
		if i > 0 {
			if got := box.y1 - prev.y1; got != pitch {
				t.Errorf("row %d step = %d, want pitch %d", i, got, pitch)
			}
		}
		prev = box
	}
}
