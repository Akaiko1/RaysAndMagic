package game

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/highscore"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type statMeta struct {
	Name string
	Ptr  *int
}

// statHoldInitialDelay - frames the user has to hold the mouse on a +button
// before hold-to-repeat starts firing. Configured for the game's 120 TPS so
// ~340 ms keeps single clicks pure (no accidental double-spend) while a
// deliberate hold takes over fast.
// statHoldRepeatRate - frames between hold-fired increments after the delay.
// At 120 TPS ~ 67 ms / ~15 stats per second when held.
const (
	statHoldInitialDelay = 40
	statHoldRepeatRate   = 8
)

// drawStatPointRow draws a single stat row with name, value, and + button
func (ui *UISystem) drawStatPointRow(screen *ebiten.Image, name string, valuePtr *int, y, plusX, plusY, btnW, btnH int, canAdd, isHover *bool, clickIn bool) bool {
	drawDebugText(screen, fmt.Sprintf("%s: %d", name, *valuePtr), plusX-148, y)

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
	// Bound to statPopupCharIdx - the character whose "+" button was clicked
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
	drawDebugText(screen, "Distribute Stat Points", popupX+16, popupY+16)
	drawDebugText(screen, "Points left:", popupX+16, popupY+44)
	drawDebugText(screen, fmt.Sprintf("%d", member.FreeStatPoints), popupX+120, popupY+44)

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
			member.RecalculateMaxStatsGrantingGain(ui.game.config)
		}
	}

	// Close button; only acts once the mouse was released after opening the popup.
	closeX := popupX + popupW - 40
	closeY := popupY + 12
	isCloseHover := mouseX >= closeX && mouseX < closeX+28 && mouseY >= closeY && mouseY < closeY+28
	if ui.drawPopupCloseButton(screen, closeX, closeY, 28, isCloseHover) && !ui.justOpenedStatPopup {
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

// drawMemberPickerPopup is the shared centered pick-a-party-member overlay
// behind the revival/heal/promotion pickers: dim, panel, title+prompt, one
// hoverable row per target index. rowLabel formats a row; onPick fires on a
// row click. onCancel==nil means not cancellable (no close X, ESC ignored).
func (ui *UISystem) drawMemberPickerPopup(screen *ebiten.Image, title, prompt string, popupW int, targets []int, rowLabel func(idx int) string, onPick func(idx int), onCancel func()) {
	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	rowH := 28
	popupH := 100 + len(targets)*rowH
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2

	// Dim background
	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})

	// Panel
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 240})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{120, 120, 180, 255})

	drawDebugText(screen, title, popupX+16, popupY+16)
	drawDebugText(screen, prompt, popupX+16, popupY+36)

	mouseX, mouseY := ebiten.CursorPosition()
	startY := popupY + 64
	for row, idx := range targets {
		y := startY + row*rowH
		isHover := mouseX >= popupX+16 && mouseX < popupX+popupW-16 &&
			mouseY >= y-2 && mouseY < y-2+rowH
		if isHover {
			drawFilledRect(screen, popupX+16, y-2, popupW-32, rowH, color.RGBA{60, 120, 180, 200})
		}
		drawDebugText(screen, rowLabel(idx), popupX+24, y+6)
		if isHover && ui.game.consumeLeftClickIn(popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			onPick(idx)
			return
		}
	}

	if onCancel == nil {
		return
	}
	if ui.drawPopupCloseButton(screen, popupX+popupW-36, popupY+12, 24, true) {
		onCancel()
		return
	}
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		onCancel()
	}
}

// drawRevivalPickerPopup draws the "Choose who to revive" overlay opened
// when a revival potion is used while 2+ party members are dead or
// unconscious. The list is recomputed every frame from the current party
// state so a member dying mid-popup naturally appears, and a member already
// revived disappears. Closing without a click cancels (potion not spent).
func (ui *UISystem) drawRevivalPickerPopup(screen *ebiten.Image) {
	g := ui.game
	targets := g.RevivablePartyIndices()
	if len(targets) == 0 {
		// No one left to revive (cured externally?) - close cleanly.
		g.resolvePickerQuickSource(g.revivalPickerItemIdx, false)
		g.revivalPickerOpen = false
		return
	}

	ui.drawMemberPickerPopup(screen, "Revive Whom?", "Click a fallen party member.", 360, targets,
		func(idx int) string {
			member := g.party.Members[idx]
			status := "Unconscious"
			if member.HasCondition(character.ConditionDead) {
				status = "Dead"
			}
			return fmt.Sprintf("%d) %s - %s  (HP:%d/%d)", idx+1, member.Name, status, member.HitPoints, member.MaxHitPoints)
		},
		func(idx int) {
			ok := g.applyReviveTo(g.revivalPickerItemIdx, idx)
			g.resolvePickerQuickSource(g.revivalPickerItemIdx, ok)
			g.revivalPickerOpen = false
		},
		func() {
			g.resolvePickerQuickSource(g.revivalPickerItemIdx, false)
			g.revivalPickerOpen = false
		})
}

// drawHealPickerPopup draws the "Heal whom?" overlay opened when a heal potion
// is used by an UNCONSCIOUS owner (who can't heal themselves) and 2+ conscious
// members are wounded. Recomputed every frame; closing without a click cancels
// (potion not spent).
func (ui *UISystem) drawHealPickerPopup(screen *ebiten.Image) {
	g := ui.game
	targets := g.HealablePartyIndices()
	if len(targets) == 0 {
		g.resolvePickerQuickSource(g.healPickerItemIdx, false)
		g.healPickerOpen = false
		return
	}

	ui.drawMemberPickerPopup(screen, "Heal Whom?", "Click a wounded party member.", 360, targets,
		func(idx int) string {
			member := g.party.Members[idx]
			return fmt.Sprintf("%d) %s  (HP:%d/%d)", idx+1, member.Name, member.HitPoints, member.MaxHitPoints)
		},
		func(idx int) {
			ok := g.applyHealTo(g.healPickerItemIdx, idx)
			g.resolvePickerQuickSource(g.healPickerItemIdx, ok)
			g.healPickerOpen = false
		},
		func() {
			g.resolvePickerQuickSource(g.healPickerItemIdx, false)
			g.healPickerOpen = false
		})
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
	drawDebugText(screen, "Tavern - Manage Roster", popupX+16, popupY+14)
	drawDebugText(screen, "Click an active hero, then a reserve hero to swap.", popupX+16, popupY+34)
	drawDebugText(screen, "Active Party", leftX, listY-16)
	drawDebugText(screen, "Reserve (tavern)", rightX, listY-16)

	mouseX, mouseY := ebiten.CursorPosition()
	label := func(m *character.MMCharacter) string {
		flag := ""
		if m.FreeStatPoints > 0 || len(m.OwedLevelChoices) > 0 {
			flag = " !"
		}
		return fmt.Sprintf("%s - %s Lv.%d%s", m.Name, m.ClassDisplayName(), m.Level, flag)
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
		drawDebugText(screen, label(m), leftX+6, y+6)
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
		drawDebugText(screen, label(m), rightX+6, y+6)
		if hover && g.consumeLeftClickIn(rightX, y-2, rightX+colW, y-2+rowH) {
			if g.rosterSelectedActive >= 0 {
				g.swapRosterMember(g.rosterSelectedActive, j)
				g.rosterSelectedActive = -1
			}
		}
	}
	if len(g.party.Reserve) == 0 {
		drawDebugText(screen, "(no benched heroes yet)", rightX+6, listY+6)
	}

	// ESC is handled in the Update input loop (edge-tracked) to avoid the menu
	// opening on the next frame; here only the close button.
	if ui.drawPopupCloseButton(screen, popupX+popupW-36, popupY+10, 24, true) {
		g.rosterScreenOpen = false
		g.rosterSelectedActive = -1
	}
}

