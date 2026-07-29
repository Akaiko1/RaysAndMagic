package game

type layoutRect struct{ x, y, w, h int }

func (r layoutRect) right() int  { return r.x + r.w }
func (r layoutRect) bottom() int { return r.y + r.h }

const (
	tabbedMenuPanelW = 700
	tabbedMenuPanelH = 640
	tabbedMenuTabW   = 120
	tabbedMenuTabH   = 35
)

type menuTabSpec struct {
	tab   MenuTab
	label string
	key   string
}

var tabbedMenuTabs = []menuTabSpec{
	{TabInventory, "Inventory", "(I)"},
	{TabCharacters, "Characters", "(C)"},
	{TabSpellbook, "Spellbook", "(M)"},
	{TabQuests, "Quests", "(J)"},
	{TabCards, "Cards", "(K)"},
}

type tabbedMenuLayout struct {
	panel   layoutRect
	tabs    []layoutRect
	close   layoutRect
	content layoutRect
}

func computeTabbedMenuLayout(screenW, screenH int) tabbedMenuLayout {
	panel := layoutRect{(screenW - tabbedMenuPanelW) / 2, (screenH - tabbedMenuPanelH) / 2, tabbedMenuPanelW, tabbedMenuPanelH}
	const frameInset = 18
	tabY := panel.y + frameInset
	tabs := make([]layoutRect, len(tabbedMenuTabs))
	for i := range tabs {
		tabs[i] = layoutRect{panel.x + 20 + i*(tabbedMenuTabW+5), tabY, tabbedMenuTabW, tabbedMenuTabH}
	}
	closeSize := 20
	contentY := panel.y + 55
	return tabbedMenuLayout{
		panel:   panel,
		tabs:    tabs,
		close:   layoutRect{panel.right() - closeSize - frameInset, panel.y + frameInset, closeSize, closeSize},
		content: layoutRect{panel.x, contentY, panel.w, panel.bottom() - 20 - contentY},
	}
}

const (
	inventoryPaperW   = inventoryPaperdollSourceW
	inventoryPaperH   = inventoryPaperdollSourceH
	inventoryGridSize = inventoryGridSourceSize
	inventoryPanelGap = 52
)

type inventoryContentLayout struct {
	paper        layoutRect
	grid         layoutRect
	pager        layoutRect
	camp         layoutRect
	quickLabel   layoutRect
	quickSlots   layoutRect
	instructions [2]layoutRect
}

func computeInventoryContentLayout(content layoutRect) inventoryContentLayout {
	blockW := inventoryPaperW + inventoryPanelGap + inventoryGridSize
	paperX := content.x + (content.w-blockW)/2
	paper := layoutRect{paperX, content.y + 48, inventoryPaperW, inventoryPaperH}
	grid := layoutRect{paper.right() + inventoryPanelGap, content.y + 84, inventoryGridSize, inventoryGridSize}
	pager := layoutRect{grid.x, grid.bottom() + 6, grid.w, pagerBtnH}
	quickW := 210
	quickSlots := layoutRect{grid.x + (grid.w-quickW)/2, grid.bottom() + 94, quickW, int(float64(quickW) / quickSlotBarAspect)}
	const campBlockH = 26 + 6 + debugTextCharHeight
	campY := grid.bottom() + (quickSlots.y-grid.bottom()-campBlockH)/2
	if minCampY := pager.bottom() + 6; campY < minCampY {
		campY = minCampY
	}
	instructionY := content.bottom() - 35
	return inventoryContentLayout{
		paper:      paper,
		grid:       grid,
		pager:      pager,
		camp:       layoutRect{grid.x, campY, grid.w, campBlockH},
		quickLabel: layoutRect{quickSlots.x, quickSlots.y - 15, quickSlots.w, debugTextCharHeight},
		quickSlots: quickSlots,
		instructions: [2]layoutRect{
			{paper.x, instructionY, content.right() - paper.x, debugTextCharHeight},
			{paper.x, instructionY + debugTextCharHeight, content.right() - paper.x, debugTextCharHeight},
		},
	}
}

type cardsContentLayout struct {
	title, subtitle layoutRect
	cards           []layoutRect
	summary         layoutRect
	labelW          int
}

type characterContentLayout struct {
	title, portraitFrame, portrait, scroll layoutRect
	instructions, pager                    layoutRect
}

