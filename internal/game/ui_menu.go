package game

import (
	"fmt"
	"image/color"
	"time"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

const menuPanelFrameSlice = 16

// drawOverlayInterfaces draws overlay interfaces like menus and dialogs.
//
// CLICKS RESOLVE TOP-DOWN, DRAWING RUNS BOTTOM-UP. The map overlay opens from a
// quest item inside the character hub, so it floats ABOVE a hub that stays open:
// its close button lands on top of the hub's bag filter tabs. Handling its input
// first is what keeps that button clickable - the hub's own handlers would
// otherwise consume the click while drawing underneath.
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
	ui.registerDisplayedModalMouse((*InputHandler).handleMainMenuMouseInput, modalLayerMainMenu)
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Dim background
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 128})

	// Panel per mode, shared with the input hit-testing via mainMenuPanelRect.
	panel := mainMenuPanelRect(w, h, ui.game.mainMenuMode)
	px, py, panelW, panelH := panel.x, panel.y, panel.w, panel.h
	style := frameGold
	if ui.game.mainMenuMode == MenuControlTips {
		style = frameBronze
	}
	ui.drawThemeFrame(screen, style, px, py, panelW, panelH)
	if ui.game.mainMenuMode == MenuMain || ui.game.mainMenuMode == MenuControlTips {
		ui.drawCornerDecor(screen, style, px-8, py-8, panelW+16, panelH+16, decorTopCorners)
	}
	if ui.game.mainMenuMode == MenuControlTips {
		ui.drawPanelInlay(screen, frameBronze, px+panelW/2, py)
	}

	switch ui.game.mainMenuMode {
	case MenuMain:
		// Title
		title := mainMenuTitleRect(panel)
		drawCenteredUIText(screen, mainMenuTitle, title.x, title.y, title.w, title.h)
		// Options
		for i, option := range mainMenuOptions {
			box, _, _ := menuRowRect(px, py, panelW, mainMenuListTopY, mainMenuRowPitch, i)
			ui.drawMenuButton(screen, option.label, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, i == ui.game.mainMenuSelection)
		}
	case MenuControlTips:
		drawScaledMetalCenteredText(screen, uitext.Text("ui.control_tips"), w/2, py+28, 2, rarityGold)
		ui.drawControlTips(screen, panel)
		back := controlTipsBackRect(panel)
		ui.drawBackButton(screen, back.x, back.y, func() { ui.game.mainMenuMode = MenuMain })

	case MenuSaveSelect, MenuLoadSelect:
		saving := ui.game.mainMenuMode == MenuSaveSelect
		l := escSaveListLayout(w, h)
		heading, hint := "Load Game", loadMenuHint
		if saving {
			heading, hint = "Save Game", saveMenuHint
		}
		ui.drawSaveList(screen, l, heading, ui.game.slotSelection, saving, !ui.game.saveRenameOpen)
		drawUITextColored(screen, clipUIText(hint, l.title.w), l.title.x, l.hintY, gameplayInk.muted)
		if ui.game.saveRenameOpen {
			ui.drawSaveRenameDialog(screen)
		} else {
			ui.drawSaveRowHoverTooltip(screen, l)
		}
	case MenuSettings:
		settingsLayout := audioSettingsPanelLayoutAt(px, py, panelW, panelH, false)
		ui.drawSettingsContent(screen, settingsLayout.px, settingsLayout.py, settingsLayout.panelW, settingsLayout.panelH, settingsLayout.contentInset)
		back := audioBackRect(settingsLayout.px, settingsLayout.py, settingsLayout.panelH, settingsLayout.contentInset)
		ui.drawBackButton(screen, back.x1, back.y1, func() {
			ui.game.closeAudioSettings()
		})
	}
}

const mainMenuTitle = "Main Menu"

// mainMenuTitleRect is the box the pause menu's title is centred in.
func mainMenuTitleRect(panel layoutRect) layoutRect {
	return layoutRect{panel.x + 32, panel.y + 16, panel.w - 64, 20}
}

