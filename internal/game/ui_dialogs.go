package game

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type statMeta struct {
	Name string
	Ptr  *int
}

// MaxStatValue is the maximum base stat value a character can have
const MaxStatValue = 99

// drawStatPointRow draws a single stat row with name, value, and + button
func drawStatPointRow(screen *ebiten.Image, name string, valuePtr *int, y, plusX, plusY, btnW, btnH int, canAdd, isHover *bool, clickIn bool) bool {
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: %d", name, *valuePtr), plusX-148, y)

	// Check if stat is already at max (99)
	atMax := *valuePtr >= MaxStatValue
	canActuallyAdd := *canAdd && !atMax

	var plusColor color.RGBA
	if canActuallyAdd && *isHover {
		plusColor = color.RGBA{80, 200, 80, 220}
	} else if atMax {
		plusColor = color.RGBA{100, 100, 100, 180} // Gray out if at max
	} else {
		plusColor = color.RGBA{60, 120, 60, 180}
	}
	vector.DrawFilledRect(screen, float32(plusX), float32(plusY), float32(btnW), float32(btnH), plusColor, false)
	drawCenteredDebugText(screen, "+", plusX, plusY, btnW, btnH)
	// Handle click
	if canActuallyAdd && *isHover && clickIn {
		(*valuePtr)++
		*canAdd = false // Only allow one per click
		return true
	}
	return false
}

// drawStatDistributionPopup draws the stat allocation popup for the selected character
func (ui *UISystem) drawStatDistributionPopup(screen *ebiten.Image) {
	charIdx := ui.game.statPopupCharIdx
	if charIdx < 0 || charIdx >= len(ui.game.party.Members) {
		return
	}
	member := ui.game.party.Members[charIdx]

	// Popup dimensions
	popupW, popupH := 340, 320
	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2

	// Draw background
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 240})

	// Draw border (replace deprecated DrawRect)
	borderCol := color.RGBA{120, 120, 180, 255}
	borderThickness := 2
	drawRectBorder(screen, popupX, popupY, popupW, popupH, borderThickness, borderCol)

	// Title
	ebitenutil.DebugPrintAt(screen, "Distribute Stat Points", popupX+16, popupY+16)
	ebitenutil.DebugPrintAt(screen, "Points left:", popupX+16, popupY+44)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", member.FreeStatPoints), popupX+120, popupY+44)

	// Stat list
	statList := []statMeta{
		{"Might", &member.Might},
		{"Intellect", &member.Intellect},
		{"Personality", &member.Personality},
		{"Endurance", &member.Endurance},
		{"Accuracy", &member.Accuracy},
		{"Speed", &member.Speed},
		{"Luck", &member.Luck},
	}
	yStart := popupY + 80
	btnW, btnH := 28, 28
	mouseX, mouseY := ebiten.CursorPosition()
	for i, stat := range statList {
		y := yStart + i*36
		plusX := popupX + 180
		plusY := y - 4
		canAdd := member.FreeStatPoints > 0
		isHover := mouseX >= plusX && mouseX < plusX+btnW && mouseY >= plusY && mouseY < plusY+btnH
		clickIn := ui.game.consumeLeftClickIn(plusX, plusY, plusX+btnW, plusY+btnH)
		if drawStatPointRow(screen, stat.Name, stat.Ptr, y, plusX, plusY, btnW, btnH, &canAdd, &isHover, clickIn) {
			member.FreeStatPoints--
			// Recalculate derived stats (HP, SP) when any stat is increased
			member.CalculateDerivedStats(ui.game.config)
		}
	}

	// Draw close button
	closeX := popupX + popupW - 40
	closeY := popupY + 12
	isCloseHover := mouseX >= closeX && mouseX < closeX+28 && mouseY >= closeY && mouseY < closeY+28
	if isCloseHover {
		drawFilledRect(screen, closeX, closeY, 28, 28, color.RGBA{200, 60, 60, 220})
	} else {
		drawFilledRect(screen, closeX, closeY, 28, 28, color.RGBA{120, 60, 60, 180})
	}
	drawCenteredDebugText(screen, "X", closeX, closeY, 28, 28)
	// Handle close click
	// Only allow closing if the mouse was released after opening the popup
	if isCloseHover && ui.game.consumeLeftClickIn(closeX, closeY, closeX+28, closeY+28) && !ui.justOpenedStatPopup {
		ui.game.statPopupOpen = false
	}

	// Handle ESC key to close popup
	if ebiten.IsKeyPressed(ebiten.KeyEscape) && !ui.justOpenedStatPopup {
		ui.game.statPopupOpen = false
	}

	// Reset justOpenedStatPopup after the first frame
	if ui.justOpenedStatPopup {
		ui.justOpenedStatPopup = false
	}
}

