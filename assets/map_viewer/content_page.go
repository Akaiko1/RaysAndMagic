package main

// Shared top page bar and scrollable catalog-card layout. Hover tooltips show
// the full data for the card under the cursor.

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"ugataima/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Layout constants for the content grid.
const (
	contentPad              = 16
	contentCardW            = 220
	contentCardH            = 96
	contentCardGap          = 12
	contentIconSize         = 64
	contentSectionH         = 28
	contentSectionMarginTop = 10
	contentSectionMarginBot = 6
)

// pageTabRect is one tab's hit-box, shared by draw and click handling so they
// can never drift apart.
type pageTabRect struct {
	page int
	x, w int
}

// pageTabLayout computes the on-screen rect of every top tab.
func pageTabLayout() []pageTabRect {
	rects := make([]pageTabRect, 0, len(pageTabDefs))
	x := 8
	for _, def := range pageTabDefs {
		w := utf8.RuneCountInString(def.label)*7 + 60
		rects = append(rects, pageTabRect{page: def.page, x: x + 4, w: w})
		x += 4 + w + 4
	}
	return rects
}

// drawPageBar draws every top-level tab defined by pageTabDefs.
func (v *viewer) drawPageBar(screen *ebiten.Image) {
	drawFilledRect(screen, 0, 0, windowWidth, pageBarHeight, color.RGBA{24, 24, 36, 255})
	rects := pageTabLayout()
	mouseX, mouseY := ebiten.CursorPosition()
	for i, r := range rects {
		def := pageTabDefs[i]
		bg := color.RGBA{36, 36, 52, 255}
		hovered := mouseY >= 4 && mouseY < pageBarHeight-4 && mouseX >= r.x && mouseX < r.x+r.w
		if hovered {
			bg = color.RGBA{46, 50, 68, 255}
		}
		if v.page == def.page {
			bg = color.RGBA{52, 64, 92, 255}
		}
		drawFilledRect(screen, r.x, 4, r.w, pageBarHeight-8, bg)
		drawRectBorder(screen, r.x, 4, r.w, pageBarHeight-8, 1, color.RGBA{90, 90, 110, 255})
		if v.page == def.page {
			drawFilledRect(screen, r.x+1, pageBarHeight-7, r.w-2, 3, viewerHeaderTextColor)
		}
		game.DrawShadedText(screen, def.label, r.x+10, 10, color.RGBA{225, 225, 235, 255})
		game.DrawShadedText(screen, def.hotkey, r.x+r.w-22, 10, color.RGBA{135, 145, 170, 255})
	}
}

// handlePageBarClick switches pages if a click landed on a tab.
func (v *viewer) handlePageBarClick() {
	mx, my := ebiten.CursorPosition()
	if my < 0 || my >= pageBarHeight {
		return
	}
	for _, r := range pageTabLayout() {
		if mx >= r.x && mx < r.x+r.w {
			v.page = r.page
			return
		}
	}
}

