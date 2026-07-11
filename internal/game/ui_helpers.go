package game

import (
	"image"
	"image/color"
	"log"
	"strings"
	"unicode/utf8"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type coloredTextSegment struct {
	text  string
	color color.Color
}

var missingTooltipIcons = make(map[string]bool)

func drawColoredTextSegments(screen *ebiten.Image, x, y int, segments []coloredTextSegment) {
	curX := x
	for _, seg := range segments {
		drawDebugTextColored(screen, seg.text, curX, y, seg.color)
		curX += debugTextWidth(seg.text)
	}
}

// partyPortraitLayout returns the fixed-pixel party-portrait layout, centered
// horizontally and anchored to the bottom of the (possibly fullscreen) viewport.
// Portrait width comes from UIConfig (not derived from screen width) so going
// fullscreen does not stretch the party row - it stays at its design size and
// the row is centered with empty side margins.
func partyPortraitLayout(g *MMGame) (portraitWidth, portraitHeight, baseLeft, startY int) {
	portraitWidth = g.config.UI.PartyPortraitWidth
	if portraitWidth <= 0 {
		portraitWidth = g.config.GetScreenWidth() / 4 // safety fallback for old configs
	}
	portraitHeight = g.config.UI.PartyPortraitHeight
	baseLeft = (g.config.GetScreenWidth() - portraitWidth*4) / 2
	if baseLeft < 0 {
		baseLeft = 0
	}
	startY = g.config.GetScreenHeight() - portraitHeight
	return
}

// wrapText delegates to the standalone wrapText function in ui_dialogs.go
func (ui *UISystem) wrapText(text string, maxWidth int) []string {
	return wrapText(text, maxWidth)
}

// Merchant buy/sell grid geometry. Two side-by-side icon grids (buy left, sell
// right), each merchantGridColsxmerchantGridRows, mirroring the inventory grid.
const (
	merchantGridCols = 4
	merchantGridRows = 3
	merchantPageSize = merchantGridCols * merchantGridRows
	merchantIconSize = 46
	merchantIconGapX = 10
	merchantRowGap   = 10
	merchantPriceH   = 14 // price line drawn under each icon
	merchantGridW    = merchantGridCols*merchantIconSize + (merchantGridCols-1)*merchantIconGapX
)

// merchantGridLayout returns the two grid origins, the grid top, and the pager
// row Y. Single source for both the renderer and the click handler so cell rects
// never drift from drawn pixels.
func merchantGridLayout(dialogX, dialogY int) (leftX, rightX, gridTop, pagerY int) {
	leftX = dialogX + 40
	rightX = dialogX + npcDialogWidth/2 + 26
	// Low enough that a two-line greeting clears the "For Sale"/"Your Items"
	// headers (at gridTop-24), which in turn clear the grid below.
	gridTop = dialogY + 108
	stride := merchantIconSize + merchantPriceH + merchantRowGap
	pagerY = gridTop + merchantGridRows*stride + 2
	return
}

// merchantCellRect returns the icon rect for slot (0..merchantPageSize-1) in a
// grid based at baseX/gridTop.
func merchantCellRect(baseX, gridTop, slot int) (x, y, w, h int) {
	col := slot % merchantGridCols
	row := slot / merchantGridCols
	stride := merchantIconSize + merchantPriceH + merchantRowGap
	x = baseX + col*(merchantIconSize+merchantIconGapX)
	y = gridTop + row*stride
	return x, y, merchantIconSize, merchantIconSize
}

// pageCount returns the number of pages needed for n items at pageSize per page
// (minimum 1, so an empty list still has a valid page 0).
func pageCount(n, pageSize int) int {
	p := (n + pageSize - 1) / pageSize
	if p < 1 {
		p = 1
	}
	return p
}

// clampPage keeps *page within [0, total-1] - call every frame so a page stays
// valid when its backing list shrinks (item bought/sold/equipped) underneath it.
func clampPage(page *int, total int) {
	if *page >= total {
		*page = total - 1
	}
	if *page < 0 {
		*page = 0
	}
}

func drawFilledRect(dst *ebiten.Image, x, y, w, h int, clr color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), clr, false)
}

