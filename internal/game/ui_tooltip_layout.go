package game

import (
	"image/color"
	"strings"

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
}

func tooltipBodyColors(rows character.CardRows, accent color.Color) []color.Color {
	if accent == nil {
		accent = color.RGBA{224, 206, 158, 255}
	}
	r, g, b, _ := accent.RGBA()
	accent = color.RGBA{max(180, uint8(r>>8)), max(160, uint8(g>>8)), max(130, uint8(b>>8)), 255}
	colors := make([]color.Color, len(rows))
	for i, row := range rows {
		colors[i] = color.RGBA{185, 192, 204, 255}
		switch row.Kind {
		case character.CardRowTitle, character.CardRowResult:
			colors[i] = color.RGBA{250, 240, 214, 255}
		case character.CardRowSection:
			colors[i] = accent
		case character.CardRowFlavor:
			colors[i] = color.RGBA{205, 180, 115, 255}
		}
	}
	return colors
}

func layoutTooltip(lines []string, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	return layoutCardTooltip(character.PlainCardRows(lines), hasIcon, maxWidth, screenH)
}

func layoutCardTooltip(rows character.CardRows, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
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
		line := row.Text
		if row.Kind == character.CardRowSpacer {
			y += 6
			continue
		}
		remaining := strings.Join(strings.Fields(line), " ")
		for remaining != "" {
			x, available := 6, width-12
			if hasIcon && y < 6+tooltipIconSize+tooltipIconGap {
				available -= tooltipIconSize + tooltipIconGap
			}
			fragment := wrapUIText(remaining, max(uiTextCharWidth, available))[0]
			layout.rows = append(layout.rows, tooltipLayoutRow{fragment, x, y, available, i})
			remaining = strings.TrimSpace(remaining[len(fragment):])
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
	styles := tooltipBodyColors(rows, plate)
	drawFilledRect(screen, x, y, layout.w, layout.h, color.RGBA{30, 30, 60, 255})
	if icon != "" && sprites != nil {
		drawImageScaled(screen, sprites.GetSprite(icon), x+layout.w-tooltipIconSize-6, y+6, tooltipIconSize, tooltipIconSize)
	}
	for _, row := range layout.rows {
		textColor := styles[row.source]
		if len(colors) == len(rows) && colors[row.source] != nil {
			textColor = colors[row.source]
		}
		if rows[row.source].Kind == character.CardRowTitle {
			if plate != nil {
				drawMetalPlate(screen, x+row.x-4, y+row.y-2, row.w+4, layout.lineHeight+2, metalPlateBase(plate))
			}
			if title != nil {
				textColor = title
			}
		} else if rows[row.source].Kind == character.CardRowSection {
			textColor = styles[row.source]
			drawFilledRect(screen, x+row.x-2, y+row.y-1, row.w+2, layout.lineHeight, color.RGBA{48, 49, 76, 255})
		}
		drawUITextColored(screen, row.text, x+row.x, y+row.y, textColor)
	}
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
	var best tooltipPairGeometry
	for {
		pair := tooltipPairGeometry{mainCap: mainCap, compareCap: available - mainCap}
		pair.mainW, pair.mainH = ui.mainTooltipSize(pair.mainCap, screenH)
		pair.compareW, pair.compareH = cardTooltipBoxSize(ui.compareTooltipRows(), false, pair.compareCap, screenH)
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

func cardTooltipBoxSize(rows character.CardRows, hasIcon bool, maxWidth, screenH int) (int, int) {
	layout := layoutCardTooltip(rows, hasIcon, maxWidth, screenH)
	return layout.w, layout.h
}