// drawLevelUpChoicePopup draws the level-up choice selection overlay.
func (ui *UISystem) drawLevelUpChoicePopup(screen *ebiten.Image) {
	req := ui.game.currentLevelUpChoice()
	if req == nil || req.charIndex < 0 || req.charIndex >= len(ui.game.party.Members) {
		return
	}

	member := ui.game.party.Members[req.charIndex]
	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	popupX, popupY, popupW, popupH, startY, rowH := levelUpChoiceLayout(req, screenW, screenH)
	mouseX, mouseY := ebiten.CursorPosition()

	// Dim background
	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})

	// Panel
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 240})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{120, 120, 180, 255})

	ebitenutil.DebugPrintAt(screen, "Level Up Choice", popupX+16, popupY+16)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s reached level %d", member.Name, req.level), popupX+16, popupY+36)

	for i, option := range req.options {
		y := startY + i*rowH
		if i == req.selection {
			drawFilledRect(screen, popupX+16, y-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}
		if option.hasMastery && option.masteryCurrent != "" && option.masteryNext != "" {
			segments := []coloredTextSegment{
				{text: option.masteryPrefix, color: color.White},
				{text: option.masteryCurrent, color: color.RGBA{240, 220, 80, 255}},
				{text: " -> ", color: color.White},
				{text: option.masteryNext, color: color.RGBA{80, 220, 80, 255}},
			}
			drawColoredTextSegments(screen, popupX+28, y, segments)
		} else {
			ebitenutil.DebugPrintAt(screen, option.label, popupX+28, y)
		}

		if isMouseHoveringBox(mouseX, mouseY, popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			var tooltip string
			switch strings.ToLower(option.choice.Type) {
			case "spell":
				tooltip = GetSpellTooltip(option.spellID, member, ui.game.combat)
			case "weapon_mastery", "armor_mastery":
				tooltip = masteryTooltipTextForSkill(option.skillType)
			case "magic_mastery":
				tooltip = magicMasteryTooltipText()
			}
			if tooltip != "" {
				ui.queueTooltip(strings.Split(tooltip, "\n"), mouseX+16, mouseY+8)
			}
		}
	}

	ebitenutil.DebugPrintAt(screen, "Use ↑/↓ or click, Enter to choose", popupX+16, popupY+popupH-22)
}

// drawNPCDialog draws the NPC dialog interface for different NPC types
func (ui *UISystem) drawNPCDialog(screen *ebiten.Image) {
	if ui.game.dialogNPC == nil {
		return
	}

	screenWidth := ui.game.config.GetScreenWidth()
	screenHeight := ui.game.config.GetScreenHeight()

	// Dialog dimensions
	dialogWidth := 600
	dialogHeight := 400
	dialogX := (screenWidth - dialogWidth) / 2
	dialogY := (screenHeight - dialogHeight) / 2

	// Draw semi-transparent overlay
	drawFilledRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{0, 0, 0, 128})

	// Draw dialog background
	drawFilledRect(screen, dialogX, dialogY, dialogWidth, dialogHeight, color.RGBA{40, 40, 60, 255})

	// Draw border
	borderColor := color.RGBA{100, 100, 120, 255}
	borderThickness := 3
	drawRectBorder(screen, dialogX, dialogY, dialogWidth, dialogHeight, borderThickness, borderColor)

	// Handle different NPC types
	switch ui.game.dialogNPC.Type {
	case "encounter":
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case "spell_trader":
		ui.drawSpellTraderDialog(screen, dialogX, dialogY, dialogHeight)
	case "merchant":
		ui.drawMerchantDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	default:
		ui.drawGenericDialog(screen, dialogX, dialogY, dialogHeight)
	}
}