func computeCharacterContentLayout(content layoutRect) characterContentLayout {
	const portraitSize, portraitGap, scrollW, scrollH = 180, 24, 420, 330
	blockW := portraitSize + portraitGap + scrollW
	cardX := content.x + (content.w-blockW)/2
	cardY := content.y + 40
	const framePad = 16
	portrait := layoutRect{cardX, cardY + framePad, portraitSize, portraitSize}
	return characterContentLayout{
		title:         layoutRect{content.x + 20, content.y + 10, content.w - 40, debugTextCharHeight},
		portraitFrame: layoutRect{portrait.x - framePad, portrait.y - framePad, portrait.w + 2*framePad, portrait.h + 2*framePad},
		portrait:      portrait,
		scroll:        layoutRect{cardX + portraitSize + portraitGap, cardY, scrollW, scrollH},
		instructions:  layoutRect{cardX, content.bottom() - 42, blockW, debugTextCharHeight},
		pager:         layoutRect{cardX + portraitSize + portraitGap, content.bottom() - 22, scrollW, pagerBtnH},
	}
}

func computeCardsContentLayout(content layoutRect) cardsContentLayout {
	const cols, icon, colGap, rowGap = 4, 84, 44, 64
	gridW := cols*icon + (cols-1)*colGap
	startX := content.x + (content.w-gridW)/2
	startY := content.y + 56
	cards := make([]layoutRect, MaxCardSlots)
	for i := range cards {
		cards[i] = layoutRect{startX + (i%cols)*(icon+colGap), startY + (i/cols)*(icon+rowGap), icon, icon}
	}
	return cardsContentLayout{
		title:    layoutRect{content.x + 30, content.y + 6, content.w - 60, debugTextCharHeight},
		subtitle: layoutRect{content.x + 30, content.y + 24, content.w - 60, debugTextCharHeight},
		cards:    cards,
		summary:  layoutRect{content.x + 30, startY + 2*(icon+rowGap) + 6, content.w - 60, content.bottom() - (startY + 2*(icon+rowGap) + 6)},
		labelW:   icon + colGap - 6,
	}
}

const (
	questCardW   = 520
	questCardGap = 8
	questPagerH  = 22
	// Quest card chrome, measured from the card's own top: the name row sits
	// above the description, the progress row / bar / claim button below it.
	// A card's HEIGHT is these plus however many description rows it needs.
	questCardDescTop      = 22
	questCardProgressGap  = 18 // description bottom -> progress bar
	questCardBarH         = 14
	questCardBottomPad    = 9
	questCardBottomChrome = questCardProgressGap + questCardBarH + questCardBottomPad
	questCardMaxDescRows  = 6 // sanity cap; hovering shows anything beyond it
)

// questCardHeight is the height a card needs to draw descRows description
// lines. THE one card-height formula, used by the page packer and the renderer.
func questCardHeight(descRows int) int {
	if descRows < 1 {
		descRows = 1
	}
	return questCardDescTop + descRows*debugTextCharHeight + questCardBottomChrome
}

// questCardCopy is one card's wrapped description plus the height it needs.
// Computed ONCE and handed to both the layout (which packs pages by height)
// and the renderer (which draws these exact lines), so a card can never be
// sized for a different amount of text than it shows.
type questCardCopy struct {
	descLines []string // what fits and is drawn
	fullLines []string // the whole wrapped description (hover tooltip source)
	height    int
}

func (c questCardCopy) descClipped() bool { return len(c.fullLines) > len(c.descLines) }

// questCardCopyFor wraps a description to the card's text width and clips it to
// maxDescRows.
func questCardCopyFor(description string, cardW, maxDescRows int) questCardCopy {
	textW := cardW - 20
	full := wrapDebugText(description, textW)
	if len(full) == 0 {
		full = []string{""}
	}
	if maxDescRows < 1 {
		maxDescRows = 1
	}
	shown := truncateWrappedLines(full, maxDescRows, textW)
	return questCardCopy{descLines: shown, fullLines: full, height: questCardHeight(len(shown))}
}

type questContentLayout struct {
	title layoutRect
	rows  []layoutRect
	pager layoutRect
	// pageStart is the index of the first card ON THIS PAGE; rows[i] belongs to
	// card pageStart+i. Pages hold a VARIABLE number of cards, so a caller must
	// never derive the offset from a page size.
	pageStart    int
	totalPages   int
	maxDescRows  int
	listAvailabl int
}

