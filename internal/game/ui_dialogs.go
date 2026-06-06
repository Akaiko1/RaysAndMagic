package game

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/highscore"
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

// statHoldInitialDelay — frames the user has to hold the mouse on a +button
// before hold-to-repeat starts firing. Configured for the game's 120 TPS so
// ~340 ms keeps single clicks pure (no accidental double-spend) while a
// deliberate hold takes over fast.
// statHoldRepeatRate — frames between hold-fired increments after the delay.
// At 120 TPS ≈ 67 ms / ~15 stats per second when held.
const (
	statHoldInitialDelay = 40
	statHoldRepeatRate   = 8
)

// drawStatPointRow draws a single stat row with name, value, and + button
func (ui *UISystem) drawStatPointRow(screen *ebiten.Image, name string, valuePtr *int, y, plusX, plusY, btnW, btnH int, canAdd, isHover *bool, clickIn bool) bool {
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
	vector.FillRect(screen, float32(plusX), float32(plusY), float32(btnW), float32(btnH), plusColor, false)
	ui.drawInterfaceIcon(screen, "icon_stat_up", plusX+2, plusY+2, btnW-4, btnH-4)
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
	// Bound to statPopupCharIdx — the character whose "+" button was clicked
	// (set at the open site in ui_hud.go). NOT selectedChar: in turn-based mode
	// selectedChar tracks the active turn, so binding to it opened the wrong
	// character's popup when you clicked "+" on someone who had already acted.
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
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	for i, stat := range statList {
		y := yStart + i*36
		plusX := popupX + 180
		plusY := y - 4
		canAdd := member.FreeStatPoints > 0
		isHover := mouseX >= plusX && mouseX < plusX+btnW && mouseY >= plusY && mouseY < plusY+btnH
		clickIn := ui.game.consumeLeftClickIn(plusX, plusY, plusX+btnW, plusY+btnH)

		// Hold-to-repeat: once the user keeps the button held over the same
		// +button past statHoldInitialDelay, fire an extra increment every
		// statHoldRepeatRate frames. Single clicks still come through clickIn
		// above unchanged.
		if isHover && mousePressed {
			if ui.statHoldStat == stat.Name {
				ui.statHoldFrames++
				if ui.statHoldFrames > statHoldInitialDelay &&
					(ui.statHoldFrames-statHoldInitialDelay)%statHoldRepeatRate == 0 {
					clickIn = true
				}
			} else {
				ui.statHoldStat = stat.Name
				ui.statHoldFrames = 0
			}
		} else if ui.statHoldStat == stat.Name {
			ui.statHoldStat = ""
			ui.statHoldFrames = 0
		}

		if ui.drawStatPointRow(screen, stat.Name, stat.Ptr, y, plusX, plusY, btnW, btnH, &canAdd, &isHover, clickIn) {
			member.FreeStatPoints--
			// Recompute HP/SP caps for the raised stat WITHOUT full-healing:
			// spending a point grants only that stat's bonus, not a free heal.
			member.RecalculateMaxStatsKeepingCurrent(ui.game.config)
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
	ui.drawInterfaceIcon(screen, "icon_close", closeX+2, closeY+2, 24, 24)
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

// drawRevivalPickerPopup draws the "Choose who to revive" overlay opened
// when a revival potion is used while 2+ party members are dead or
// unconscious. The list is recomputed every frame from the current party
// state so a member dying mid-popup naturally appears, and a member already
// revived disappears. Closing without a click cancels (potion not spent).
func (ui *UISystem) drawRevivalPickerPopup(screen *ebiten.Image) {
	targets := ui.game.RevivablePartyIndices()
	if len(targets) == 0 {
		// No one left to revive (cured externally?) — close cleanly.
		ui.game.revivalPickerOpen = false
		return
	}

	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	popupW := 360
	rowH := 28
	popupH := 100 + len(targets)*rowH
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2

	// Dim background
	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})

	// Panel
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 240})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{120, 120, 180, 255})

	ebitenutil.DebugPrintAt(screen, "Revive Whom?", popupX+16, popupY+16)
	ebitenutil.DebugPrintAt(screen, "Click a fallen party member.", popupX+16, popupY+36)

	mouseX, mouseY := ebiten.CursorPosition()
	startY := popupY + 64
	for row, idx := range targets {
		y := startY + row*rowH
		member := ui.game.party.Members[idx]
		isHover := mouseX >= popupX+16 && mouseX < popupX+popupW-16 &&
			mouseY >= y-2 && mouseY < y-2+rowH
		if isHover {
			drawFilledRect(screen, popupX+16, y-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}

		status := "Unconscious"
		if member.HasCondition(character.ConditionDead) {
			status = "Dead"
		}
		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("%d) %s — %s  (HP:%d/%d)", idx+1, member.Name, status, member.HitPoints, member.MaxHitPoints),
			popupX+24, y+6)

		if isHover && ui.game.consumeLeftClickIn(popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			ui.game.applyReviveTo(ui.game.revivalPickerItemIdx, idx)
			ui.game.revivalPickerOpen = false
			return
		}
	}

	// Close (X) button — cancel without spending the potion.
	closeX := popupX + popupW - 36
	closeY := popupY + 12
	if mouseX >= closeX && mouseX < closeX+24 && mouseY >= closeY && mouseY < closeY+24 {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{200, 60, 60, 220})
	} else {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{120, 60, 60, 180})
	}
	ui.drawInterfaceIcon(screen, "icon_close", closeX+2, closeY+2, 20, 20)
	if ui.game.consumeLeftClickIn(closeX, closeY, closeX+24, closeY+24) {
		ui.game.revivalPickerOpen = false
		return
	}
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		ui.game.revivalPickerOpen = false
	}
}