// drawEncounterDialog draws dialog for encounter NPCs
func (ui *UISystem) drawEncounterDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, _ int) {
	npc := ui.game.dialogNPC

	// Draw title
	titleText := npc.Name
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw encounter description
	if npc.DialogueData != nil {
		// Wrap long text
		greeting := npc.DialogueData.Greeting
		lines := ui.wrapText(greeting, 70)
		for i, line := range lines {
			ebitenutil.DebugPrintAt(screen, line, dialogX+20, dialogY+50+i*16)
		}

		// Draw choices if this is first visit or encounter is repeatable
		if !npc.Visited || (npc.EncounterData != nil && !npc.EncounterData.FirstVisitOnly) {
			choicesY := dialogY + 50 + len(lines)*16 + 20

			if npc.DialogueData.ChoicePrompt != "" {
				ebitenutil.DebugPrintAt(screen, npc.DialogueData.ChoicePrompt, dialogX+20, choicesY)
				choicesY += 20
			}

			for i, choice := range npc.DialogueData.Choices {
				choiceY := choicesY + i*25
				choiceText := fmt.Sprintf("%d. %s", i+1, choice.Text)

				// Highlight selected choice
				if i == ui.game.selectedChoice {
					drawFilledRect(screen, dialogX+20, choiceY-2, dialogWidth-40, 20, color.RGBA{100, 100, 0, 128})
				}

				ebitenutil.DebugPrintAt(screen, choiceText, dialogX+25, choiceY)
			}
		} else {
			// Already visited
			ebitenutil.DebugPrintAt(screen, "The shipwreck appears empty now.", dialogX+20, dialogY+150)
			ebitenutil.DebugPrintAt(screen, "Press ESC to leave.", dialogX+20, dialogY+180)
		}
	}
}

// drawSpellTraderDialog draws dialog for spell trader NPCs
func (ui *UISystem) drawSpellTraderDialog(screen *ebiten.Image, dialogX, dialogY, dialogHeight int) {
	// Draw title
	titleText := fmt.Sprintf("Spell Trader - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw greeting
	greetingText := "Welcome! I can teach you powerful spells for gold."
	ebitenutil.DebugPrintAt(screen, greetingText, dialogX+20, dialogY+50)

	// Draw party gold
	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+400, dialogY+20)

	// Draw character selection header
	ebitenutil.DebugPrintAt(screen, "Select Character:", dialogX+20, dialogY+80)

	// Get currently selected spell for eligibility checking
	var selectedSpell *character.NPCSpell
	if ui.game.selectedSpellKey != "" {
		selectedSpell = ui.game.dialogNPC.SpellData[ui.game.selectedSpellKey]
	}

	// Draw character list
	for i, member := range ui.game.party.Members {
		y := dialogY + 100 + (i * UIRowSpacing)
		charText := fmt.Sprintf("%d. %s (Level %d %s)", i+1, member.Name, member.Level, member.GetClassName())

		// Check if character can learn the selected spell
		canLearn := selectedSpell != nil && ui.characterCanLearnSpell(member, selectedSpell)
		alreadyKnows := selectedSpell != nil && ui.characterKnowsSpell(member, selectedSpell.Name)

		// Color coding and text based on eligibility
		bgColor, statusText := ui.getCharacterSpellStatus(i, canLearn, alreadyKnows, selectedSpell != nil)
		charText += statusText

		// Draw background if needed
		ui.drawUIBackground(screen, dialogX+15, y-2, UICharacterBackgroundWidth, UIRowHeight, bgColor)

		ebitenutil.DebugPrintAt(screen, charText, dialogX+20, y)
	}

	// Draw spells section
	spellsY := dialogY + 100 + (len(ui.game.party.Members) * UIRowSpacing) + 20
	ebitenutil.DebugPrintAt(screen, "Available Spells:", dialogX+20, spellsY)

	// Draw spell list using deterministic ordering
	spellKeys := ui.getAvailableSpellKeys()
	for spellIndex, spellKey := range spellKeys {
		npcSpell := ui.game.dialogNPC.SpellData[spellKey]
		y := spellsY + 20 + (spellIndex * UIRowSpacing)
		spellText := fmt.Sprintf("%d. %s - %d gold", spellIndex+1, npcSpell.Name, npcSpell.Cost)

		// Check if character already knows this spell
		if ui.game.selectedCharIdx >= 0 && ui.game.selectedCharIdx < len(ui.game.party.Members) {
			selectedChar := ui.game.party.Members[ui.game.selectedCharIdx]
			if ui.characterKnowsSpell(selectedChar, npcSpell.Name) {
				spellText += " (Already Known)"
			}
		}

		// Highlight selected spell
		if spellIndex == ui.game.dialogSelectedSpell {
			ui.drawUIBackground(screen, dialogX+15, y-2, UISpellBackgroundWidth, UIRowHeight, UIColorSpellSelection)
		}

		ebitenutil.DebugPrintAt(screen, spellText, dialogX+20, y)
	}

	// Draw instructions
	instructionsY := dialogY + dialogHeight - 95
	ebitenutil.DebugPrintAt(screen, "Arrow Keys: Navigate  |  Mouse: Click to Select", dialogX+20, instructionsY)
	ebitenutil.DebugPrintAt(screen, "Enter or Double-Click: Purchase Spell", dialogX+20, instructionsY+15)
	ebitenutil.DebugPrintAt(screen, "Escape: Close Dialog", dialogX+20, instructionsY+30)
	ebitenutil.DebugPrintAt(screen, "Green: Can Learn  |  Red: Cannot Learn  |  Gray: Knows Spell", dialogX+20, instructionsY+45)
}

