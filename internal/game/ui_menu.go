package game

import (
	"fmt"
	"image/color"
	"time"

	"ugataima/internal/character"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// drawOverlayInterfaces draws overlay interfaces like menus and dialogs
func (ui *UISystem) drawOverlayInterfaces(screen *ebiten.Image) {
	if ui.game.menuOpen {
		ui.drawTabbedMenu(screen)
	}

	// Draw main menu (ESC)
	if ui.game.mainMenuOpen {
		ui.drawMainMenu(screen)
	}

	// Draw dialog if active
	if ui.game.dialogActive {
		ui.drawNPCDialog(screen)
	}

	if ui.game.mapOverlayOpen {
		ui.drawMapOverlay(screen)
	}
}

// drawMainMenu renders the ESC main menu overlay
func (ui *UISystem) drawMainMenu(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Dim background
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 128})

	// Panel
	panelW, panelH := 300, 220
	if ui.game.mainMenuMode == MenuMain {
		panelW = 360
		panelH = 320
	}
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	drawFilledRect(screen, px, py, panelW, panelH, color.RGBA{20, 20, 40, 230})
	drawRectBorder(screen, px, py, panelW, panelH, 2, color.RGBA{100, 100, 160, 255})

	switch ui.game.mainMenuMode {
	case MenuMain:
		// Title
		ebitenutil.DebugPrintAt(screen, "Main Menu", px+16, py+14)
		// Options
		startY := py + 56
		for i, label := range mainMenuOptions {
			y := startY + i*32
			if i == ui.game.mainMenuSelection {
				drawFilledRect(screen, px+16, y-4, panelW-32, 28, color.RGBA{60, 120, 180, 200})
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
		tips := []string{
			"Controls:",
			"WASD: Move  QE: Strafe",
			"Space: Attack  F: Cast  H: Heal",
			"I: Inventory  C: Characters  M: Spellbook",
			"1-4: Select",
			"Tab: Toggle Mode (TB/RT)",
		}
		tipsY := startY + len(mainMenuOptions)*32 + 10
		for i, tip := range tips {
			ebitenutil.DebugPrintAt(screen, tip, px+16, tipsY+i*14)
		}
	case MenuSaveSelect:
		ebitenutil.DebugPrintAt(screen, "Save Game - Select Slot", px+16, py+14)
		startY := py + 56
		for i := 0; i < 5; i++ {
			y := startY + i*32
			sum := GetSaveSlotSummary(i)
			label := fmt.Sprintf("Slot %d", i+1)
			if sum.Exists {
				mode := "RT"
				if sum.TurnBased {
					mode = "TB"
				}
				// show time (short) and mode
				t := sum.SavedAt
				if len(t) > 19 {
					t = t[:19]
				} // RFC3339 short
				label = fmt.Sprintf("%s  [%s %s]", label, mode, t)
			}
			if i == ui.game.slotSelection {
				drawFilledRect(screen, px+16, y-4, panelW-32, 28, color.RGBA{80, 180, 80, 200})
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
	case MenuLoadSelect:
		ebitenutil.DebugPrintAt(screen, "Load Game - Select Slot", px+16, py+14)
		startY := py + 56
		for i := 0; i < 5; i++ {
			y := startY + i*32
			sum := GetSaveSlotSummary(i)
			label := fmt.Sprintf("Slot %d", i+1)
			if sum.Exists {
				mode := "RT"
				if sum.TurnBased {
					mode = "TB"
				}
				t := sum.SavedAt
				if len(t) > 19 {
					t = t[:19]
				}
				label = fmt.Sprintf("%s  [%s %s]", label, mode, t)
			}
			if i == ui.game.slotSelection {
				drawFilledRect(screen, px+16, y-4, panelW-32, 28, color.RGBA{180, 120, 60, 200})
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
	}
}

// drawTabbedMenu draws the tabbed menu interface with mouse click support
func (ui *UISystem) drawTabbedMenu(screen *ebiten.Image) {
	// Panel dimensions
	panelWidth := 700
	panelHeight := 500 // Increased from 441 to accommodate taller character cards
	panelX := (ui.game.config.GetScreenWidth() - panelWidth) / 2
	panelY := (ui.game.config.GetScreenHeight() - panelHeight) / 2

	// Draw main background
	drawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{0, 0, 30, 230}) // Dark blue background

	// Tab dimensions
	tabWidth := 120
	tabHeight := 35
	tabY := panelY + 10

	// Draw tabs
	tabs := []struct {
		tab   MenuTab
		label string
		key   string
	}{
		{TabInventory, "Inventory", "(I)"},
		{TabCharacters, "Characters", "(C)"},
		{TabSpellbook, "Spellbook", "(M)"},
		{TabQuests, "Quests", "(J)"},
	}

	for i, tabInfo := range tabs {
		tabX := panelX + 20 + (i * (tabWidth + 5)) // Reduced spacing between tabs

		// Determine tab state and colors
		isActive := ui.game.currentTab == tabInfo.tab
		var tabBgColor, tabBorderColor color.RGBA

		if isActive {
			// Active tab: lighter background, matches panel, no bottom border
			tabBgColor = color.RGBA{0, 0, 30, 230}        // Same as panel background
			tabBorderColor = color.RGBA{80, 80, 120, 255} // Light border
		} else {
			// Inactive tab: darker background, full border
			tabBgColor = color.RGBA{20, 20, 40, 200}     // Darker background
			tabBorderColor = color.RGBA{60, 60, 90, 255} // Darker border
		}

		// Draw tab background
		drawFilledRect(screen, tabX, tabY, tabWidth, tabHeight, tabBgColor)
		drawFilledRect(screen, tabX, tabY, tabWidth, 2, tabBorderColor)
		drawFilledRect(screen, tabX, tabY, 2, tabHeight, tabBorderColor)
		drawFilledRect(screen, tabX+tabWidth-2, tabY, 2, tabHeight, tabBorderColor)
		if !isActive {
			drawFilledRect(screen, tabX, tabY+tabHeight-2, tabWidth, 2, tabBorderColor)
		}

		// Draw tab text (using standard debug print for now)
		if isActive {
			ebitenutil.DebugPrintAt(screen, tabInfo.label, tabX+10, tabY+8)
		} else {
			ebitenutil.DebugPrintAt(screen, tabInfo.label, tabX+10, tabY+8)
		}
		ebitenutil.DebugPrintAt(screen, tabInfo.key, tabX+10, tabY+20)

		// Handle mouse clicks on tabs
		ui.handleTabClick(tabX, tabY, tabWidth, tabHeight, tabInfo.tab)
	}

	// Draw main panel border that connects with active tab
	panelBorderColor := color.RGBA{80, 80, 120, 255}

	// Top border (with gap for active tab)
	activeTabIndex := 0
	for i, tabInfo := range tabs {
		if ui.game.currentTab == tabInfo.tab {
			activeTabIndex = i
			break
		}
	}

	activeTabX := panelX + 20 + (activeTabIndex * (tabWidth + 5))

	// Left part of top border (before active tab)
	if activeTabX > panelX {
		drawFilledRect(screen, panelX, panelY, activeTabX-panelX, 2, panelBorderColor)
	}

	// Right part of top border (after active tab)
	rightStart := activeTabX + tabWidth
	if rightStart < panelX+panelWidth {
		drawFilledRect(screen, rightStart, panelY, (panelX+panelWidth)-rightStart, 2, panelBorderColor)
	}

	// Left, right, and bottom borders of main panel
	drawFilledRect(screen, panelX, panelY, 2, panelHeight, panelBorderColor)
	drawFilledRect(screen, panelX+panelWidth-2, panelY, 2, panelHeight, panelBorderColor)
	drawFilledRect(screen, panelX, panelY+panelHeight-2, panelWidth, 2, panelBorderColor)

	// Draw X close button in top-right corner
	closeButtonSize := 20
	closeButtonX := panelX + panelWidth - closeButtonSize - 5
	closeButtonY := panelY + 5

	// Handle mouse clicks on close button
	ui.handleCloseButtonClick(closeButtonX, closeButtonY, closeButtonSize, closeButtonSize)

	// Draw close button background
	mouseX, mouseY := ebiten.CursorPosition()
	isCloseHovering := mouseX >= closeButtonX && mouseX < closeButtonX+closeButtonSize &&
		mouseY >= closeButtonY && mouseY < closeButtonY+closeButtonSize

	if isCloseHovering {
		drawFilledRect(screen, closeButtonX, closeButtonY, closeButtonSize, closeButtonSize, color.RGBA{150, 50, 50, 200}) // Red hover
	} else {
		drawFilledRect(screen, closeButtonX, closeButtonY, closeButtonSize, closeButtonSize, color.RGBA{100, 100, 100, 150}) // Gray normal
	}

	// Draw X text
	ebitenutil.DebugPrintAt(screen, "X", closeButtonX+6, closeButtonY+4)

	// Draw content area
	contentY := tabY + tabHeight + 10
	contentHeight := panelHeight - tabHeight - 40

	// Draw content based on selected tab
	switch ui.game.currentTab {
	case TabInventory:
		ui.drawInventoryContent(screen, panelX, contentY, contentHeight)
	case TabCharacters:
		ui.drawCharactersContent(screen, panelX, contentY)
	case TabSpellbook:
		ui.drawSpellbookContent(screen, panelX, contentY, contentHeight)
	case TabQuests:
		ui.drawQuestsContent(screen, panelX, contentY, contentHeight)
	}
}

// handleTabClick checks if mouse clicked on a tab and switches to it
func (ui *UISystem) handleTabClick(tabX, tabY, tabWidth, tabHeight int, tab MenuTab) {
	if ui.game.consumeLeftClickIn(tabX, tabY, tabX+tabWidth, tabY+tabHeight) {
		ui.game.currentTab = tab
	}
}

// handleCharacterCardClick checks if mouse clicked on a character card and selects that character
func (ui *UISystem) handleCharacterCardClick(cardX, cardY, cardWidth, cardHeight, characterIndex int) {
	if ui.game.consumeLeftClickIn(cardX, cardY, cardX+cardWidth, cardY+cardHeight) {
		ui.game.selectedChar = characterIndex
	}
}

// handleCloseButtonClick checks if mouse clicked on the close button and closes the menu
func (ui *UISystem) handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight int) {
	if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
		ui.game.menuOpen = false
	}
}

// handleSpellbookSchoolClick checks if mouse clicked on a magic school and selects it
func (ui *UISystem) handleSpellbookSchoolClick(schoolX, schoolY, schoolWidth, schoolHeight int, schoolIndex int, school character.MagicSchool) {
	if ui.game.consumeLeftClickIn(schoolX, schoolY, schoolX+schoolWidth, schoolY+schoolHeight) {
		currentTime := ui.game.mouseLeftClickAt
		delta := currentTime - ui.game.lastSchoolClickTime
		doubleClick := ui.game.lastSchoolClickedIdx == schoolIndex && delta < doubleClickWindowMs

		ui.game.selectedSchool = schoolIndex
		ui.game.selectedSpell = 0 // Reset spell selection when changing school

		if doubleClick {
			ui.game.collapsedSpellSchools[school] = !ui.game.collapsedSpellSchools[school]
		}

		ui.game.lastSchoolClickTime = currentTime
		ui.game.lastSchoolClickedIdx = schoolIndex
	}
}

// handleSpellbookSpellClick checks if mouse clicked on a spell and selects it
func (ui *UISystem) handleSpellbookSpellClick(spellX, spellY, spellWidth, spellHeight, schoolIndex, spellIndex int) {
	if ui.game.consumeLeftClickIn(spellX, spellY, spellX+spellWidth, spellY+spellHeight) {
		currentTime := ui.game.mouseLeftClickAt

		// Check for double-click (within 500ms of last click on same spell)
		delta := currentTime - ui.game.lastSpellClickTime
		doubleClick := ui.game.lastClickedSpell == spellIndex &&
			ui.game.lastClickedSchool == schoolIndex &&
			delta < doubleClickWindowMs

		// Update selection for highlight and keyboard navigation
		ui.game.selectedSchool = schoolIndex
		ui.game.selectedSpell = spellIndex

		if doubleClick {
			// Double-click detected - cast the spell directly
			ui.game.combat.CastSelectedSpell()
		}

		// Update click tracking
		ui.game.lastSpellClickTime = currentTime
		ui.game.lastClickedSpell = spellIndex
		ui.game.lastClickedSchool = schoolIndex
	}
}

// updateMouseState should be called once per frame before input handling.
func (ui *UISystem) updateMouseState() {
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	rightJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	now := time.Now().UnixMilli()
	ui.game.pruneClickQueues(now)

	if leftJustPressed {
		x, y := ebiten.CursorPosition()
		ui.game.mouseLeftClicks = append(ui.game.mouseLeftClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseLeftClickX, ui.game.mouseLeftClickY = x, y
	}
	if rightJustPressed {
		x, y := ebiten.CursorPosition()
		ui.game.mouseRightClicks = append(ui.game.mouseRightClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseRightClickX, ui.game.mouseRightClickY = x, y
	}
}
