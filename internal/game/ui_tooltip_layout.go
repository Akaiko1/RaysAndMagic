package game

import (
	"hash/maphash"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/graphics"
)

// Cards keep results first within each mechanic. Their text reclaims
// the icon column below the image; the same rows drive measurement and drawing.
type tooltipLayoutRow struct {
	text    string
	x, y, w int
	source  int
}

type tooltipGeometry struct {
	rows       []tooltipLayoutRow
	w, h       int
	lineHeight int
	tinted     [][]styledRun[color.Color] // each laid row's runs in their school tints; nil = untinted
	tintsDone  bool                       // tinted is worked out: the card was drawn (drawnCardTooltip)
}

// tooltipAccent is the section tint of a card under a title plate: the
// plate's hue, kept light enough to read.
func tooltipAccent(plate color.Color) color.Color {
	if plate == nil {
		plate = color.RGBA{224, 206, 158, 255}
	}
	r, g, b, _ := plate.RGBA()
	return color.RGBA{max(180, uint8(r>>8)), max(160, uint8(g>>8)), max(130, uint8(b>>8)), 255}
}

// tooltipRowColor is a row's text color by its role.
func tooltipRowColor(kind character.CardRowKind, accent color.Color) color.Color {
	switch kind {
	case character.CardRowTitle, character.CardRowResult:
		return color.RGBA{250, 240, 214, 255}
	case character.CardRowSection:
		return accent
	case character.CardRowFlavor:
		return color.RGBA{205, 180, 115, 255}
	}
	return color.RGBA{185, 192, 204, 255}
}

func layoutTooltip(lines []string, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	return layoutCardTooltip(character.PlainCardRows(lines), hasIcon, maxWidth, screenH)
}

// tooltipLayoutKey names a laid-out card: its content, the room it had and
// the glyph widths it was measured in.
type tooltipLayoutKey struct {
	rows              uint64
	hasIcon           bool
	maxWidth, screenH int
	font              int
}

// tooltipLayouts keeps the cards laid out lately, so a card on screen is
// wrapped and tinted once, not every frame; a width only measured is never
// tinted. Keyed by content: hover code rebuilds the rows each frame, and an
// unchanged rebuild still hits.
var (
	tooltipLayouts    = map[tooltipLayoutKey]tooltipGeometry{}
	tooltipLayoutSeed = maphash.MakeSeed()
	tooltipSegments   []coloredTextSegment // drawTooltipLayout's scratch
)

// tooltipLayoutMemo bounds tooltipLayouts above one frame's needs: a
// comparison pair's width search lays both cards out every 28 px from half
// the screen to the comparison's minimum - about 124 layouts on a 3840 px
// wide interface. A smaller memo would clear itself every frame there.
const tooltipLayoutMemo = 256

// layoutCardTooltip is a card's geometry, what measuring needs; a drawn card
// takes drawnCardTooltip, which adds its tints.
func layoutCardTooltip(rows character.CardRows, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	layout, _ := memoCardTooltip(rows, hasIcon, maxWidth, screenH)
	return layout
}

// drawnCardTooltip is the card as drawn: its geometry with the rows' school
// tints, worked out once for each laid-out card that is drawn.
func drawnCardTooltip(rows character.CardRows, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	layout, key := memoCardTooltip(rows, hasIcon, maxWidth, screenH)
	if !layout.tintsDone {
		layout.tinted, layout.tintsDone = tooltipTintedFragments(rows, layout.rows), true
		tooltipLayouts[key] = layout
	}
	return layout
}

func memoCardTooltip(rows character.CardRows, hasIcon bool, maxWidth, screenH int) (tooltipGeometry, tooltipLayoutKey) {
	return memoHashedCardTooltip(cardRowsHash(rows), rows, hasIcon, maxWidth, screenH)
}

// memoHashedCardTooltip is memoCardTooltip for a card already hashed: a
// search over widths hashes its cards once.
func memoHashedCardTooltip(hash uint64, rows character.CardRows, hasIcon bool, maxWidth, screenH int) (tooltipGeometry, tooltipLayoutKey) {
	key := tooltipLayoutKey{hash, hasIcon, maxWidth, screenH, uiFontGeneration}
	if layout, ok := tooltipLayouts[key]; ok {
		return layout, key
	}
	layout := fitCardTooltip(rows, hasIcon, maxWidth, screenH)
	if len(tooltipLayouts) >= tooltipLayoutMemo {
		clear(tooltipLayouts)
	}
	tooltipLayouts[key] = layout
	return layout, key
}