// drawPromotionPickerPopup lists the party members eligible for the pending
// promotion (Archmage/Lich) and applies it to whomever the player clicks.
// Mirrors the revival picker. Cannot be cancelled - a quest/phylactery has
// already committed to the promotion by the time this opens.
func (ui *UISystem) drawPromotionPickerPopup(screen *ebiten.Image) {
	g := ui.game
	kind := g.promotionPickerKind
	var targets []int
	title := "Who Becomes the Archmage?"
	if kind == character.PromotionArchmage {
		targets = g.eligibleArchmageIndices()
	} else {
		targets = g.eligibleLichIndices()
		title = "Who Becomes the Lich?"
	}
	if len(targets) == 0 {
		g.promotionPickerOpen = false
		return
	}

	ui.drawMemberPickerPopup(screen, title, "Click a party member.", 380, targets,
		func(idx int) string {
			member := g.party.Members[idx]
			return fmt.Sprintf("%d) %s the %s (Lv.%d)", idx+1, member.Name, member.Class.String(), member.Level)
		},
		func(idx int) {
			itemIdx := g.promotionPickerItemIdx
			g.promotionPickerOpen = false
			g.applyPromotionKind(kind, idx, itemIdx)
		},
		nil)
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
	drawDebugText(screen, title, popupX+16, popupY+16)
	if req.isMultiSelect() {
		drawDebugText(screen, fmt.Sprintf("%s: choose %d (%d selected)", member.Name, req.maxSelections, req.selectedCount()), popupX+16, popupY+36)
	} else {
		drawDebugText(screen, fmt.Sprintf("%s reached level %d", member.Name, req.level), popupX+16, popupY+36)
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
			drawDebugText(screen, option.label, popupX+40, y)
		}

		if isMouseHoveringBox(mouseX, mouseY, popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			var tooltip string
			switch strings.ToLower(option.choice.Type) {
			case "spell":
				tooltip = GetSpellTooltip(option.spellID, member, ui.game.combat, tooltipDetailHeld())
			case "weapon_mastery", "armor_mastery":
				tooltip = masteryTooltipTextForSkill(option.skillType)
			case "magic_mastery":
				tooltip = magicMasteryTooltipText()
			}
			if tooltip != "" {
				lines := strings.Split(tooltip, "\n")
				if strings.ToLower(option.choice.Type) == "spell" {
					plate := color.Color(nil)
					if def, err := spells.GetSpellDefinitionByID(option.spellID); err == nil {
						plate = schoolPlateColor(def.School)
					}
					ui.queueTitledTooltipIcon(lines, nil, plate, nil, spellTooltipIconName(option.spellID), mouseX+16, mouseY+8)
				} else {
					ui.queueTooltipIcon(lines, "", mouseX+16, mouseY+8)
				}
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
		drawDebugText(screen, "Up/Down move - Space toggles - Enter confirms", popupX+16, popupY+popupH-22)
	} else {
		drawDebugText(screen, "Use Up/Down or click, Enter to choose", popupX+16, popupY+popupH-22)
	}
}

// drawNPCDialog draws the NPC dialog interface for different NPC types
func (ui *UISystem) drawNPCDialog(screen *ebiten.Image) {
	if ui.game.dialogNPC == nil {
		return
	}

	screenWidth := ui.game.config.GetScreenWidth()
	screenHeight := ui.game.config.GetScreenHeight()

	// Dialog dimensions (single source: npcDialogLayout)
	dlg := npcDialogLayout(ui.game)
	dialogX, dialogY, dialogWidth, dialogHeight := dlg.x, dlg.y, dlg.w, dlg.h

	// Draw semi-transparent overlay
	drawFilledRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{0, 0, 0, 128})

	// Draw dialog background
	drawFilledRect(screen, dialogX, dialogY, dialogWidth, dialogHeight, color.RGBA{40, 40, 60, 255})

	// Draw border
	borderColor := color.RGBA{100, 100, 120, 255}
	borderThickness := 3
	drawRectBorder(screen, dialogX, dialogY, dialogWidth, dialogHeight, borderThickness, borderColor)

	// Handle different NPC capabilities (data-driven)
	switch npcDialogKindFor(ui.game.dialogNPC) {
	case dialogKindSpellTrader:
		ui.drawSpellTraderDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case dialogKindChoices:
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case dialogKindSkillTrainer:
		ui.drawSkillTrainerDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case dialogKindMerchant:
		ui.drawMerchantDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case dialogKindArenaGladiator:
		ui.drawArenaGladiatorDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case dialogKindCardCollector:
		ui.drawCardCollectorDialog(screen, dialogX, dialogY, dialogHeight)
	default:
		ui.drawGenericDialog(screen, dialogX, dialogY, dialogHeight)
	}
}

// drawEncounterDialog draws dialog for encounter NPCs
func (ui *UISystem) drawEncounterDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, _ int) {
	npc := ui.game.dialogNPC

	// Draw title
	titleText := npc.Name
	drawDebugText(screen, titleText, dialogX+20, dialogY+20)

	ui.drawDialogueChoicesBody(screen, npc, dialogX, dialogY+50, dialogWidth)
}

// drawDialogueChoicesBody renders the state-driven dialogue text and the
// currently valid choices - shared by encounter dialogs and the spell trader's
// Quests tab so both stay aligned with the input handler's choice list.
func (ui *UISystem) drawDialogueChoicesBody(screen *ebiten.Image, npc *character.NPC, dialogX, textY, dialogWidth int) {
	if npc.DialogueData == nil {
		return
	}
	dialogY := textY - dialogueBodyTextY
	layout := ui.game.dialogueLayout(npc, dialogWidth, npcDialogHeight)
	for i, line := range layout.bodyLines {
		drawDebugText(screen, line, dialogX+20, textY+i*dialogueLineHeight)
	}

	choices := ui.game.visibleNPCChoices(npc)
	if len(choices) == 0 {
		drawDebugText(screen, "Press ESC to leave.", dialogX+20, dialogY+layout.exitY)
		return
	}
	if layout.promptY >= 0 {
		prompt := npc.DialogueData.ChoicePrompt
		if layout.choiceCount < len(choices) {
			prompt = fmt.Sprintf("%s (%d-%d/%d)", prompt, layout.firstChoice+1, layout.firstChoice+layout.choiceCount, len(choices))
		}
		drawDebugText(screen, clipDebugText(prompt, dialogWidth-40), dialogX+20, dialogY+layout.promptY)
	}
	for i, choice := range choices {
		if i < layout.firstChoice || i >= layout.firstChoice+layout.choiceCount {
			continue
		}
		x, y, w, h := ui.game.dialogueChoiceRect(npc, i, dialogX, dialogY, dialogWidth)
		choiceText := clipDebugText(fmt.Sprintf("%d. %s", i+1, ui.game.dialogueChoiceLabel(choice)), w-10)
		if i == ui.game.selectedChoice {
			drawFilledRect(screen, x, y, w, h, color.RGBA{100, 100, 0, 128})
		}
		drawDebugText(screen, choiceText, x+5, y+2)
	}
}