func drawImageScaled(dst, src *ebiten.Image, x, y, w, h int) {
	if src == nil || w <= 0 || h <= 0 {
		return
	}
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(float64(w)/float64(srcW), float64(h)/float64(srcH))
	opts.GeoM.Translate(float64(x), float64(y))
	// Shrinking with the default nearest filter drops whole source rows/columns,
	// which clips thin baked-in details - e.g. an icon's frame on the trailing
	// (right/bottom) edges. Linear filtering (mipmaps kick in automatically for
	// shrink) resamples instead and keeps them. Upscales stay nearest so pixel
	// art is not blurred.
	if w < srcW || h < srcH {
		opts.Filter = ebiten.FilterLinear
	}
	dst.DrawImage(src, opts)
}

func (ui *UISystem) drawInterfaceIcon(screen *ebiten.Image, name string, x, y, w, h int) {
	icon := ui.game.sprites.GetSprite(name)
	drawImageScaled(screen, icon, x, y, w, h)
}

// drawPopupCloseButton draws the standard red close-X button (hover-brightened)
// and reports whether a queued left click landed on it. canClick=false still
// draws but leaves any queued click unconsumed (e.g. mid-drag, popup just opened).
func (ui *UISystem) drawPopupCloseButton(screen *ebiten.Image, x, y, size int, canClick bool) bool {
	mouseX, mouseY := ebiten.CursorPosition()
	btnCol := color.RGBA{120, 60, 60, 180}
	if mouseX >= x && mouseX < x+size && mouseY >= y && mouseY < y+size {
		btnCol = color.RGBA{200, 60, 60, 220}
	}
	drawFilledRect(screen, x, y, size, size, btnCol)
	ui.drawInterfaceIcon(screen, "icon_close", x+2, y+2, size-4, size-4)
	return canClick && ui.game.consumeLeftClickIn(x, y, x+size, y+size)
}

func drawNineSlice(dst, src *ebiten.Image, x, y, w, h, slice int) {
	if src == nil || w <= 0 || h <= 0 || slice <= 0 {
		return
	}
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= slice*2 || srcH <= slice*2 || w <= slice*2 || h <= slice*2 {
		drawImageScaled(dst, src, x, y, w, h)
		return
	}

	drawPart := func(srcX, srcY, srcW, srcH, dstX, dstY, dstW, dstH int) {
		if dstW <= 0 || dstH <= 0 {
			return
		}
		part := src.SubImage(image.Rect(srcX, srcY, srcX+srcW, srcY+srcH)).(*ebiten.Image)
		drawImageScaled(dst, part, dstX, dstY, dstW, dstH)
	}

	centerSrcW := srcW - slice*2
	centerSrcH := srcH - slice*2
	centerDstW := w - slice*2
	centerDstH := h - slice*2

	drawPart(0, 0, slice, slice, x, y, slice, slice)
	drawPart(srcW-slice, 0, slice, slice, x+w-slice, y, slice, slice)
	drawPart(0, srcH-slice, slice, slice, x, y+h-slice, slice, slice)
	drawPart(srcW-slice, srcH-slice, slice, slice, x+w-slice, y+h-slice, slice, slice)

	drawPart(slice, 0, centerSrcW, slice, x+slice, y, centerDstW, slice)
	drawPart(slice, srcH-slice, centerSrcW, slice, x+slice, y+h-slice, centerDstW, slice)
	drawPart(0, slice, slice, centerSrcH, x, y+slice, slice, centerDstH)
	drawPart(srcW-slice, slice, slice, centerSrcH, x+w-slice, y+slice, slice, centerDstH)

	drawPart(slice, slice, centerSrcW, centerSrcH, x+slice, y+slice, centerDstW, centerDstH)
}

// drawRectBorder draws a rectangle border of given thickness and color
func drawRectBorder(dst *ebiten.Image, x, y, w, h, thickness int, clr color.Color) {
	// Top border
	vector.FillRect(dst, float32(x-thickness), float32(y-thickness), float32(w+2*thickness), float32(thickness), clr, false)
	// Bottom border
	vector.FillRect(dst, float32(x-thickness), float32(y+h), float32(w+2*thickness), float32(thickness), clr, false)
	// Left border
	vector.FillRect(dst, float32(x-thickness), float32(y), float32(thickness), float32(h), clr, false)
	// Right border
	vector.FillRect(dst, float32(x+w), float32(y), float32(thickness), float32(h), clr, false)
}

const tooltipCompareGap = 8
const tooltipIconSize = 64
const tooltipIconGap = 8