// drawRosterScreen is the tavern party-management modal: a left column of the 4
// active members and a right column of the reserve roster. Click an active slot
// to select it, then click a reserve hero to swap them (they keep all gear/XP/
// skills). A "!" flags a hero with unspent stat points or an owed level-up choice.
func (ui *UISystem) drawRosterScreen(screen *ebiten.Image) {
	g := ui.game
	screenW := g.config.GetScreenWidth()
	screenH := g.config.GetScreenHeight()
	popupW, popupH := 560, 360
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2
	rowH := 30
	colW := (popupW - 48) / 2
	leftX := popupX + 16
	rightX := popupX + 32 + colW
	listY := popupY + 70

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 150})
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 244})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{150, 110, 52, 230})
	ebitenutil.DebugPrintAt(screen, "Tavern — Manage Roster", popupX+16, popupY+14)
	ebitenutil.DebugPrintAt(screen, "Click an active hero, then a reserve hero to swap.", popupX+16, popupY+34)
	ebitenutil.DebugPrintAt(screen, "Active Party", leftX, listY-16)
	ebitenutil.DebugPrintAt(screen, "Reserve (tavern)", rightX, listY-16)

	mouseX, mouseY := ebiten.CursorPosition()
	label := func(m *character.MMCharacter) string {
		flag := ""
		if m.FreeStatPoints > 0 || len(m.OwedLevelChoices) > 0 {
			flag = " !"
		}
		return fmt.Sprintf("%s — %s Lv.%d%s", m.Name, m.ClassDisplayName(), m.Level, flag)
	}

	// Active column
	for i, m := range g.party.Members {
		y := listY + i*rowH
		hover := mouseX >= leftX && mouseX < leftX+colW && mouseY >= y-2 && mouseY < y-2+rowH
		if i == g.rosterSelectedActive {
			drawFilledRect(screen, leftX, y-2, colW, rowH, color.RGBA{90, 120, 60, 220})
		} else if hover {
			drawFilledRect(screen, leftX, y-2, colW, rowH, color.RGBA{60, 120, 180, 180})
		}
		ebitenutil.DebugPrintAt(screen, label(m), leftX+6, y+6)
		if hover && g.consumeLeftClickIn(leftX, y-2, leftX+colW, y-2+rowH) {
			g.rosterSelectedActive = i
		}
	}

	// Reserve column
	for j, m := range g.party.Reserve {
		y := listY + j*rowH
		hover := mouseX >= rightX && mouseX < rightX+colW && mouseY >= y-2 && mouseY < y-2+rowH
		if hover {
			drawFilledRect(screen, rightX, y-2, colW, rowH, color.RGBA{60, 120, 180, 180})
		}
		ebitenutil.DebugPrintAt(screen, label(m), rightX+6, y+6)
		if hover && g.consumeLeftClickIn(rightX, y-2, rightX+colW, y-2+rowH) {
			if g.rosterSelectedActive >= 0 {
				g.swapRosterMember(g.rosterSelectedActive, j)
				g.rosterSelectedActive = -1
			}
		}
	}
	if len(g.party.Reserve) == 0 {
		ebitenutil.DebugPrintAt(screen, "(no benched heroes yet)", rightX+6, listY+6)
	}

	// Close button
	closeX := popupX + popupW - 36
	closeY := popupY + 10
	if mouseX >= closeX && mouseX < closeX+24 && mouseY >= closeY && mouseY < closeY+24 {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{200, 60, 60, 220})
	} else {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{120, 60, 60, 180})
	}
	ui.drawInterfaceIcon(screen, "icon_close", closeX+2, closeY+2, 20, 20)
	if g.consumeLeftClickIn(closeX, closeY, closeX+24, closeY+24) || ebiten.IsKeyPressed(ebiten.KeyEscape) {
		g.rosterScreenOpen = false
		g.rosterSelectedActive = -1
	}
}