// tabGreetingWrapColumns wraps the greeting on tab-style dialogs (spell trader,
// merchant, skill trainer) to the box width. Keep these greetings to <=2 lines:
// the portrait strip sits at dialogY+78, just below a two-line greeting.
const tabGreetingWrapColumns = 78

// Spell-trader layout constants.
const (
	spellTraderPortraitSize = 56
	spellTraderPortraitGap  = 28 // wider gap so name labels under portraits don't overlap.
	spellTraderIconSize     = 48
	spellTraderIconGap      = 10
	spellTraderGridCols     = 6
	// Portrait strip sits this far below the dialog top - low enough that a
	// two-line greeting clears the selected-character frame above it.
	spellTraderPortraitTop = 92
	// Two icon rows per page is the most that fits between the grid top and the
	// instructions without overlapping; the rest paginate.
	spellTraderGridRows = 2
	spellTraderPerPage  = spellTraderGridCols * spellTraderGridRows
)

// spellTraderGridTop is the Y of the spell icon grid (just below the portraits).
func spellTraderGridTop(dialogY int) int {
	return dialogY + spellTraderPortraitTop + spellTraderPortraitSize + 32
}

// spellTraderPortraitRect returns the screen rect for the i-th party
// portrait in the spell-trader dialog.
func spellTraderPortraitRect(dialogX, dialogY, i int) (x, y, w, h int) {
	stripW := 4*spellTraderPortraitSize + 3*spellTraderPortraitGap
	startX := dialogX + (600-stripW)/2
	return startX + i*(spellTraderPortraitSize+spellTraderPortraitGap),
		dialogY + spellTraderPortraitTop,
		spellTraderPortraitSize,
		spellTraderPortraitSize
}

// spellTraderIconRect returns the screen rect for the spell icon in page-slot
// `slot` (0..spellTraderPerPage-1) - NOT the global spell index. Renderer and
// input both map page*perPage+slot to the global spell, so the grid only ever
// shows one page's worth and click rects line up with what's drawn.
func spellTraderIconRect(dialogX, dialogY, slot int) (x, y, w, h int) {
	gridW := spellTraderGridCols*spellTraderIconSize + (spellTraderGridCols-1)*spellTraderIconGap
	startX := dialogX + (600-gridW)/2
	gridY := spellTraderGridTop(dialogY)
	row := slot / spellTraderGridCols
	col := slot % spellTraderGridCols
	cellH := spellTraderIconSize + 14 // icon + cost line
	return startX + col*(spellTraderIconSize+spellTraderIconGap),
		gridY + row*(cellH+8),
		spellTraderIconSize,
		spellTraderIconSize
}

// spellTraderPagerY is the Y of the page nav row (below the two icon rows).
func spellTraderPagerY(dialogY int) int {
	cellH := spellTraderIconSize + 14
	return spellTraderGridTop(dialogY) + spellTraderGridRows*(cellH+8) + 4
}

// drawDialogFolderTabs renders the clickable folder tabs along a dialog's top
// edge (plain rect placeholders until the dedicated tab sprites land) and
// switches g.dialogTab on click. Tab-key cycling stays in the input handlers.
func (ui *UISystem) drawDialogFolderTabs(screen *ebiten.Image, dialogX, dialogY int, labels []string) {
	const tabW, tabH = 110, 32
	tabY := dialogY - tabH + 4 // tucked into the panel edge like folder tabs
	for i, label := range labels {
		tabX := dialogX + 16 + i*(tabW+6)
		fill := color.RGBA{30, 30, 45, 255}
		if ui.game.dialogTab == i {
			fill = color.RGBA{70, 70, 100, 255}
		}
		drawFilledRect(screen, tabX, tabY, tabW, tabH, fill)
		drawRectBorder(screen, tabX, tabY, tabW, tabH, 2, color.RGBA{100, 100, 120, 255})
		drawCenteredDebugText(screen, label, tabX, tabY, tabW, tabH)
		if ui.game.consumeLeftClickIn(tabX, tabY, tabX+tabW, tabY+tabH) {
			ui.game.dialogTab = i
			ui.game.selectedChoice = 0
			// A tab switch re-indexes the buy grid: fresh page, and any
			// in-flight double-click must not buy across tabs.
			ui.game.merchantBuyPage = 0
			ui.game.resetDialogClickTracker()
		}
	}
}

// drawSpellTraderDialog draws an icon-based spell trader UI: 4-character
// portrait strip at top, icon grid for spells below, tooltip on hover.
func (ui *UISystem) drawSpellTraderDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	titleText := fmt.Sprintf("Spell Trader - %s", ui.game.dialogNPC.Name)
	drawDebugText(screen, clipDebugText(titleText, layout.title.w), layout.title.x, layout.title.y)

	// Quest-giving traders carry a second tab: clickable folder tabs along the
	// dialog's top edge (same sprites as the party menu); Tab key also switches
	// (see handleSpellTraderInput).
	if npcHasChoiceDialog(ui.game.dialogNPC) {
		ui.drawDialogFolderTabs(screen, dialogX, dialogY, []string{"Spells", "Quests"})
		if ui.game.dialogTab == 1 {
			ui.drawDialogueChoicesBody(screen, ui.game.dialogNPC, dialogX, dialogY+50, dialogWidth)
			return
		}
	}

	greetingText := "Welcome! I can teach you powerful spells for gold."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greetingText = ui.game.dialogNPC.DialogueData.Greeting
	}
	drawWrappedDebugText(screen, greetingText, layout.greeting, 2, dialogueLineHeight)

	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	drawDebugText(screen, clipDebugText(goldText, layout.balance.w), layout.balance.x, layout.balance.y)

	// Portrait strip - click to switch active character.
	mouseX, mouseY := ebiten.CursorPosition()
	for i, member := range ui.game.party.Members {
		x, y, w, h := spellTraderPortraitRect(dialogX, dialogY, i)
		if i == ui.game.selectedCharIdx {
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{210, 170, 80, 240})
		} else if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{120, 120, 160, 200})
		}
		ui.drawPortraitCover(screen, ui.game.bigPortraitName(member), x, y, w, h)
		label := fmt.Sprintf("%s L%d", member.Name, member.Level)
		// +6 (not +2) so the label clears the selection frame's bottom edge (y+h+3).
		drawCenteredDebugText(screen, clipDebugText(label, w+16), x-8, y+h+6, w+16, debugTextCharHeight)
	}

	// Spell icon grid.
	spellKeys := npcSpellKeys(ui.game.dialogNPC)
	var selectedChar *character.MMCharacter
	if ui.game.selectedCharIdx >= 0 && ui.game.selectedCharIdx < len(ui.game.party.Members) {
		selectedChar = ui.game.party.Members[ui.game.selectedCharIdx]
	}

	// Paginate: only the current page's icons are drawn (two rows), the rest
	// reached via the pager below - so the grid never runs into the instructions.
	pages := pageCount(len(spellKeys), spellTraderPerPage)
	clampPage(&ui.game.spellTraderPage, pages)
	pageStart := ui.game.spellTraderPage * spellTraderPerPage

	var hoverSpellIdx = -1
	for slot := 0; slot < spellTraderPerPage; slot++ {
		i := pageStart + slot
		if i >= len(spellKeys) {
			break
		}
		spellKey := spellKeys[i]
		x, y, w, h := spellTraderIconRect(dialogX, dialogY, slot)
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

		// Cost under icon (+6 clears the icon's selection frame at y+h+3).
		costText := fmt.Sprintf("%d g", npcSpell.Cost)
		drawCenteredDebugText(screen, costText, x-4, y+h+6, w+8, debugTextCharHeight)

		// Dim overlay if known.
		if alreadyKnows {
			drawFilledRect(screen, x, y, w, h, color.RGBA{0, 0, 0, 130})
		}

		if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
			hoverSpellIdx = i
		}
	}

	// Hover tooltip - name + school + cost + requirements.
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

	// Page nav (only renders when there's more than one page).
	gridW := spellTraderGridCols*spellTraderIconSize + (spellTraderGridCols-1)*spellTraderIconGap
	if ui.drawPager(screen, dialogX+(600-gridW)/2, spellTraderPagerY(dialogY), gridW, &ui.game.spellTraderPage, pages, true) {
		// Page changed: move the selection onto the new page so a keyboard
		// purchase (Enter) can't buy a now-hidden spell from the previous page.
		if first := ui.game.spellTraderPage * spellTraderPerPage; first < len(spellKeys) {
			ui.game.dialogSelectedSpell = first
			ui.game.selectedSpellKey = spellKeys[first]
		}
	}

	// Instructions (two condensed lines).
	drawDebugText(screen, clipDebugText("Click portrait & spell to select  |  Double-click spell: buy  |  ESC: close", layout.footer[0].w), layout.footer[0].x, layout.footer[0].y)
	drawDebugText(screen, clipDebugText("Gold=selected   Green=learnable   Red=cannot   Gray=known   |   Hover: details", layout.footer[1].w), layout.footer[1].x, layout.footer[1].y)
}