// drawContentPage renders the scrollable grid of cards.
func (v *viewer) drawContentPage(screen *ebiten.Image) {
	cards := v.pageCards[v.page]
	contentScroll := v.pageScroll[v.page]
	if len(cards) == 0 {
		ebitenutil.DebugPrintAt(screen, "no content loaded", contentPad, pageBarHeight+contentPad)
		return
	}

	areaX := contentPad
	areaY := pageBarHeight + contentPad
	areaW := windowWidth - 2*contentPad
	areaH := windowHeight - areaY - contentPad

	clip := screen.SubImage(image.Rect(areaX, areaY, areaX+areaW, areaY+areaH)).(*ebiten.Image)
	clip.Fill(color.RGBA{20, 20, 30, 255})

	cardsPerRow := (areaW + contentCardGap) / (contentCardW + contentCardGap)
	if cardsPerRow < 1 {
		cardsPerRow = 1
	}

	mouseX, mouseY := ebiten.CursorPosition()
	var hovered *contentCard

	y := areaY - contentScroll
	prevSection := ""
	colInRow := 0
	for i := range cards {
		card := &cards[i]

		if card.section != prevSection {
			if prevSection != "" {
				// Bring `y` to next row baseline if a partial row was in flight.
				if colInRow != 0 {
					y += contentCardH + contentCardGap
					colInRow = 0
				}
				y += contentSectionMarginTop
			}
			drawSectionHeader(clip, card.section, areaX, y, areaW)
			y += contentSectionH + contentSectionMarginBot
			prevSection = card.section
			colInRow = 0
		}

		cardX := areaX + colInRow*(contentCardW+contentCardGap)
		cardY := y
		isHovered := pointInRect(mouseX, mouseY, cardX, cardY, contentCardW, contentCardH) &&
			mouseY >= areaY && mouseY < areaY+areaH
		if cardY+contentCardH >= areaY && cardY < areaY+areaH {
			v.drawCard(clip, card, cardX, cardY, isHovered)
		}
		if isHovered {
			hovered = card
		}

		colInRow++
		if colInRow >= cardsPerRow {
			colInRow = 0
			y += contentCardH + contentCardGap
		}
	}

	if hovered != nil {
		drawCardTooltip(screen, hovered, mouseX, mouseY, areaX, areaW)
	}
}

// maxContentScroll computes how far down the user can scroll the content
// grid. Returns 0 if all rows fit on screen.
func (v *viewer) maxContentScroll() int {
	if len(v.pageCards[v.page]) == 0 {
		return 0
	}
	areaW := windowWidth - 2*contentPad
	cardsPerRow := (areaW + contentCardGap) / (contentCardW + contentCardGap)
	if cardsPerRow < 1 {
		cardsPerRow = 1
	}
	areaH := windowHeight - (pageBarHeight + contentPad) - contentPad

	total := 0
	prevSection := ""
	colInRow := 0
	for _, card := range v.pageCards[v.page] {
		if card.section != prevSection {
			if prevSection != "" {
				if colInRow != 0 {
					total += contentCardH + contentCardGap
					colInRow = 0
				}
				total += contentSectionMarginTop
			}
			total += contentSectionH + contentSectionMarginBot
			prevSection = card.section
			colInRow = 0
		}
		colInRow++
		if colInRow >= cardsPerRow {
			colInRow = 0
			total += contentCardH + contentCardGap
		}
	}
	if colInRow > 0 {
		total += contentCardH
	}
	if total <= areaH {
		return 0
	}
	return total - areaH
}

func drawSectionHeader(dst *ebiten.Image, label string, x, y, w int) {
	drawHeaderBandRect(dst, x, y, w, contentSectionH)
	ebitenutil.DebugPrintAt(dst, label, x+10, y+7)
}

// drawCard renders a single card: icon on the left, name + subtitle stacked
// on the right. Cards have a soft border so they read as discrete entities.
func (v *viewer) drawCard(dst *ebiten.Image, c *contentCard, x, y int, hovered bool) {
	bg := color.RGBA{32, 32, 44, 255}
	border := color.RGBA{72, 72, 92, 255}
	if hovered {
		bg = color.RGBA{39, 42, 56, 255}
		border = color.RGBA{105, 145, 185, 255}
	}
	drawFilledRect(dst, x, y, contentCardW, contentCardH, bg)
	drawRectBorder(dst, x, y, contentCardW, contentCardH, 1, border)
	drawFilledRect(dst, x+1, y+1, 3, contentCardH-2, cardAccentColor(c))

	// Icon area: centered vertically on the left.
	iconX := x + 8
	iconY := y + (contentCardH-contentIconSize)/2
	if icon := v.iconForCard(c); icon != nil {
		drawImageScaled(dst, icon, iconX, iconY, contentIconSize, contentIconSize)
	} else {
		// Placeholder so the layout doesn't collapse when art is missing.
		drawFilledRect(dst, iconX, iconY, contentIconSize, contentIconSize, color.RGBA{52, 52, 68, 255})
		drawRectBorder(dst, iconX, iconY, contentIconSize, contentIconSize, 1, color.RGBA{80, 80, 100, 255})
		ebitenutil.DebugPrintAt(dst, "?", iconX+contentIconSize/2-3, iconY+contentIconSize/2-7)
	}

	textX := x + 8 + contentIconSize + 10
	textY := y + 8
	// Wrap the subtitle to the text column width instead of truncating, so long
	// stat lines like "Dmg 7  Range 6  +Intellect" stay fully readable.
	textW := contentCardW - (8 + contentIconSize + 10) - 8
	maxChars := textW / 7
	if maxChars < 6 {
		maxChars = 6
	}
	game.DrawShadedText(dst, truncate(c.name, maxChars), textX, textY, game.RarityColor(c.rarity))
	lines := wrapTooltipLines(c.subtitle, maxChars)
	const maxSubtitleLines = 4 // card height fits name + ~4 wrapped lines
	for i, ln := range lines {
		if i >= maxSubtitleLines {
			break
		}
		ebitenutil.DebugPrintAt(dst, ln, textX, textY+18+i*14)
	}
}