// cardRowsHash is the content of rows the layout and its tints read.
func cardRowsHash(rows character.CardRows) uint64 {
	var h maphash.Hash
	h.SetSeed(tooltipLayoutSeed)
	for _, row := range rows {
		h.WriteString(row.Text)
		h.WriteByte(0)
		h.WriteByte(byte(row.Kind))
		for _, run := range row.Runs {
			h.WriteString(run.Text)
			h.WriteByte(0)
			h.WriteString(run.School)
			h.WriteByte(0)
			if run.Whole {
				h.WriteByte(1)
			}
		}
		h.WriteByte(0xff)
	}
	return h.Sum64()
}

func fitCardTooltip(rows character.CardRows, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	// Prefer a readable column, widening before tightening the line spacing.
	if len(rows) == 0 {
		return tooltipGeometry{}
	}
	naturalWidth, structured := 0, false
	for _, row := range rows {
		line := row.Text
		naturalWidth = max(naturalWidth, uiTextWidth(line)+12+tooltipTextOffset(hasIcon))
		structured = structured || row.Kind == character.CardRowSection
	}
	width := min(560, naturalWidth, maxWidth)
	if structured {
		width = min(560, maxWidth)
	}
	for _, spacing := range []int{16, 14} {
		for w := width; ; w = min(w+56, maxWidth) {
			layout := tooltipLayoutRows(rows, hasIcon, w, spacing)
			if layout.h <= screenH-2*tooltipScreenMargin || (w == maxWidth && spacing == 14) {
				return layout
			}
			if w == maxWidth {
				break
			}
		}
	}
	return tooltipGeometry{}
}

func tooltipLayoutRows(rows character.CardRows, hasIcon bool, width, spacing int) tooltipGeometry {
	layout := tooltipGeometry{w: width, lineHeight: spacing}
	y := 6
	for i, row := range rows {
		if row.Kind == character.CardRowSpacer {
			y += 6
			continue
		}
		for words := row.Words(); len(words) > 0; {
			x, available := 6, width-12
			if hasIcon && y < 6+tooltipIconSize+tooltipIconGap {
				available -= tooltipIconSize + tooltipIconGap
			}
			var fragment string
			fragment, words = firstUIWordsLine(words, max(uiTextCharWidth, available))
			layout.rows = append(layout.rows, tooltipLayoutRow{fragment, x, y, available, i})
			y += spacing
		}
	}
	layout.h = y + 6
	if hasIcon {
		layout.h = max(layout.h, tooltipIconSize+12)
	}
	return layout
}

func drawTooltipLayout(screen *ebiten.Image, rows character.CardRows, colors []color.Color, plate, title color.Color, icon string, x, y int, layout tooltipGeometry, sprites *graphics.SpriteManager) {
	accent := tooltipAccent(plate)
	drawFilledRect(screen, x, y, layout.w, layout.h, color.RGBA{30, 30, 60, 255})
	if icon != "" && sprites != nil {
		drawImageScaled(screen, sprites.GetSprite(icon), x+layout.w-tooltipIconSize-6, y+6, tooltipIconSize, tooltipIconSize)
	}
	for i, row := range layout.rows {
		kind := rows[row.source].Kind
		textColor := tooltipRowColor(kind, accent)
		if len(colors) == len(rows) && colors[row.source] != nil && kind != character.CardRowSection {
			textColor = colors[row.source]
		}
		if kind == character.CardRowTitle {
			if plate != nil {
				drawMetalPlate(screen, x+row.x-4, y+row.y-2, row.w+4, layout.lineHeight+2, metalPlateBase(plate))
			}
			if title != nil {
				textColor = title
			}
		} else if kind == character.CardRowSection {
			drawFilledRect(screen, x+row.x-2, y+row.y-1, row.w+2, layout.lineHeight, color.RGBA{48, 49, 76, 255})
		}
		if layout.tinted == nil || layout.tinted[i] == nil {
			drawUITextColored(screen, row.text, x+row.x, y+row.y, textColor)
			continue
		}
		segs := tooltipSegments[:0]
		for _, r := range layout.tinted[i] {
			tint := r.style
			if tint == nil {
				tint = textColor
			}
			segs = append(segs, coloredTextSegment{r.text, tint})
		}
		tooltipSegments = segs
		drawColoredTextSegments(screen, x+row.x, y+row.y, segs)
	}
}