// Skill-trainer layout.
const (
	skillTrainerPortraitSize = 80
	skillTrainerPortraitGap  = 24
	skillTrainerListTop      = 56 // first mastery row offset inside the popup
	skillTrainerFooterH      = 68 // pager row + instructions line at the popup bottom
)

// skillTrainerPageSize is how many mastery rows fit between the popup header
// and its footer. Geometry-derived and shared by renderer and input, so the
// list can never run over the pager/instructions.
func skillTrainerPageSize(popupH int) int {
	n := (popupH - skillTrainerListTop - skillTrainerFooterH) / UIRowSpacing
	if n < 1 {
		n = 1
	}
	return n
}

func skillTrainerPortraitRect(dialogX, dialogY, dialogWidth, i int) (x, y, w, h int) {
	stripW := 4*skillTrainerPortraitSize + 3*skillTrainerPortraitGap
	startX := dialogX + (dialogWidth-stripW)/2
	return startX + i*(skillTrainerPortraitSize+skillTrainerPortraitGap),
		dialogY + 128,
		skillTrainerPortraitSize,
		skillTrainerPortraitSize
}

// skillTrainerOptionRect returns the rect for the row-th mastery option ON THE
// CURRENT PAGE inside the modal popup (top-anchored).
func skillTrainerOptionRect(popupX, popupY, row int) (x, y, w, h int) {
	return popupX + 12, popupY + skillTrainerListTop + row*UIRowSpacing, 396, UIRowHeight
}

func skillTrainerPopupRect(dialogX, dialogY, dialogWidth, dialogHeight int) (x, y, w, h int) {
	popupW := 420
	popupH := dialogHeight - 60
	return dialogX + (dialogWidth-popupW)/2, dialogY + 30, popupW, popupH
}

func (ui *UISystem) drawSkillTrainerDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	titleText := fmt.Sprintf("Mastery Trainer - %s", ui.game.dialogNPC.Name)
	drawDebugText(screen, clipDebugText(titleText, layout.title.w), layout.title.x, layout.title.y)

	greeting := "Choose a character to view trainable masteries."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greeting = ui.game.dialogNPC.DialogueData.Greeting
	}
	drawWrappedDebugText(screen, greeting, layout.greeting, 2, dialogueLineHeight)
	drawDebugText(screen, clipDebugText(fmt.Sprintf("Party Gold: %d", ui.game.party.Gold), layout.balance.w), layout.balance.x, layout.balance.y)

	// Portrait row.
	mouseX, mouseY := ebiten.CursorPosition()
	for i, member := range ui.game.party.Members {
		x, y, w, h := skillTrainerPortraitRect(dialogX, dialogY, dialogWidth, i)
		hover := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
		if hover && !ui.game.skillTrainerPopup {
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{210, 170, 80, 240})
		}
		ui.drawPortraitCover(screen, ui.game.bigPortraitName(member), x, y, w, h)
		// +8/+24 keep both labels clear of the hover/selection frame (y+h+3).
		drawCenteredDebugText(screen, clipDebugText(member.Name, w+16), x-8, y+h+8, w+16, debugTextCharHeight)
		drawCenteredDebugText(screen, clipDebugText(fmt.Sprintf("Level %d %s", member.Level, member.ClassDisplayName()), w+16), x-8, y+h+24, w+16, debugTextCharHeight)
	}

	drawDebugText(screen, clipDebugText("Click a portrait to view trainable masteries  |  ESC: close", layout.footer[0].w), layout.footer[0].x, layout.footer[0].y)

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
	drawDebugText(screen, fmt.Sprintf("Gold: %d", ui.game.party.Gold), px+12, py+30)

	options := trainerOptions(member)
	if len(options) == 0 {
		drawCenteredDebugText(screen, "All known masteries are already maxed.", px, py+ph/2-8, pw, 16)
	} else {
		mouseX, mouseY := ebiten.CursorPosition()
		pageSize := skillTrainerPageSize(ph)
		pages := pageCount(len(options), pageSize)
		clampPage(&ui.game.skillTrainerPage, pages)
		start := ui.game.skillTrainerPage * pageSize
		for row := 0; row < pageSize; row++ {
			idx := start + row
			if idx >= len(options) {
				break
			}
			option := options[idx]
			x, y, w, h := skillTrainerOptionRect(px, py, row)
			hover := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
			if idx == ui.game.dialogSelectedSpell {
				ui.drawUIBackground(screen, x, y-2, w, h+4, color.RGBA{80, 130, 70, 200})
			} else if hover {
				ui.drawUIBackground(screen, x, y-2, w, h+4, color.RGBA{60, 70, 100, 160})
			}
			label := fmt.Sprintf("%s:  %s -> %s   %d gold", option.Label, option.Current.String(), option.Next.String(), option.Cost)
			if option.Cost > ui.game.party.Gold {
				label += "  (Need Gold)"
			}
			drawDebugText(screen, label, x+6, y)
		}
		// Pager sits between the last row slot and the instructions line; it
		// consumes its own clicks in the draw pass (drawPager convention).
		if ui.drawPager(screen, px+12, py+ph-46, 396, &ui.game.skillTrainerPage, pages, true) {
			// Keep the highlight on the visible page so Enter / double-click
			// always act on what's shown.
			ui.game.dialogSelectedSpell = ui.game.skillTrainerPage * pageSize
		}
	}

	drawDebugText(screen, "Click to select  |  Double-click: train  |  ESC/Back: party list", px+12, py+ph-22)
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

