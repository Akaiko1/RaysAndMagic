package game

import (
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type coloredTextSegment struct {
	text  string
	color color.Color
}

func drawColoredTextSegments(screen *ebiten.Image, x, y int, segments []coloredTextSegment) {
	curX := x
	for _, seg := range segments {
		drawDebugTextColored(screen, seg.text, curX, y, seg.color)
		curX += debugTextWidth(seg.text)
	}
}

// getAvailableSpellKeys returns the list of spell keys available from the current NPC in deterministic order
func (ui *UISystem) getAvailableSpellKeys() []string {
	if ui.game.dialogNPC == nil || ui.game.dialogNPC.SpellData == nil {
		return []string{}
	}

	keys := make([]string, 0, len(ui.game.dialogNPC.SpellData))
	for key := range ui.game.dialogNPC.SpellData {
		keys = append(keys, key)
	}

	// Sort keys to ensure deterministic ordering and prevent UI blinking
	sort.Strings(keys)

	return keys
}

func (ui *UISystem) getSchoolName(school character.MagicSchool) string {
	names := map[character.MagicSchool]string{
		character.MagicBody:   "Body",
		character.MagicMind:   "Mind",
		character.MagicSpirit: "Spirit",
		character.MagicFire:   "Fire",
		character.MagicWater:  "Water",
		character.MagicAir:    "Air",
		character.MagicEarth:  "Earth",
		character.MagicLight:  "Light",
		character.MagicDark:   "Dark",
	}
	if name, exists := names[school]; exists {
		return name
	}
	return "Unknown"
}

// isMouseOverCharacter checks if the mouse cursor is over a specific character portrait
func (ui *UISystem) isMouseOverCharacter(mouseX, mouseY, charIndex, portraitWidth, portraitHeight, startY int) bool {
	charX := charIndex * portraitWidth
	return mouseX >= charX && mouseX < charX+portraitWidth &&
		mouseY >= startY && mouseY < startY+portraitHeight
}

// getClassName returns the class name for a character class
func (ui *UISystem) getClassName(class character.CharacterClass) string {
	names := map[character.CharacterClass]string{
		character.ClassKnight:   "Knight",
		character.ClassPaladin:  "Paladin",
		character.ClassArcher:   "Archer",
		character.ClassCleric:   "Cleric",
		character.ClassSorcerer: "Sorcerer",
		character.ClassDruid:    "Druid",
	}
	if name, exists := names[class]; exists {
		return name
	}
	return "Unknown"
}

// wrapText wraps text to fit within specified width (UISystem method)
func (ui *UISystem) wrapText(text string, maxWidth int) []string {
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= maxWidth {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

func drawFilledRect(dst *ebiten.Image, x, y, w, h int, clr color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	vector.DrawFilledRect(dst, float32(x), float32(y), float32(w), float32(h), clr, false)
}

// drawRectBorder draws a rectangle border of given thickness and color
func drawRectBorder(dst *ebiten.Image, x, y, w, h, thickness int, clr color.Color) {
	// Top border
	vector.DrawFilledRect(dst, float32(x-thickness), float32(y-thickness), float32(w+2*thickness), float32(thickness), clr, false)
	// Bottom border
	vector.DrawFilledRect(dst, float32(x-thickness), float32(y+h), float32(w+2*thickness), float32(thickness), clr, false)
	// Left border
	vector.DrawFilledRect(dst, float32(x-thickness), float32(y), float32(thickness), float32(h), clr, false)
	// Right border
	vector.DrawFilledRect(dst, float32(x+w), float32(y), float32(thickness), float32(h), clr, false)
}

const tooltipCompareGap = 8

func tooltipBoxSize(lines []string) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	bgWidth := 0
	for _, line := range lines {
		if w := debugTextWidth(line) + 12; w > bgWidth {
			bgWidth = w
		}
	}
	bgHeight := len(lines)*16 + 8
	return bgWidth, bgHeight
}

// drawTooltip draws a tooltip with the given text lines at the specified position
func drawTooltip(screen *ebiten.Image, lines []string, colors []color.Color, x, y int) {
	bgWidth, bgHeight := tooltipBoxSize(lines)
	drawFilledRect(screen, x, y, bgWidth, bgHeight, color.RGBA{30, 30, 60, 255})
	if len(colors) == len(lines) && len(colors) > 0 {
		for i, line := range lines {
			drawDebugTextColored(screen, line, x+6, y+6+i*16, colors[i])
		}
		return
	}
	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, x+6, y+6+i*16)
	}
}