func tooltipBoxSizeWithIcon(lines []string, hasIcon bool) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	iconSpace := 0
	if hasIcon {
		iconSpace = tooltipIconSize + tooltipIconGap
	}
	bgWidth := 0
	for _, line := range lines {
		if w := debugTextWidth(line) + 12 + iconSpace; w > bgWidth {
			bgWidth = w
		}
	}
	bgHeight := len(lines)*16 + 8
	if hasIcon && bgHeight < tooltipIconSize+12 {
		bgHeight = tooltipIconSize + 12
	}
	return bgWidth, bgHeight
}

// drawTooltip draws a tooltip with the given text lines at the specified
// position. Lines that don't fit between the tooltip's x position and the
// right screen edge are word-wrapped onto multiple rows; the colors slice
// (if provided) is expanded so each wrapped row keeps its original color.
// flipTooltipY keeps a tooltip box on screen vertically. y arrives as
// cursorY+8 (below the cursor); if a box of bgHeight would run off the bottom,
// flip it ABOVE the cursor, clamping to the top if it's taller than that space
// too. The caller resolves this ONCE for side-by-side cards (main + compare)
// so they share a top edge instead of flipping independently.
func flipTooltipY(y, bgHeight, screenH int) int {
	if y+bgHeight > screenH {
		y = y - bgHeight - 16 // y-8 = cursor, then an 8px gap above it
		if y < 0 {
			y = 0
		}
	}
	return y
}

// maxRight bounds word-wrapping: lines wrap to fit between x and maxRight. Callers
// pass the screen width for a lone tooltip, or a tighter column edge so two
// side-by-side cards (item + its comparison) each wrap within their own column.
func drawTooltip(screen *ebiten.Image, lines []string, colors []color.Color, titlePlate, titleText color.Color, iconName string, x, y, maxRight int, sprites *graphics.SpriteManager) {
	hasIcon := iconName != "" && sprites != nil
	lines, colors = wrapTooltipLines(lines, colors, x, maxRight, tooltipTextOffset(hasIcon))
	bgWidth, bgHeight := tooltipBoxSizeWithIcon(lines, hasIcon)

	// y is already resolved on-screen by the caller (flipTooltipY). Keep a
	// defensive top clamp only.
	if y < 0 {
		y = 0
	}

	drawFilledRect(screen, x, y, bgWidth, bgHeight, color.RGBA{30, 30, 60, 255})
	textX := x + 6
	if hasIcon {
		drawImageScaled(screen, sprites.GetSprite(iconName), x+6, y+6, tooltipIconSize, tooltipIconSize)
		textX += tooltipIconSize + tooltipIconGap
	}

	// Rarity nameplate: a brushed-metal band (darkened metal of the rarity hue)
	// behind the first line, with the name drawn in its normal rarity color +
	// black outline so the shiny text still reads.
	titleStart := 0
	if titlePlate != nil && len(lines) > 0 {
		if plateW := x + bgWidth - 4 - (textX - 4); plateW > 0 {
			drawMetalPlate(screen, textX-4, y+4, plateW, 18, metalPlateBase(titlePlate))
		}
		if titleText != nil {
			drawDebugTextColored(screen, lines[0], textX, y+6, titleText)
		} else {
			drawDebugText(screen, lines[0], textX, y+6) // "as before": plain white + outline
		}
		titleStart = 1
	}

	hasColors := len(colors) == len(lines) && len(colors) > 0
	for i := titleStart; i < len(lines); i++ {
		if hasColors {
			drawDebugTextColored(screen, lines[i], textX, y+6+i*16, colors[i])
		} else {
			drawDebugText(screen, lines[i], textX, y+6+i*16)
		}
	}
}

// metalPlateBase returns the nameplate's base color: a darkened metal of the
// same hue as the rarity text (so the bright text reads on it). Common's white
// becomes mid-grey, so common/uncommon plates look like steel, not flat white.
func metalPlateBase(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 9), uint8(g >> 9), uint8(b >> 9), 255} // 8-bit value x 0.5
}

// drawMetalPlate fills a rect with the same vertical brushed-metal gradient the
// rarity text uses (bright top -> dark bottom), giving a metallic nameplate band.
func drawMetalPlate(screen *ebiten.Image, x, y, w, h int, base color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	const band = 2
	for sy := 0; sy < h; sy += band {
		sh := band
		if sy+sh > h {
			sh = h - sy
		}
		drawFilledRect(screen, x, y+sy, w, sh, metalShade(base, (float64(sy)+float64(sh)/2)/float64(h)))
	}
}