// drawMerchantDialog draws an icon-based buy/sell UI: a paginated stock grid on
// the left, the party's inventory grid on the right, with a full item card on
// hover. Click geometry is shared with the input handler via merchantGridLayout
// / merchantCellRect, and the pager flips the page state on MMGame so both sides
// agree.
func (ui *UISystem) drawMerchantDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	titleText := fmt.Sprintf("Merchant - %s", ui.game.dialogNPC.Name)
	drawDebugText(screen, clipDebugText(titleText, layout.title.w), layout.title.x, layout.title.y)
	// The tabbed gladiator dialog keeps its (long) greeting on the Talk tab -
	// the Shop tab goes straight to the grids or the text floods them.
	if npcDialogKindFor(ui.game.dialogNPC) != dialogKindArenaGladiator {
		greeting := "Bring your wares. I pay fair coin."
		if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
			greeting = ui.game.dialogNPC.DialogueData.Greeting
		}
		greetingArea := layout.greeting
		greetingArea.y += 2
		drawWrappedDebugText(screen, greeting, greetingArea, 2, dialogueLineHeight)
	}
	balanceText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	if ui.game.dialogNPC.Currency == character.CurrencyArenaPoints {
		balanceText = fmt.Sprintf("Arena Points: %d", ui.game.party.ArenaPoints)
	} else if name, ok := currencyItemName(ui.game.dialogNPC.Currency); ok {
		balanceText = fmt.Sprintf("%ss: %d", name, ui.game.party.CountItemsByName(name))
	}
	drawDebugText(screen, clipDebugText(balanceText, layout.balance.w), layout.balance.x, layout.balance.y)

	leftX, rightX, gridTop, pagerY := merchantGridLayout(dialogX, dialogY)
	mouseX, mouseY := ebiten.CursorPosition()

	// Headers + faint divider between the two halves. Headers sit at gridTop-24
	// so they clear the two-line greeting above and the icon frames below.
	drawDebugText(screen, "For Sale", leftX, gridTop-24)
	drawDebugText(screen, "Your Items", rightX, gridTop-24)
	drawFilledRect(screen, dialogX+dialogWidth/2, gridTop-6, 1, merchantGridRows*(merchantIconSize+merchantPriceH+merchantRowGap), color.RGBA{90, 90, 110, 120})

	var tooltipItem items.Item
	var tooltipHasItem bool

	// Set-shop merchants (the Clockmaker) group their stock under folder tabs;
	// the classic single grid stays for untabbed shops. The visible slice is
	// shared with the click handler (merchantVisibleStock) so indices agree.
	if tabs := ui.game.merchantShopTabs(); len(tabs) > 0 {
		ui.drawDialogFolderTabs(screen, dialogX, dialogY, tabs)
	}

	// Buy grid (left): merchant stock.
	stock := ui.game.merchantVisibleStock()
	buyPages := pageCount(len(stock), merchantPageSize)
	clampPage(&ui.game.merchantBuyPage, buyPages)
	if len(stock) == 0 {
		drawDebugText(screen, "(No stock for sale)", leftX, gridTop)
	} else {
		start := ui.game.merchantBuyPage * merchantPageSize
		for slot := 0; slot < merchantPageSize; slot++ {
			idx := start + slot
			if idx >= len(stock) {
				break
			}
			entry := stock[idx]
			x, y, w, h := merchantCellRect(leftX, gridTop, slot)
			soldOut := !entry.InStock()
			if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
				drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 230})
				tooltipItem = entry.Item
				tooltipHasItem = true
			}
			ui.drawInventoryItemIcon(screen, entry.Item, x, y, w, h, 4, !soldOut)
			priceText := fmt.Sprintf("%d g", ui.game.merchantBuyPrice(entry.Cost))
			if ui.game.dialogNPC.Currency == character.CurrencyArenaPoints {
				priceText = fmt.Sprintf("%d ap", entry.Cost) // flat price, victory currency
			} else if _, ok := character.CurrencyItemKey(ui.game.dialogNPC.Currency); ok {
				priceText = fmt.Sprintf("x%d", entry.Cost) // flat price, item-backed currency
			}
			if soldOut {
				priceText = "sold out"
			}
			drawCenteredDebugText(screen, priceText, x-6, y+h, w+12, merchantPriceH)
		}
	}
	pagerChanged := ui.drawPager(screen, leftX, pagerY, merchantGridW, &ui.game.merchantBuyPage, buyPages, true)

	// Sell grid (right): party inventory.
	if !ui.game.dialogNPC.SellAvailable {
		drawDebugText(screen, "(Not buying goods)", rightX, gridTop)
	} else {
		inv := ui.game.party.Inventory
		sellPages := pageCount(len(inv), merchantPageSize)
		clampPage(&ui.game.merchantSellPage, sellPages)
		start := ui.game.merchantSellPage * merchantPageSize
		for slot := 0; slot < merchantPageSize; slot++ {
			idx := start + slot
			if idx >= len(inv) {
				break
			}
			item := inv[idx]
			x, y, w, h := merchantCellRect(rightX, gridTop, slot)
			value := item.Attributes["value"]
			if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
				drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 230})
				tooltipItem = item
				tooltipHasItem = true
				if key := itemCardKey(item); key != "" {
					ui.fullArtCardKey = key
				}
			}
			ui.drawInventoryItemIcon(screen, item, x, y, w, h, 4, value > 0)
			priceText := "no value"
			if value > 0 {
				priceText = fmt.Sprintf("%d g", ui.game.merchantSellPrice(value))
			}
			drawCenteredDebugText(screen, priceText, x-6, y+h, w+12, merchantPriceH)
		}
		if ui.drawPager(screen, rightX, pagerY, merchantGridW, &ui.game.merchantSellPage, sellPages, true) {
			pagerChanged = true
		}
	}
	// A page flip is a navigation action between item clicks - break any in-flight
	// double-click so the same absolute index across pages can't buy/sell by surprise.
	if pagerChanged {
		ui.game.resetDialogClickTracker()
	}

	// Full item card on hover, floating at the cursor (drawn over everything via
	// the queued tooltip pass). selectedChar is bounds-guarded - a stale index
	// (party shrank) must not panic on this per-frame hover path. We resolve to a
	// real member (clamping a stale index) so the formatter never sees a nil char.
	if tooltipHasItem {
		{
			// Shop tooltips show the ITEM's own base numbers (nil char = base
			// view) - never scaled by whichever party member is selected.
			tip := GetItemTooltip(tooltipItem, nil, ui.game.combat, tooltipDetailHeld())
			if tip != "" {
				lines := ui.appendCardArtHint(strings.Split(tip, "\n"), itemCardKey(tooltipItem))
				plate, titleText := ui.itemTitleColors(tooltipItem)
				var bodyColors []color.Color
				if titleText != nil { // gear keeps its rarity-metal body
					bodyColors = ui.rarityBodyColors(tooltipItem, len(lines))
				}
				ui.queueTitledTooltipIcon(lines, bodyColors, plate, titleText, itemTooltipIconName(tooltipItem), mouseX+16, mouseY+8)
			}
		}
	}

	drawDebugText(screen, clipDebugText("Hover: details  |  Double-click: buy (left) / sell (right)", layout.footer[0].w), layout.footer[0].x, layout.footer[0].y)
	drawDebugText(screen, "ESC: Close", layout.footer[1].x, layout.footer[1].y)
}

