package game

// bookLayout is the shared open-book geometry used by the spellbook and trap
// book tabs: book placement, source-coordinate mappers for the 1024x512 book
// art, and the 2x2-per-page card grid metrics.
type bookLayout struct {
	bookX, bookY, bookW, bookH int
	scaleX, scaleY             float64

	cols, cardsPerPage int
	gridY              int
	cardW, cardH       int
	iconSize           int
	cardGap, rowGap    int
	gridW              int
	pageOriginX        [2]int
	gridMaxY           int
}

func computeBookLayout(panelX, contentY, contentHeight int) bookLayout {
	var l bookLayout
	l.bookX = panelX + 24
	// Push the book down so the bookmark flags have room between the menu tabs and the book.
	l.bookY = contentY + 60
	l.bookW = 652
	l.bookH = l.bookW / 2
	if maxBookH := contentHeight - 94; l.bookH > maxBookH {
		l.bookH = maxBookH
		l.bookW = l.bookH * 2
		l.bookX = panelX + (700-l.bookW)/2
	}
	l.scaleX = float64(l.bookW) / 1024.0
	l.scaleY = float64(l.bookH) / 512.0

	// 2x2 grid per page (left + right) = up to 8 cards visible at once.
	l.cols = 2
	l.cardsPerPage = 4
	l.gridY = l.srcY(118)
	l.cardW = l.srcW(180)
	l.cardH = l.srcH(150)
	l.iconSize = l.srcW(96)
	// Clamp icon size so name + stats rows fit below it without overlap at small scales.
	if maxIcon := l.cardH - 2*debugTextCharHeight - 12; l.iconSize > maxIcon {
		l.iconSize = maxIcon
	}
	if l.iconSize < 16 {
		l.iconSize = 16
	}
	l.cardGap = l.srcW(18)
	l.rowGap = l.srcH(14)
	// Centre the grid on the parchment area of each page. Source-coord centres
	// measured from the book sprite: left page parchment spans x=87..468
	// (centre 278), right page spans x=558..936 (centre 747).
	l.gridW = l.cols*l.cardW + (l.cols-1)*l.cardGap
	l.pageOriginX = [2]int{l.srcX(278) - l.gridW/2, l.srcX(747) - l.gridW/2}
	l.gridMaxY = l.srcY(460)
	return l
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