// drawMerchantDialog draws a simple seller UI to sell party items
func (ui *UISystem) drawMerchantDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	// Title and greeting
	titleText := fmt.Sprintf("Merchant - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)
	greeting := "Bring your wares. I pay fair coin."
	ebitenutil.DebugPrintAt(screen, greeting, dialogX+20, dialogY+50)

	// Gold
	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+400, dialogY+20)

	// Header
	listY := dialogY + 90
	ebitenutil.DebugPrintAt(screen, "Click an item to sell it: (shows first 15)", dialogX+20, listY)

	// List inventory with values
	startY := listY + 20
	maxItems := 15
	for i := 0; i < len(ui.game.party.Inventory) && i < maxItems; i++ {
		item := ui.game.party.Inventory[i]
		y := startY + i*UIRowSpacing
		// Price from attributes
		price := item.Attributes["value"]
		prefix := fmt.Sprintf("%2d. ", i+1)
		nameField := fmt.Sprintf("%-24s", item.Name)
		suffix := fmt.Sprintf("  %4d gold", price)

		// Hover effect
		mouseX, mouseY := ebiten.CursorPosition()
		isHover := mouseX >= dialogX+18 && mouseX <= dialogX+dialogWidth-18 && mouseY >= y-2 && mouseY <= y-2+UIRowHeight
		if isHover {
			ui.drawUIBackground(screen, dialogX+15, y-2, dialogWidth-30, UIRowHeight, color.RGBA{40, 80, 40, 120})
		}
		drawColoredTextSegments(screen, dialogX+20, y, []coloredTextSegment{
			{text: prefix, color: color.White},
			{text: nameField, color: ui.itemRarityColor(item)},
			{text: suffix, color: color.White},
		})
	}

	// Instructions
	instructionsY := dialogY + dialogHeight - 60
	ebitenutil.DebugPrintAt(screen, "Double-click item: Sell  |  ESC: Close", dialogX+20, instructionsY)
}

// drawGenericDialog draws basic dialog for other NPC types
func (ui *UISystem) drawGenericDialog(screen *ebiten.Image, dialogX, dialogY, _ int) {
	npc := ui.game.dialogNPC

	// Draw title
	titleText := npc.Name
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw basic greeting
	if npc.DialogueData != nil && npc.DialogueData.Greeting != "" {
		lines := ui.wrapText(npc.DialogueData.Greeting, 70)
		for i, line := range lines {
			ebitenutil.DebugPrintAt(screen, line, dialogX+20, dialogY+50+i*16)
		}
	}

	ebitenutil.DebugPrintAt(screen, "Press ESC to close", dialogX+20, dialogY+200)
}

// drawGameOverOverlay draws a simple game over screen with options
func (ui *UISystem) drawGameOverOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()
	// Darken background
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 180})

	// Text
	centerX := w/2 - 160
	centerY := h/2 - 30
	ebitenutil.DebugPrintAt(screen, "GAME OVER", centerX+80, centerY-30)
	ebitenutil.DebugPrintAt(screen, "Press N: New Game", centerX, centerY)
	ebitenutil.DebugPrintAt(screen, "Press L: Load Game", centerX, centerY+20)
}

