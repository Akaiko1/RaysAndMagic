package game

import (
	"fmt"

	uitext "ugataima/assets/text"
	"ugataima/internal/stash"
)

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

// textLineBox is the bounding box of a left-aligned drawUIText(text, x, y).
func textLineBox(name, text string, x, y int) uiBox {
	return uiBox{name, x, y, uiTextWidth(text), uiTextCharHeight}
}

// centeredTextBox is the bounding box of drawCenteredUIText(text, x, y, w, h).
func centeredTextBox(name, text string, x, y, w, h int) uiBox {
	tw := uiTextWidth(text)
	return uiBox{name, x + (w-tw)/2, y + (h-uiTextCharHeight)/2, tw, uiTextCharHeight}
}

func namedLayoutBox(name string, r layoutRect) uiBox {
	return uiBox{name, r.x, r.y, r.w, r.h}
}

func mainMenuLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	panel := mainMenuPanelRect(screenW, screenH, MenuMain)
	title := mainMenuTitleRect(panel)
	boxes := []uiBox{centeredTextBox("title", mainMenuTitle, title.x, title.y, title.w, title.h)}
	for i, option := range mainMenuOptions {
		row, _, _ := menuRowRect(panel.x, panel.y, panel.w, mainMenuListTopY, mainMenuRowPitch, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("option-%d-%s", i, option.key), row.x1, row.y1, row.x2 - row.x1, row.y2 - row.y1})
	}
	return namedLayoutBox("main-menu", panel), boxes
}

func audioSettingsLayoutBoxes(screenW, screenH int, ornate bool) (uiBox, []uiBox) {
	return settingsLayoutBoxes(screenW, screenH, ornate, settingsTabSound)
}

func settingsLayoutBoxes(screenW, screenH int, ornate bool, tab settingsTabKind) (uiBox, []uiBox) {
	layout := makeAudioSettingsPanelLayout(screenW, screenH, ornate)
	px, py, panelW, panelH := layout.px, layout.py, layout.panelW, layout.panelH
	region := uiBox{"audio-settings", px, py, panelW, panelH}
	boxes := []uiBox{{"title", px + (panelW-uiTextWidth("Settings")*2)/2, py + 22, uiTextWidth("Settings") * 2, uiTextCharHeight * 2}}
	for tab := range settingsTabLabels {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("tab-%d", tab), settingsTabRect(px, py, panelW, settingsTabKind(tab))))
	}
	if tab == settingsTabCombat {
		for page := range gameplayPageLabels {
			boxes = append(boxes, namedLayoutBox(fmt.Sprintf("page-%d", page), gameplayPageRect(px, py, gameplayPage(page))))
		}
		for _, wide := range []bool{false, true} {
			boxes = append(boxes, namedLayoutBox(fmt.Sprintf("view-%v", wide), viewChoiceRect(px, py, panelW, wide)))
		}
		for i, text := range combatSettingsTextLayout(px, py, panelW) {
			b := text.box
			name := fmt.Sprintf("text-%d", i)
			boxes = append(boxes, textLineBox(name, text.label, b.x, b.y))
		}
		for _, enabled := range []bool{false, true} {
			boxes = append(boxes, namedLayoutBox(fmt.Sprintf("choice-%v", enabled), combatOverlayChoiceRect(px, py, panelW, enabled)))
		}
	} else {
		for row, def := range audioSettingDefinitions {
			r := audioSelectionRect(px, py, panelW, layout.contentInset, row)
			boxes = append(boxes, uiBox{fmt.Sprintf("slider-%s", def.label), r.x1, r.y1, r.x2 - r.x1, r.y2 - r.y1})
		}
	}
	back := audioBackRect(px, py, panelH, layout.contentInset)
	hint, hintX, hintY := settingsHintPosition(tab, gameplayPageView, "", px, py, panelW, panelH, layout.contentInset)
	boxes = append(boxes,
		uiBox{"back", back.x1, back.y1, back.x2 - back.x1, back.y2 - back.y1},
		textLineBox("hint", hint, hintX, hintY),
	)
	return region, boxes
}

func tabbedMenuLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	l := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	region := namedLayoutBox("tabbed-menu", l.panel)
	boxes := make([]uiBox, 0, len(l.tabs)+2)
	for i, tab := range l.tabs {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("tab-%d", i), tab))
	}
	boxes = append(boxes, namedLayoutBox("close", l.close), namedLayoutBox("content", l.content))
	return region, boxes
}

func inventoryLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	l := computeInventoryContentLayout(menu.content, 1)
	return namedLayoutBox("inventory-content", menu.content), []uiBox{
		namedLayoutBox("personal-grid", l.personalGrid), namedLayoutBox("personal-pager", l.personalPager),
		namedLayoutBox("shared-filters", l.categories),
		namedLayoutBox("personal-filters", l.personalCategories),
		namedLayoutBox("equipment-heading", l.headings[0]),
		namedLayoutBox("personal-heading", l.headings[1]),
		namedLayoutBox("shared-heading", l.headings[2]),
		namedLayoutBox("gold", l.resources[0]),
		namedLayoutBox("food", l.resources[1]),
		namedLayoutBox("paperdoll", l.paper),
		namedLayoutBox("inventory-grid", l.grid),
		namedLayoutBox("pager", l.pager),
		namedLayoutBox("quick-slots", l.quickSlots),
	}
}

func cardsLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	l := computeCardsContentLayout(menu.content)
	boxes := []uiBox{namedLayoutBox("title", l.title), namedLayoutBox("subtitle", l.subtitle)}
	for i, card := range l.cards {
		cardAndLabels := layoutRect{card.x - (l.labelW-card.w)/2, card.y, l.labelW, card.h + 2 + 2*uiTextCharHeight}
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("card-%d", i), cardAndLabels))
	}
	boxes = append(boxes, namedLayoutBox("summary", l.summary))
	return namedLayoutBox("cards-content", menu.content), boxes
}

func charactersLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	l := computeCharacterContentLayout(menu.content)
	return namedLayoutBox("characters-content", menu.content), []uiBox{
		namedLayoutBox("title", l.title),
		namedLayoutBox("profile", l.profile),
		namedLayoutBox("attributes", l.attributes),
		namedLayoutBox("magic", l.magic),
		namedLayoutBox("skills", l.skills),
		namedLayoutBox("combat", l.combat),
		namedLayoutBox("instructions", l.instructions),
	}
}

func bookLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	l := computeBookLayout(menu.content, 1)
	return namedLayoutBox("book-content", menu.content), []uiBox{
		namedLayoutBox("header", l.header),
		{"book", l.bookX, l.bookY, l.bookW, l.bookH},
		namedLayoutBox("quick-slots", l.quick),
		namedLayoutBox("pager", l.pager),
		namedLayoutBox("controls", l.controls),
	}
}

func questsLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	menu := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	// Layout-overlay probe: a page of default-height cards is enough to draw
	// the debug boxes; real cards size themselves to their own copy.
	probe := make([]questCardCopy, 100)
	for i := range probe {
		probe[i] = questCardCopy{descLines: []string{""}, fullLines: []string{""}, height: questCardHeight(2)}
	}
	l := computeQuestContentLayout(menu.content, probe, 0)
	boxes := []uiBox{namedLayoutBox("title", l.title)}
	for i, row := range l.rows {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("quest-%d", i), row))
	}
	boxes = append(boxes, namedLayoutBox("pager", l.pager))
	return namedLayoutBox("quests-content", menu.content), boxes
}

func mapOverlayLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	l := computeMapOverlayLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	boxes := []uiBox{
		namedLayoutBox("title", l.title),
		namedLayoutBox("close", l.close),
		namedLayoutBox("map", l.body),
		namedLayoutBox("subtitle", l.subtitle),
		namedLayoutBox("legend", l.legend),
		namedLayoutBox("overview", l.overview),
	}
	if l.index.w > 0 {
		boxes = append(boxes, namedLayoutBox("index", l.index))
	}
	return namedLayoutBox("map-panel", l.panel), boxes
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
		boxes = append(boxes, uiBox{fmt.Sprintf("portrait-%d", i), x - 8, y, w + 16, h + 6 + uiTextCharHeight})
	}
	// The pager is as wide as the spell grid it pages.
	pager := spellTraderPagerRect(dialog.x, dialog.y)
	gridH := spellTraderGridRows*spellTraderRowPitch - spellTraderRowGap
	boxes = append(boxes,
		uiBox{"spell-grid", pager.x, spellTraderGridTop(dialog.y), pager.w, gridH},
		namedLayoutBox("pager", pager),
		namedLayoutBox("footer", layoutRect{l.footer[0].x, l.footer[0].y, l.footer[0].w, 2 * uiTextCharHeight}),
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
		boxes = append(boxes, uiBox{fmt.Sprintf("portrait-%d", i), x - 8, y, w + 16, h + 24 + uiTextCharHeight})
	}
	boxes = append(boxes, namedLayoutBox("footer", l.footer[0]))
	return namedLayoutBox("trainer-dialog", dialog), boxes
}

func merchantDialogLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog := centeredRect(screenW, screenH, npcDialogWidth, merchantDialogHeight)
	l := computeMerchantDialogSectionLayout(dialog)
	leftX, rightX, gridTop, pagerY := merchantGridLayout(dialog.x, dialog.y)
	gridH := merchantGridRows*(merchantIconSize+merchantPriceH+merchantRowGap) - merchantRowGap
	boxes := []uiBox{
		namedLayoutBox("shop-display", merchantDisplayRect(dialog)),
		namedLayoutBox("title", l.title), namedLayoutBox("balance", l.balance), namedLayoutBox("greeting", l.greeting),
		textLineBox("buy-heading", "For Sale", leftX, gridTop-72),
		textLineBox("sell-heading", "Your Items", rightX, gridTop-72),
		{"buy-grid", leftX, gridTop, merchantGridW, gridH},
		{"sell-grid", rightX, gridTop, merchantGridW, gridH},
		{"buy-pager", leftX, pagerY, merchantGridW, pagerBtnH},
		{"sell-pager", rightX, pagerY, merchantGridW, pagerBtnH},
		namedLayoutBox("footer", layoutRect{l.footer[0].x, l.footer[0].y, l.footer[0].w, 2 * uiTextCharHeight}),
	}
	// The largest shipped set-shop has four stock tabs along the same rail.
	for i := 0; i < 4; i++ {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("stock-tab-%d", i), merchantFolderTabRect(dialog.x, dialog.y, i, 4)))
	}
	for side, x := range []int{leftX, rightX} {
		for i, r := range inventoryTabRects(x, gridTop-48, merchantGridW) {
			boxes = append(boxes, namedLayoutBox(fmt.Sprintf("category-%d-%d", side, i), r))
		}
	}
	// The decorative display intentionally extends above the modal's input
	// rectangle. Check against the complete visible region, not just its body.
	display := merchantDisplayRect(dialog)
	return namedLayoutBox("merchant-dialog", layoutRect{dialog.x, display.y, dialog.w, dialog.bottom() - display.y}), boxes
}

func cardCollectorLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	dialog := layoutRect{(screenW - npcDialogWidth) / 2, (screenH - npcDialogHeight) / 2, npcDialogWidth, npcDialogHeight}
	l := computeNPCDialogSectionLayout(dialog, false)
	c := makeCardCollectorLayout(dialog.x, dialog.y)
	boxes := []uiBox{
		namedLayoutBox("title", l.title), namedLayoutBox("greeting", l.greeting),
		textLineBox("collection-heading", uitext.Text("dialog.collection_active_effects"), c.collectionHeading.x, c.collectionHeading.y),
		textLineBox("inventory-heading", uitext.Text("dialog.your_cards_double_click_to_add"), c.looseHeading.x, c.looseHeading.y),
	}
	for i := 0; i < MaxCardSlots; i++ {
		x, y, w, h := cardCollectorSlotRect(dialog.x, dialog.y, i)
		boxes = append(boxes, uiBox{fmt.Sprintf("active-card-%d", i), x, y, w, h})
	}
	boxes = append(boxes,
		namedLayoutBox("inventory-cards", c.looseGrid),
		namedLayoutBox("pager", c.pager),
		namedLayoutBox("footer", l.footer[0]),
	)
	return namedLayoutBox("card-collector", dialog), boxes
}

// stashLayoutBoxes returns the stash modal's region and its section boxes.
func stashLayoutBoxes(screenW, screenH int) (uiBox, []uiBox) {
	popup, L := computeStashLayout(screenW, screenH)
	return namedLayoutBox("stash-popup", popup), append([]uiBox{
		textLineBox("title", uitext.Text("stash.title"), popup.x+16, popup.y+14),
		{"hint", popup.x + 16, popup.y + 34, min(uiTextWidth(uitext.Text("stash.hint")), popup.w-72), uiTextCharHeight},
	}, stashManagerBoxes(L)...)
}

// stashManagerBoxes are the stash grids and their labels in either host.
func stashManagerBoxes(L stashLayout) []uiBox {
	// The widest the counter gets: every cell full.
	counter := fmt.Sprintf("%d / %d", stash.SlotCount, stash.SlotCount)
	return []uiBox{
		textLineBox("chest-heading", uitext.Text("stash.chest"), L.chest.x, L.headY),
		textLineBox("chest-counter", counter, L.chest.right()-uiTextWidth(counter), L.headY),
		namedLayoutBox("chest-cells", L.chest),
		{"chest-note", L.chest.x, L.noteY, min(uiTextWidth(uitext.Text("stash.shared")), L.chest.w), uiTextCharHeight},
		textLineBox("bag-heading", uitext.Text("stash.bag"), L.bag.x, L.headY),
		namedLayoutBox("bag-cells", L.bag),
		{"pager", L.bag.x, L.noteY, L.bag.w, pagerBtnH},
	}
}

// saveListBoxes are the shared save list's sections: the 2x heading, the
// rows, the pager, and the widest hint line at its origin.
func saveListBoxes(l saveListLayout, heading, hint string, hintX int, page int) []uiBox {
	hw, hh := 2*uiTextWidth(heading)+4, 2*uiTextCharHeight
	boxes := []uiBox{{"title", l.title.x + (l.title.w-hw)/2, l.title.y + (l.title.h-hh)/2, hw, hh}}
	for i, r := range l.rows {
		boxes = append(boxes, namedLayoutBox(fmt.Sprintf("row-%d", page*saveRowsPerPage+i), r))
	}
	pageLabel := fmt.Sprintf("Page %d of %d", savePageCount, savePageCount)
	return append(boxes,
		namedLayoutBox("pager-prev", l.prev),
		namedLayoutBox("pager-next", l.next),
		centeredTextBox("page", pageLabel, l.page.x, l.page.y, l.page.w, l.page.h),
		textLineBox("hint", hint, hintX, l.hintY),
	)
}

// saveMenuLayoutBoxes returns the in-game save/load menu's panel and sections
// for the given page.
func saveMenuLayoutBoxes(screenW, screenH, page int, modeLoad bool) (uiBox, []uiBox) {
	l := escSaveListLayout(screenW, screenH)
	heading, hint := "Save Game", saveMenuHint
	if modeLoad {
		heading, hint = "Load Game", loadMenuHint
	}
	return namedLayoutBox("save-menu", l.panel), saveListBoxes(l, heading, hint, l.title.x, page)
}

