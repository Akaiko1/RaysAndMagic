package game

import (
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
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

func tooltipSectionHeading(line string) bool {
	switch line {
	case "DAMAGE", "DAMAGE PER TICK", "HEALING", "CRITICAL", "ATTACK", "EFFECTS",
		"DEFENSE", "CASTING", "ZONE", "PLACEMENT", "CONTROL", "USAGE", "REQUIREMENTS",
		"RECOVERY", "DURATION", "MASTERY", "GRANDMASTER", "TRIGGER", "LIMITS",
		"ATTRIBUTES", "RESISTANCES", "CHANGES", "REAL TIME", "TURN BASED", equipmentSetSectionTitle:
		return true
	}
	return false
}

func tooltipBodyColors(lines []string, accent color.Color) []color.Color {
	if accent == nil {
		accent = color.RGBA{224, 206, 158, 255}
	}
	r, g, b, _ := accent.RGBA()
	// School and wood nameplates can be dark; section text must remain legible.
	accent = color.RGBA{max(180, uint8(r>>8)), max(160, uint8(g>>8)), max(130, uint8(b>>8)), 255}
	colors := make([]color.Color, len(lines))
	for i, line := range lines {
		colors[i] = color.RGBA{185, 192, 204, 255}
		if i == 0 {
			colors[i] = color.RGBA{250, 240, 214, 255}
		}
		if tooltipSectionHeading(line) {
			colors[i] = accent
		}
		for _, prefix := range []string{"Total Damage:", "Critical Damage:", "Chance:", "RT Cooldown:", "Range:", "Strikes per attack:", "Total Healing:", "Total per tick:", "Total Stun:", "Total Root:", "Item Armor Class:", "Cost:", "Damage:", "Current ", "Base Duration:", "Radius:", "Target:", "Targets:", "Restores ", "Summon HP:", "Summon Damage:"} {
			if strings.HasPrefix(line, prefix) {
				colors[i] = color.RGBA{250, 240, 214, 255}
			}
		}
	}
	return colors
}

func layoutTooltip(lines []string, hasIcon bool, maxWidth, screenH int) tooltipGeometry {
	// Prefer a readable column, widening before tightening the line spacing.
	if len(lines) == 0 {
		return tooltipGeometry{}
	}
	naturalWidth, structured := 0, false
	for _, line := range lines {
		naturalWidth = max(naturalWidth, debugTextWidth(line)+12+tooltipTextOffset(hasIcon))
		structured = structured || tooltipSectionHeading(line)
	}
	width := min(560, naturalWidth, maxWidth)
	if structured {
		width = min(560, maxWidth)
	}
	for _, spacing := range []int{16, 14} {
		for w := width; ; w = min(w+56, maxWidth) {
			layout := tooltipLayoutRows(lines, hasIcon, w, spacing)
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

func tooltipLayoutRows(lines []string, hasIcon bool, width, spacing int) tooltipGeometry {
	layout := tooltipGeometry{w: width, lineHeight: spacing}
	y := 6
	for i, line := range lines {
		if line == "" {
			y += 6
			continue
		}
		remaining := strings.Join(strings.Fields(line), " ")
		for remaining != "" {
			x, available := 6, width-12
			if hasIcon && y < 6+tooltipIconSize+tooltipIconGap {
				available -= tooltipIconSize + tooltipIconGap
			}
			chars := max(1, available/debugTextCharWidth)
			fragment := wrapText(remaining, chars)[0]
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

func drawTooltipLayout(screen *ebiten.Image, lines []string, colors []color.Color, plate, title color.Color, icon string, x, y int, layout tooltipGeometry, sprites *graphics.SpriteManager) {
	styles := tooltipBodyColors(lines, plate)
	drawFilledRect(screen, x, y, layout.w, layout.h, color.RGBA{30, 30, 60, 255})
	if icon != "" && sprites != nil {
		drawImageScaled(screen, sprites.GetSprite(icon), x+layout.w-tooltipIconSize-6, y+6, tooltipIconSize, tooltipIconSize)
	}
	for _, row := range layout.rows {
		textColor := styles[row.source]
		if len(colors) == len(lines) && colors[row.source] != nil {
			textColor = colors[row.source]
		}
		if row.source == 0 {
			if plate != nil {
				drawMetalPlate(screen, x+row.x-4, y+row.y-2, row.w+4, layout.lineHeight+2, metalPlateBase(plate))
			}
			if title != nil {
				textColor = title
			}
		} else if tooltipSectionHeading(lines[row.source]) {
			textColor = styles[row.source]
			drawFilledRect(screen, x+row.x-2, y+row.y-1, row.w+2, layout.lineHeight, color.RGBA{48, 49, 76, 255})
		}
		drawDebugTextColored(screen, row.text, x+row.x, y+row.y, textColor)
	}
}

func (ui *UISystem) mainTooltipSize(maxWidth, screenH int) (int, int) {
	layout := layoutTooltip(ui.tooltipLines, ui.tooltipIcon != "", maxWidth, screenH)
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
		pair.compareW, pair.compareH = tooltipBoxSizeForScreen(ui.tooltipCompareLines, ui.tooltipCompareColors, false, 0, pair.compareCap, screenH)
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
	drawTooltip(screen, ui.tooltipLines, ui.tooltipColors, ui.tooltipTitleColor, ui.tooltipTitleText, ui.tooltipIcon, x, y, x+maxWidth, ui.game.sprites)
}