// drawVictoryOverlay draws the victory screen with score and options
func (ui *UISystem) drawVictoryOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Darken background with golden tint
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{30, 25, 0, 200})

	// Get score data
	scoreData := ui.game.GetScoreData()
	finalScore := CalculateScore(scoreData)
	playTimeStr := FormatPlayTime(scoreData.PlayTime)

	centerX := w / 2
	startY := h/2 - 120

	// Victory header
	ebitenutil.DebugPrintAt(screen, "=== VICTORY! ===", centerX-70, startY)
	ebitenutil.DebugPrintAt(screen, "You have slain all four dragons!", centerX-120, startY+25)
	ebitenutil.DebugPrintAt(screen, "The realm is saved!", centerX-70, startY+45)

	// Score details
	ebitenutil.DebugPrintAt(screen, "--- Final Score ---", centerX-75, startY+80)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Score: %d", finalScore), centerX-50, startY+100)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Gold: %d", scoreData.Gold), centerX-50, startY+120)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Experience: %d", scoreData.TotalExperience), centerX-70, startY+140)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Avg Level: %d", scoreData.AverageLevel), centerX-55, startY+160)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Time: %s", playTimeStr), centerX-50, startY+180)

	// Instructions
	if !ui.game.victoryScoreSaved {
		ebitenutil.DebugPrintAt(screen, "Enter your name:", centerX-60, startY+220)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("> %s_", ui.game.victoryNameInput), centerX-80, startY+240)
		ebitenutil.DebugPrintAt(screen, "Press ENTER to save score", centerX-90, startY+270)
		ebitenutil.DebugPrintAt(screen, "Press ESC for main menu", centerX-85, startY+290)
	} else {
		ebitenutil.DebugPrintAt(screen, "Score saved!", centerX-45, startY+220)
		ebitenutil.DebugPrintAt(screen, "Press H to view High Scores", centerX-100, startY+250)
		ebitenutil.DebugPrintAt(screen, "Press ESC for main menu", centerX-85, startY+270)
	}
}

// drawHighScoresOverlay draws the high scores table
func (ui *UISystem) drawHighScoresOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Darken background
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 30, 220})

	scores, err := LoadHighScores()
	if err != nil {
		ebitenutil.DebugPrintAt(screen, "Error loading high scores", w/2-90, h/2)
		return
	}

	centerX := w / 2
	startY := 60

	// Header
	ebitenutil.DebugPrintAt(screen, "=== HIGH SCORES ===", centerX-75, startY)

	// Column headers
	ebitenutil.DebugPrintAt(screen, "Rank  Name           Score    Time", centerX-140, startY+40)
	ebitenutil.DebugPrintAt(screen, "----  ----           -----    ----", centerX-140, startY+55)

	// Entries
	if len(scores.Entries) == 0 {
		ebitenutil.DebugPrintAt(screen, "No scores yet!", centerX-50, startY+80)
	} else {
		for i, entry := range scores.Entries {
			line := fmt.Sprintf("%2d.   %-14s %6d   %s", i+1, truncateName(entry.PlayerName, 14), entry.Score, entry.PlayTime)
			ebitenutil.DebugPrintAt(screen, line, centerX-140, startY+80+i*20)
		}
	}

	// Instructions
	ebitenutil.DebugPrintAt(screen, "Press ESC to close", centerX-70, h-50)
}