// drawPromotionPickerPopup lists the party members eligible for the pending
// promotion (Archmage/Lich) and applies it to whomever the player clicks.
// Mirrors the revival picker. Cannot be cancelled — a quest/phylactery has
// already committed to the promotion by the time this opens.
func (ui *UISystem) drawPromotionPickerPopup(screen *ebiten.Image) {
	kind := ui.game.promotionPickerKind
	var targets []int
	title := "Promote Whom?"
	if kind == character.PromotionArchmage {
		targets = ui.game.eligibleArchmageIndices()
		title = "Who Becomes the Archmage?"
	} else {
		targets = ui.game.eligibleLichIndices()
		title = "Who Becomes the Lich?"
	}
	if len(targets) == 0 {
		ui.game.promotionPickerOpen = false
		return
	}

	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	popupW := 380
	rowH := 28
	popupH := 100 + len(targets)*rowH
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 240})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{120, 120, 180, 255})

	ebitenutil.DebugPrintAt(screen, title, popupX+16, popupY+16)
	ebitenutil.DebugPrintAt(screen, "Click a party member.", popupX+16, popupY+36)

	mouseX, mouseY := ebiten.CursorPosition()
	startY := popupY + 64
	for row, idx := range targets {
		y := startY + row*rowH
		member := ui.game.party.Members[idx]
		isHover := mouseX >= popupX+16 && mouseX < popupX+popupW-16 && mouseY >= y-2 && mouseY < y-2+rowH
		if isHover {
			drawFilledRect(screen, popupX+16, y-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}
		ebitenutil.DebugPrintAt(screen,
			fmt.Sprintf("%d) %s the %s (Lv.%d)", idx+1, member.Name, member.Class.String(), member.Level),
			popupX+24, y+6)
		if isHover && ui.game.consumeLeftClickIn(popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			itemIdx := ui.game.promotionPickerItemIdx
			ui.game.promotionPickerOpen = false
			ui.game.applyPromotionKind(kind, idx, itemIdx)
			return
		}
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

	title := req.title
	if title == "" {
		title = "Level Up Choice"
	}
	ebitenutil.DebugPrintAt(screen, title, popupX+16, popupY+16)
	if req.isMultiSelect() {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: choose %d (%d selected)", member.Name, req.maxSelections, req.selectedCount()), popupX+16, popupY+36)
	} else {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s reached level %d", member.Name, req.level), popupX+16, popupY+36)
	}

	for i, option := range req.options {
		y := startY + i*rowH
		if i == req.selection {
			drawFilledRect(screen, popupX+16, y-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}
		// Multi-select: a check icon marks toggled options; cursor uses the icon too.
		if req.isMultiSelect() && i < len(req.selected) && req.selected[i] {
			ui.drawInterfaceIcon(screen, "icon_level_choice", popupX+20, y, 16, 16)
		} else if !req.isMultiSelect() && i == req.selection {
			ui.drawInterfaceIcon(screen, "icon_level_choice", popupX+20, y, 16, 16)
		}
		if option.hasMastery && option.masteryCurrent != "" && option.masteryNext != "" {
			segments := []coloredTextSegment{
				{text: option.masteryPrefix, color: color.White},
				{text: option.masteryCurrent, color: color.RGBA{240, 220, 80, 255}},
				{text: " -> ", color: color.White},
				{text: option.masteryNext, color: color.RGBA{80, 220, 80, 255}},
			}
			drawColoredTextSegments(screen, popupX+40, y, segments)
		} else {
			ebitenutil.DebugPrintAt(screen, option.label, popupX+40, y)
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
				icon := ""
				if strings.ToLower(option.choice.Type) == "spell" {
					icon = spellTooltipIconName(option.spellID)
				}
				ui.queueTooltipIcon(strings.Split(tooltip, "\n"), icon, mouseX+16, mouseY+8)
			}
		}
	}

	if req.isMultiSelect() {
		// Confirm row, drawn just below the options.
		cy := startY + len(req.options)*rowH
		ready := req.selectedCount() == req.maxSelections
		confirmCol := color.RGBA{150, 150, 150, 255}
		if ready {
			confirmCol = color.RGBA{90, 220, 90, 255}
		}
		if req.selection == req.confirmRowIndex() {
			drawFilledRect(screen, popupX+16, cy-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}
		drawDebugTextColored(screen, "Confirm", popupX+40, cy, confirmCol)
		ebitenutil.DebugPrintAt(screen, "↑/↓ move · Space toggles · Enter confirms", popupX+16, popupY+popupH-22)
	} else {
		ebitenutil.DebugPrintAt(screen, "Use ↑/↓ or click, Enter to choose", popupX+16, popupY+popupH-22)
	}
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

	// Handle different NPC capabilities (data-driven)
	switch {
	case npcHasChoiceDialog(ui.game.dialogNPC):
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case npcHasSpellTrading(ui.game.dialogNPC):
		ui.drawSpellTraderDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case npcHasSkillTraining(ui.game.dialogNPC):
		ui.drawSkillTrainerDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case npcHasMerchant(ui.game.dialogNPC):
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
			visitedMessage := ""
			if npc.DialogueData != nil {
				visitedMessage = npc.DialogueData.VisitedMessage
			}
			if visitedMessage != "" {
				lines := ui.wrapText(visitedMessage, 70)
				for i, line := range lines {
					ebitenutil.DebugPrintAt(screen, line, dialogX+20, dialogY+150+i*16)
				}
				ebitenutil.DebugPrintAt(screen, "Press ESC to leave.", dialogX+20, dialogY+150+len(lines)*16+20)
			} else {
				ebitenutil.DebugPrintAt(screen, "Press ESC to leave.", dialogX+20, dialogY+150)
			}
		}
	}
}

// Spell-trader layout constants.
const (
	spellTraderPortraitSize = 56
	spellTraderPortraitGap  = 28 // wider gap so name labels under portraits don't overlap.
	spellTraderIconSize     = 48
	spellTraderIconGap      = 10
	spellTraderGridCols     = 6
)

// spellTraderPortraitRect returns the screen rect for the i-th party
// portrait in the spell-trader dialog.
func spellTraderPortraitRect(dialogX, dialogY, i int) (x, y, w, h int) {
	stripW := 4*spellTraderPortraitSize + 3*spellTraderPortraitGap
	startX := dialogX + (600-stripW)/2
	return startX + i*(spellTraderPortraitSize+spellTraderPortraitGap),
		dialogY + 78,
		spellTraderPortraitSize,
		spellTraderPortraitSize
}

// spellTraderIconRect returns the screen rect for the i-th spell icon.
func spellTraderIconRect(dialogX, dialogY, i int) (x, y, w, h int) {
	gridW := spellTraderGridCols*spellTraderIconSize + (spellTraderGridCols-1)*spellTraderIconGap
	startX := dialogX + (600-gridW)/2
	gridY := dialogY + 78 + spellTraderPortraitSize + 32
	row := i / spellTraderGridCols
	col := i % spellTraderGridCols
	cellH := spellTraderIconSize + 14 // icon + cost line
	return startX + col*(spellTraderIconSize+spellTraderIconGap),
		gridY + row*(cellH+8),
		spellTraderIconSize,
		spellTraderIconSize
}

// drawSpellTraderDialog draws an icon-based spell trader UI: 4-character
// portrait strip at top, icon grid for spells below, tooltip on hover.
func (ui *UISystem) drawSpellTraderDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	titleText := fmt.Sprintf("Spell Trader - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	greetingText := "Welcome! I can teach you powerful spells for gold."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greetingText = ui.game.dialogNPC.DialogueData.Greeting
	}
	ebitenutil.DebugPrintAt(screen, greetingText, dialogX+20, dialogY+44)

	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+dialogWidth-160, dialogY+20)

	// Portrait strip — click to switch active character.
	mouseX, mouseY := ebiten.CursorPosition()
	for i, member := range ui.game.party.Members {
		x, y, w, h := spellTraderPortraitRect(dialogX, dialogY, i)
		if i == ui.game.selectedCharIdx {
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{210, 170, 80, 240})
		} else if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{120, 120, 160, 200})
		}
		portrait := ui.game.sprites.GetSprite(ui.game.portraitSpriteName(member))
		drawImageScaled(screen, portrait, x, y, w, h)
		label := fmt.Sprintf("%s L%d", member.Name, member.Level)
		drawCenteredDebugText(screen, label, x-8, y+h+2, w+16, debugTextCharHeight)
	}

	// Spell icon grid.
	spellKeys := npcSpellKeys(ui.game.dialogNPC)
	var selectedChar *character.MMCharacter
	if ui.game.selectedCharIdx >= 0 && ui.game.selectedCharIdx < len(ui.game.party.Members) {
		selectedChar = ui.game.party.Members[ui.game.selectedCharIdx]
	}

	var hoverSpellIdx = -1
	for i, spellKey := range spellKeys {
		x, y, w, h := spellTraderIconRect(dialogX, dialogY, i)
		npcSpell := ui.game.dialogNPC.SpellData[spellKey]

		// Determine status for the selected character.
		canLearn := selectedChar != nil && ui.characterCanLearnSpell(selectedChar, npcSpell)
		alreadyKnows := selectedChar != nil && characterKnowsSpellByName(selectedChar, npcSpell.Name)

		// Frame: selected = bright gold; can-learn = green; cannot = red; known = gray.
		switch {
		case i == ui.game.dialogSelectedSpell:
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{220, 180, 60, 255})
		case alreadyKnows:
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{120, 120, 120, 220})
		case selectedChar != nil && canLearn:
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{80, 200, 80, 220})
		case selectedChar != nil:
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{200, 80, 80, 220})
		}

		// Icon.
		iconName := spellTooltipIconName(spells.SpellID(spellKey))
		if ui.game.sprites.HasSprite(iconName) {
			drawImageScaled(screen, ui.game.sprites.GetSprite(iconName), x, y, w, h)
		} else {
			drawFilledRect(screen, x, y, w, h, color.RGBA{42, 32, 45, 255})
			drawCenteredDebugText(screen, spellInitials(npcSpell.Name), x, y, w, h)
		}

		// Cost under icon.
		costText := fmt.Sprintf("%d g", npcSpell.Cost)
		drawCenteredDebugText(screen, costText, x-4, y+h+2, w+8, debugTextCharHeight)

		// Dim overlay if known.
		if alreadyKnows {
			drawFilledRect(screen, x, y, w, h, color.RGBA{0, 0, 0, 130})
		}

		if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
			hoverSpellIdx = i
		}
	}

	// Hover tooltip — name + school + cost + requirements.
	if hoverSpellIdx >= 0 {
		spellKey := spellKeys[hoverSpellIdx]
		npcSpell := ui.game.dialogNPC.SpellData[spellKey]
		lines := []string{
			npcSpell.Name,
			fmt.Sprintf("%s school   %d gold", strings.Title(npcSpell.School), npcSpell.Cost),
		}
		if npcSpell.Requirements != nil {
			if npcSpell.Requirements.MinLevel > 0 {
				lines = append(lines, fmt.Sprintf("Requires char level %d", npcSpell.Requirements.MinLevel))
			}
			for _, req := range npcSpell.Requirements.Schools {
				lines = append(lines, fmt.Sprintf("Requires %s magic L%d", strings.Title(req.School), req.MinLevel))
			}
		}
		ui.queueTooltipIcon(lines, spellTooltipIconName(spells.SpellID(spellKey)), mouseX+16, mouseY+8)
	}

	// Instructions.
	instructionsY := dialogY + dialogHeight - 60
	ebitenutil.DebugPrintAt(screen, "Click portrait: select character  |  Hover spell: details", dialogX+20, instructionsY)
	ebitenutil.DebugPrintAt(screen, "Click spell: select  |  Double-click: purchase  |  ESC: close", dialogX+20, instructionsY+15)
	ebitenutil.DebugPrintAt(screen, "Gold frame: selected | Green: can learn | Red: cannot | Gray: known", dialogX+20, instructionsY+30)
}