// questCardListAvailable is the vertical space the card list may use.
func questCardListAvailable(content layoutRect) (listTop, avail int, pager layoutRect) {
	listTop = content.y + 40
	pager = layoutRect{content.x + 20, content.bottom() - questPagerH, questCardW, questPagerH}
	return listTop, pager.y - listTop, pager
}

// questCardMaxDescRowsFor is how many description rows a single card may show
// without outgrowing the whole list area.
func questCardMaxDescRowsFor(avail int) int {
	rows := (avail - questCardDescTop - questCardBottomChrome) / debugTextCharHeight
	if rows < 1 {
		rows = 1
	}
	if rows > questCardMaxDescRows {
		rows = questCardMaxDescRows
	}
	return rows
}

// computeQuestContentLayout packs variable-height cards into pages: a page
// takes as many cards as fit its available height, so a page of terse quests
// shows more of them than a page of wordy ones.
func computeQuestContentLayout(content layoutRect, copies []questCardCopy, page int) questContentLayout {
	listTop, avail, pager := questCardListAvailable(content)
	layout := questContentLayout{
		title:        layoutRect{content.x + 20, content.y + 10, content.w - 40, debugTextCharHeight},
		pager:        pager,
		maxDescRows:  questCardMaxDescRowsFor(avail),
		listAvailabl: avail,
	}

	// Greedy packing: every page holds at least one card even if it is taller
	// than the list, so an over-long entry still renders instead of vanishing.
	type pageRange struct{ start, end int }
	var pages []pageRange
	for start := 0; start < len(copies); {
		used, end := 0, start
		for end < len(copies) {
			need := copies[end].height
			if end > start {
				need += questCardGap
			}
			if used+need > avail && end > start {
				break
			}
			used += need
			end++
		}
		pages = append(pages, pageRange{start, end})
		start = end
	}
	if len(pages) == 0 {
		layout.totalPages = 1
		return layout
	}
	layout.totalPages = len(pages)
	if page < 0 {
		page = 0
	}
	if page >= len(pages) {
		page = len(pages) - 1
	}
	current := pages[page]
	layout.pageStart = current.start
	y := listTop
	for i := current.start; i < current.end; i++ {
		layout.rows = append(layout.rows, layoutRect{content.x + 20, y, questCardW, copies[i].height})
		y += copies[i].height + questCardGap
	}
	return layout
}

type mapOverlayLayout struct {
	panel, title, close, body layoutRect
}

type npcDialogSectionLayout struct {
	panel, title, balance, greeting, body layoutRect
	footer                                [2]layoutRect
}

func computeNPCDialogSectionLayout(dialog layoutRect, hasBalance bool) npcDialogSectionLayout {
	titleW := dialog.w - 40
	balance := layoutRect{}
	if hasBalance {
		titleW = dialog.w - 220
		balance = layoutRect{dialog.right() - 160, dialog.y + 20, 140, debugTextCharHeight}
	}
	footerY := dialog.bottom() - 38
	return npcDialogSectionLayout{
		panel:    dialog,
		title:    layoutRect{dialog.x + 20, dialog.y + 20, titleW, debugTextCharHeight},
		balance:  balance,
		greeting: layoutRect{dialog.x + 20, dialog.y + 44, min(dialog.w-40, tabGreetingWrapColumns*debugTextCharWidth), 2 * dialogueLineHeight},
		body:     layoutRect{dialog.x + 20, dialog.y + 92, dialog.w - 40, footerY - (dialog.y + 92) - 8},
		footer: [2]layoutRect{
			{dialog.x + 20, footerY, dialog.w - 40, debugTextCharHeight},
			{dialog.x + 20, footerY + debugTextCharHeight, dialog.w - 40, debugTextCharHeight},
		},
	}
}

func computeMapOverlayLayout(screenW, screenH int) mapOverlayLayout {
	panelW := min(720, max(320, int(float64(screenW)*0.75)))
	panelH := min(560, max(240, int(float64(screenH)*0.75)))
	panel := layoutRect{(screenW - panelW) / 2, (screenH - panelH) / 2, panelW, panelH}
	return mapOverlayLayout{
		panel: panel,
		title: layoutRect{panel.x + 16, panel.y + 12, panel.w - 58, debugTextCharHeight},
		close: layoutRect{panel.right() - 26, panel.y + 10, 16, 16},
		body:  layoutRect{panel.x + 18, panel.y + 36, panel.w - 36, panel.h - 54},
	}
}