// wrapTooltipLines wraps lines that overflow the right screen edge given the
// tooltip's x position. Returns the wrapped lines plus a parallel colors slice
// (each wrapped fragment inherits the original line's color, or nil if no
// colors were provided). Minimum wrap width is 16 chars so we don't produce
// pathological single-char columns when x is very close to the right edge.
func wrapTooltipLines(lines []string, colors []color.Color, x, screenW, textOffsetPx int) ([]string, []color.Color) {
	availablePx := screenW - x - 12 - textOffsetPx // 6px padding each side
	maxChars := availablePx / debugTextCharWidth
	if maxChars < 16 {
		maxChars = 16
	}

	needsWrap := false
	for _, line := range lines {
		if utf8.RuneCountInString(line) > maxChars {
			needsWrap = true
			break
		}
	}
	if !needsWrap {
		return lines, colors
	}

	hasColors := len(colors) == len(lines) && len(colors) > 0
	wrapped := make([]string, 0, len(lines))
	var wrappedColors []color.Color
	if hasColors {
		wrappedColors = make([]color.Color, 0, len(lines))
	}
	for i, line := range lines {
		fragments := wrapText(line, maxChars)
		wrapped = append(wrapped, fragments...)
		if hasColors {
			for range fragments {
				wrappedColors = append(wrappedColors, colors[i])
			}
		}
	}
	return wrapped, wrappedColors
}

func tooltipTextOffset(hasIcon bool) int {
	if hasIcon {
		return tooltipIconSize + tooltipIconGap
	}
	return 0
}

func tooltipBoxSizeForScreen(lines []string, colors []color.Color, hasIcon bool, x, screenW int) (int, int) {
	wrapped, _ := wrapTooltipLines(lines, colors, x, screenW, tooltipTextOffset(hasIcon))
	return tooltipBoxSizeWithIcon(wrapped, hasIcon)
}

// tooltipPairX positions two side-by-side hover cards (main + comparison) near
// cursorX, shifting the pair left so it stays within screenW. The comparison sits
// flush to the right of the main (compareX = mainX + mainW + gap), so the two
// columns can never overlap - unlike the old "place compare by its unwrapped width
// then clamp to the screen edge", which buried the main under a very wide compare.
func tooltipPairX(cursorX, mainW, compareW, gap, screenW int) (mainX, compareX int) {
	mainX = cursorX
	if mainX+mainW+gap+compareW > screenW {
		mainX = screenW - (mainW + gap + compareW)
	}
	if mainX < 0 {
		mainX = 0
	}
	return mainX, mainX + mainW + gap
}

func (ui *UISystem) queueTooltip(lines []string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = nil
	ui.tooltipTitleColor = nil
	ui.tooltipTitleText = nil
	ui.tooltipIcon = ""
	ui.tooltipX = x
	ui.tooltipY = y
}

func (ui *UISystem) queueTooltipIcon(lines []string, icon string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = nil
	ui.tooltipTitleColor = nil
	ui.tooltipTitleText = nil
	ui.tooltipIcon = ui.validTooltipIcon(icon)
	ui.tooltipX = x
	ui.tooltipY = y
}

// queueTitledTooltipIcon queues a tooltip whose first line (the name) gets a
// metallic nameplate (plate base) with the name in titleText (nil = plain white
// name). bodyColors tints lines BELOW the name (nil = plain white body); gear
// keeps its rarity-metal body, spells/traps stay white. Plate hue: rarity for
// gear, school for spells, wood for traps.
func (ui *UISystem) queueTitledTooltipIcon(lines []string, bodyColors []color.Color, plate, titleText color.Color, icon string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = bodyColors
	ui.tooltipTitleColor = plate
	ui.tooltipTitleText = titleText
	ui.tooltipIcon = ui.validTooltipIcon(icon)
	ui.tooltipX = x
	ui.tooltipY = y
}

func (ui *UISystem) validTooltipIcon(icon string) string {
	if icon == "" || ui == nil || ui.game == nil || ui.game.sprites == nil {
		return ""
	}
	if ui.game.sprites.HasSprite(icon) {
		return icon
	}
	if !missingTooltipIcons[icon] {
		missingTooltipIcons[icon] = true
		log.Printf("[UI] Missing tooltip icon sprite: %s", icon)
	}
	return ""
}