// Card-collector layout. The 8 collection slots sit in one compact row; the
// party's loose cards sit in a grid below. Geometry is shared with the input
// handler (cardCollectorSlotRect / cardCollectorInvRect) so click rects match.
const (
	cardCollSlotSize = 48
	cardCollSlotGap  = 8
	cardInvSize      = 56
	cardInvCols      = 4
	cardInvGap       = 14
	cardInvMaxShown  = cardInvCols * 2 // up to 8 loose cards shown at once
)

func cardCollectorSlotRect(dialogX, dialogY, slot int) (x, y, w, h int) {
	gridW := MaxCardSlots*cardCollSlotSize + (MaxCardSlots-1)*cardCollSlotGap
	startX := dialogX + (npcDialogWidth-gridW)/2
	return startX + slot*(cardCollSlotSize+cardCollSlotGap), dialogY + 114, cardCollSlotSize, cardCollSlotSize
}

// cardInvTop is the Y of the loose-card grid; cardInvRowPitch the row stride.
// Pulled up enough to leave room for the page nav row beneath the two rows.
const (
	cardInvTop      = 196
	cardInvRowPitch = cardInvSize + cardInvGap + 2
)

func cardCollectorInvRect(dialogX, dialogY, slot int) (x, y, w, h int) {
	gridW := cardInvCols*cardInvSize + (cardInvCols-1)*cardInvGap
	startX := dialogX + (npcDialogWidth-gridW)/2
	r, c := slot/cardInvCols, slot%cardInvCols
	return startX + c*(cardInvSize+cardInvGap), dialogY + cardInvTop + r*cardInvRowPitch, cardInvSize, cardInvSize
}

// drawCardCell draws one card cell - the card's art when key is set, else a
// placeholder frame (with emptyLabel). Returns whether the cursor is over it.
// Shared by the collector dialog and the Cards menu tab so the cell looks and
// hit-tests identically in both.
func (ui *UISystem) drawCardCell(screen *ebiten.Image, key string, x, y, size int, emptyLabel string) bool {
	if key == "" {
		drawFilledRect(screen, x, y, size, size, color.RGBA{30, 30, 44, 255})
		drawRectBorder(screen, x, y, size, size, 1, color.RGBA{80, 80, 100, 200})
		if emptyLabel != "" {
			drawCenteredDebugText(screen, emptyLabel, x, y, size, size)
		}
		return false
	}
	ui.drawInventoryItemIcon(screen, items.CreateItemFromYAML(key), x, y, size, size, 3, true)
	mx, my := ebiten.CursorPosition()
	hovered := isMouseHoveringBox(mx, my, x, y, x+size, y+size)
	if hovered {
		ui.fullArtCardKey = key
	}
	return hovered
}

// appendCardArtHint adds the SHIFT hint to a card tooltip when full art exists.
func (ui *UISystem) appendCardArtHint(lines []string, key string) []string {
	if _, ok := ui.game.cardFullArtSprite(key); ok {
		return append(lines, "Hold SHIFT to view the art")
	}
	return lines
}

// drawCardFullArtOverlay dims the screen and shows a card's full art fitted
// to it (drawn while SHIFT is held over a card).
func (ui *UISystem) drawCardFullArtOverlay(screen *ebiten.Image, sprite string) {
	img := ui.game.sprites.GetSprite(sprite)
	if img == nil {
		return
	}
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	drawFilledRect(screen, 0, 0, sw, sh, color.RGBA{0, 0, 0, 195})
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	scale := 0.92 * float64(sw) / float64(iw)
	if v := 0.92 * float64(sh) / float64(ih); v < scale {
		scale = v
	}
	w, h := float64(iw)*scale, float64(ih)*scale
	x, y := (float64(sw)-w)/2, (float64(sh)-h)/2
	opts := &ebiten.DrawImageOptions{}
	opts.Filter = ebiten.FilterLinear
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(x, y)
	screen.DrawImage(img, opts)
	drawRectBorder(screen, int(x)-2, int(y)-2, int(w)+4, int(h)+4, 2, color.RGBA{210, 170, 80, 235})
}

// drawCardCollectorDialog draws the monster-card collection UI: the 8 active
// slots on top, the party's loose cards below. Double-click a loose card to slot
// it, double-click a slotted card to take it back. Art-based with hover tooltips.
func (ui *UISystem) drawCardCollectorDialog(screen *ebiten.Image, dialogX, dialogY, dialogHeight int) {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, npcDialogWidth, dialogHeight}, false)
	drawDebugText(screen, clipDebugText(fmt.Sprintf("Card Collector - %s", ui.game.dialogNPC.Name), layout.title.w), layout.title.x, layout.title.y)
	greeting := "Cards, is it? Hand them here and I'll pin them to your collection."
	if ui.game.dialogNPC.DialogueData != nil && ui.game.dialogNPC.DialogueData.Greeting != "" {
		greeting = ui.game.dialogNPC.DialogueData.Greeting
	}
	drawWrappedDebugText(screen, greeting, layout.greeting, 2, dialogueLineHeight)

	mouseX, mouseY := ebiten.CursorPosition()
	var hoverLines []string

	// Active collection (8 slots).
	drawDebugText(screen, "Collection (active effects)", dialogX+20, dialogY+96)
	for slot := 0; slot < MaxCardSlots; slot++ {
		x, y, w, h := cardCollectorSlotRect(dialogX, dialogY, slot)
		key := ui.game.cardCollectionKey(slot)
		if ui.drawCardCell(screen, key, x, y, w, "+") {
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 235})
			if def := cardDef(key); def != nil {
				hoverLines = ui.appendCardArtHint([]string{def.Name, cardEffectText(def), "", "Double-click to remove"}, key)
			}
		}
	}

	// Loose cards in the party inventory (paginated - the pack can hold more than
	// one page of cards).
	cardIdx := ui.game.inventoryCardIndices()
	drawDebugText(screen, "Your cards (double-click to add)", dialogX+20, dialogY+176)
	if len(cardIdx) == 0 {
		drawDebugText(screen, "(No loose cards to add)", dialogX+20, dialogY+200)
	}
	invPages := pageCount(len(cardIdx), cardInvMaxShown)
	clampPage(&ui.game.cardCollectorInvPage, invPages)
	invStart := ui.game.cardCollectorInvPage * cardInvMaxShown
	for slot := 0; slot < cardInvMaxShown; slot++ {
		i := invStart + slot
		if i >= len(cardIdx) {
			break
		}
		x, y, w, h := cardCollectorInvRect(dialogX, dialogY, slot)
		key := itemCardKey(ui.game.party.Inventory[cardIdx[i]])
		if ui.drawCardCell(screen, key, x, y, w, "") {
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{80, 200, 80, 235})
			if def := cardDef(key); def != nil {
				hoverLines = ui.appendCardArtHint([]string{def.Name, cardEffectText(def), "", "Double-click to add to collection"}, key)
			}
		}
	}
	invGridW := cardInvCols*cardInvSize + (cardInvCols-1)*cardInvGap
	ui.drawPager(screen, dialogX+(npcDialogWidth-invGridW)/2, dialogY+cardInvTop+2*cardInvRowPitch-4, invGridW, &ui.game.cardCollectorInvPage, invPages, true)

	drawDebugText(screen, clipDebugText("Double-click a card to slot it  |  Double-click a slotted card to take it back", layout.footer[0].w), layout.footer[0].x, layout.footer[0].y)

	if hoverLines != nil {
		ui.queueTooltip(hoverLines, mouseX+16, mouseY+8)
	}
}

