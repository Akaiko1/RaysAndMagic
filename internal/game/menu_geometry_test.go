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