func (ui *UISystem) queueTooltipComparison(lines []string, colors []color.Color) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipCompareLines = lines
	ui.tooltipCompareColors = colors
	ui.tooltipCompareTitle = nil
}

// queueTitledTooltipComparison queues the side-by-side comparison card with a
// metallic nameplate on its first line.
func (ui *UISystem) queueTitledTooltipComparison(lines []string, bodyColors []color.Color, plate, titleText color.Color) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipCompareLines = lines
	ui.tooltipCompareColors = bodyColors
	ui.tooltipCompareTitle = plate
	ui.tooltipCompareText = titleText
}

// rarityBodyColors paints every tooltip line in the item's rarity metal - the
// original gear-tooltip body look (the name line's color is overridden by the
// nameplate's titleText).
func (ui *UISystem) rarityBodyColors(item items.Item, n int) []color.Color {
	if n <= 0 {
		return nil
	}
	c := ui.itemRarityColor(item)
	colors := make([]color.Color, n)
	for i := range colors {
		colors[i] = c
	}
	return colors
}

// schoolPlateColor maps a magic school to its nameplate base hue (darkened +
// brushed by drawMetalPlate). Used for spell-item / spellbook nameplates.
func schoolPlateColor(school string) color.Color {
	switch strings.ToLower(school) {
	case "fire":
		return color.RGBA{220, 70, 40, 255}
	case "water":
		return color.RGBA{60, 130, 220, 255}
	case "air":
		return color.RGBA{150, 205, 225, 255}
	case "earth":
		return color.RGBA{150, 120, 60, 255}
	case "spirit":
		return color.RGBA{230, 215, 120, 255}
	case "mind":
		return color.RGBA{200, 120, 215, 255}
	case "body":
		return color.RGBA{90, 195, 110, 255}
	case "light":
		return color.RGBA{240, 225, 140, 255}
	case "dark":
		return color.RGBA{135, 95, 170, 255}
	default:
		return color.RGBA{120, 128, 148, 255} // steel
	}
}

// woodPlateColor is the trap nameplate base (brown).
var woodPlateColor = color.RGBA{135, 92, 48, 255}

// itemTitleColors returns the nameplate (plate base, name-text) for an item:
// spells use their school hue with the normal white name, traps use wood with
// the white name, and everything else uses the rarity metal for BOTH.
func (ui *UISystem) itemTitleColors(item items.Item) (plate, text color.Color) {
	switch item.Type {
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		return schoolPlateColor(item.SpellSchool), nil
	case items.ItemTrap:
		return woodPlateColor, nil
	default:
		r := ui.itemRarityColor(item)
		return r, r
	}
}

func itemTooltipIconName(item items.Item) string {
	switch item.Type {
	case items.ItemWeapon:
		_, key, ok := config.GetWeaponDefinitionByName(item.Name)
		if !ok {
			key = items.GetWeaponKeyByName(item.Name)
		}
		if key != "" {
			return "icon_weapon_" + key
		}
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		if item.SpellEffect != "" {
			return spellTooltipIconName(spells.SpellID(item.SpellEffect))
		}
	case items.ItemTrap:
		if def, ok := config.GetTrapDefinition(string(item.SpellEffect)); ok {
			return def.Icon
		}
	}
	_, key, ok := config.GetItemDefinitionByName(item.Name)
	if ok && key != "" {
		return "icon_item_" + key
	}
	return ""
}

func spellTooltipIconName(spellID spells.SpellID) string {
	if spellID == "" {
		return ""
	}
	return "icon_spell_" + string(spellID)
}

const (
	debugTextCharWidth  = 6
	debugTextCharHeight = 16
)

var (
	debugTextScratch  *ebiten.Image
	debugTextScratchW int
	debugTextScratchH int
)

func debugTextWidth(text string) int {
	return utf8.RuneCountInString(text) * debugTextCharWidth
}

// clipDebugText shortens text with a trailing ".." so it fits maxW pixels in the
// fixed-width debug font - for grid labels that must not overrun their cell.
func clipDebugText(text string, maxW int) string {
	if maxW <= 0 || debugTextWidth(text) <= maxW {
		return text
	}
	maxRunes := maxW / debugTextCharWidth
	if maxRunes <= 2 {
		return ""
	}
	r := []rune(text)
	if len(r) <= maxRunes {
		return text
	}
	return string(r[:maxRunes-2]) + ".."
}

