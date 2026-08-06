package game

// bookLayout is the shared open-book geometry used by the spellbook and trap
// book tabs: book placement, source-coordinate mappers for the 1024x512 book
// art, and the 2x2-per-page card grid metrics.
type bookLayout struct {
	content                        layoutRect
	header, quick, pager, controls layoutRect
	bookX, bookY, bookW, bookH     int
	scaleX, scaleY                 float64

	cols, cardsPerPage int
	gridY              int
	cardW, cardH       int
	iconSize           int
	cardGap, rowGap    int
	gridW              int
	pageOriginX        [2]int
	gridMaxY           int
}

const (
	bookLeftPageInnerX  = 87
	bookRightPageInnerX = 558
	bookPageInnerY      = 64
	bookPageInnerW      = 381
	bookPageInnerBottom = 460
	bookSpellGridTopY   = 90
	bookSpellCardH      = 158
	bookSchoolTabW      = 72
	bookSchoolTabH      = 112
	bookSchoolTabGap    = 10
	bookSchoolTabStartX = 40
	bookSchoolTabLift   = 10
)

func computeBookLayout(content layoutRect) bookLayout {
	var l bookLayout
	l.content = content
	l.header = layoutRect{content.x + 20, content.y + 4, content.w - 40, 20}
	const (
		topReserve    = 90
		footerReserve = 96
		maxBookH      = 640
	)
	l.bookH = min(maxBookH, min(content.h-topReserve-footerReserve, (content.w-32)/2))
	if l.bookH < 1 {
		l.bookH = 1
	}
	l.bookW = l.bookH * 2
	l.bookX = content.x + (content.w-l.bookW)/2
	l.bookY = content.y + topReserve
	l.scaleX = float64(l.bookW) / 1024.0
	l.scaleY = float64(l.bookH) / 512.0

	// 2x2 grid per page (left + right) = up to 8 cards visible at once.
	l.cols = 2
	l.cardsPerPage = 4
	l.gridY = l.srcY(bookSpellGridTopY)
	l.cardW = l.srcW(162)
	l.cardH = l.srcH(bookSpellCardH)
	l.iconSize = l.srcW(100)
	// Clamp icon size so name + stats rows fit below it without overlap at small scales.
	if maxIcon := l.cardH - 2*debugTextCharHeight - 12; l.iconSize > maxIcon {
		l.iconSize = maxIcon
	}
	if l.iconSize < 16 {
		l.iconSize = 16
	}
	l.cardGap = l.srcW(18)
	l.rowGap = l.srcH(12)
	// Centre the grid inside the measured inner parchment frame on each page.
	// These bounds are also the clipping contract verified by layout tests.
	l.gridW = l.cols*l.cardW + (l.cols-1)*l.cardGap
	leftInner := l.pageInnerRect(0)
	rightInner := l.pageInnerRect(1)
	l.pageOriginX = [2]int{leftInner.x + (leftInner.w-l.gridW)/2, rightInner.x + (rightInner.w-l.gridW)/2}
	l.gridMaxY = l.srcY(bookPageInnerBottom)
	quickW := min(360, max(240, l.bookW/3))
	l.quick = layoutRect{l.bookX + (l.bookW-quickW)/2, l.bookY + l.bookH + 8, quickW, int(float64(quickW) / quickSlotBarAspect)}
	l.pager = layoutRect{l.bookX + l.bookW - 180, l.quick.y + (l.quick.h-pagerBtnH)/2, 180, pagerBtnH}
	l.controls = layoutRect{content.x + 20, content.bottom() - debugTextCharHeight, content.w - 40, debugTextCharHeight}
	return l
}

func (l bookLayout) cardsPerSpread() int { return 2 * l.cardsPerPage }

// schoolTabRects is the single geometry source for both bookmark drawing and
// input. The whole drawn sprite is interactive; its lower inserted portion is
// still safely above the first spell-card row.
func (l bookLayout) schoolTabRects(count, selected int) []layoutRect {
	if count <= 0 {
		return nil
	}
	tabW := max(44, l.srcW(bookSchoolTabW))
	tabH := max(68, l.srcH(bookSchoolTabH))
	gap := max(4, l.srcW(bookSchoolTabGap))
	startX := l.bookX + l.srcW(bookSchoolTabStartX)
	hiddenH := int(float64(tabH) * 0.45)
	baseY := l.bookY - (tabH - hiddenH)
	rects := make([]layoutRect, count)
	for i := range rects {
		y := baseY
		if i == selected {
			y -= l.srcH(bookSchoolTabLift)
		}
		rects[i] = layoutRect{x: startX + i*(tabW+gap), y: y, w: tabW, h: tabH}
	}
	return rects
}

func (l bookLayout) pageInnerRect(page int) layoutRect {
	x := bookLeftPageInnerX
	if page == 1 {
		x = bookRightPageInnerX
	}
	return layoutRect{
		x: l.srcX(x),
		y: l.srcY(bookPageInnerY),
		w: l.srcW(bookPageInnerW),
		h: l.srcY(bookPageInnerBottom) - l.srcY(bookPageInnerY),
	}
}

// srcX/srcY/srcW/srcH map 1024x512 book-art source coordinates to screen.
func (l bookLayout) srcX(v int) int { return l.bookX + int(float64(v)*l.scaleX) }
func (l bookLayout) srcY(v int) int { return l.bookY + int(float64(v)*l.scaleY) }
func (l bookLayout) srcW(v int) int { return int(float64(v) * l.scaleX) }
func (l bookLayout) srcH(v int) int { return int(float64(v) * l.scaleY) }

// cardPos returns the top-left of card i in book order (left page fills first).
func (l bookLayout) cardPos(i int) (x, y int) {
	page := i / l.cardsPerPage
	local := i % l.cardsPerPage
	x = l.pageOriginX[page] + (local%l.cols)*(l.cardW+l.cardGap)
	y = l.gridY + (local/l.cols)*(l.cardH+l.rowGap)
	return x, y
}
