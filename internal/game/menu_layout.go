package game

import "fmt"

// Menu layout boxes. Each collision-prone menu exposes its sections (headings,
// item grids, buttons, pagers) as labelled rectangles derived from the SAME
// constants the renderer uses. menu_layout_test.go iterates these to guarantee
// no section overlaps another or spills outside its container — a regression
// guard against the text/grid/button collisions that creep in when a panel is
// resized or a row added. To cover a new menu, add a builder and list it in the
// test's table.

// uiBox is a labelled rectangle in screen space.
type uiBox struct {
	Name       string
	X, Y, W, H int
}

func (b uiBox) right() int  { return b.X + b.W }
func (b uiBox) bottom() int { return b.Y + b.H }

// overlaps reports whether two boxes share interior area (shared edges do not count).
func (b uiBox) overlaps(o uiBox) bool {
	return b.X < o.right() && o.X < b.right() && b.Y < o.bottom() && o.Y < b.bottom()
}

// contains reports whether b fully encloses o.
func (b uiBox) contains(o uiBox) bool {
	return o.X >= b.X && o.Y >= b.Y && o.right() <= b.right() && o.bottom() <= b.bottom()
}

// textLineBox is the bounding box of a left-aligned drawDebugText(text, x, y).
func textLineBox(name, text string, x, y int) uiBox {
	return uiBox{name, x, y, debugTextWidth(text), debugTextCharHeight}
}

// centeredTextBox is the bounding box of drawCenteredDebugText(text, x, y, w, h).
func centeredTextBox(name, text string, x, y, w, h int) uiBox {
	tw := debugTextWidth(text)
	return uiBox{name, x + (w-tw)/2, y + (h-debugTextCharHeight)/2, tw, debugTextCharHeight}
}

// stashLayoutBoxes returns the stash modal's region and its section boxes.
func stashLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	L := computeStashLayout(screenW, screenH)
	region := uiBox{"stash-popup", L.popupX, L.popupY, stashPopupW, stashPopupH}
	gridX := L.centerX - L.gridW/2
	chestH := L.chestRows*stashCellSize + (L.chestRows-1)*stashCellGap
	bagH := L.bagRows*stashCellSize + (L.bagRows-1)*stashCellGap
	boxes := []uiBox{
		textLineBox("title", "Tavern Stash", L.popupX+16, L.popupY+14),
		textLineBox("subtitle", stashSubtitle, L.popupX+16, L.popupY+34),
		centeredTextBox("chest-heading", "Chest", L.popupX, L.chestTop-16, stashPopupW, 14),
		{"chest-cells", gridX, L.chestTop, L.gridW, chestH},
		centeredTextBox("bag-heading", "Your Bag", L.popupX, L.invTop-16, stashPopupW, 14),
		{"bag-cells", gridX, L.invTop, L.gridW, bagH},
		{"pager", gridX, L.pagerY, L.gridW, 18},
	}
	return region, boxes
}

// saveMenuLayoutBoxes returns the in-game save/load menu's region (panel + the
// pager strip below it) and its section boxes for the given page.
func saveMenuLayoutBoxes(screenW, screenH, page int, modeLoad bool) (uiBox, []uiBox) {
	px := (screenW - saveMenuPanelW) / 2
	py := (screenH - saveMenuPanelH) / 2
	prev, next := savePagerButtonRects(px, py, saveMenuPanelW, saveMenuPanelH)
	region := uiBox{"save-menu", px, py, saveMenuPanelW, next.y2 - py + 4}

	title, help := "Save Game - Select Slot", "Enter: Save  R: Rename  Left/Right: Page"
	if modeLoad {
		title, help = "Load Game - Select Slot", "Enter: Load  Left/Right: Page"
	}
	boxes := []uiBox{
		textLineBox("title", title, px+16, py+14),
		textLineBox("help", help, px+16, py+32),
	}
	for i := 0; i < saveRowsPerPage; i++ {
		row := page*saveRowsPerPage + i
		y := py + saveMenuListTopY + i*saveMenuRowPitch
		boxes = append(boxes, textLineBox(fmt.Sprintf("row-%d", row), saveRowLabel(row), px+28, y))
	}
	boxes = append(boxes,
		uiBox{"pager-prev", prev.x1, prev.y1, prev.x2 - prev.x1, prev.y2 - prev.y1},
		uiBox{"pager-next", next.x1, next.y1, next.x2 - next.x1, next.y2 - next.y1},
	)
	return region, boxes
}

// entryLoadLayoutBoxes returns the title-screen Load list's panel and section
// boxes (rows, Prev/Next buttons, Back) for the given page.
func entryLoadLayoutBoxes(screenW, screenH, page int) (uiBox, []uiBox) {
	px := (screenW - entryLoadPanelW) / 2
	py := (screenH - entryLoadPanelH) / 2
	region := uiBox{"entry-load-panel", px, py, entryLoadPanelW, entryLoadPanelH}
	rowX := px + menuFrameInset
	rowW := entryLoadPanelW - 2*menuFrameInset
	startY := py + menuFrameInset + 22

	boxes := []uiBox{textLineBox("title", "Load Game", px+menuFrameInset, py+menuFrameInset-4)}
	for i := 0; i < saveRowsPerPage; i++ {
		row := page*saveRowsPerPage + i
		y := startY + i*entryLoadRowH
		boxes = append(boxes, uiBox{fmt.Sprintf("row-%d", row), rowX, y, rowW, entryLoadRowH - 8})
	}
	pagerY := startY + saveRowsPerPage*entryLoadRowH + 6
	const pbW, pbH = 96, 26
	boxes = append(boxes,
		uiBox{"pager-prev", rowX, pagerY, pbW, pbH},
		uiBox{"pager-next", rowX + rowW - pbW, pagerY, pbW, pbH},
		uiBox{"back", px + menuFrameInset, pagerY + pbH + 12, 110, 30}, // drawBackButton size
	)
	return region, boxes
}