// drawGenericDialog draws basic dialog for other NPC types
func (ui *UISystem) drawGenericDialog(screen *ebiten.Image, dialogX, dialogY, _ int) {
	npc := ui.game.dialogNPC
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, npcDialogWidth, npcDialogHeight}, false)

	// Draw title
	titleText := npc.Name
	drawDebugText(screen, clipDebugText(titleText, layout.title.w), layout.title.x, layout.title.y)

	// Draw basic greeting
	if npc.DialogueData != nil && npc.DialogueData.Greeting != "" {
		maxLines := (layout.footer[0].y - layout.greeting.y - 12) / dialogueLineHeight
		drawWrappedDebugText(screen, npc.DialogueData.Greeting, layout.greeting, maxLines, dialogueLineHeight)
	}

	drawDebugText(screen, "Press ESC to close", layout.footer[0].x, layout.footer[0].y)
}

// drawGameOverOverlay draws a simple game over screen with options
func (ui *UISystem) drawGameOverOverlay(screen *ebiten.Image) {
	g := ui.game
	w := g.config.GetScreenWidth()
	h := g.config.GetScreenHeight()

	// Dark blood-red veil over the frozen scene.
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{45, 0, 0, 215})

	// Big GAME OVER heading + subtitle.
	drawScaledCenteredText(screen, "GAME OVER", w/2, h/2-150, 4.0, color.RGBA{220, 60, 50, 255})
	drawCenteredTextWithShadow(screen, "Your party has fallen.", w/2-140, h/2-104, 280, 16, color.RGBA{205, 185, 185, 255})

	// Button menu (same look as the title screen).
	btns := []struct {
		label  string
		action func()
	}{
		{"New Game", func() { g.startNewGameWithParty(character.NewParty(g.config)) }},
		{"Load Game", func() { g.returnToMainMenu(); g.entryMenuMode = EntryMenuLoad; g.slotSelection = 0; g.savePage = 0 }},
		{"Main Menu", func() { g.returnToMainMenu() }},
		{"Quit", func() { g.exitRequested = true }},
	}
	const btnW, btnH, gap = 280, 50, 14
	bx := (w - btnW) / 2
	startY := h/2 - 30
	mx, my := ebiten.CursorPosition()
	for i, b := range btns {
		by := startY + i*(btnH+gap)
		hover := isMouseHoveringBox(mx, my, bx, by, bx+btnW, by+btnH)
		ui.drawMenuButton(screen, "", b.label, bx, by, btnW, btnH, hover)
		if g.consumeLeftClickIn(bx, by, bx+btnW, by+btnH) {
			b.action()
			return
		}
	}
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
	drawDebugText(screen, "VICTORY!", centerX-70, startY)
	drawDebugText(screen, "You have slain all four dragons!", centerX-120, startY+25)
	drawDebugText(screen, "The realm is saved!", centerX-70, startY+45)

	// Score details
	drawDebugText(screen, "Final Score", centerX-75, startY+80)
	drawDebugText(screen, fmt.Sprintf("Score: %d", finalScore), centerX-50, startY+100)
	drawDebugText(screen, fmt.Sprintf("Gold Earned: %d", scoreData.Gold), centerX-75, startY+120)
	drawDebugText(screen, fmt.Sprintf("Total XP: %d", scoreData.TotalExperience), centerX-65, startY+140)
	drawDebugText(screen, fmt.Sprintf("Avg Level: %d", scoreData.AverageLevel), centerX-55, startY+160)
	drawDebugText(screen, fmt.Sprintf("Time: %s", playTimeStr), centerX-50, startY+180)

	// Instructions
	if !ui.game.victoryScoreSaved {
		drawDebugText(screen, "Enter your name:", centerX-60, startY+220)
		drawDebugText(screen, fmt.Sprintf("> %s_", ui.game.victoryNameInput), centerX-80, startY+240)
		drawDebugText(screen, "Press ENTER to save score", centerX-90, startY+270)
		drawDebugText(screen, "Press ESC to continue", centerX-80, startY+290)
	} else {
		drawDebugText(screen, "Score saved!", centerX-45, startY+220)
		drawDebugText(screen, "Press H to view High Scores", centerX-100, startY+250)
		drawDebugText(screen, "Press ESC to continue", centerX-80, startY+270)
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
		drawDebugText(screen, "Error loading high scores", w/2-90, h/2)
		return
	}

	centerX := w / 2
	startY := 60

	// Header
	drawDebugText(screen, "HIGH SCORES", centerX-75, startY)

	// Column headers
	drawDebugText(screen, "Rank  Name           Score    Time", centerX-140, startY+40)
	drawFilledRect(screen, centerX-140, startY+56, 290, 1, color.RGBA{120, 120, 150, 255})

	// Entries
	if len(scores.Entries) == 0 {
		drawDebugText(screen, "No scores yet!", centerX-50, startY+80)
	} else {
		for i, entry := range scores.Entries {
			line := fmt.Sprintf("%2d.   %-14s %6d   %s", i+1, truncateName(entry.PlayerName, 14), entry.Score, entry.PlayTime)
			drawDebugText(screen, line, centerX-140, startY+80+i*20)
		}
	}

	// Instructions
	drawDebugText(screen, "Press ESC to close", centerX-70, h-50)
}