// Skill-trainer layout.
const (
	skillTrainerPortraitSize = 80
	skillTrainerPortraitGap  = 24
)

func skillTrainerPortraitRect(dialogX, dialogY, dialogWidth, i int) (x, y, w, h int) {
	stripW := 4*skillTrainerPortraitSize + 3*skillTrainerPortraitGap
	startX := dialogX + (dialogWidth-stripW)/2
	return startX + i*(skillTrainerPortraitSize+skillTrainerPortraitGap),
		dialogY + 120,
		skillTrainerPortraitSize,
		skillTrainerPortraitSize
}

// skillTrainerOptionRect returns the rect for the i-th mastery option
// inside the modal popup (top-anchored).
func skillTrainerOptionRect(popupX, popupY, i int) (x, y, w, h int) {
	return popupX + 12, popupY + 56 + i*UIRowSpacing, 396, UIRowHeight
}

func skillTrainerPopupRect(dialogX, dialogY, dialogWidth, dialogHeight int) (x, y, w, h int) {
	popupW := 420
	popupH := dialogHeight - 60
	return dialogX + (dialogWidth-popupW)/2, dialogY + 30, popupW, popupH
}

func (ui *UISystem) drawSkillTrainerDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	titleText := fmt.Sprintf("Mastery Trainer - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	greeting := "Choose a character to view trainable masteries."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greeting = ui.game.dialogNPC.DialogueData.Greeting
	}
	ebitenutil.DebugPrintAt(screen, greeting, dialogX+20, dialogY+44)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Party Gold: %d", ui.game.party.Gold), dialogX+dialogWidth-160, dialogY+20)

	// Portrait row.
	mouseX, mouseY := ebiten.CursorPosition()
	for i, member := range ui.game.party.Members {
		x, y, w, h := skillTrainerPortraitRect(dialogX, dialogY, dialogWidth, i)
		hover := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
		if hover && !ui.game.skillTrainerPopup {
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{210, 170, 80, 240})
		}
		drawImageScaled(screen, ui.game.sprites.GetSprite(ui.game.portraitSpriteName(member)), x, y, w, h)
		drawCenteredDebugText(screen, member.Name, x-8, y+h+4, w+16, debugTextCharHeight)
		drawCenteredDebugText(screen, fmt.Sprintf("Level %d %s", member.Level, member.ClassDisplayName()), x-8, y+h+20, w+16, debugTextCharHeight)
	}

	instructionsY := dialogY + dialogHeight - 40
	ebitenutil.DebugPrintAt(screen, "Click a portrait to view trainable masteries  |  ESC: close", dialogX+20, instructionsY)

	// Modal popup on top when character was clicked.
	if ui.game.skillTrainerPopup &&
		ui.game.selectedCharIdx >= 0 &&
		ui.game.selectedCharIdx < len(ui.game.party.Members) {
		ui.drawSkillTrainerPopup(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	}
}

