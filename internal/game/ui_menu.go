package game

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"ugataima/internal/character"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const menuPanelFrameSlice = 16

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

	// Panel size per mode (shared with the input hit-testing via menuPanelSize).
	panelW, panelH := menuPanelSize(ui.game.mainMenuMode)
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	drawFilledRect(screen, px, py, panelW, panelH, color.RGBA{20, 20, 40, 230})
	drawRectBorder(screen, px, py, panelW, panelH, 2, color.RGBA{100, 100, 160, 255})

	switch ui.game.mainMenuMode {
	case MenuMain:
		// Title
		drawDebugText(screen, "Main Menu", px+16, py+14)
		// Options
		for i, label := range mainMenuOptions {
			box, tx, ty := menuRowRect(px, py, panelW, mainMenuListTopY, mainMenuRowPitch, i)
			if i == ui.game.mainMenuSelection {
				drawFilledRect(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, color.RGBA{60, 120, 180, 200})
			}
			drawDebugText(screen, label, tx, ty)
		}
		tipsY := py + mainMenuTipsTopY()
		for i, tip := range mainMenuControlTips {
			drawDebugText(screen, tip, px+16, tipsY+i*debugTextCharHeight)
		}
	case MenuSaveSelect:
		drawDebugText(screen, "Save Game - Select Slot", px+16, py+14)
		drawDebugText(screen, "Enter: Save  R: Rename  Left/Right: Page", px+16, py+32)
		ui.drawSaveRowList(screen, px, py, panelW, panelH, color.RGBA{80, 180, 80, 200})
		if ui.game.saveRenameOpen {
			ui.drawSaveRenameDialog(screen)
		} else {
			ui.drawSaveRowHoverTooltip(screen, px, py, panelW)
		}
	case MenuLoadSelect:
		drawDebugText(screen, "Load Game - Select Slot", px+16, py+14)
		drawDebugText(screen, "Enter: Load  Left/Right: Page", px+16, py+32)
		ui.drawSaveRowList(screen, px, py, panelW, panelH, color.RGBA{180, 120, 60, 200})
		ui.drawSaveRowHoverTooltip(screen, px, py, panelW)
	}
}

// pagerRect is a button hitbox shared between the save-menu draw and input code.
type pagerRect struct{ x1, y1, x2, y2 int }

// savePagerButtonRects returns the Prev and Next button boxes for the save/load
// menus: a strip just below the panel, buttons pinned to the left and right
// edges. Shared by drawSaveRowList (render) and navigateSavePage (clicks).
func savePagerButtonRects(px, py, panelW, panelH int) (prev, next pagerRect) {
	const bw, bh = 84, 26
	y := py + panelH + 6
	prev = pagerRect{px, y, px + bw, y + bh}
	next = pagerRect{px + panelW - bw, y, px + panelW, y + bh}
	return
}

// drawSaveRowList renders the saveRowsPerPage rows of the current page (row 0 of
// page 0 is the load-only Autosave) plus a Prev/Next pager strip below the panel.
// highlight tints the selected row.
func (ui *UISystem) drawSaveRowList(screen *ebiten.Image, px, py, panelW, panelH int, highlight color.RGBA) {
	g := ui.game
	for i := 0; i < saveRowsPerPage; i++ {
		row := g.savePage*saveRowsPerPage + i
		box, tx, ty := menuRowRect(px, py, panelW, saveMenuListTopY, saveMenuRowPitch, i)
		sum := GetSaveRowSummary(row)
		label := saveRowLabel(row)
		if sum.Name != "" && !saveRowIsAutosave(row) {
			label = fmt.Sprintf("%s: %s", label, truncateSaveName(sum.Name, 18))
		}
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
		} else if saveRowIsAutosave(row) {
			label = fmt.Sprintf("%s  (empty)", label)
		}
		if i == g.slotSelection {
			drawFilledRect(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, highlight)
		}
		// Saves from THIS playthrough glow so your own run stands out. Matched
		// by run id: member names collide across runs (the default roster is
		// identical every new game) and shift within one (tavern swaps).
		if sum.Exists && sum.RunID != "" && sum.RunID == g.playthroughID {
			drawFilledRect(screen, box.x1, box.y1, 4, box.y2-box.y1, color.RGBA{110, 200, 110, 255})
			drawRectBorder(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, 1, color.RGBA{110, 200, 110, 160})
		}
		drawDebugText(screen, label, tx, ty)
	}
	ui.drawSavePagerStrip(screen, px, py, panelW, panelH)
	// Note: the hover tooltip is drawn by the menu case AFTER this, so it sits on
	// top of the pager strip instead of being covered by it.
}