// drawMapOverlay renders the current map with NPCs and teleporters.
func (ui *UISystem) drawMapOverlay(screen *ebiten.Image) {
	if ui.game.world == nil {
		ui.game.mapOverlayOpen = false
		return
	}

	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})

	panelW := int(float64(screenW) * 0.75)
	panelH := int(float64(screenH) * 0.75)
	if panelW > 720 {
		panelW = 720
	}
	if panelH > 560 {
		panelH = 560
	}
	if panelW < 320 {
		panelW = 320
	}
	if panelH < 240 {
		panelH = 240
	}
	panelX := (screenW - panelW) / 2
	panelY := (screenH - panelH) / 2

	drawFilledRect(screen, panelX, panelY, panelW, panelH, color.RGBA{20, 20, 40, 230})
	drawRectBorder(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{100, 100, 160, 255})

	title := "World Map"
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil && mapCfg.Name != "" {
			title = fmt.Sprintf("World Map - %s", mapCfg.Name)
		}
	}
	ebitenutil.DebugPrintAt(screen, title, panelX+16, panelY+12)

	closeX := panelX + panelW - 26
	closeY := panelY + 10
	drawFilledRect(screen, closeX, closeY, 16, 16, color.RGBA{200, 60, 60, 220})
	drawCenteredDebugText(screen, "X", closeX, closeY, 16, 16)
	if ui.game.consumeLeftClickIn(closeX, closeY, closeX+16, closeY+16) {
		ui.game.mapOverlayOpen = false
	}

	mapPadding := 18
	mapX := panelX + mapPadding
	mapY := panelY + 36
	mapW := panelW - mapPadding*2
	mapH := panelH - 54

	worldW := ui.game.world.Width
	worldH := ui.game.world.Height
	if worldW <= 0 || worldH <= 0 {
		return
	}

	tileSize := mapW / worldW
	if alt := mapH / worldH; alt < tileSize {
		tileSize = alt
	}
	if tileSize < 2 {
		tileSize = 2
	}

	originX := mapX + (mapW-worldW*tileSize)/2
	originY := mapY + (mapH-worldH*tileSize)/2

	floorColor := color.RGBA{60, 110, 60, 255}
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil {
			floorColor = color.RGBA{uint8(mapCfg.DefaultFloorColor[0]), uint8(mapCfg.DefaultFloorColor[1]), uint8(mapCfg.DefaultFloorColor[2]), 255}
		}
	}

	for y := 0; y < worldH; y++ {
		for x := 0; x < worldW; x++ {
			tile := ui.game.world.Tiles[y][x]
			cellColor := floorColor
			switch tile {
			case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
				cellColor = color.RGBA{40, 40, 50, 255}
			case world.TileWater:
				cellColor = color.RGBA{40, 90, 160, 255}
			case world.TileDeepWater:
				cellColor = color.RGBA{25, 60, 120, 255}
			case world.TileVioletTeleporter:
				cellColor = color.RGBA{170, 80, 200, 255}
			case world.TileRedTeleporter:
				cellColor = color.RGBA{200, 70, 70, 255}
			}

			drawX := originX + x*tileSize
			drawY := originY + y*tileSize
			vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), cellColor, false)
		}
	}

	// NPCs overlay
	npcColor := color.RGBA{255, 220, 0, 255}
	for _, npc := range ui.game.world.NPCs {
		nx := int(npc.X / float64(ui.game.config.GetTileSize()))
		ny := int(npc.Y / float64(ui.game.config.GetTileSize()))
		if nx < 0 || nx >= worldW || ny < 0 || ny >= worldH {
			continue
		}
		drawX := originX + nx*tileSize
		drawY := originY + ny*tileSize
		size := tileSize
		if size < 3 {
			size = 3
		}
		vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(size), float32(size), npcColor, false)
	}

	// Quest markers overlay (top 3 active quests with RGB colors)
	ui.drawQuestMarkersOnMap(screen, originX, originY, tileSize, worldW, worldH)

	// Player position overlay (cyan dot)
	playerTileX := int(ui.game.camera.X / float64(ui.game.config.GetTileSize()))
	playerTileY := int(ui.game.camera.Y / float64(ui.game.config.GetTileSize()))
	if playerTileX >= 0 && playerTileX < worldW && playerTileY >= 0 && playerTileY < worldH {
		drawX := originX + playerTileX*tileSize
		drawY := originY + playerTileY*tileSize
		markerSize := tileSize + 2
		if markerSize < 5 {
			markerSize = 5
		}
		// Draw player as a cyan circle with border
		vector.DrawFilledCircle(screen, float32(drawX)+float32(tileSize)/2, float32(drawY)+float32(tileSize)/2, float32(markerSize)/2, color.RGBA{50, 200, 255, 255}, true)
		vector.StrokeCircle(screen, float32(drawX)+float32(tileSize)/2, float32(drawY)+float32(tileSize)/2, float32(markerSize)/2, 1, color.RGBA{255, 255, 255, 255}, true)
	}
}

