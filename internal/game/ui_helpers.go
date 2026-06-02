package game

import (
	"fmt"
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

// isMouseOverCharacter checks if the mouse cursor is over a specific character portrait.
// baseLeft is the x-offset where the (centered) party row begins.
func (ui *UISystem) isMouseOverCharacter(mouseX, mouseY, charIndex, portraitWidth, portraitHeight, startY, baseLeft int) bool {
	charX := baseLeft + charIndex*portraitWidth
	return mouseX >= charX && mouseX < charX+portraitWidth &&
		mouseY >= startY && mouseY < startY+portraitHeight
}

// partyPortraitLayout returns the fixed-pixel party-portrait layout, centered
// horizontally and anchored to the bottom of the (possibly fullscreen) viewport.
// Portrait width comes from UIConfig (not derived from screen width) so going
// fullscreen does not stretch the party row — it stays at its design size and
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

func merchantDialogLayout(screenW, screenH int) (dialogX, dialogY, dialogW, dialogH, listY, leftX, rightX, colW, rowH int) {
	dialogW = 600
	dialogH = 400
	dialogX = (screenW - dialogW) / 2
	dialogY = (screenH - dialogH) / 2
	rowH = UIRowSpacing
	listY = dialogY + 120
	colW = dialogW/2 - 40
	leftX = dialogX + 20
	rightX = dialogX + dialogW/2 + 10
	return
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
	// which clips thin baked-in details — e.g. an icon's frame on the trailing
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
func drawTooltip(screen *ebiten.Image, lines []string, colors []color.Color, iconName string, x, y int, sprites *graphics.SpriteManager) {
	screenW := screen.Bounds().Dx()

	hasIcon := iconName != "" && sprites != nil
	lines, colors = wrapTooltipLines(lines, colors, x, screenW, tooltipTextOffset(hasIcon))
	bgWidth, bgHeight := tooltipBoxSizeWithIcon(lines, hasIcon)
	drawFilledRect(screen, x, y, bgWidth, bgHeight, color.RGBA{30, 30, 60, 255})
	textX := x + 6
	if hasIcon {
		drawImageScaled(screen, sprites.GetSprite(iconName), x+6, y+6, tooltipIconSize, tooltipIconSize)
		textX += tooltipIconSize + tooltipIconGap
	}
	if len(colors) == len(lines) && len(colors) > 0 {
		for i, line := range lines {
			drawDebugTextColored(screen, line, textX, y+6+i*16, colors[i])
		}
		return
	}
	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, textX, y+6+i*16)
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

func (ui *UISystem) queueTooltip(lines []string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = nil
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
	ui.tooltipIcon = ui.validTooltipIcon(icon)
	ui.tooltipX = x
	ui.tooltipY = y
}

func (ui *UISystem) queueTooltipColoredIcon(lines []string, colors []color.Color, icon string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = colors
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

func drawCenteredDebugText(screen *ebiten.Image, text string, x, y, w, h int) {
	if text == "" {
		return
	}
	textW := debugTextWidth(text)
	textH := debugTextCharHeight
	drawX := x + (w-textW)/2
	drawY := y + (h-textH)/2
	ebitenutil.DebugPrintAt(screen, text, drawX, drawY)
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

func drawDebugTextColored(screen *ebiten.Image, text string, x, y int, col color.Color) {
	if text == "" {
		return
	}
	w := debugTextWidth(text) + 2
	h := debugTextCharHeight
	ensureDebugTextScratch(w, h)
	debugTextScratch.Fill(color.RGBA{0, 0, 0, 0})

	// Offset by -1 so the rendered text aligns with DebugPrintAt's left edge.
	ebitenutil.DebugPrintAt(debugTextScratch, text, -1, 0)

	opts := &ebiten.DrawImageOptions{}
	r, g, b, a := col.RGBA()
	opts.ColorScale.Scale(float32(r)/65535, float32(g)/65535, float32(b)/65535, float32(a)/65535)
	opts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(debugTextScratch, opts)
}

func rarityColor(rarity string) color.Color {
	switch strings.ToLower(rarity) {
	case "uncommon":
		return color.RGBA{192, 192, 192, 255} // Silver
	case "rare":
		return color.RGBA{255, 215, 0, 255} // Gold
	case "legendary":
		return color.RGBA{220, 80, 20, 255} // Deep orange-red
	default:
		return color.White // Common/default
	}
}

func (ui *UISystem) itemRarity(item items.Item) string {
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
	return rarityColor(ui.itemRarity(item))
}

func (ui *UISystem) itemTooltipColors(item items.Item, lines []string) []color.Color {
	if len(lines) == 0 {
		return nil
	}
	colors := make([]color.Color, len(lines))
	c := ui.itemRarityColor(item)
	for i := range colors {
		colors[i] = c
	}
	return colors
}

// isMouseHoveringBox checks if the mouse is hovering over a rectangular area
func isMouseHoveringBox(mouseX, mouseY, x1, y1, x2, y2 int) bool {
	return mouseX >= x1 && mouseX < x2 && mouseY >= y1 && mouseY < y2
}

// Stat tooltips include the scaling divisors used in combat formulas so the
// description never drifts from the actual numbers — see balance.go. The
// "weapons that scale with X" phrasing matches what `bonus_stat` actually
// gates in weapons.yaml (any weapon can pick any stat).
func statTooltipText(stat string) string {
	switch strings.ToLower(stat) {
	case "might":
		return fmt.Sprintf("Adds Might/%d to damage of weapons that scale with Might.", WeaponPrimaryStatDivisor)
	case "intellect":
		return fmt.Sprintf("Adds Intellect/%d to weapons that scale with Intellect; also adds to max spell points.", WeaponPrimaryStatDivisor)
	case "personality":
		return "Adds to max spell points and increases SP regen rate."
	case "endurance":
		return "Increases max HP and armor-class scaling on equipped armor."
	case "accuracy":
		return fmt.Sprintf("Adds Accuracy/%d to damage of weapons that scale with Accuracy.", WeaponPrimaryStatDivisor)
	case "speed":
		return fmt.Sprintf("Reduces real-time action cooldowns. In turn-based mode grants extra actions per turn (Speed >%d → 2 actions, >%d → 3).",
			character.SpeedActionSlot2Threshold, character.SpeedActionSlot3Threshold)
	case "luck":
		return "Improves critical chance and dodges."
	default:
		return ""
	}
}

func masteryTooltipTextForSkill(skill character.SkillType) string {
	switch skill {
	case character.SkillSword, character.SkillDagger, character.SkillAxe, character.SkillSpear,
		character.SkillBow, character.SkillMace, character.SkillStaff:
		return fmt.Sprintf("Weapon Mastery: +%d base damage per mastery level.", MasteryWeaponDamagePerLevel)
	case character.SkillLeather, character.SkillChain, character.SkillPlate, character.SkillShield:
		return fmt.Sprintf("Armor Mastery: +%d base AC per mastery level.", MasteryArmorACPerLevel)
	// Misc skills — descriptions read the SAME constants the mechanics use, so
	// tooltip and combat can't drift. All scale by tier (Novice gives none).
	case character.SkillBodybuilding:
		return fmt.Sprintf("Bodybuilding: +%d max HP per mastery level.", character.BodybuildingHPPerTier)
	case character.SkillMeditation:
		return fmt.Sprintf("Meditation: +%d spell points per regen tick per mastery level (faster mana recovery).", character.MeditationRegenPerTier)
	case character.SkillLearning:
		return fmt.Sprintf("Learning: +%d%% experience gained per mastery level.", LearningXPPctPerTier)
	case character.SkillArmsMaster:
		return fmt.Sprintf("Arms Master: +%d damage with any weapon per mastery level.", ArmsMasterDamagePerTier)
	case character.SkillMerchant:
		return fmt.Sprintf("Merchant: %d%% better buy/sell prices per mastery level (the party's best applies).", MerchantPricePctPerTier)
	case character.SkillDisarmTrap:
		return fmt.Sprintf("Disarm Trap: -%d incoming damage per mastery level (placeholder until trap tiles are added).", DisarmTrapDamageReductionPerTier)
	case character.SkillRepair:
		return "Repair: no effect yet (planned: equipment durability)."
	case character.SkillIdentifyItem:
		return "Identify Item: no effect yet (planned: reveal unidentified loot)."
	default:
		return ""
	}
}

func magicMasteryTooltipText() string {
	return fmt.Sprintf("Magic Mastery: +%d to base spell effects per mastery level.", MasterySpellEffectPerLevel)
}

// drawUIBackground draws a colored background rectangle for UI elements (DRY helper)
func (ui *UISystem) drawUIBackground(screen *ebiten.Image, x, y, width, height int, bgColor color.RGBA) {
	if bgColor.A > 0 {
		drawFilledRect(screen, x, y, width, height, bgColor)
	}
}

// getCharacterSpellStatus returns the background color and status text for a character in spell trader dialog (DRY helper)
func (ui *UISystem) getCharacterSpellStatus(charIndex int, canLearn, alreadyKnows, spellSelected bool) (color.RGBA, string) {
	if charIndex == ui.game.selectedCharIdx {
		// Selected character - blue background
		return UIColorSelectedCharacter, ""
	} else if alreadyKnows {
		// Already knows spell - gray background
		return UIColorKnowsSpell, " (Knows Spell)"
	} else if canLearn {
		// Can learn spell - green tint
		return UIColorCanLearn, " (Can Learn)"
	} else if spellSelected {
		// Cannot learn spell - red tint
		return UIColorCannotLearn, " (Cannot Learn)"
	} else {
		// No spell selected - no background
		return color.RGBA{0, 0, 0, 0}, ""
	}
}