// drawMapOverlay renders the current map with NPCs and teleporters.
func (ui *UISystem) drawMapOverlay(screen *ebiten.Image) {
	if ui.game.world == nil {
		ui.game.mapOverlayOpen = false
		return
	}

	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	layout := computeMapOverlayLayout(screenW, screenH)

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 140})
	drawFilledRect(screen, layout.panel.x, layout.panel.y, layout.panel.w, layout.panel.h, color.RGBA{20, 20, 40, 230})
	drawRectBorder(screen, layout.panel.x, layout.panel.y, layout.panel.w, layout.panel.h, 2, color.RGBA{100, 100, 160, 255})

	title := "World Map"
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil && mapCfg.Name != "" {
			title = fmt.Sprintf("World Map - %s", mapCfg.Name)
		}
	}
	drawDebugText(screen, clipDebugText(title, layout.title.w), layout.title.x, layout.title.y)

	drawFilledRect(screen, layout.close.x, layout.close.y, layout.close.w, layout.close.h, color.RGBA{200, 60, 60, 220})
	ui.drawInterfaceIcon(screen, "icon_close", layout.close.x, layout.close.y, layout.close.w, layout.close.h)
	if ui.game.consumeLeftClickIn(layout.close.x, layout.close.y, layout.close.right(), layout.close.bottom()) {
		ui.game.mapOverlayOpen = false
	}
	mapX, mapY, mapW, mapH := layout.body.x, layout.body.y, layout.body.w, layout.body.h

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

	defaultWallColor := color.RGBA{40, 40, 50, 255}
	for y := 0; y < worldH; y++ {
		for x := 0; x < worldW; x++ {
			// Per-tile floor color: on the unified world each tile keeps ITS
			// region's biome color regardless of where the party stands.
			fc := ui.game.floorColorForTile(x, y, [3]int{60, 110, 60})
			floorColor := color.RGBA{uint8(fc[0]), uint8(fc[1]), uint8(fc[2]), 255}
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
			// constants - pull their colour from the tile manager so they show
			// up on the map overlay too.
			if !matched && world.GlobalTileManager != nil {
				if td := world.GlobalTileManager.GetTileData(tile); td != nil {
					if td.Solid {
						mc := config.TileMapColor(td)
						cellColor = color.RGBA{uint8(mc[0]), uint8(mc[1]), uint8(mc[2]), 255}
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
		drawDebugText(screen, questNum, int(centerX)-3, int(centerY)-6)
	}
}

// drawQuestsContent draws the quests tab content
func (ui *UISystem) drawQuestsContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	content := layoutRect{panelX, contentY, tabbedMenuPanelW, contentHeight}
	layout := computeQuestContentLayout(content, 0)
	drawDebugText(screen, "ACTIVE QUESTS", layout.title.x, layout.title.y)

	// Check if quest manager is available
	if ui.game.questManager == nil {
		drawDebugText(screen, "No quests available.", layout.rows[0].x, layout.rows[0].y)
		return
	}

	allQuests := ui.game.questManager.GetAllQuests()
	if len(allQuests) == 0 {
		drawDebugText(screen, "No active quests.", layout.rows[0].x, layout.rows[0].y)
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
	layout = computeQuestContentLayout(content, len(allQuests))
	// Clamp every frame so the page stays valid when quests are added/removed.
	if ui.questPage >= layout.totalPages {
		ui.questPage = layout.totalPages - 1
	}
	if ui.questPage < 0 {
		ui.questPage = 0
	}
	pageStart := ui.questPage * layout.pageSize
	pageEnd := pageStart + layout.pageSize
	if pageEnd > len(allQuests) {
		pageEnd = len(allQuests)
	}

	for rowIndex, quest := range allQuests[pageStart:pageEnd] {
		row := layout.rows[rowIndex]
		questY, questWidth := row.y, row.w
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
		drawFilledRect(screen, row.x, row.y, row.w, row.h, bgColor)

		// Draw quest border
		borderColor := color.RGBA{80, 80, 120, 255}
		if quest.Completed && !quest.RewardsClaimed {
			borderColor = color.RGBA{100, 200, 100, 255} // Green border for claimable
		}
		vector.StrokeRect(screen, float32(row.x), float32(row.y), float32(row.w), float32(row.h), 2, borderColor, false)

		// Quest name
		namePrefix := ""
		if quest.Completed {
			namePrefix = "[DONE] "
		}
		drawDebugText(screen, clipDebugText(namePrefix+quest.Definition.Name, questWidth-20), row.x+10, questY+6)

		// The card reserves two description rows; the last gets an ellipsis when
		// authored copy is longer.
		descLines := truncateWrappedLines(wrapDebugText(quest.Definition.Description, questWidth-20), 2, questWidth-20)
		for i, line := range descLines {
			drawDebugText(screen, line, row.x+10, questY+22+i*debugTextCharHeight)
		}

		// Bottom row: Progress on left, Rewards on right
		bottomY := questY + 54

		// Progress for counted quests (kill / interact) - both advance a
		// CurrentCount toward TargetCount, so they share the bar.
		if quest.Definition.Type == "kill" || quest.Definition.Type == "interact" {
			progressText := quest.GetProgressString()
			drawDebugText(screen, progressText, row.x+10, bottomY)

			// Draw progress bar below text
			barX := row.x + 10
			barY := questY + 72
			barWidth := 180
			barHeight := 14

			// Background bar
			drawFilledRect(screen, barX, barY, barWidth, barHeight, color.RGBA{20, 20, 20, 255})

			// Progress fill
			progress := 0.0
			if target := quest.Target(); target > 0 {
				progress = float64(quest.CurrentCount) / float64(target)
			}
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
			drawDebugText(screen, objectiveText, row.x+10, bottomY)
		}

		// Rewards section (right side)
		rewardsX := row.x + 280
		rewardsText := "Reward: " + questRewardSummary(quest.Definition.Rewards.Gold, quest.Definition.Rewards.ArenaPoints, quest.Definition.Rewards.Experience)
		drawDebugText(screen, clipDebugText(rewardsText, row.right()-rewardsX-10), rewardsX, bottomY)

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

	}

	ui.drawQuestPager(screen, layout.pager.x, layout.pager.y, layout.pager.w, layout.totalPages)
}

// drawQuestPager draws "Page X/Y" plus prev/next buttons under the quest list
// and handles their clicks. No-op when every quest fits on one page.
func (ui *UISystem) drawQuestPager(screen *ebiten.Image, x, y, width, totalPages int) {
	if totalPages <= 1 {
		return
	}
	if ui.drawPagerButton(screen, x, y, "<", ui.questPage > 0) {
		ui.questPage--
	}
	if ui.drawPagerButton(screen, x+width-pagerBtnW, y, ">", ui.questPage < totalPages-1) {
		ui.questPage++
	}
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/%d", ui.questPage+1, totalPages), x, y+2, width, pagerBtnH-2)
}

// characterKnowsSpell checks if a character already knows a spell
// characterCanLearnSpell checks if a character can learn a specific spell based on class and magic schools
func (ui *UISystem) characterCanLearnSpell(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	return canCharacterLearnNPCSpell(char, spellData)
}

// claimQuestReward claims the reward for a completed quest (UI journal entry).
func (ui *UISystem) claimQuestReward(questID string) {
	ui.game.claimQuestReward(questID)
}

// questRewardSummary is the shared UI/log wording for quest currencies and XP.
func questRewardSummary(gold, arenaPoints, experience int) string {
	parts := make([]string, 0, 3)
	if gold > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", gold))
	}
	if arenaPoints > 0 {
		parts = append(parts, fmt.Sprintf("%d arena points", arenaPoints))
	}
	if experience > 0 {
		parts = append(parts, fmt.Sprintf("%d XP", experience))
	}
	if len(parts) == 0 {
		return "no reward"
	}
	return strings.Join(parts, " / ")
}

// claimQuestReward claims a completed quest's reward and marks it claimed.
// This is the single source of truth for the quest journal (J) and NPC turn-in.
func (g *MMGame) claimQuestReward(questID string) bool {
	if g.questManager == nil {
		return false
	}
	rewards, err := g.questManager.ClaimRewards(questID)
	if err != nil {
		g.AddCombatMessage(fmt.Sprintf("Cannot claim reward: %s", err.Error()))
		return false
	}
	if rewards.Gold > 0 {
		g.awardGold(rewards.Gold)
	}
	if rewards.ArenaPoints > 0 {
		g.awardArenaPoints(rewards.ArenaPoints)
	}
	// Single XP source so Learning bonuses and bench training apply (active party,
	// reserve, and captives all share quest XP).
	if rewards.Experience > 0 {
		g.grantSharedXP(rewards.Experience)
	}
	if quest := g.questManager.GetQuest(questID); quest != nil {
		g.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Received %s!",
			quest.Definition.Name, questRewardSummary(rewards.Gold, rewards.ArenaPoints, rewards.Experience)))
	}
	return true
}

// truncateName truncates a name to maxLen displayed characters.
func truncateName(name string, maxLen int) string {
	return truncateRunes(name, maxLen, "..")
}
