package game

import "fmt"

// Menu layout boxes. Each collision-prone menu exposes its sections (headings,
// item grids, buttons, pagers) as labelled rectangles derived from the SAME
// constants the renderer uses. menu_layout_test.go iterates these to guarantee
// no section overlaps another or spills outside its container - a regression
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

func namedLayoutBox(name string, r layoutRect) uiBox {
	return uiBox{name, r.x, r.y, r.w, r.h}
}

func mainMenuLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	px := (screenW - mainMenuPanelW) / 2
	py := (screenH - mainMenuPanelH) / 2
	region := uiBox{"main-menu", px, py, mainMenuPanelW, mainMenuPanelH}
	boxes := []uiBox{textLineBox("title", "Main Menu", px+16, py+14)}
	for i, label := range mainMenuOptions {
		row, _, textY := menuRowRect(px, py, mainMenuPanelW, mainMenuListTopY, mainMenuRowPitch, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("option-%d-%s", i, label), row.x1, row.y1, row.x2 - row.x1, row.y2 - row.y1})
		_ = textY
	}
	for i, tip := range mainMenuControlTips {
		boxes = append(boxes, textLineBox(fmt.Sprintf("tip-%d", i), tip, px+16, py+mainMenuTipsTopY()+i*debugTextCharHeight))
	}
	return region, boxes
}

func tabbedMenuLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	l := computeTabbedMenuLayout(screenW, screenH)
	region := namedLayoutBox("tabbed-menu", l.panel)
	boxes := make([]uiBox, 0, len(l.tabs)+2)
	for i, tab := range l.tabs {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("tab-%d", i), tab))
	}
	boxes = append(boxes, namedLayoutBox("close", l.close), namedLayoutBox("content", l.content))
	return region, boxes
}

func inventoryLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, screenH)
	l := computeInventoryContentLayout(menu.content)
	quickBlock := layoutRect{l.quickLabel.x, l.quickLabel.y, l.quickSlots.w, l.quickSlots.bottom() - l.quickLabel.y}
	return namedLayoutBox("inventory-content", menu.content), []uiBox{
		namedLayoutBox("paperdoll", l.paper),
		namedLayoutBox("inventory-grid", l.grid),
		namedLayoutBox("pager", l.pager),
		namedLayoutBox("camp", l.camp),
		namedLayoutBox("quick-slots", quickBlock),
		namedLayoutBox("instructions-1", l.instructions[0]),
		namedLayoutBox("instructions-2", l.instructions[1]),
	}
}

func cardsLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, screenH)
	l := computeCardsContentLayout(menu.content)
	boxes := []uiBox{namedLayoutBox("title", l.title), namedLayoutBox("subtitle", l.subtitle)}
	for i, card := range l.cards {
		cardAndLabels := layoutRect{card.x - (l.labelW-card.w)/2, card.y, l.labelW, card.h + 2 + 2*debugTextCharHeight}
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("card-%d", i), cardAndLabels))
	}
	boxes = append(boxes, namedLayoutBox("summary", l.summary))
	return namedLayoutBox("cards-content", menu.content), boxes
}

func charactersLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, screenH)
	l := computeCharacterContentLayout(menu.content)
	return namedLayoutBox("characters-content", menu.content), []uiBox{
		namedLayoutBox("title", l.title),
		namedLayoutBox("portrait", l.portraitFrame),
		namedLayoutBox("character-scroll", l.scroll),
		namedLayoutBox("instructions", l.instructions),
		namedLayoutBox("pager", l.pager),
	}
}

func bookLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, screenH)
	l := computeBookLayout(menu.content.x, menu.content.y, menu.content.h)
	quickW := 360
	quickY := l.bookY + l.bookH + 16
	quickH := int(float64(quickW) / quickSlotBarAspect)
	return namedLayoutBox("book-content", menu.content), []uiBox{
		{"book", l.bookX, l.bookY, l.bookW, l.bookH},
		{"quick-slots", l.bookX + (l.bookW-quickW)/2, quickY - 16, quickW, quickH + 16},
		{"controls", l.bookX + 20, menu.content.bottom() - 28, l.bookW - 40, debugTextCharHeight},
	}
}

func questsLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, screenH)
	l := computeQuestContentLayout(menu.content, 100)
	boxes := []uiBox{namedLayoutBox("title", l.title)}
	for i, row := range l.rows {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("quest-%d", i), row))
	}
	boxes = append(boxes, namedLayoutBox("pager", l.pager))
	return namedLayoutBox("quests-content", menu.content), boxes
}

func mapOverlayLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	l := computeMapOverlayLayout(screenW, screenH)
	return namedLayoutBox("map-panel", l.panel), []uiBox{
		namedLayoutBox("title", l.title),
		namedLayoutBox("close", l.close),
		namedLayoutBox("map", l.body),
	}
}

func npcDialogRegion(screenW, screenH int) (layoutRect, npcDialogSectionLayout) {
	dialog := layoutRect{(screenW - npcDialogWidth) / 2, (screenH - npcDialogHeight) / 2, npcDialogWidth, npcDialogHeight}
	return dialog, computeNPCDialogSectionLayout(dialog, true)
}

func spellTraderLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog, l := npcDialogRegion(screenW, screenH)
	boxes := []uiBox{
		namedLayoutBox("title", l.title), namedLayoutBox("balance", l.balance), namedLayoutBox("greeting", l.greeting),
	}
	for i := 0; i < 4; i++ {
		x, y, w, h := spellTraderPortraitRect(dialog.x, dialog.y, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("portrait-%d", i), x - 8, y, w + 16, h + 6 + debugTextCharHeight})
	}
	gridW := spellTraderGridCols*spellTraderIconSize + (spellTraderGridCols-1)*spellTraderIconGap
	gridX := dialog.x + (dialog.w-gridW)/2
	gridY := spellTraderGridTop(dialog.y)
	cellH := spellTraderIconSize + 14
	gridH := spellTraderGridRows*(cellH+8) - 8
	boxes = append(boxes,
		uiBox{"spell-grid", gridX, gridY, gridW, gridH},
		uiBox{"pager", gridX, spellTraderPagerY(dialog.y), gridW, pagerBtnH},
		namedLayoutBox("footer", layoutRect{l.footer[0].x, l.footer[0].y, l.footer[0].w, 2 * debugTextCharHeight}),
	)
	return namedLayoutBox("spell-trader", dialog), boxes
}

func trainerDialogLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog, l := npcDialogRegion(screenW, screenH)
	boxes := []uiBox{
		namedLayoutBox("title", l.title), namedLayoutBox("balance", l.balance), namedLayoutBox("greeting", l.greeting),
	}
	for i := 0; i < 4; i++ {
		x, y, w, h := skillTrainerPortraitRect(dialog.x, dialog.y, dialog.w, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("portrait-%d", i), x - 8, y, w + 16, h + 24 + debugTextCharHeight})
	}
	boxes = append(boxes, namedLayoutBox("footer", l.footer[0]))
	return namedLayoutBox("trainer-dialog", dialog), boxes
}

func merchantDialogLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog, l := npcDialogRegion(screenW, screenH)
	leftX, rightX, gridTop, pagerY := merchantGridLayout(dialog.x, dialog.y)
	gridH := merchantGridRows*(merchantIconSize+merchantPriceH+merchantRowGap) - merchantRowGap
	boxes := []uiBox{
		namedLayoutBox("title", l.title), namedLayoutBox("balance", l.balance), namedLayoutBox("greeting", l.greeting),
		textLineBox("buy-heading", "For Sale", leftX, gridTop-24),
		textLineBox("sell-heading", "Your Items", rightX, gridTop-24),
		{"buy-grid", leftX, gridTop, merchantGridW, gridH},
		{"sell-grid", rightX, gridTop, merchantGridW, gridH},
		{"buy-pager", leftX, pagerY, merchantGridW, pagerBtnH},
		{"sell-pager", rightX, pagerY, merchantGridW, pagerBtnH},
		namedLayoutBox("footer", layoutRect{l.footer[0].x, l.footer[0].y, l.footer[0].w, 2 * debugTextCharHeight}),
	}
	return namedLayoutBox("merchant-dialog", dialog), boxes
}

func cardCollectorLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog := layoutRect{(screenW - npcDialogWidth) / 2, (screenH - npcDialogHeight) / 2, npcDialogWidth, npcDialogHeight}
	l := computeNPCDialogSectionLayout(dialog, false)
	boxes := []uiBox{
		namedLayoutBox("title", l.title), namedLayoutBox("greeting", l.greeting),
		textLineBox("collection-heading", "Collection (active effects)", dialog.x+20, dialog.y+96),
		textLineBox("inventory-heading", "Your cards (double-click to add)", dialog.x+20, dialog.y+176),
	}
	for i := 0; i < MaxCardSlots; i++ {
		x, y, w, h := cardCollectorSlotRect(dialog.x, dialog.y, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("active-card-%d", i), x, y, w, h})
	}
	invGridW := cardInvCols*cardInvSize + (cardInvCols-1)*cardInvGap
	invGridX := dialog.x + (dialog.w-invGridW)/2
	boxes = append(boxes,
		uiBox{"inventory-cards", invGridX, dialog.y + cardInvTop, invGridW, 2*cardInvSize + cardInvRowPitch - cardInvSize},
		uiBox{"pager", invGridX, dialog.y + cardInvTop + 2*cardInvRowPitch - 4, invGridW, pagerBtnH},
		namedLayoutBox("footer", l.footer[0]),
	)
	return namedLayoutBox("card-collector", dialog), boxes
}

// stashLayoutBoxes returns the stash modal's region and its section boxes.
func stashLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	L := computeStashLayout(screenW, screenH)
	region := uiBox{"stash-popup", L.popupX, L.popupY, stashPopupW, stashPopupH}
	gridX := L.centerX - L.gridW/2
	chestH := L.chestRows*stashCellSize + (L.chestRows-1)*stashCellGap
	bagH := L.bagRows*stashCellSize + (L.bagRows-1)*stashCellGap
	toggle := stashToggleRect(L)
	boxes := []uiBox{
		textLineBox("title", "Tavern Stash", L.popupX+16, L.popupY+14),
		textLineBox("subtitle", stashSubtitle, L.popupX+16, L.popupY+34),
		// "Card Vault - cards only" is the wider of the two tab headings; check it.
		centeredTextBox("chest-heading", "Card Vault - cards only", L.popupX, L.chestTop-16, stashPopupW, 14),
		{"chest-tab-toggle", toggle.Min.X, toggle.Min.Y, toggle.Dx(), toggle.Dy()},
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

// skillTrainerPopupLayoutBoxes returns the mastery-trainer popup's region and
// its sections with a FULL page of option rows: header, gold label, rows,
// pager strip and instructions line. The dialog itself is fixed-size but the
// page size is geometry-derived, so this guards the row-count/footer contract.
func skillTrainerPopupLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialogX := (screenW - npcDialogWidth) / 2
	dialogY := (screenH - npcDialogHeight) / 2
	px, py, pw, ph := skillTrainerPopupRect(dialogX, dialogY, npcDialogWidth, npcDialogHeight)
	region := uiBox{"trainer-popup", px, py, pw, ph}

	boxes := []uiBox{
		centeredTextBox("header", "Charname - Trainable Masteries", px, py+10, pw, 18),
		textLineBox("gold", "Gold: 999999", px+12, py+30),
		{Name: "pager", X: px + 12, Y: py + ph - 46, W: 396, H: 18},
		textLineBox("instructions", "Click to select  |  Double-click: train  |  ESC/Back: party list", px+12, py+ph-22),
	}
	for row := 0; row < skillTrainerPageSize(ph); row++ {
		x, y, w, h := skillTrainerOptionRect(px, py, row)
		// -2/+4: the hover/selection background pads the row rect vertically.
		boxes = append(boxes, uiBox{fmt.Sprintf("option-row-%d", row), x, y - 2, w, h + 4})
	}
	return region, boxes
}
