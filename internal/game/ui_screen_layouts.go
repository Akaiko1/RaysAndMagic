package game

type layoutRect struct{ x, y, w, h int }

func (r layoutRect) right() int  { return r.x + r.w }
func (r layoutRect) bottom() int { return r.y + r.h }

const (
	// These are minimum logical dimensions, not the runtime panel size. The
	// character hub fills the current viewport through computeTabbedMenuLayout.
	tabbedMenuPanelW = 700
	tabbedMenuPanelH = 640
	tabbedMenuTabH   = 35
	tabbedMenuInset  = 10
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
	panel, close, content layoutRect
	tabs                  []layoutRect
}

// computeTabbedMenuLayout fills the gameplay viewport above the party HUD.
// viewportBottom is explicit so hiding the HUD can restore the full height
// without duplicating the party-card height contract here.
func computeTabbedMenuLayout(screenW, viewportBottom int) tabbedMenuLayout {
	panel := layoutRect{tabbedMenuInset, tabbedMenuInset, screenW - 2*tabbedMenuInset, viewportBottom - 2*tabbedMenuInset}
	const (
		frameInset = 18
		tabGap     = 5
		maxTabW    = 170
	)
	closeSize := 22
	close := layoutRect{panel.right() - closeSize - frameInset, panel.y + frameInset, closeSize, closeSize}
	tabY := panel.y + frameInset
	tabAvailW := close.x - 12 - (panel.x + frameInset)
	tabW := (tabAvailW - (len(tabbedMenuTabs)-1)*tabGap) / len(tabbedMenuTabs)
	if tabW > maxTabW {
		tabW = maxTabW
	}
	tabsW := len(tabbedMenuTabs)*tabW + (len(tabbedMenuTabs)-1)*tabGap
	tabX := panel.x + frameInset + max(0, (tabAvailW-tabsW)/2)
	tabs := make([]layoutRect, len(tabbedMenuTabs))
	for i := range tabs {
		tabs[i] = layoutRect{tabX + i*(tabW+tabGap), tabY, tabW, tabbedMenuTabH}
	}
	contentY := tabY + tabbedMenuTabH + 10
	return tabbedMenuLayout{
		panel:   panel,
		tabs:    tabs,
		close:   close,
		content: layoutRect{panel.x + frameInset, contentY, panel.w - 2*frameInset, panel.bottom() - frameInset - contentY},
	}
}

const (
	inventoryPaperW   = inventoryPaperdollLayoutW
	inventoryPaperH   = inventoryPaperdollLayoutH
	inventoryGridSize = inventoryGridLayoutSize
	inventoryPanelGap = 52
)

type inventoryContentLayout struct {
	paper        layoutRect
	grid         layoutRect
	pager        layoutRect
	camp         layoutRect
	quickSlots   layoutRect
	instructions [2]layoutRect
}