func (ui *UISystem) drawSkillTrainerPopup(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	px, py, pw, ph := skillTrainerPopupRect(dialogX, dialogY, dialogWidth, dialogHeight)
	drawFilledRect(screen, px, py, pw, ph, color.RGBA{30, 30, 50, 245})
	drawRectBorder(screen, px, py, pw, ph, 3, color.RGBA{180, 150, 80, 240})

	member := ui.game.party.Members[ui.game.selectedCharIdx]
	header := fmt.Sprintf("%s - Trainable Masteries", member.Name)
	drawCenteredDebugText(screen, header, px, py+10, pw, 18)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Gold: %d", ui.game.party.Gold), px+12, py+30)

	options := trainerOptions(member)
	if len(options) == 0 {
		drawCenteredDebugText(screen, "All known masteries are already maxed.", px, py+ph/2-8, pw, 16)
	} else {
		mouseX, mouseY := ebiten.CursorPosition()
		maxOptions := (ph - 90) / UIRowSpacing
		for i := 0; i < len(options) && i < maxOptions; i++ {
			option := options[i]
			x, y, w, h := skillTrainerOptionRect(px, py, i)
			hover := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
			if i == ui.game.dialogSelectedSpell {
				ui.drawUIBackground(screen, x, y-2, w, h+4, color.RGBA{80, 130, 70, 200})
			} else if hover {
				ui.drawUIBackground(screen, x, y-2, w, h+4, color.RGBA{60, 70, 100, 160})
			}
			label := fmt.Sprintf("%s:  %s -> %s   %d gold", option.Label, option.Current.String(), option.Next.String(), option.Cost)
			if option.Cost > ui.game.party.Gold {
				label += "  (Need Gold)"
			}
			ebitenutil.DebugPrintAt(screen, label, x+6, y)
		}
	}

	ebitenutil.DebugPrintAt(screen, "Click to select  |  Double-click: train  |  ESC/Back: party list", px+12, py+ph-22)
}