// drawSaveRowHoverTooltip shows play time + the saved party (name, level, class)
// for the row under the cursor, so you can tell saves apart before loading.
func (ui *UISystem) drawSaveRowHoverTooltip(screen *ebiten.Image, px, py, panelW int) {
	mx, my := ebiten.CursorPosition()
	for i := 0; i < saveRowsPerPage; i++ {
		box, _, _ := menuRowRect(px, py, panelW, saveMenuListTopY, saveMenuRowPitch, i)
		if mx < box.x1 || mx >= box.x2 || my < box.y1 || my >= box.y2 {
			continue
		}
		sum := GetSaveRowSummary(ui.game.savePage*saveRowsPerPage + i)
		if !sum.Exists {
			return
		}
		var lines []string
		if sum.PlayTime != "" {
			lines = append(lines, "Play time: "+sum.PlayTime)
		}
		for _, m := range sum.Party {
			lines = append(lines, fmt.Sprintf("%s  Lv.%d %s", m.Name, m.Level, character.CharacterClass(m.Class).String()))
		}
		if len(lines) > 0 {
			ui.drawTooltipLines(screen, mx+16, my+16, lines)
		}
		return
	}
}

// drawTooltipLines renders a small bordered tooltip box of text lines, clamped to
// the screen so it never spills off the edge.
func (ui *UISystem) drawTooltipLines(screen *ebiten.Image, x, y int, lines []string) {
	boxW := 0
	for _, l := range lines {
		if lw := debugTextWidth(l); lw > boxW {
			boxW = lw
		}
	}
	boxW += 16
	boxH := len(lines)*16 + 10
	sw := ui.game.config.GetScreenWidth()
	sh := ui.game.config.GetScreenHeight()
	if x+boxW > sw {
		x = sw - boxW
	}
	if y+boxH > sh {
		y = sh - boxH
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	drawFilledRect(screen, x, y, boxW, boxH, color.RGBA{12, 12, 28, 240})
	drawRectBorder(screen, x, y, boxW, boxH, 1, color.RGBA{120, 120, 180, 230})
	for i, l := range lines {
		drawDebugText(screen, l, x+8, y+6+i*16)
	}
}

// drawSavePagerStrip draws the Prev/Next buttons and the page indicator on a
// short strip below the menu panel. Inactive buttons (at the first/last page) are
// dimmed. Mirrors the hitboxes from savePagerButtonRects.
func (ui *UISystem) drawSavePagerStrip(screen *ebiten.Image, px, py, panelW, panelH int) {
	g := ui.game
	prev, next := savePagerButtonRects(px, py, panelW, panelH)
	stripY := prev.y1
	stripH := prev.y2 - prev.y1
	drawFilledRect(screen, px, stripY, panelW, stripH, color.RGBA{18, 18, 34, 235})
	drawRectBorder(screen, px, stripY, panelW, stripH, 1, color.RGBA{100, 100, 160, 220})

	drawPagerBtn := func(r pagerRect, label string, enabled bool) {
		fill := color.RGBA{60, 60, 100, 230}
		if !enabled {
			fill = color.RGBA{35, 35, 55, 200}
		}
		drawFilledRect(screen, r.x1, r.y1, r.x2-r.x1, r.y2-r.y1, fill)
		drawRectBorder(screen, r.x1, r.y1, r.x2-r.x1, r.y2-r.y1, 1, color.RGBA{120, 120, 180, 230})
		drawCenteredDebugText(screen, label, r.x1, r.y1+(stripH-12)/2, r.x2-r.x1, 12)
	}
	drawPagerBtn(prev, "< Prev", true) // pages wrap - both directions always live
	drawPagerBtn(next, "Next >", true)
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/%d", g.savePage+1, savePageCount), px, stripY+(stripH-12)/2, panelW, 12)
}

func truncateSaveName(name string, max int) string {
	return truncateRunes(name, max, "...")
}

func (ui *UISystem) drawSaveRenameDialog(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()
	dialogW, dialogH := 420, 140
	x := (w - dialogW) / 2
	y := (h - dialogH) / 2
	drawFilledRect(screen, x, y, dialogW, dialogH, color.RGBA{15, 15, 35, 235})
	drawRectBorder(screen, x, y, dialogW, dialogH, 2, color.RGBA{120, 120, 180, 255})

	title := fmt.Sprintf("Rename %s", saveRowLabel(ui.game.saveRenameSlot))
	drawCenteredDebugText(screen, title, x, y+10, dialogW, 20)

	inputBoxX := x + 24
	inputBoxY := y + 48
	inputBoxW := dialogW - 48
	inputBoxH := 28
	drawFilledRect(screen, inputBoxX, inputBoxY, inputBoxW, inputBoxH, color.RGBA{30, 30, 60, 240})
	drawRectBorder(screen, inputBoxX, inputBoxY, inputBoxW, inputBoxH, 1, color.RGBA{140, 140, 200, 255})

	input := ui.game.saveRenameInput
	if input == "" {
		input = "(empty)"
	}
	drawCenteredDebugText(screen, input, inputBoxX, inputBoxY, inputBoxW, inputBoxH)

	drawDebugText(screen, "Enter: Confirm  Esc: Cancel", x+60, y+90)
}

// drawTabbedMenu draws the tabbed menu interface with mouse click support
func (ui *UISystem) drawTabbedMenu(screen *ebiten.Image) {
	layout := computeTabbedMenuLayout(ui.game.config.GetScreenWidth(), ui.game.config.GetScreenHeight())

	// Draw main background and frame
	menuFrame := ui.game.sprites.GetSprite("menu_panel_frame")
	drawNineSlice(screen, menuFrame, layout.panel.x, layout.panel.y, layout.panel.w, layout.panel.h, menuPanelFrameSlice)

	for i, tabInfo := range tabbedMenuTabs {
		tabRect := layout.tabs[i]

		isActive := ui.game.currentTab == tabInfo.tab
		tabSpriteName := "menu_tab_inactive"
		if isActive {
			tabSpriteName = "menu_tab_active"
		}
		drawImageScaled(screen, ui.game.sprites.GetSprite(tabSpriteName), tabRect.x, tabRect.y, tabRect.w, tabRect.h)

		// Draw tab text centered
		topHalf := tabRect.h / 2
		drawCenteredDebugText(screen, tabInfo.label, tabRect.x, tabRect.y, tabRect.w, topHalf)
		drawCenteredDebugText(screen, tabInfo.key, tabRect.x, tabRect.y+topHalf, tabRect.w, tabRect.h-topHalf)

		// Handle mouse clicks on tabs
		ui.handleTabClick(tabRect.x, tabRect.y, tabRect.w, tabRect.h, tabInfo.tab)
	}

	// Handle mouse clicks on close button
	ui.handleCloseButtonClick(layout.close.x, layout.close.y, layout.close.w, layout.close.h)

	// Draw close button background
	mouseX, mouseY := ebiten.CursorPosition()
	isCloseHovering := mouseX >= layout.close.x && mouseX < layout.close.right() &&
		mouseY >= layout.close.y && mouseY < layout.close.bottom()

	if isCloseHovering {
		drawFilledRect(screen, layout.close.x, layout.close.y, layout.close.w, layout.close.h, color.RGBA{150, 50, 50, 200}) // Red hover
	} else {
		drawFilledRect(screen, layout.close.x, layout.close.y, layout.close.w, layout.close.h, color.RGBA{100, 100, 100, 150}) // Gray normal
	}

	ui.drawInterfaceIcon(screen, "icon_close", layout.close.x, layout.close.y, layout.close.w, layout.close.h)

	// Draw content based on selected tab
	switch ui.game.currentTab {
	case TabInventory:
		ui.drawInventoryContent(screen, layout.content.x, layout.content.y, layout.content.h)
	case TabCharacters:
		ui.drawCharactersContent(screen, layout.content.x, layout.content.y, layout.content.h)
	case TabSpellbook:
		ui.drawSpellbookContent(screen, layout.content.x, layout.content.y, layout.content.h)
	case TabQuests:
		ui.drawQuestsContent(screen, layout.content.x, layout.content.y, layout.content.h)
	case TabCards:
		ui.drawCardsContent(screen, layout.content.x, layout.content.y, layout.content.h)
	}

	// Carried drag icon (topmost) + cancel of any drop that landed on nothing.
	ui.drawDragCarried(screen)
}

// drawCardsContent shows the party's active monster-card collection as an
// art grid (icon + name + effect per slot) plus a combined-effects summary.
// View-only: cards are slotted/removed at the Card Collector NPC.
func (ui *UISystem) drawCardsContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	content := layoutRect{panelX, contentY, tabbedMenuPanelW, contentHeight}
	layout := computeCardsContentLayout(content)
	drawDebugText(screen, "Active Card Collection", layout.title.x, layout.title.y)
	drawDebugText(screen, "Slot or remove cards at the Card Collector in the desert.", layout.subtitle.x, layout.subtitle.y)

	mouseX, mouseY := ebiten.CursorPosition()
	var hover []string

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
			drawCenteredDebugText(screen, clipDebugText(def.Name, labelW), labelX, y+icon+2, labelW, 14)
			drawCenteredDebugText(screen, clipDebugText(cardEffectText(def), labelW), labelX, y+icon+2+debugTextCharHeight, labelW, 14)
			if hovered {
				hover = ui.appendCardArtHint([]string{def.Name, cardEffectText(def)}, key)
			}
		}
	}

	// Combined totals: fold the active cards, format via the shared CardEffectLines.
	summary := "No active card effects."
	if parts := ui.game.cardCollectionAggregate().CardEffectLines(); len(parts) > 0 {
		summary = "Active: " + strings.Join(parts, ", ")
	}
	// Wrap to the panel width so a full 8-card list doesn't run off the edge.
	for i, line := range wrapDebugText(summary, layout.summary.w) {
		drawDebugText(screen, line, layout.summary.x, layout.summary.y+i*debugTextCharHeight)
	}

	if hover != nil {
		ui.queueTooltip(hover, mouseX+16, mouseY+8)
	}
}