// entryLoadLayoutBoxes returns the title-screen Load page's panel and sections
// (heading, rows, pager, hint, Back) for the given page.
func entryLoadLayoutBoxes(screenW, screenH, page int) (uiBox, []uiBox) {
	l := entrySaveListLayout(screenW, screenH)
	boxes := saveListBoxes(l, "Load Game", entryLoadHint, l.title.right()-uiTextWidth(entryLoadHint), page)
	return namedLayoutBox("entry-load-panel", l.panel), append(boxes, namedLayoutBox("back", l.back))
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
	l := makeSkillTrainerPopupLayout(px, py, pw, ph)

	boxes := []uiBox{
		centeredTextBox("header", uitext.Text("dialog.trainable_masteries", "Charname"), l.header.x, l.header.y, l.header.w, l.header.h),
		textLineBox("gold", uitext.Text("dialog.gold", 999999), l.gold.x, l.gold.y),
		namedLayoutBox("pager", l.pager),
		textLineBox("instructions", uitext.Text("dialog.click_to_select_double_click_train_esc"), l.hint.x, l.hint.y),
	}
	for row := 0; row < skillTrainerPageSize(ph); row++ {
		x, y, w, h := skillTrainerOptionRect(px, py, row)
		// -2/+4: the hover/selection background pads the row rect vertically.
		boxes = append(boxes, uiBox{fmt.Sprintf("option-row-%d", row), x, y - 2, w, h + 4})
	}
	return region, boxes
}

// collectionLayoutBoxes returns the Storage Clerk's collections screen: its
// panel and the leaf sections of one tab (showcase or binder) beside the bag.
func collectionLayoutBoxes(screenW, screenH, tab int) (uiBox, []uiBox) {
	L := computeCollectionLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH), tab)
	region := namedLayoutBox("collection-panel", L.panel)
	title, progress, about := uitext.Text("collection.tab_showcase"), uitext.Text("collection.progress_showcase", 99, 99), uitext.Text("collection.about_showcase")
	if tab == collectionTabBinder {
		title, progress, about = uitext.Text("collection.tab_binder"), uitext.Text("collection.progress_binder", 99, 99, 999), uitext.Text("collection.about_binder", stash.AlbumCopies)
	}
	boxes := []uiBox{
		namedLayoutBox("tab-showcase", L.tabs[0]),
		namedLayoutBox("tab-binder", L.tabs[1]),
		namedLayoutBox("close", L.close),
		textLineBox("title", title, L.page.x+16, L.page.y+10),
		textLineBox("progress", progress, L.page.right()-16-uiTextWidth(progress), L.page.y+10),
		{"about", L.page.x + 16, L.page.y + 28, min(uiTextWidth(about), L.page.w-32), uiTextCharHeight},
		centeredTextBox("bag-heading", uitext.Text("collection.bag"), L.side.x, L.side.y+10, L.side.w, 16),
		namedLayoutBox("bag-cells", L.bagGrid),
		namedLayoutBox("bag-pager", L.pager),
		{"bag-hint", L.hint.x, L.hint.y, L.hint.w, len(wrapUIText(collectionBagHint(tab == collectionTabBinder), L.hint.w)) * collectionHintLineH},
		// The side column's bottom edge: the hint must end above it.
		{"bag-column-end", L.side.x, L.side.bottom(), L.side.w, 1},
	}
	if tab == collectionTabBinder {
		for i, page := range L.pages {
			boxes = append(boxes, namedLayoutBox(fmt.Sprintf("binder-page-%d", i), page))
		}
		boxes = append(boxes, namedLayoutBox("page-turner", L.turner))
	} else {
		// The case's brass trim runs one unit outside its glass.
		trim := layoutRect{L.glass.x - 1, L.glass.y - 1, L.glass.w + 2, L.glass.h + 2}
		boxes = append(boxes, namedLayoutBox("showcase-case", trim), namedLayoutBox("showcase-scrollbar", L.track))
	}
	return region, boxes
}
