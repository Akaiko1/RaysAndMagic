package game

import "ugataima/internal/quests"

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
	{TabCharacters, "Characters", "(P)"},
	{TabSpellbook, "Spellbook", "(M)"},
	{TabQuests, "Quests", "(J)"},
	{TabCards, "Cards", "(K)"},
}

type tabbedMenuLayout struct {
	panel, close, content layoutRect
	tabs                  []layoutRect
}

// centeredRect is the one rule every centred window follows: a w x h box in
// the middle of the current screenW x screenH frame. Windows recompute it each
// frame, so an interface size change recentres them at once.
func centeredRect(screenW, screenH, w, h int) layoutRect {
	return layoutRect{(screenW - w) / 2, (screenH - h) / 2, w, h}
}

// mainMenuPanelRect is the ESC menu panel for a mode: one source for drawing,
// hover and clicks.
func mainMenuPanelRect(screenW, screenH int, mode MainMenuMode) layoutRect {
	w, h := menuPanelSize(mode, screenW, screenH)
	return centeredRect(screenW, screenH, w, h)
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
	inventoryPaperW          = inventoryPaperdollLayoutW
	inventoryPaperH          = inventoryPaperdollLayoutH
	inventoryGridSize        = inventoryGridLayoutSize
	inventoryPagerToQuickGap = 8
)

type inventoryContentLayout struct {
	paper, grid, personalGrid                  layoutRect
	pager, personalPager                       layoutRect
	categories, personalCategories, quickSlots layoutRect
	headings                                   [3]layoutRect
	resources                                  [2]layoutRect
	textScale                                  int
}

