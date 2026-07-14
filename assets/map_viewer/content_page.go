package main

// Items & Spells page: top tab bar shared with the Maps page, plus a
// scrollable grid of content cards grouped under section headers, with a
// hover tooltip showing full data for the card under the cursor.

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

// drawPageBar draws the top-level tabs (Maps / Items / Spells / Characters / Skills).
func (v *viewer) drawPageBar(screen *ebiten.Image) {
	drawFilledRect(screen, 0, 0, windowWidth, pageBarHeight, color.RGBA{24, 24, 36, 255})
	rects := pageTabLayout()
	for i, r := range rects {
		def := pageTabDefs[i]
		bg := color.RGBA{36, 36, 52, 255}
		if v.page == def.page {
			bg = color.RGBA{60, 80, 130, 255}
		}
		drawFilledRect(screen, r.x, 4, r.w, pageBarHeight-8, bg)
		drawRectBorder(screen, r.x, 4, r.w, pageBarHeight-8, 1, color.RGBA{90, 90, 110, 255})
		ebitenutil.DebugPrintAt(screen, def.label, r.x+10, 10)
		ebitenutil.DebugPrintAt(screen, def.hotkey, r.x+r.w-22, 10)
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
		if cardY+contentCardH >= areaY && cardY < areaY+areaH {
			v.drawCard(clip, card, cardX, cardY)
		}
		if pointInRect(mouseX, mouseY, cardX, cardY, contentCardW, contentCardH) && mouseY >= areaY && mouseY < areaY+areaH {
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
func (v *viewer) drawCard(dst *ebiten.Image, c *contentCard, x, y int) {
	drawFilledRect(dst, x, y, contentCardW, contentCardH, color.RGBA{32, 32, 44, 255})
	drawRectBorder(dst, x, y, contentCardW, contentCardH, 1, color.RGBA{72, 72, 92, 255})

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

// drawCardTooltip draws a multi-line tooltip near the cursor with full card
// data. Positioned to stay within the content area bounds.
func drawCardTooltip(screen *ebiten.Image, c *contentCard, mouseX, mouseY, areaX, areaW int) {
	lines := []string{c.name}
	if c.section != "" {
		lines = append(lines, "  ["+c.section+"]")
	}
	if c.description != "" {
		lines = append(lines, "")
		lines = append(lines, wrapTooltipLines(c.description, 64)...)
	}
	if c.flavor != "" {
		lines = append(lines, "")
		lines = append(lines, wrapTooltipLines(`"`+c.flavor+`"`, 64)...)
	}
	if len(c.tooltipRows) > 0 {
		lines = append(lines, "")
		lines = append(lines, c.tooltipRows...)
	}

	const lineH = 14
	maxLineW := 0
	for _, ln := range lines {
		if w := utf8.RuneCountInString(ln) * 7; w > maxLineW {
			maxLineW = w
		}
	}
	boxW := maxLineW + 16
	boxH := len(lines)*lineH + 12

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

	drawFilledRect(screen, boxX, boxY, boxW, boxH, color.RGBA{18, 18, 28, 240})
	drawRectBorder(screen, boxX, boxY, boxW, boxH, 1, color.RGBA{120, 120, 150, 255})
	for i, ln := range lines {
		if i == 0 {
			// Name line wears the game's rarity metal (gradient for metal tiers).
			game.DrawShadedText(screen, ln, boxX+8, boxY+6, game.RarityColor(c.rarity))
			continue
		}
		ebitenutil.DebugPrintAt(screen, ln, boxX+8, boxY+6+i*lineH)
	}
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