// partyMerchantTier returns the best Merchant mastery tier among active members.
func (g *MMGame) partyMerchantTier() int {
	best := 0
	for _, m := range g.party.Members {
		if m != nil {
			if t := m.MerchantTier(); t > best {
				best = t
			}
		}
	}
	return best
}

// merchantBuyPrice / merchantSellPrice apply the party's Merchant haggling:
// cheaper to buy, more gold when selling, scaled by the best Merchant tier.
func (g *MMGame) merchantBuyPrice(base int) int {
	p := base - base*g.partyMerchantTier()*MerchantPricePctPerTier/100
	if p < 1 {
		p = 1
	}
	return p
}

func (g *MMGame) merchantSellPrice(base int) int {
	return base + base*g.partyMerchantTier()*MerchantPricePctPerTier/100
}

// drawMerchantDialog draws a buy/sell UI for merchant NPCs
func (ui *UISystem) drawMerchantDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	// Title and greeting
	titleText := fmt.Sprintf("Merchant - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)
	greeting := "Bring your wares. I pay fair coin."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greeting = ui.game.dialogNPC.DialogueData.Greeting
	}
	ebitenutil.DebugPrintAt(screen, greeting, dialogX+20, dialogY+50)

	// Gold
	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+400, dialogY+20)

	_, _, _, _, listY, leftX, rightX, colW, rowH := merchantDialogLayout(ui.game.config.GetScreenWidth(), ui.game.config.GetScreenHeight())

	// Headers
	ebitenutil.DebugPrintAt(screen, "For Sale:", leftX, listY-20)
	ebitenutil.DebugPrintAt(screen, "Your Items:", rightX, listY-20)

	// Merchant stock list
	maxItems := 12
	if len(ui.game.dialogNPC.MerchantStock) == 0 {
		ebitenutil.DebugPrintAt(screen, "(No stock for sale)", leftX, listY)
	} else {
		for i := 0; i < len(ui.game.dialogNPC.MerchantStock) && i < maxItems; i++ {
			entry := ui.game.dialogNPC.MerchantStock[i]
			item := entry.Item
			y := listY + i*rowH
			stockLabel := fmt.Sprintf("%2d. %s", i+1, item.Name)
			if entry.Quantity <= 0 {
				stockLabel += " (Sold Out)"
			}
			priceLabel := fmt.Sprintf("  %4d gold", ui.game.merchantBuyPrice(entry.Cost))

			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= leftX-2 && mouseX <= leftX+colW && mouseY >= y-2 && mouseY <= y-2+rowH
			if isHover {
				ui.drawUIBackground(screen, leftX-5, y-2, colW+10, rowH, color.RGBA{40, 80, 40, 120})
			}
			drawColoredTextSegments(screen, leftX, y, []coloredTextSegment{
				{text: stockLabel, color: ui.itemRarityColor(item)},
				{text: priceLabel, color: color.White},
			})
		}
	}

	// Player inventory list (sell side)
	if !ui.game.dialogNPC.SellAvailable {
		ebitenutil.DebugPrintAt(screen, "(Not buying goods)", rightX, listY)
	} else {
		for i := 0; i < len(ui.game.party.Inventory) && i < maxItems; i++ {
			item := ui.game.party.Inventory[i]
			y := listY + i*rowH
			price := ui.game.merchantSellPrice(item.Attributes["value"])
			prefix := fmt.Sprintf("%2d. ", i+1)
			nameField := fmt.Sprintf("%-18s", item.Name)
			suffix := fmt.Sprintf("  %4d gold", price)

			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= rightX-2 && mouseX <= rightX+colW && mouseY >= y-2 && mouseY <= y-2+rowH
			if isHover {
				ui.drawUIBackground(screen, rightX-5, y-2, colW+10, rowH, color.RGBA{40, 80, 40, 120})
			}
			drawColoredTextSegments(screen, rightX, y, []coloredTextSegment{
				{text: prefix, color: color.White},
				{text: nameField, color: ui.itemRarityColor(item)},
				{text: suffix, color: color.White},
			})
		}
	}

	// Instructions
	instructionsY := dialogY + dialogHeight - 60
	ebitenutil.DebugPrintAt(screen, "Double-click left list: Buy  |  Double-click right list: Sell", dialogX+20, instructionsY)
	ebitenutil.DebugPrintAt(screen, "ESC: Close", dialogX+20, instructionsY+15)
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
	finalScore := highscore.Calculate(scoreData)
	playTimeStr := highscore.FormatPlayTime(scoreData.PlayTime)

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

	scores, err := highscore.Load()
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
	ui.drawInterfaceIcon(screen, "icon_close", closeX, closeY, 16, 16)
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

	defaultWallColor := color.RGBA{40, 40, 50, 255}
	for y := 0; y < worldH; y++ {
		for x := 0; x < worldW; x++ {
			tile := ui.game.world.Tiles[y][x]
			cellColor := floorColor
			matched := true
			switch tile {
			case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
				cellColor = defaultWallColor
			case world.TileWater:
				cellColor = color.RGBA{40, 90, 160, 255}
			case world.TileDeepWater:
				cellColor = color.RGBA{25, 60, 120, 255}
			case world.TileVioletTeleporter:
				cellColor = color.RGBA{170, 80, 200, 255}
			case world.TileRedTeleporter:
				cellColor = color.RGBA{200, 70, 70, 255}
			default:
				matched = false
			}
			// Dynamic tiles (corals, sand dunes, etc.) aren't in the predefined
			// constants — pull their colour from the tile manager so they show
			// up on the map overlay too.
			if !matched && world.GlobalTileManager != nil {
				if td := world.GlobalTileManager.GetTileData(tile); td != nil {
					if td.Solid {
						cellColor = color.RGBA{uint8(td.WallColor[0]), uint8(td.WallColor[1]), uint8(td.WallColor[2]), 255}
					} else if td.FloorNearColor != [3]int{} {
						cellColor = color.RGBA{uint8(td.FloorNearColor[0]), uint8(td.FloorNearColor[1]), uint8(td.FloorNearColor[2]), 255}
					}
				}
			}

			drawX := originX + x*tileSize
			drawY := originY + y*tileSize
			vector.FillRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), cellColor, false)
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
		vector.FillRect(screen, float32(drawX), float32(drawY), float32(size), float32(size), npcColor, false)
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
	const questGap = 8

	// Paginate so a long quest log never spills past the panel. Reserve the
	// bottom strip for the pager; fit as many whole entries as the panel allows.
	const pagerH = 22
	listTop := contentY + 40
	listBottom := contentY + contentHeight - pagerH
	pageSize := (listBottom - listTop) / (questHeight + questGap)
	if pageSize < 1 {
		pageSize = 1
	}
	totalPages := (len(allQuests) + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	// Clamp every frame so the page stays valid when quests are added/removed.
	if ui.questPage >= totalPages {
		ui.questPage = totalPages - 1
	}
	if ui.questPage < 0 {
		ui.questPage = 0
	}
	pageStart := ui.questPage * pageSize
	pageEnd := pageStart + pageSize
	if pageEnd > len(allQuests) {
		pageEnd = len(allQuests)
	}

	for _, quest := range allQuests[pageStart:pageEnd] {
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

		questY += questHeight + questGap
	}

	ui.drawQuestPager(screen, panelX+20, contentY+contentHeight-pagerH, questWidth, totalPages)
}