// pagerRect is a button hitbox shared between draw and input code.
type pagerRect struct{ x1, y1, x2, y2 int }

func saveRenameDialogRect(screenW, screenH int) layoutRect {
	return centeredRect(screenW, screenH, 420, 140)
}

func (ui *UISystem) drawSaveRenameDialog(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()
	dialog := saveRenameDialogRect(w, h)
	x, y, dialogW, dialogH := dialog.x, dialog.y, dialog.w, dialog.h
	ui.drawThemeFrame(screen, frameSilver, x, y, dialogW, dialogH)

	title := fmt.Sprintf("Rename %s", saveRowLabel(ui.game.saveRenameSlot))
	drawCenteredUIText(screen, title, x, y+10, dialogW, 20)

	inputBoxX := x + 24
	inputBoxY := y + 48
	inputBoxW := dialogW - 48
	inputBoxH := 28
	ui.drawThemeFrame(screen, frameSilver, inputBoxX, inputBoxY, inputBoxW, inputBoxH)

	input := ui.game.saveRenameInput
	if input == "" {
		input = "(empty)"
	}
	drawCenteredUIText(screen, input, inputBoxX, inputBoxY, inputBoxW, inputBoxH)

	drawUIText(screen, "Enter: Confirm  Esc: Cancel", x+60, y+90)
}

// drawTabbedMenu draws the tabbed menu interface with mouse click support
func (ui *UISystem) drawTabbedMenu(screen *ebiten.Image) {
	viewportBottom := gameplayViewportBottom(ui.game)
	layout := computeTabbedMenuLayout(ui.game.config.GetScreenWidth(), viewportBottom)

	// The hub owns only the gameplay viewport. The persistent party HUD below
	// stays fully visible and remains the one mouse selector for characters.
	drawFilledRect(screen, 0, 0, uiBounds(screen).Dx(), viewportBottom, color.RGBA{5, 7, 12, 205})
	ui.drawPatternFrame(screen, "menu_panel_frame", layout.panel.x, layout.panel.y, layout.panel.w, layout.panel.h, menuPanelFrameSlice)
	ui.drawCornerDecor(screen, frameGold, layout.panel.x-8, layout.panel.y-8, layout.panel.w+16, layout.panel.h+16, decorBottomCorners)

	for i, tabInfo := range tabbedMenuTabs {
		tabRect := layout.tabs[i]

		isActive := ui.game.currentTab == tabInfo.tab
		ui.drawButtonFrame(screen, tabRect.x, tabRect.y, tabRect.w, tabRect.h, isActive)

		// One line leaves the decorative top/bottom rails clear at every scale.
		drawCenteredUIText(screen, tabInfo.label+" "+tabInfo.key, tabRect.x, tabRect.y, tabRect.w, tabRect.h)

		// A modal above the hub owns the click queue; tabs must not consume one of
		// its buttons through the overlay (topModalLayer).
		if !ui.modalLayerOwnsInput() {
			ui.handleTabClick(tabRect.x, tabRect.y, tabRect.w, tabRect.h, tabInfo.tab)
		}
	}
	// Handle mouse clicks on close button
	if !ui.modalLayerOwnsInput() {
		ui.handleCloseButtonClick(layout.close.x, layout.close.y, layout.close.w, layout.close.h)
	}

	ui.drawCloseButtonVisual(screen, layout.close.x, layout.close.y, layout.close.w, layout.close.h)

	// Draw content based on selected tab
	switch ui.game.currentTab {
	case TabInventory:
		ui.drawInventoryContent(screen, layout.content)
	case TabCharacters:
		ui.drawCharactersContent(screen, layout.content)
	case TabSpellbook:
		ui.drawSpellbookContent(screen, layout.content)
	case TabQuests:
		ui.drawQuestsContent(screen, layout.content)
	case TabCards:
		ui.drawCardsContent(screen, layout.content)
	}

	// Carried drag icon (topmost) + cancel of any drop that landed on nothing.
	ui.drawDragCarried(screen)
}