// tooltipTintedFragments gives each wrapped fragment of a tinted card row its
// runs in their school tints (nil = the row's own color); untinted fragments
// stay nil.
func tooltipTintedFragments(rows character.CardRows, laid []tooltipLayoutRow) [][]styledRun[color.Color] {
	var out [][]styledRun[color.Color]
	for start := 0; start < len(laid); {
		end := start + 1
		for end < len(laid) && laid[end].source == laid[start].source {
			end++
		}
		if runs := rows[laid[start].source].Runs; len(runs) > 0 {
			if out == nil {
				out = make([][]styledRun[color.Color], len(laid))
			}
			texts := make([]string, end-start)
			for k := range texts {
				texts[k] = laid[start+k].text
			}
			copy(out[start:end], cardRunLines(runs, texts))
		}
		start = end
	}
	return out
}

// cardRunLines splits the wrapped lines of one card row back into its runs,
// each in its school tint (nil = the row's own color).
func cardRunLines(runs []character.CardRun, lines []string) [][]styledRun[color.Color] {
	styled := make([]styledRun[color.Color], len(runs))
	for k, r := range runs {
		styled[k] = styledRun[color.Color]{r.Text, nil}
		if r.School != "" {
			styled[k].style = SchoolTextColor(r.School)
		}
	}
	return styledLines(styled, func(string) []string { return lines })
}

// TintedRun is a piece of a card line in its school tint; nil Tint is the
// row's own color.
type TintedRun struct {
	Text string
	Tint color.Color
}

// CardRowLineRuns is what the game draws on each wrapped line of a card row
// with runs: the pieces in their school tints. The editor's catalog cards draw
// with it; a row without runs gives nil.
func CardRowLineRuns(row character.CardRow, lines []string) [][]TintedRun {
	if len(row.Runs) == 0 {
		return nil
	}
	styled := cardRunLines(row.Runs, lines)
	out := make([][]TintedRun, len(styled))
	for i, line := range styled {
		for _, r := range line {
			out[i] = append(out[i], TintedRun{r.text, r.style})
		}
	}
	return out
}

func (ui *UISystem) mainTooltipSize(maxWidth, screenH int) (int, int) {
	layout := layoutCardTooltip(ui.mainTooltipRows(), ui.tooltipIcon != "", maxWidth, screenH)
	return layout.w, layout.h
}

type tooltipPairGeometry struct {
	mainCap, compareCap int
	mainW, mainH        int
	compareW, compareH  int
}

func (ui *UISystem) queuedTooltipPairLayout(screenW, screenH int) tooltipPairGeometry {
	available := screenW - 2*tooltipScreenMargin - tooltipCompareGap
	mainCap := tooltipColumnWidth(screenW, 2)
	main, compare := ui.mainTooltipRows(), ui.compareTooltipRows()
	mainHash, compareHash := cardRowsHash(main), cardRowsHash(compare)
	var best tooltipPairGeometry
	for {
		pair := tooltipPairGeometry{mainCap: mainCap, compareCap: available - mainCap}
		mainLayout, _ := memoHashedCardTooltip(mainHash, main, ui.tooltipIcon != "", pair.mainCap, screenH)
		compareLayout, _ := memoHashedCardTooltip(compareHash, compare, false, pair.compareCap, screenH)
		pair.mainW, pair.mainH = mainLayout.w, mainLayout.h
		pair.compareW, pair.compareH = compareLayout.w, compareLayout.h
		if best.mainW == 0 || max(pair.mainH, pair.compareH) < max(best.mainH, best.compareH) {
			best = pair
		}
		if max(pair.mainH, pair.compareH) <= screenH-2*tooltipScreenMargin {
			return pair
		}
		// Long cards can borrow width from a shorter comparison, while
		// retaining a readable comparison column and the normal screen margins.
		if mainCap >= available-224 {
			return best
		}
		mainCap = min(mainCap+28, available-224)
	}
}

func (ui *UISystem) drawMainTooltip(screen *ebiten.Image, x, y, maxWidth int) {
	drawCardTooltip(screen, ui.mainTooltipRows(), ui.tooltipColors, ui.tooltipTitleColor, ui.tooltipTitleText, ui.tooltipIcon, x, y, x+maxWidth, ui.game.sprites)
}

// Legacy unstructured queues keep a plain-text projection for text consumers.
func (ui *UISystem) mainTooltipRows() character.CardRows {
	if ui.tooltipRows != nil {
		return ui.tooltipRows
	}
	return character.PlainCardRows(ui.tooltipLines)
}

func (ui *UISystem) compareTooltipRows() character.CardRows {
	if ui.tooltipCompareRows != nil {
		return ui.tooltipCompareRows
	}
	return character.PlainCardRows(ui.tooltipCompareLines)
}