// drawQuestMarkersOnMap draws quest objective markers on the map overlay
// Shows top 3 active quests with RGB color coding (1=Red, 2=Green, 3=Blue)
func (ui *UISystem) drawQuestMarkersOnMap(screen *ebiten.Image, originX, originY, tileSize, worldW, worldH int) {
	if ui.game.questManager == nil {
		return
	}

	// Get active quests (not completed)
	activeQuests := ui.game.questManager.GetActiveQuests()
	if len(activeQuests) == 0 {
		return
	}

	// Limit to top 3 quests
	maxQuests := 3
	if len(activeQuests) < maxQuests {
		maxQuests = len(activeQuests)
	}

	// Quest marker colors: Red, Green, Blue for quests 1, 2, 3
	questColors := []color.RGBA{
		{255, 80, 80, 255}, // Red for quest 1
		{80, 255, 80, 255}, // Green for quest 2
		{80, 80, 255, 255}, // Blue for quest 3
	}

	// Get current map key to filter markers
	currentMapKey := ""
	if world.GlobalWorldManager != nil {
		currentMapKey = world.GlobalWorldManager.CurrentMapKey
	}

	for i := 0; i < maxQuests; i++ {
		quest := activeQuests[i]
		def := quest.Definition

		// Skip quests without marker coordinates
		if def.MarkerX == 0 && def.MarkerY == 0 {
			continue
		}

		// Skip quests for different maps
		if def.MarkerMap != "" && def.MarkerMap != currentMapKey {
			continue
		}

		markerX := def.MarkerX
		markerY := def.MarkerY

		// Validate coordinates are within world bounds
		if markerX < 0 || markerX >= worldW || markerY < 0 || markerY >= worldH {
			continue
		}

		// Calculate screen position
		drawX := originX + markerX*tileSize
		drawY := originY + markerY*tileSize
		markerSize := tileSize + 4
		if markerSize < 8 {
			markerSize = 8
		}

		// Draw quest marker as a diamond shape with number
		centerX := float32(drawX) + float32(tileSize)/2
		centerY := float32(drawY) + float32(tileSize)/2
		halfSize := float32(markerSize) / 2

		// Draw diamond shape using lines
		markerColor := questColors[i]
		vector.StrokeLine(screen, centerX, centerY-halfSize, centerX+halfSize, centerY, 2, markerColor, true)
		vector.StrokeLine(screen, centerX+halfSize, centerY, centerX, centerY+halfSize, 2, markerColor, true)
		vector.StrokeLine(screen, centerX, centerY+halfSize, centerX-halfSize, centerY, 2, markerColor, true)
		vector.StrokeLine(screen, centerX-halfSize, centerY, centerX, centerY-halfSize, 2, markerColor, true)

		// Draw quest number in center
		questNum := fmt.Sprintf("%d", i+1)
		ebitenutil.DebugPrintAt(screen, questNum, int(centerX)-3, int(centerY)-6)
	}
}

// drawQuestsContent draws the quests tab content
func (ui *UISystem) drawQuestsContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== ACTIVE QUESTS ===", panelX+20, contentY+10)

	// Check if quest manager is available
	if ui.game.questManager == nil {
		ebitenutil.DebugPrintAt(screen, "No quests available.", panelX+20, contentY+40)
		return
	}

	allQuests := ui.game.questManager.GetAllQuests()
	if len(allQuests) == 0 {
		ebitenutil.DebugPrintAt(screen, "No active quests.", panelX+20, contentY+40)
		return
	}

	// Sort quests: active first, then completed
	sort.Slice(allQuests, func(i, j int) bool {
		if allQuests[i].Completed != allQuests[j].Completed {
			return !allQuests[i].Completed // Active quests first
		}
		return allQuests[i].Definition.Name < allQuests[j].Definition.Name
	})

	mouseX, mouseY := ebiten.CursorPosition()
	questY := contentY + 40
	questHeight := 95 // Height of each quest entry (increased for wrapped text)
	questWidth := 520

	for _, quest := range allQuests {
		// Draw quest background
		// Different colors based on quest status
		var bgColor color.RGBA
		if quest.Completed && !quest.RewardsClaimed {
			bgColor = color.RGBA{40, 80, 40, 200} // Green for completed, reward available
		} else if quest.Completed {
			bgColor = color.RGBA{40, 40, 40, 150} // Gray for completed and claimed
		} else {
			bgColor = color.RGBA{30, 30, 60, 200} // Blue for active
		}
		drawFilledRect(screen, panelX+20, questY, questWidth, questHeight, bgColor)

		// Draw quest border
		borderColor := color.RGBA{80, 80, 120, 255}
		if quest.Completed && !quest.RewardsClaimed {
			borderColor = color.RGBA{100, 200, 100, 255} // Green border for claimable
		}
		vector.StrokeRect(screen, float32(panelX+20), float32(questY), float32(questWidth), float32(questHeight), 2, borderColor, false)

		// Quest name
		namePrefix := ""
		if quest.Completed {
			namePrefix = "[DONE] "
		}
		ebitenutil.DebugPrintAt(screen, namePrefix+quest.Definition.Name, panelX+30, questY+6)

		// Quest description - wrap to fit within box (max ~70 chars per line)
		descLines := wrapText(quest.Definition.Description, 70)
		for i, line := range descLines {
			if i >= 2 { // Max 2 lines for description
				break
			}
			ebitenutil.DebugPrintAt(screen, line, panelX+30, questY+22+(i*14))
		}

		// Bottom row: Progress on left, Rewards on right
		bottomY := questY + 54

		// Progress for kill quests
		if quest.Definition.Type == "kill" {
			progressText := quest.GetProgressString()
			ebitenutil.DebugPrintAt(screen, progressText, panelX+30, bottomY)

			// Draw progress bar below text
			barX := panelX + 30
			barY := questY + 72
			barWidth := 180
			barHeight := 14

			// Background bar
			drawFilledRect(screen, barX, barY, barWidth, barHeight, color.RGBA{20, 20, 20, 255})

			// Progress fill
			progress := float64(quest.CurrentCount) / float64(quest.Definition.TargetCount)
			if progress > 1 {
				progress = 1
			}
			fillWidth := int(float64(barWidth) * progress)
			if fillWidth > 0 {
				var fillColor color.RGBA
				if quest.Completed {
					fillColor = color.RGBA{80, 200, 80, 255} // Green when complete
				} else {
					fillColor = color.RGBA{80, 150, 200, 255} // Blue while in progress
				}
				drawFilledRect(screen, barX, barY, fillWidth, barHeight, fillColor)
			}

			// Progress bar border
			vector.StrokeRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), 1, color.RGBA{100, 100, 100, 255}, false)
		} else if quest.Definition.Type == "encounter" {
			// Encounter quests show objective text instead of progress bar
			var objectiveText string
			if quest.Completed {
				objectiveText = "All enemies defeated!"
			} else {
				objectiveText = "Defeat all enemies"
			}
			ebitenutil.DebugPrintAt(screen, objectiveText, panelX+30, bottomY)
		}

		// Rewards section (right side)
		rewardsX := panelX + 300
		rewardsText := fmt.Sprintf("Reward: %dg / %dxp", quest.Definition.Rewards.Gold, quest.Definition.Rewards.Experience)
		ebitenutil.DebugPrintAt(screen, rewardsText, rewardsX, bottomY)

		// Claim button for completed quests with unclaimed rewards
		if quest.Completed && !quest.RewardsClaimed {
			buttonX := rewardsX
			buttonY := questY + 72
			buttonWidth := 110
			buttonHeight := 16

			isHovering := isMouseHoveringBox(mouseX, mouseY, buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight)

			if isHovering {
				drawFilledRect(screen, buttonX, buttonY, buttonWidth, buttonHeight, color.RGBA{100, 200, 100, 255}) // Bright green on hover
			} else {
				drawFilledRect(screen, buttonX, buttonY, buttonWidth, buttonHeight, color.RGBA{60, 150, 60, 255}) // Green
			}
			drawCenteredDebugText(screen, "Claim Reward", buttonX, buttonY, buttonWidth, buttonHeight)

			// Handle click on claim button
			if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
				ui.claimQuestReward(quest.ID)
			}
		}

		questY += questHeight + 8
	}
}