// drawCardsContent shows the party's active monster-card collection as an
// art grid (icon + name + effect per slot) plus a combined-effects summary.
// View-only: cards are slotted/removed at the Card Collector NPC.
func (ui *UISystem) drawCardsContent(screen *ebiten.Image, content layoutRect) {
	layout := computeCardsContentLayout(content)
	drawCenteredUIText(screen, "Active Card Collection", layout.title.x, layout.title.y, layout.title.w, layout.title.h)
	drawCenteredUIText(screen, "Slot or remove cards at the Card Collector in the desert.", layout.subtitle.x, layout.subtitle.y, layout.subtitle.w, layout.subtitle.h)

	mouseX, mouseY := uiCursorPosition()
	var hover character.CardRows

	for slot := 0; slot < MaxCardSlots; slot++ {
		card := layout.cards[slot]
		x, y, icon := card.x, card.y, card.w
		key := ui.game.cardCollectionKey(slot)
		hovered := ui.drawCardCell(screen, key, x, y, icon, "empty")
		if def := cardDef(key); def != nil {
			// Label box = one column pitch minus a small gutter, centered on the
			// card, with text clipped to fit - so neighbouring labels never collide.
			labelW := layout.labelW
			labelX := x - (labelW-icon)/2
			drawCenteredUIText(screen, clipUIText(def.Name, labelW), labelX, y+icon+2, labelW, 14)

			if hovered {
				hover = ui.appendCardArtHintRows(cardItemTooltipRows(key), key)
			}
		}
	}

	ui.drawCardEffectsList(screen, layout.summary)

	if hover != nil {
		ui.queueCardTooltip(hover, nil, nil, nil, "", mouseX+16, mouseY+8)
	}
}

// handleTabClick checks if mouse clicked on a tab and switches to it
func (ui *UISystem) handleTabClick(tabX, tabY, tabWidth, tabHeight int, tab MenuTab) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandClick, layoutRect{tabX, tabY, tabWidth, tabHeight}, func() { ui.handleTabClick(tabX, tabY, tabWidth, tabHeight, tab) })
		return
	}

	if ui.game.consumeLeftClickIn(tabX, tabY, tabX+tabWidth, tabY+tabHeight) {
		if tab == TabSpellbook && ui.game.currentTab != TabSpellbook {
			// Entering the spellbook fresh: no spell highlighted until user picks one.
			ui.game.selectedSpell = -1
			ui.spellPage = 0
		}
		ui.game.currentTab = tab
	}
}

// handleCloseButtonClick checks if mouse clicked on the close button and closes the menu
func (ui *UISystem) handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight int) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandClick, layoutRect{buttonX, buttonY, buttonWidth, buttonHeight}, func() { ui.handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight) })
		return
	}

	if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
		ui.game.menuOpen = false
	}
}

// dispatchCharacterHubWorldAction closes the hub before a world-facing action
// observes game state. Refused actions restore the hub unless a follow-up modal
// took ownership (target selection is not yet a committed action). The return
// value still reports commitment, so opening a picker never spends an action.
// Mouse and keyboard dispatch share this one transition.
func (g *MMGame) dispatchCharacterHubWorldAction(action func() bool) bool {
	wasOpen := g.menuOpen
	g.menuOpen = false
	if action() {
		return true
	}
	if topModalLayerFor(g, false) == modalLayerNone {
		g.menuOpen = wasOpen
	}
	return false
}