// drawQuestPager draws "Page X/Y" plus prev/next buttons under the quest list
// and handles their clicks. No-op when every quest fits on one page.
func (ui *UISystem) drawQuestPager(screen *ebiten.Image, x, y, width, totalPages int) {
	if totalPages <= 1 {
		return
	}
	const btnW, btnH = 30, 18
	mouseX, mouseY := ebiten.CursorPosition()

	drawBtn := func(bx int, label string, enabled bool) bool {
		bg := color.RGBA{70, 50, 30, 210}
		switch {
		case !enabled:
			bg = color.RGBA{45, 40, 38, 160}
		case isMouseHoveringBox(mouseX, mouseY, bx, y, bx+btnW, y+btnH):
			bg = color.RGBA{120, 90, 50, 230}
		}
		drawFilledRect(screen, bx, y, btnW, btnH, bg)
		drawRectBorder(screen, bx, y, btnW, btnH, 1, color.RGBA{150, 110, 52, 220})
		drawCenteredDebugText(screen, label, bx, y+2, btnW, btnH-2)
		return enabled && ui.game.consumeLeftClickIn(bx, y, bx+btnW, y+btnH)
	}

	if drawBtn(x, "<", ui.questPage > 0) {
		ui.questPage--
	}
	if drawBtn(x+width-btnW, ">", ui.questPage < totalPages-1) {
		ui.questPage++
	}
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/%d", ui.questPage+1, totalPages), x, y+2, width, btnH-2)
}

// characterKnowsSpell checks if a character already knows a spell
// characterCanLearnSpell checks if a character can learn a specific spell based on class and magic schools
func (ui *UISystem) characterCanLearnSpell(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	return canCharacterLearnNPCSpell(char, spellData)
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

	// Award experience via the single XP source so Learning bonuses and bench
	// training apply (active party, reserve, and captives all share quest XP).
	if rewards.Experience > 0 {
		ui.game.grantSharedXP(rewards.Experience)
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