// humanizeKey turns a snake/kebab content key into a display label:
// "dark_elf" -> "Dark Elf". Shared UI helper for races, schools, etc.
func humanizeKey(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	parts := strings.Fields(s)
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// centeredTextPos is the top-left pixel at which `text` renders centered in the
// box (x,y,w,h) for the debug font - shared by every centered-text drawer.
func centeredTextPos(text string, x, y, w, h int) (int, int) {
	return x + (w-debugTextWidth(text))/2, y + (h-debugTextCharHeight)/2
}

func drawCenteredDebugText(screen *ebiten.Image, text string, x, y, w, h int) {
	if text == "" {
		return
	}
	drawX, drawY := centeredTextPos(text, x, y, w, h)
	drawDebugText(screen, text, drawX, drawY)
}

// drawCenteredTextWithShadow centers text in the box. The dark outline is built
// into drawDebugTextColored game-wide, so this just centers.
func drawCenteredTextWithShadow(screen *ebiten.Image, text string, x, y, w, h int, fg color.Color) {
	if text == "" {
		return
	}
	drawX, drawY := centeredTextPos(text, x, y, w, h)
	drawDebugTextColored(screen, text, drawX, drawY, fg)
}

// drawDebugTextShadowed is kept as a name for existing callers; the outline now
// lives in drawDebugTextColored, so it is a thin alias.
func drawDebugTextShadowed(screen *ebiten.Image, text string, x, y int, fg color.Color) {
	drawDebugTextColored(screen, text, x, y, fg)
}

func ensureDebugTextScratch(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if debugTextScratch == nil || debugTextScratchW < width || debugTextScratchH < height {
		if debugTextScratchW < width {
			debugTextScratchW = width
		}
		if debugTextScratchH < height {
			debugTextScratchH = height
		}
		debugTextScratch = ebiten.NewImage(debugTextScratchW, debugTextScratchH)
	}
}

// textOutlineOffsets are the 8 neighbour directions for the dark text outline.
var textOutlineOffsets = [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

// drawScaledCenteredText draws text scaled by `scale`, centered on (cx, cy), with
// a dark outline that scales with it - for emphasis headings (e.g. GAME OVER).
func drawScaledCenteredText(screen *ebiten.Image, text string, cx, cy int, scale float64, col color.Color) {
	if text == "" {
		return
	}
	w := debugTextWidth(text) + 2
	h := debugTextCharHeight
	ensureDebugTextScratch(w, h)
	debugTextScratch.Fill(color.RGBA{0, 0, 0, 0})
	ebitenutil.DebugPrintAt(debugTextScratch, text, -1, 0)
	glyphs := debugTextScratch.SubImage(image.Rect(0, 0, w, h)).(*ebiten.Image)
	x := float64(cx) - float64(w)*scale/2
	y := float64(cy) - float64(h)*scale/2
	blit := func(ox, oy float64, c color.Color) {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(x+ox, y+oy)
		r, g, b, a := c.RGBA()
		op.ColorScale.Scale(float32(r)/65535, float32(g)/65535, float32(b)/65535, float32(a)/65535)
		screen.DrawImage(glyphs, op)
	}
	outline := color.RGBA{0, 0, 0, 235}
	for _, d := range textOutlineOffsets {
		blit(float64(d[0])*scale, float64(d[1])*scale, outline)
	}
	blit(0, 0, col)
}

// drawDebugText draws left-aligned OUTLINED white text - the game-wide default,
// replacing raw ebitenutil.DebugPrintAt(screen, ...) so every label stays legible
// over any background.
func drawDebugText(screen *ebiten.Image, text string, x, y int) {
	drawDebugTextColored(screen, text, x, y, color.White)
}

// Outlined labels are composed once per (text, color) into small cached images
// so each on-screen label costs ONE DrawImage per frame instead of 9-10 blits
// (the party HUD alone emits ~40 labels/frame). Two-generation eviction: when
// the live map fills it becomes the fallback generation and hits promote back,
// so per-frame-unique strings (FPS counter, ticking HP) age out without ever
// re-rasterizing the stable HUD set.
const outlinedLabelCacheMax = 256

type outlinedLabelKey struct {
	text       string
	r, g, b, a uint32
}

var (
	outlinedLabelCache     = make(map[outlinedLabelKey]*ebiten.Image)
	outlinedLabelCachePrev = map[outlinedLabelKey]*ebiten.Image{}
)

func outlinedLabelImage(text string, col color.Color) *ebiten.Image {
	r, g, b, a := col.RGBA()
	key := outlinedLabelKey{text, r, g, b, a}
	if img, ok := outlinedLabelCache[key]; ok {
		return img
	}
	img, ok := outlinedLabelCachePrev[key]
	if !ok {
		img = renderOutlinedLabel(text, col)
	}
	// Dropped images are reclaimed by GC (ebiten deallocates on collect); no
	// explicit Deallocate - an evicted image may already be enqueued this frame.
	if len(outlinedLabelCache) >= outlinedLabelCacheMax {
		outlinedLabelCachePrev = outlinedLabelCache
		outlinedLabelCache = make(map[outlinedLabelKey]*ebiten.Image, outlinedLabelCacheMax)
	}
	outlinedLabelCache[key] = img
	return img
}

// renderOutlinedLabel rasterizes text once into the scratch and composes the
// 8-direction dark outline + colored body into a (w+2)x(h+2) image; the body
// sits at (1,1) so the outline fits inside the bounds.
func renderOutlinedLabel(text string, col color.Color) *ebiten.Image {
	w := debugTextWidth(text) + 2
	h := debugTextCharHeight
	ensureDebugTextScratch(w, h)
	debugTextScratch.Fill(color.RGBA{0, 0, 0, 0})

	// Offset by -1 so the rendered text aligns with DebugPrintAt's left edge.
	ebitenutil.DebugPrintAt(debugTextScratch, text, -1, 0)
	glyphs := debugTextScratch.SubImage(image.Rect(0, 0, w, h)).(*ebiten.Image)

	img := ebiten.NewImage(w+2, h+2)
	blit := func(dx, dy int, c color.Color) {
		opts := &ebiten.DrawImageOptions{}
		r, g, b, a := c.RGBA()
		opts.ColorScale.Scale(float32(r)/65535, float32(g)/65535, float32(b)/65535, float32(a)/65535)
		opts.GeoM.Translate(float64(1+dx), float64(1+dy))
		img.DrawImage(glyphs, opts)
	}
	outline := color.RGBA{0, 0, 0, 235}
	for _, d := range textOutlineOffsets {
		blit(d[0], d[1], outline)
	}
	// Rarity metals render as a vertical gradient (shiny); everything else flat.
	if base, ok := asMetal(col); ok {
		drawMetalBody(img, 1, 1, w, h, base)
	} else {
		blit(0, 0, col)
	}
	return img
}

// drawDebugTextColored draws left-aligned text wrapped in a dark 8-direction
// outline (the character-sheet look, now game-wide) via the label cache.
func drawDebugTextColored(screen *ebiten.Image, text string, x, y int, col color.Color) {
	if text == "" {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x-1), float64(y-1))
	screen.DrawImage(outlinedLabelImage(text, col), opts)
}

func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

// metalShade is the metallic ramp at vertical fraction t (0 top ... 1 bottom): a
// bright highlight at the top, the base tint in the middle, a dark edge at the
// bottom - the beveled shiny-metal look for gold/silver/legendary names.
func metalShade(base color.RGBA, t float64) color.RGBA {
	to := color.RGBA{255, 255, 255, base.A} // highlight
	k := (0.5 - t) / 0.5 * 0.6              // up to +60% toward white at the very top
	if t >= 0.5 {
		to = color.RGBA{0, 0, 0, base.A} // shadow
		k = (t - 0.5) / 0.5 * 0.5        // up to -50% toward black at the bottom
	}
	return color.RGBA{
		lerpByte(base.R, to.R, k),
		lerpByte(base.G, to.G, k),
		lerpByte(base.B, to.B, k),
		base.A,
	}
}

// drawMetalBody fills the already-rasterized glyph (in debugTextScratch) with the
// metalShade gradient, blitting it in thin horizontal bands top->bottom.
func drawMetalBody(screen *ebiten.Image, x, y, w, h int, base color.RGBA) {
	const band = 2
	for sy := 0; sy < h; sy += band {
		sh := band
		if sy+sh > h {
			sh = h - sy
		}
		c := metalShade(base, (float64(sy)+float64(sh)/2)/float64(h))
		strip := debugTextScratch.SubImage(image.Rect(0, sy, w, sy+sh)).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		r, g, b, a := c.RGBA()
		op.ColorScale.Scale(float32(r)/65535, float32(g)/65535, float32(b)/65535, float32(a)/65535)
		op.GeoM.Translate(float64(x), float64(y+sy))
		screen.DrawImage(strip, op)
	}
}

// Rarity metals - the SINGLE definition of each tier's tint. Silver is light and
// cool (blue > red); gold and legendary are warm. Listed in metallicColors so
// drawDebugTextColored renders them as a vertical metal GRADIENT (shiny names)
// rather than a flat fill.
var (
	raritySilver  = color.RGBA{210, 216, 230, 255} // uncommon
	rarityGold    = color.RGBA{255, 215, 0, 255}   // rare
	rarityFire    = color.RGBA{220, 80, 20, 255}   // legendary
	rarityEmerald = color.RGBA{70, 220, 130, 255}  // unique (arena tier)
)

// metallicColors marks which base tints get the metal-gradient text treatment.
var metallicColors = map[color.RGBA]bool{
	raritySilver:  true,
	rarityGold:    true,
	rarityFire:    true,
	rarityEmerald: true,
}

// asMetal reports whether col is a registered rarity metal (so the text renders
// as a gradient), returning the concrete RGBA base.
func asMetal(col color.Color) (color.RGBA, bool) {
	if rc, ok := col.(color.RGBA); ok && metallicColors[rc] {
		return rc, true
	}
	return color.RGBA{}, false
}

func rarityColor(rarity string) color.Color {
	switch strings.ToLower(rarity) {
	case "uncommon":
		return raritySilver
	case "rare":
		return rarityGold
	case "legendary":
		return rarityFire
	case "unique":
		return rarityEmerald
	default:
		return color.White // Common/default
	}
}

var (
	// Distinct from rarityGold so the combat log stays a flat fill (not metal).
	combatMessageGold   = color.RGBA{255, 205, 40, 255}
	combatMessagePurple = color.RGBA{190, 100, 255, 255}
	combatMessageOrange = color.RGBA{255, 140, 40, 255}
	combatMessageYellow = color.RGBA{255, 230, 90, 255}
)

func lootMessageColor(drops []items.Item) color.Color {
	for _, item := range drops {
		if strings.EqualFold(item.Rarity, "legendary") {
			return rarityColor("legendary")
		}
	}
	return combatMessageGold
}

func itemRarity(item items.Item) string {
	if item.Rarity != "" {
		return item.Rarity
	}
	if item.Type == items.ItemWeapon {
		if def, _, ok := config.GetWeaponDefinitionByName(item.Name); ok && def != nil && def.Rarity != "" {
			return def.Rarity
		}
	}
	return "common"
}

func (ui *UISystem) itemRarityColor(item items.Item) color.Color {
	return rarityColor(itemRarity(item))
}

// rarityTier ranks rarities for "highest wins" comparisons (higher = rarer).
// rarityTier delegates to config.RarityTier (the single rarity-ordering SSoT).
func rarityTier(rarity string) int { return config.RarityTier(rarity) }

// isMouseHoveringBox checks if the mouse is hovering over a rectangular area
func isMouseHoveringBox(mouseX, mouseY, x1, y1, x2, y2 int) bool {
	return mouseX >= x1 && mouseX < x2 && mouseY >= y1 && mouseY < y2
}

// statTooltipText quotes the canonical stat description from the character
// catalog - one source for the in-game tooltip and the map editor.
func statTooltipText(stat string) string {
	return character.StatDescription(stat)
}

// masteryTooltipTextForSkill returns the canonical skill description. The text
// (and the constants behind it) live in the character package so the in-game
// tooltip, combat, and the map editor all share one source - see
// character.SkillType.Description.
func masteryTooltipTextForSkill(skill character.SkillType) string {
	return skill.Description()
}

func magicMasteryTooltipText() string {
	return character.MagicMasteryDescription()
}

// drawUIBackground draws a colored background rectangle for UI elements (DRY helper)
func (ui *UISystem) drawUIBackground(screen *ebiten.Image, x, y, width, height int, bgColor color.RGBA) {
	if bgColor.A > 0 {
		drawFilledRect(screen, x, y, width, height, bgColor)
	}
}