func (ui *UISystem) queueTooltip(lines []string, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = nil
	ui.tooltipX = x
	ui.tooltipY = y
}

func (ui *UISystem) queueTooltipColored(lines []string, colors []color.Color, x, y int) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipLines = lines
	ui.tooltipColors = colors
	ui.tooltipX = x
	ui.tooltipY = y
}

func (ui *UISystem) queueTooltipComparison(lines []string, colors []color.Color) {
	if len(lines) == 0 {
		return
	}
	ui.tooltipCompareLines = lines
	ui.tooltipCompareColors = colors
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

func statTooltipText(stat string) string {
	switch strings.ToLower(stat) {
	case "might":
		return "Increases melee damage."
	case "intellect":
		return "Increases spell damage and spell points."
	case "personality":
		return "Increases spell points and mana regen."
	case "endurance":
		return "Increases health and armor scaling."
	case "accuracy":
		return "Increases hit chance and ranged damage."
	case "speed":
		return "Reduces action cooldowns."
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
		return "Weapon Mastery: +2 base damage per mastery level."
	case character.SkillLeather, character.SkillChain, character.SkillPlate, character.SkillShield:
		return "Armor Mastery: +1 base AC per mastery level."
	default:
		return ""
	}
}

func magicMasteryTooltipText() string {
	return "Magic Mastery: +5 to base spell effects per mastery level."
}

func (ui *UISystem) getSkillName(skill character.SkillType) string {
	names := map[character.SkillType]string{
		character.SkillSword:        "Sword",
		character.SkillDagger:       "Dagger",
		character.SkillAxe:          "Axe",
		character.SkillSpear:        "Spear",
		character.SkillBow:          "Bow",
		character.SkillMace:         "Mace",
		character.SkillStaff:        "Staff",
		character.SkillLeather:      "Leather",
		character.SkillChain:        "Chain",
		character.SkillPlate:        "Plate",
		character.SkillShield:       "Shield",
		character.SkillBodybuilding: "Bodybuilding",
		character.SkillMeditation:   "Meditation",
		character.SkillMerchant:     "Merchant",
		character.SkillRepair:       "Repair",
		character.SkillIdentifyItem: "Identify Item",
		character.SkillDisarmTrap:   "Disarm Trap",
		character.SkillLearning:     "Learning",
		character.SkillArmsMaster:   "Arms Master",
	}
	if name, exists := names[skill]; exists {
		return name
	}
	return "Unknown"
}

func (ui *UISystem) getMasteryName(mastery character.SkillMastery) string {
	switch mastery {
	case character.MasteryNovice:
		return "Novice"
	case character.MasteryExpert:
		return "Expert"
	case character.MasteryMaster:
		return "Master"
	case character.MasteryGrandMaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

// getConditionName returns the condition name for a character condition
func (ui *UISystem) getConditionName(condition character.Condition) string {
	names := map[character.Condition]string{
		character.ConditionNormal:      "Normal",
		character.ConditionPoisoned:    "Poisoned",
		character.ConditionDiseased:    "Diseased",
		character.ConditionCursed:      "Cursed",
		character.ConditionAsleep:      "Asleep",
		character.ConditionFear:        "Fear",
		character.ConditionParalyzed:   "Paralyzed",
		character.ConditionUnconscious: "Unconscious",
		character.ConditionDead:        "Dead",
		character.ConditionStone:       "Stone",
		character.ConditionEradicated:  "Eradicated",
	}
	if name, exists := names[condition]; exists {
		return name
	}
	return "Unknown"
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
