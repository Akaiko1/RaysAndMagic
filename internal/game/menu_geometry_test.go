package game

import "testing"

// TestMenuPanelSizePerMode locks the per-mode panel dimensions to a single
// source (menuPanelSize), used by both the draw code and the input hit-testing.
func TestMenuPanelSizePerMode(t *testing.T) {
	if w, h := menuPanelSize(MenuMain, 1280, 720); w != mainMenuPanelW || h != mainMenuPanelH {
		t.Errorf("MenuMain panel = %dx%d, want %dx%d", w, h, mainMenuPanelW, mainMenuPanelH)
	}
	for _, mode := range []MainMenuMode{MenuSaveSelect, MenuLoadSelect} {
		if w, h := menuPanelSize(mode, 1280, 720); w != saveMenuPanelW || h != saveMenuPanelH {
			t.Errorf("mode %d panel = %dx%d, want save %dx%d", mode, w, h, saveMenuPanelW, saveMenuPanelH)
		}
	}
	if w, h := menuPanelSize(MenuSettings, 1280, 720); w != settingsMenuPanelW || h != settingsMenuPanelH {
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
	g := &MMGame{
		menuState: menuState{
			mainMenuSelection: settingsIndex,
			audioSliderDrag:   2,
		},
	}
	(&InputHandler{game: g}).activateMainMenuSelection()
	if g.mainMenuMode != MenuSettings || g.audioSliderDrag != -1 {
		t.Fatalf("Settings action produced mode %d and drag %d", g.mainMenuMode, g.audioSliderDrag)
	}
}

func TestAudioSettingsGeometryFollowsPanelWidth(t *testing.T) {
	const px, py = 100, 50
	for _, panelW := range []int{400, settingsMenuPanelW, 560} {
		r := audioSliderRect(px, py, panelW, 0)
		if r.x2 != px+panelW-audioSliderRightPad {
			t.Errorf("panel width %d: slider right = %d, want %d", panelW, r.x2, px+panelW-audioSliderRightPad)
		}
		for _, contentInset := range []int{audioSettingsInset, audioMenuContentInset} {
			columnRight := -1
			for _, label := range []string{"0%", "50%", "100%"} {
				percentX := audioPercentX(px, panelW, contentInset, label)
				if percentX < r.x2+audioPercentGap {
					t.Errorf("panel width %d inset %d: %q starts at %d before percentage column %d", panelW, contentInset, label, percentX, r.x2+audioPercentGap)
				}
				right := percentX + uiTextWidth(label)
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
	selection := audioSelectionRect(px, py, settingsMenuPanelW, audioSettingsInset, 0)
	if selection.x1 < px+audioSettingsInset || selection.x2 > px+settingsMenuPanelW-audioSettingsInset {
		t.Fatalf("selection bounds [%d,%d] enter ornate frame content band [%d,%d]", selection.x1, selection.x2, px+audioSettingsInset, px+settingsMenuPanelW-audioSettingsInset)
	}
	back := audioBackRect(px, py, settingsMenuPanelH, audioSettingsInset)
	if back.x2-back.x1 != menuBackButtonW || back.y2-back.y1 != menuBackButtonH {
		t.Fatalf("back rect = %dx%d, want shared button size %dx%d", back.x2-back.x1, back.y2-back.y1, menuBackButtonW, menuBackButtonH)
	}
	_, _, hintY := audioHintPosition(px, py, settingsMenuPanelW, settingsMenuPanelH, audioSettingsInset)
	contentBottom := py + settingsMenuPanelH - audioSettingsInset
	if hintY < py+audioSettingsInset || hintY+uiTextCharHeight > contentBottom {
		t.Fatalf("hint y-range [%d,%d] leaves ornate content range [%d,%d]", hintY, hintY+uiTextCharHeight, py+audioSettingsInset, contentBottom)
	}
}

func TestAudioSettingsPanelLayoutVariants(t *testing.T) {
	const screenW, screenH = 1000, 800
	for _, test := range []struct {
		name      string
		ornate    bool
		wantInset int
	}{
		{name: "entry ornate", ornate: true, wantInset: audioSettingsInset},
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

// TestMenuRowRectContract pins the shared row geometry: rows step by exactly
// `pitch`, keep the constant height, share x-bounds inset symmetrically inside
// the panel, and the text baseline sits inside the box. This is the single source the draw highlight, hover tooltip,
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
		if left, right := box.x1-px, px+panelW-box.x2; left <= 0 || left != right {
			t.Errorf("row %d x-bounds = [%d,%d], want a symmetric inset inside panel [%d,%d]", i, box.x1, box.x2, px, px+panelW)
		}
		if ty < box.y1 || ty > box.y2 || tx < box.x1 {
			t.Errorf("row %d text baseline (%d,%d) outside box %+v", i, tx, ty, box)
		}
		if i > 0 {
			if got := box.y1 - prev.y1; got != pitch {
				t.Errorf("row %d step = %d, want pitch %d", i, got, pitch)
			}
			if box.x1 != prev.x1 || box.x2 != prev.x2 {
				t.Errorf("row %d x-bounds [%d,%d] differ from row %d [%d,%d]", i, box.x1, box.x2, i-1, prev.x1, prev.x2)
			}
		}
		prev = box
	}
}