func computeInventoryContentLayout(content layoutRect) inventoryContentLayout {
	// Compact windows give the bags more of the available width. The paperdoll
	// retains its aspect ratio, while text switches only between native and 2x.
	textScale, gap := readingTextScale, 24
	paperW, paperH := float64(inventoryPaperW), float64(inventoryPaperH)
	if content.w < 1200 || content.h < 600 {
		textScale, gap = 1, 16
		paperW, paperH = paperW*0.75, paperH*0.75
	}
	labelH := (uiTextCharHeight + 2) * textScale
	// The doll column carries its own header - the name, then Gold and Food,
	// side by side on a compact frame where every row is dear - and the bag
	// columns theirs (heading, filters). With 2x text the two headers are the
	// same height, so all three columns share one top rail.
	resourceRows := 2
	if textScale == 1 {
		resourceRows = 1
	}
	const maxArtScale = 2.4
	widthScale := min(maxArtScale, float64(content.w-32)/float64(int(paperW)+2*inventoryGridSize+2*gap))
	topH := labelH + 18 + max(2*labelH, inventoryFilterHeight(int(inventoryGridSize*widthScale)))
	dollTopH := labelH + 18 + resourceRows*labelH
	const footerH = 8 + pagerBtnH + inventoryPagerToQuickGap + quickSlotTabLabelSpace
	const quickLogicalW = 320
	// The paperdoll takes the whole height below its header. The bag column
	// (grid, pager, quick slots) grows with it until the column fills the
	// height too, and width the bags leave unused goes to the doll, so a short
	// frame never holds the doll to the bags' size.
	paperMax := float64(content.h-dollTopH) / paperH
	gridScale := max(0.1, min(widthScale, paperMax, (float64(content.h-topH)-footerH)/(inventoryGridSize+quickLogicalW/quickSlotBarAspect)))
	gs, space := int(inventoryGridSize*gridScale), int(float64(gap)*gridScale)
	paperScale := max(0.1, min(maxArtScale, paperMax, float64(content.w-32-2*gs-2*space)/paperW))
	pw, ph := int(paperW*paperScale), int(paperH*paperScale)
	filterH := inventoryFilterHeight(gs)
	quickW := int(quickLogicalW * gridScale)
	quickH := int(float64(quickW) / quickSlotBarAspect)
	blockW, blockH := pw+2*gs+2*space, max(dollTopH+ph, topH+gs+footerH+quickH)
	x, top := content.x+(content.w-blockW)/2, content.y+(content.h-blockH)/2
	y := top + topH
	paper := layoutRect{x, top + dollTopH, pw, ph}
	personal := layoutRect{paper.right() + space, y, gs, gs}
	shared := layoutRect{personal.right() + space, y, gs, gs}
	pagerW := min(220, max(120, gs*2/3))
	pager := layoutRect{shared.x + (gs-pagerW)/2, shared.bottom() + 8, pagerW, pagerBtnH}
	personalPager := layoutRect{personal.x + (gs-pagerW)/2, personal.bottom() + 8, pagerW, pagerBtnH}
	quick := layoutRect{personal.x + (shared.right()-personal.x-quickW)/2, top + blockH - quickH, quickW, quickH}
	resourceY := top + labelH + 8
	filterY := resourceY + (topH-labelH-18-filterH)/2
	resources := [2]layoutRect{{paper.x, resourceY, pw, labelH}, {paper.x, resourceY + labelH, pw, labelH}}
	if resourceRows == 1 {
		resources = [2]layoutRect{{paper.x, resourceY, pw / 2, labelH}, {paper.x + pw/2, resourceY, pw - pw/2, labelH}}
	}
	return inventoryContentLayout{
		textScale: textScale,
		paper:     paper, grid: shared, personalGrid: personal, pager: pager, personalPager: personalPager,
		categories: layoutRect{shared.x, filterY, gs, filterH}, personalCategories: layoutRect{personal.x, filterY, gs, filterH}, quickSlots: quick,
		headings:  [3]layoutRect{{paper.x, top, pw, labelH}, {personal.x, top, gs, labelH}, {shared.x, top, gs, labelH}},
		resources: resources,
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
		instructionH = uiTextCharHeight
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
		title:         layoutRect{blockX, blockY + 6, blockW, uiTextCharHeight},
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
	const cols, gap = 4, 12
	gridW := min(760, content.w*57/100)
	icon := min(170, (gridW-3*gap)/cols)
	icon = min(icon, max(64, (content.h-100)/2-32))
	gridW = cols*icon + 3*gap
	startX, startY := content.x+12, content.y+64
	cards := make([]layoutRect, MaxCardSlots)
	for i := range cards {
		cards[i] = layoutRect{startX + (i%cols)*(icon+gap), startY + (i/cols)*(icon+38), icon, icon}
	}
	sx := startX + gridW + 24
	return cardsContentLayout{
		title:    layoutRect{content.x + 12, content.y + 6, content.w - 24, 18},
		subtitle: layoutRect{content.x + 12, content.y + 30, content.w - 24, 18},
		cards:    cards, labelW: icon + 6,
		summary: layoutRect{sx, startY, content.right() - sx - 12, content.bottom() - startY - 8},
	}
}

const (
	questCardMaxW = 1120
	questCardGap  = 8
	questPagerH   = 22
	// Quest card chrome, measured from the card's own top: the name row sits
	// above the description, the progress row / bar / claim button below it.
	// A card's HEIGHT is these plus however many description rows it needs.
	questCardDescTop      = 26
	questCardProgressGap  = 18 // description bottom -> progress bar
	questCardBarH         = 20
	questCardBottomPad    = 8
	questCardBottomChrome = questCardProgressGap + questCardBarH + questCardBottomPad
	questCardLineHeight   = 16
	questCardMinPerPage   = 4
	questCardMaxDescRows  = 6 // sanity cap; hovering shows anything beyond it
)

// questCardHeight is the height a card needs to draw descRows description
// lines. THE one card-height formula, used by the page packer and the renderer.
func questCardHeight(descRows int) int {
	if descRows < 1 {
		descRows = 1
	}
	return questCardDescTop + descRows*questCardLineHeight + questCardBottomChrome
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
	textW := cardW - 36
	full := wrapUIText(description, textW)
	if len(full) == 0 {
		full = []string{""}
	}
	if maxDescRows < 1 {
		maxDescRows = 1
	}
	shown := truncateWrappedLines(full, maxDescRows, textW)
	return questCardCopy{descLines: shown, fullLines: full, height: questCardHeight(len(shown))}
}

const questRewardIconSize = 36
const questRewardIconStep = 42

// Reserve the claim button independently of status, so reward icons stay in
// place as a quest progresses from active to ready to claimed.
func questRewardIconColumns(cardW int) int {
	return max(1, (cardW/2-26-148)/questRewardIconStep)
}

func questCardCopyForQuest(q *quests.Quest, cardW, maxDescRows int) questCardCopy {
	c := questCardCopyFor(q.Description(), cardW, maxDescRows)
	n := len(q.Definition.Rewards.Items) + len(q.Definition.Rewards.ItemPool)
	if n > 0 {
		cols := questRewardIconColumns(cardW)
		rows := (n + cols - 1) / cols
		c.height += max(0, rows*questRewardIconStep-questCardBarH)
	}
	return c
}

func questRewardIconRect(r layoutRect, c questCardCopy, index int) layoutRect {
	cols := questRewardIconColumns(r.w)
	return layoutRect{r.x + r.w/2 + 10 + (index%cols)*questRewardIconStep,
		r.y + questCardDescTop + len(c.descLines)*questCardLineHeight + questCardProgressGap + (index/cols)*questRewardIconStep,
		questRewardIconSize, questRewardIconSize}
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
// while reserving room for at least four entries at supported resolutions.
func questCardMaxDescRowsFor(avail int) int {
	budget := (avail - (questCardMinPerPage-1)*questCardGap) / questCardMinPerPage
	rows := (budget - questCardDescTop - questCardBottomChrome) / questCardLineHeight
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
		title:        layoutRect{content.x + 20, content.y + 10, content.w - 40, uiTextCharHeight},
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
	titleW := dialog.w - 72
	balance := layoutRect{}
	if hasBalance {
		titleW = dialog.w - 220
		balance = layoutRect{dialog.right() - 190, dialog.y + 20, 140, uiTextCharHeight}
	}
	footerY := dialog.bottom() - 38
	return npcDialogSectionLayout{
		panel:    dialog,
		title:    layoutRect{dialog.x + 20, dialog.y + 20, titleW, uiTextCharHeight},
		balance:  balance,
		greeting: layoutRect{dialog.x + 20, dialog.y + 44, min(dialog.w-40, uiColumnsWidth(tabGreetingWrapColumns)), 2 * dialogueLineHeight},
		body:     layoutRect{dialog.x + 20, dialog.y + 92, dialog.w - 40, footerY - (dialog.y + 92) - 8},
		footer: [2]layoutRect{
			{dialog.x + 20, footerY, dialog.w - 40, uiTextCharHeight},
			{dialog.x + 20, footerY + uiTextCharHeight, dialog.w - 40, uiTextCharHeight},
		},
	}
}

func computeMapOverlayLayout(screenW, screenH int) mapOverlayLayout {
	panelW := min(720, max(320, int(float64(screenW)*0.75)))
	panelH := min(560, max(240, int(float64(screenH)*0.75)))
	panel := centeredRect(screenW, screenH, panelW, panelH)
	return mapOverlayLayout{
		panel: panel,
		title: layoutRect{panel.x + 16, panel.y + 12, panel.w - 58, uiTextCharHeight},
		close: layoutRect{panel.right() - 26, panel.y + 10, 16, 16},
		body:  layoutRect{panel.x + 18, panel.y + 36, panel.w - 36, panel.h - 54},
	}
}