// handleTabClick checks if mouse clicked on a tab and switches to it
func (ui *UISystem) handleTabClick(tabX, tabY, tabWidth, tabHeight int, tab MenuTab) {
	if ui.game.consumeLeftClickIn(tabX, tabY, tabX+tabWidth, tabY+tabHeight) {
		if tab == TabSpellbook && ui.game.currentTab != TabSpellbook {
			// Entering the spellbook fresh: no spell highlighted until user picks one.
			ui.game.selectedSpell = -1
		}
		ui.game.currentTab = tab
	}
}

// handleCloseButtonClick checks if mouse clicked on the close button and closes the menu
func (ui *UISystem) handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight int) {
	if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
		ui.game.menuOpen = false
	}
}

// handleSpellbookSchoolClick checks if mouse clicked on a magic school and selects it
func (ui *UISystem) handleSpellbookSchoolClick(schoolX, schoolY, schoolWidth, schoolHeight int, schoolIndex int, school character.MagicSchoolID) {
	if ui.game.consumeLeftClickIn(schoolX, schoolY, schoolX+schoolWidth, schoolY+schoolHeight) {
		currentTime := ui.game.mouseLeftClickAt
		doubleClick := ui.game.lastSchoolClickedIdx == schoolIndex &&
			withinDoubleClickWindow(currentTime, ui.game.lastSchoolClickTime)

		ui.game.selectedSchool = schoolIndex
		// Don't auto-select a spell - wait for the user to click one.
		ui.game.selectedSpell = -1

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

// handleSpellbookSpellClick checks if mouse clicked on a spell and selects it
func (ui *UISystem) handleSpellbookSpellClick(spellX, spellY, spellWidth, spellHeight, schoolIndex, spellIndex int) {
	if ui.game.consumeLeftClickIn(spellX, spellY, spellX+spellWidth, spellY+spellHeight) {
		currentTime := ui.game.mouseLeftClickAt

		// Check for a fast second click on the same spell.
		doubleClick := ui.game.lastClickedSpell == spellIndex &&
			ui.game.lastClickedSchool == schoolIndex &&
			withinDoubleClickWindow(currentTime, ui.game.lastSpellClickTime)

		// Update selection for highlight and keyboard navigation
		ui.game.selectedSchool = schoolIndex
		ui.game.selectedSpell = spellIndex

		if doubleClick {
			// Double-click detected - cast the spell directly. In turn-based
			// mode, a successful cast consumes one action slot for the active
			// character (just like F-key on the equipped spell), so it can't
			// be spammed beyond their Speed-derived budget.
			if cast, spellID := ui.game.combat.CastSelectedSpell(); cast {
				currentChar := ui.game.party.Members[ui.game.selectedChar]
				ui.game.consumeSelectedCharActionWithRTCooldown(ui.game.combat.SpellCooldownFrames(currentChar, spellID))
			}
			ui.game.lastSpellClickTime = 0
			ui.game.lastClickedSpell = -1
			ui.game.lastClickedSchool = -1
			return
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
	// Buffered clicks never cross a UI-layer boundary: on a modal<->world flip
	// drop the queues (a click aimed at one layer must not fire in the next).
	// Runs before this frame's clicks enqueue; within one layer (dialog
	// double-clicks) no flip occurs.
	if allowed := ui.game.worldClickAllowed(); allowed != ui.game.prevWorldClickAllowed {
		ui.game.mouseLeftClicks = ui.game.mouseLeftClicks[:0]
		ui.game.mouseRightClicks = ui.game.mouseRightClicks[:0]
		ui.game.prevWorldClickAllowed = allowed
	}
	ui.game.pruneClickQueues(now)
	ui.updateQuickDrag()
	ui.updateStashDrag()

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