// characterKnowsSpell checks if a character already knows a spell
func (ui *UISystem) characterKnowsSpell(char *character.MMCharacter, spellName string) bool {
	for _, magicSkill := range char.MagicSchools {
		for _, spellID := range magicSkill.KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.Name == spellName {
				return true
			}
		}
	}
	return false
}

// characterCanLearnSpell checks if a character can learn a specific spell based on class and magic schools
func (ui *UISystem) characterCanLearnSpell(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	if spellData == nil {
		return false
	}

	// Check class restrictions for magic schools
	switch spellData.School {
	case "water":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "fire":
		return char.Class == character.ClassSorcerer
	case "air":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassArcher
	case "earth":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassDruid
	case "body":
		return char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "mind":
		return char.Class == character.ClassCleric
	case "spirit":
		return char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "light":
		return char.Class == character.ClassCleric
	case "dark":
		return false // Dark magic typically not learnable from NPCs
	default:
		return false
	}
}

// claimQuestReward claims the reward for a completed quest
func (ui *UISystem) claimQuestReward(questID string) {
	if ui.game.questManager == nil {
		return
	}

	rewards, err := ui.game.questManager.ClaimRewards(questID)
	if err != nil {
		ui.game.AddCombatMessage(fmt.Sprintf("Cannot claim reward: %s", err.Error()))
		return
	}

	// Award gold
	if rewards.Gold > 0 {
		ui.game.party.Gold += rewards.Gold
	}

	// Award experience to all living party members
	if rewards.Experience > 0 {
		for _, member := range ui.game.party.Members {
			if member.HitPoints > 0 {
				member.Experience += rewards.Experience
				ui.game.combat.checkLevelUp(member)
			}
		}
	}

	quest := ui.game.questManager.GetQuest(questID)
	if quest != nil {
		ui.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Received %d gold and %d XP!",
			quest.Definition.Name, rewards.Gold, rewards.Experience))
	}
}

// truncateName truncates a name to maxLen characters
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-2] + ".."
}

// wrapText wraps text to fit within a given character width (standalone)
func wrapText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= maxChars {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