// handleSpellbookSchoolClick checks if mouse clicked on a magic school and selects it
func (ui *UISystem) handleSpellbookSchoolClick(bounds layoutRect, schoolIndex int, school character.MagicSchoolID) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandClick, bounds, func() { ui.handleSpellbookSchoolClick(bounds, schoolIndex, school) })
		return
	}

	if ui.modalLayerOwnsInput() {
		return
	}
	if ui.game.consumeLeftClickIn(bounds.x, bounds.y, bounds.right(), bounds.bottom()) {
		currentTime := ui.game.mouseLeftClickAt
		doubleClick := ui.game.lastSchoolClickedIdx == schoolIndex &&
			withinDoubleClickWindow(currentTime, ui.game.lastSchoolClickTime)

		ui.game.selectedSchool = schoolIndex
		// Don't auto-select a spell - wait for the user to click one.
		ui.game.selectedSpell = -1
		ui.spellPage = 0

		if doubleClick {
			ui.game.collapsedSpellSchools[school] = !ui.game.collapsedSpellSchools[school]
			ui.game.lastSchoolClickTime = 0
			ui.game.lastSchoolClickedIdx = -1
			return
		}

		ui.game.lastSchoolClickTime = currentTime
		ui.game.lastSchoolClickedIdx = schoolIndex
	}
}

// syncCharacterHubClickContext prevents one screen's first click from pairing
// with a click made for another character or tab. It runs before any Draw-phase
// hub widget consumes the queued click.
func (ui *UISystem) syncCharacterHubClickContext() {
	if ui.game == nil || !ui.game.menuOpen {
		ui.hubInteractionOpen = false
		return
	}
	if ui.hubInteractionOpen && ui.hubInteractionChar == ui.game.selectedChar && ui.hubInteractionTab == ui.game.currentTab {
		return
	}
	ui.inventoryContextOpen, ui.inventoryContextOwner = false, nil
	ui.personalInventoryPage = 0
	ui.hubInteractionOpen = true
	ui.hubInteractionChar = ui.game.selectedChar
	ui.hubInteractionTab = ui.game.currentTab
	ui.lastClickTime = time.Time{}
	ui.lastClickedItem = -1
	ui.lastEquipClickTime = time.Time{}
	ui.lastClickedSlot = items.EquipSlot(-1)
	ui.game.lastBookClickTime = 0
	ui.game.lastClickedBookEntry = -1
	ui.game.lastClickedBookGroup = -1
	ui.game.lastSchoolClickTime = 0
	ui.game.lastSchoolClickedIdx = -1
}

// Both books bind a double-click to the quick action. The callback only
// supplies the book's selection and equipment rules; gesture ownership is shared.
func (ui *UISystem) handleBookEntryClick(bounds layoutRect, group, index int, selectEntry, equipEntry func()) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandClick, bounds, func() { ui.handleBookEntryClick(bounds, group, index, selectEntry, equipEntry) })
		return
	}
	if ui.modalLayerOwnsInput() || !ui.game.consumeLeftClickIn(bounds.x, bounds.y, bounds.right(), bounds.bottom()) {
		return
	}
	g := ui.game
	now := g.mouseLeftClickAt
	doubleClick := g.lastClickedBookEntry == index && g.lastClickedBookGroup == group && withinDoubleClickWindow(now, g.lastBookClickTime)
	selectEntry()
	if doubleClick {
		equipEntry()
		g.lastBookClickTime, g.lastClickedBookEntry, g.lastClickedBookGroup = 0, -1, -1
		return
	}
	g.lastBookClickTime, g.lastClickedBookEntry, g.lastClickedBookGroup = now, index, group
}

func (ui *UISystem) handleSpellbookSpellClick(x, y, w, h, schoolIndex, spellIndex int) {
	ui.handleBookEntryClick(layoutRect{x, y, w, h}, schoolIndex, spellIndex, func() {
		ui.game.selectedSchool, ui.game.selectedSpell = schoolIndex, spellIndex
	}, func() { ui.game.combat.EquipSelectedSpell() })
}

const bookControlsHint = "Up/Down: Navigate  Enter/F: Use  Click: Select  Double-click: Equip quick slot"

// updateMouseState should be called once per frame before input handling.