func computeInventoryContentLayout(content layoutRect) inventoryContentLayout {
	const (
		topReserve    = 42
		bottomReserve = 34
	)
	bodyTop := content.y + topReserve
	bodyBottom := content.bottom() - bottomReserve
	bodyH := max(1, bodyBottom-bodyTop)
	scale := min(1.35,
		min(float64(bodyH)/float64(inventoryPaperH),
			float64(content.w-40)/float64(inventoryPaperW+inventoryPanelGap+inventoryGridSize)))
	if scale <= 0 {
		scale = 1
	}
	paperW := max(1, int(float64(inventoryPaperW)*scale))
	paperH := max(1, int(float64(inventoryPaperH)*scale))
	gridSize := max(1, int(float64(inventoryGridSize)*scale))
	gap := max(18, int(float64(inventoryPanelGap)*scale))
	blockW := paperW + gap + gridSize
	paperX := content.x + (content.w-blockW)/2
	paper := layoutRect{paperX, bodyTop + (bodyH-paperH)/2, paperW, paperH}
	// The authored paperdoll and inventory panels share one top rail at every
	// scale. Category tabs sit in the reserved space immediately above the grid.
	grid := layoutRect{paper.right() + gap, paper.y, gridSize, gridSize}
	pager := layoutRect{grid.x, grid.bottom() + 6, grid.w, pagerBtnH}
	campY := pager.bottom() + 7
	instructionY := content.bottom() - 2*debugTextCharHeight
	// Quick slots share the paperdoll's bottom rail. Their maximum height is the
	// remaining space after the Camp result and the quick-slot label; reserving
	// the result line even while empty prevents a successful rest from moving or
	// overlapping anything.
	quickTopMin := campY + inventoryCampButtonNoticeBlock +
		inventoryCampToQuickLabelGap + quickSlotTabLabelSpace
	maxQuickH := max(1, paper.bottom()-quickTopMin)
	maxQuickW := max(1, int(float64(maxQuickH)*quickSlotBarAspect))
	quickW := min(grid.w, min(maxQuickW, max(160, int(260*scale))))
	quickH := max(1, int(float64(quickW)/quickSlotBarAspect))
	quickSlots := layoutRect{grid.x + (grid.w-quickW)/2, paper.bottom() - quickH, quickW, quickH}
	return inventoryContentLayout{
		paper:      paper,
		grid:       grid,
		pager:      pager,
		camp:       layoutRect{grid.x, campY, grid.w, inventoryCampButtonNoticeBlock},
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
	title, profile, portraitFrame, portrait layoutRect
	attributes, magic, skills, combat       layoutRect
	instructions                            layoutRect
}

func computeCharacterContentLayout(content layoutRect) characterContentLayout {
	const (
		outerPad     = 12
		gap          = 12
		framePad     = 16
		maxContentW  = 1180
		maxContentH  = 610
		titleBlockH  = 28
		instructionH = debugTextCharHeight
	)
	blockW := min(maxContentW, max(1, content.w-2*outerPad))
	blockH := min(maxContentH, content.h)
	blockX := content.x + (content.w-blockW)/2
	blockY := content.y + (content.h-blockH)/2
	bodyY := blockY + titleBlockH
	bodyBottom := blockY + blockH - instructionH - 8
	bodyH := max(1, bodyBottom-bodyY)
	profileW := max(188, min(240, blockW/5))
	profile := layoutRect{blockX, bodyY, profileW, bodyH}
	rightX := profile.right() + gap
	rightW := blockX + blockW - rightX
	topH := max(142, min(180, bodyH/3))
	attributesW := (rightW - gap) / 2
	attributes := layoutRect{rightX, bodyY, attributesW, topH}
	magic := layoutRect{attributes.right() + gap, bodyY, rightW - attributesW - gap, topH}
	bottomY := bodyY + topH + gap
	bottomH := bodyBottom - bottomY
	skillsW := max(250, rightW*42/100)
	if skillsW > rightW-gap-280 {
		skillsW = rightW - gap - 280
	}
	skills := layoutRect{rightX, bottomY, skillsW, bottomH}
	combat := layoutRect{skills.right() + gap, bottomY, rightW - skillsW - gap, bottomH}
	portraitSize := min(profile.w-2*framePad-16, min(210, bodyH*42/100))
	portrait := layoutRect{profile.x + (profile.w-portraitSize)/2, profile.y + 54, portraitSize, portraitSize}
	return characterContentLayout{
		title:         layoutRect{blockX, blockY + 6, blockW, debugTextCharHeight},
		portraitFrame: layoutRect{portrait.x - framePad, portrait.y - framePad, portrait.w + 2*framePad, portrait.h + 2*framePad},
		portrait:      portrait,
		profile:       profile,
		attributes:    attributes,
		magic:         magic,
		skills:        skills,
		combat:        combat,
		instructions:  layoutRect{blockX, blockY + blockH - instructionH, blockW, instructionH},
	}
}

func computeCardsContentLayout(content layoutRect) cardsContentLayout {
	cols := 4
	if content.w >= 1080 {
		cols = MaxCardSlots
	}
	const icon, colGap, rowGap = 92, 30, 58
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
		summary:  layoutRect{content.x + 30, startY + ((MaxCardSlots+cols-1)/cols)*(icon+rowGap) + 6, content.w - 60, content.bottom() - (startY + ((MaxCardSlots+cols-1)/cols)*(icon+rowGap) + 6)},
		labelW:   icon + colGap - 6,
	}
}

const (
	questCardMaxW = 1120
	questCardGap  = 8
	questPagerH   = 22
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
	cardW        int
}

// questCardListAvailable is the vertical space the card list may use.
func questCardListAvailable(content layoutRect) (listTop, avail int, pager layoutRect) {
	listTop = content.y + 40
	cardW := min(questCardMaxW, content.w-40)
	cardX := content.x + (content.w-cardW)/2
	pager = layoutRect{cardX, content.bottom() - questPagerH, cardW, questPagerH}
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
		cardW:        pager.w,
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
		layout.rows = append(layout.rows, layoutRect{pager.x, y, pager.w, copies[i].height})
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