type tooltipLineKind int

const (
	tooltipLineBody tooltipLineKind = iota
	tooltipLineTitle
	tooltipLineCategory
	tooltipLineDescription
	tooltipLineFlavor
	tooltipLineSection
	tooltipLineSpacer
)

type tooltipLine struct {
	text string
	kind tooltipLineKind
}

func isTooltipSection(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.Contains(trimmed, ":") {
		return false
	}
	hasLetter := false
	for _, r := range trimmed {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return hasLetter
}

func cardTooltipLines(c *contentCard) []tooltipLine {
	lines := []tooltipLine{{text: c.name, kind: tooltipLineTitle}}
	if c.section != "" {
		lines = append(lines, tooltipLine{text: c.section, kind: tooltipLineCategory})
	}
	appendSpacer := func() {
		if len(lines) > 0 && lines[len(lines)-1].kind != tooltipLineSpacer {
			lines = append(lines, tooltipLine{kind: tooltipLineSpacer})
		}
	}
	if c.description != "" {
		appendSpacer()
		for _, line := range wrapTooltipLines(c.description, 64) {
			lines = append(lines, tooltipLine{text: line, kind: tooltipLineDescription})
		}
	}
	if c.flavor != "" {
		appendSpacer()
		for _, line := range wrapTooltipLines(`"`+c.flavor+`"`, 64) {
			lines = append(lines, tooltipLine{text: line, kind: tooltipLineFlavor})
		}
	}
	if len(c.tooltipRows) > 0 {
		appendSpacer()
		for _, text := range c.tooltipRows {
			kind := tooltipLineBody
			switch {
			case text == "":
				kind = tooltipLineSpacer
			case isTooltipSection(text):
				kind = tooltipLineSection
			}
			lines = append(lines, tooltipLine{text: text, kind: kind})
		}
	}
	return lines
}

func tooltipLineHeight(kind tooltipLineKind) int {
	switch kind {
	case tooltipLineTitle:
		return 18
	case tooltipLineSection:
		return 18
	case tooltipLineSpacer:
		return 7
	default:
		return 14
	}
}

func cardTooltipSize(c *contentCard) (width, height int) {
	lines := cardTooltipLines(c)
	maxLineW := 0
	for _, line := range lines {
		if w := utf8.RuneCountInString(line.text) * 7; w > maxLineW {
			maxLineW = w
		}
	}
	height = 16
	for _, line := range lines {
		height += tooltipLineHeight(line.kind)
	}
	return maxLineW + 24, height
}

// drawCardTooltip draws a multi-line tooltip near the cursor with full card
// data. Positioned to stay within the content area bounds.
func drawCardTooltip(screen *ebiten.Image, c *contentCard, mouseX, mouseY, areaX, areaW int) {
	lines := cardTooltipLines(c)
	boxW, boxH := cardTooltipSize(c)

	boxX := mouseX + 16
	boxY := mouseY + 12
	if boxX+boxW > areaX+areaW {
		boxX = mouseX - boxW - 8
	}
	if boxX < 4 {
		boxX = 4
	}
	if boxY+boxH > windowHeight-4 {
		boxY = windowHeight - boxH - 4
	}
	if boxY < pageBarHeight+4 {
		boxY = pageBarHeight + 4
	}

	drawFilledRect(screen, boxX, boxY, boxW, boxH, color.RGBA{16, 17, 25, 248})
	drawRectBorder(screen, boxX, boxY, boxW, boxH, 1, color.RGBA{100, 120, 150, 255})
	drawFilledRect(screen, boxX+1, boxY+1, boxW-2, 3, cardAccentColor(c))

	y := boxY + 8
	for _, line := range lines {
		h := tooltipLineHeight(line.kind)
		switch line.kind {
		case tooltipLineTitle:
			game.DrawShadedText(screen, line.text, boxX+10, y, game.RarityColor(c.rarity))
		case tooltipLineCategory:
			game.DrawShadedText(screen, strings.ToUpper(line.text), boxX+10, y, color.RGBA{130, 145, 175, 255})
		case tooltipLineDescription:
			game.DrawShadedText(screen, line.text, boxX+10, y, color.RGBA{215, 215, 225, 255})
		case tooltipLineFlavor:
			game.DrawShadedText(screen, line.text, boxX+10, y, color.RGBA{205, 180, 115, 255})
		case tooltipLineSection:
			drawTooltipSectionLine(screen, line.text, boxX+7, y, boxW-14, h)
		case tooltipLineBody:
			drawTooltipBodyLine(screen, line.text, boxX+10, y)
		}
		y += h
	}
}

func cardAccentColor(c *contentCard) color.RGBA {
	if c != nil && c.rarity != "" {
		r, g, b, a := game.RarityColor(c.rarity).RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	}
	if c != nil {
		switch c.kind {
		case cardWeapon:
			return color.RGBA{210, 105, 80, 255}
		case cardSpell:
			return color.RGBA{95, 155, 225, 255}
		case cardSkill:
			return viewerHeaderTextColor
		}
	}
	return color.RGBA{165, 175, 195, 255}
}

func drawTooltipBodyLine(screen *ebiten.Image, text string, x, y int) {
	label, value, ok := strings.Cut(text, ":")
	if !ok || strings.TrimSpace(label) == "" || strings.TrimSpace(value) == "" {
		game.DrawShadedText(screen, text, x, y, color.RGBA{225, 225, 235, 255})
		return
	}
	label += ":"
	game.DrawShadedText(screen, label, x, y, color.RGBA{145, 170, 205, 255})
	valueX := x + utf8.RuneCountInString(label)*7 + 5
	game.DrawShadedText(screen, strings.TrimSpace(value), valueX, y, color.RGBA{225, 225, 235, 255})
}

func drawTooltipSectionLine(screen *ebiten.Image, text string, x, y, w, h int) {
	drawHeaderBandForTextRow(screen, x, y, w, h)
	game.DrawShadedText(screen, text, x+4, y, viewerHeaderTextColor)
}

// wrapTooltipLines does a simple word-wrap to keep tooltips readable.
func wrapTooltipLines(s string, maxRunes int) []string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return []string{s}
	}
	words := strings.Fields(s)
	var lines []string
	current := ""
	for _, w := range words {
		if current == "" {
			current = w
			continue
		}
		if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(w) > maxRunes {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	if maxRunes < 1 {
		return ""
	}
	r := []rune(s)
	// "..." is 3 runes; reserve room for it so the result never exceeds
	// maxRunes. Too narrow for text + ellipsis -> hard-cut to maxRunes.
	if maxRunes <= 3 {
		return string(r[:maxRunes])
	}
	return string(r[:maxRunes-3]) + "..."
}
