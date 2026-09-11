package game

import "ugataima/internal/character"

// These adapters consume only buffered clicks. Draw registers the visible
// surface; Update runs it before discarding unmatched modal events. They also
// serve the existing direct input entry points without polling keys or wheels.
func (ih *InputHandler) handleMainMenuMouseInput() {
	g := ih.game
	panelW, panelH := menuPanelSize(g.mainMenuMode)
	px, py := (g.config.GetScreenWidth()-panelW)/2, (g.config.GetScreenHeight()-panelH)/2
	switch g.mainMenuMode {
	case MenuMain:
		x, y, ok := g.leftClickPosition()
		if ok && g.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			ih.mainMenuHoverSelect(x, y, len(mainMenuOptions), panelW, panelH, mainMenuListTopY, mainMenuRowPitch)
			ih.activateMainMenuSelection()
		}
	case MenuSaveSelect:
		ih.handleSaveLoadMouseInput(px, py, panelW, panelH, true, ih.doSaveToSelectedRow)
	case MenuLoadSelect:
		ih.handleSaveLoadMouseInput(px, py, panelW, panelH, false, ih.doLoadFromSelectedRow)
	}
}

func (ih *InputHandler) handleSaveLoadMouseInput(px, py, panelW, panelH int, allowRename bool, activate func()) bool {
	g := ih.game
	if g.saveRenameOpen {
		return false
	}
	if ih.navigateSavePageMouse(px, py, panelW, panelH) {
		return true
	}
	// Rename exists only on the Save surface, never on Load or Autosave.
	if allowRename && ih.handleSaveRowRename(px, py, panelW) {
		return true
	}
	x, y, ok := g.leftClickPosition()
	if ok && g.consumeLeftClickIn(px, py+saveMenuListTopY-6, px+panelW, py+saveMenuListTopY-6+saveRowsPerPage*saveMenuRowPitch) {
		ih.mainMenuHoverSelect(x, y, saveRowsPerPage, panelW, panelH, saveMenuListTopY, saveMenuRowPitch)
		activate()
		return true
	}
	return false
}

func (ih *InputHandler) navigateSavePageMouse(px, py, panelW, panelH int) bool {
	g := ih.game
	pl, pr := savePagerButtonRects(px, py, panelW, panelH)
	switch {
	case g.consumeLeftClickIn(pl.x1, pl.y1, pl.x2, pl.y2):
		g.savePage = (g.savePage + savePageCount - 1) % savePageCount
	case g.consumeLeftClickIn(pr.x1, pr.y1, pr.x2, pr.y2):
		g.savePage = (g.savePage + 1) % savePageCount
	default:
		return false
	}
	return true
}

func (ih *InputHandler) handleLevelUpChoiceMouseInput() {
	g := ih.game
	req := g.currentLevelUpChoice()
	if req == nil || len(req.options) == 0 {
		return
	}
	x, _, w, _, startY, rowH := levelUpChoiceLayout(req, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	for i := range req.options {
		y := startY + i*rowH
		if g.consumeLeftClickIn(x+16, y-2, x+w-16, y-2+rowH) {
			req.selection = i
			if req.isMultiSelect() {
				g.toggleLevelUpSelection(i)
			} else {
				g.consumeLevelUpChoice(i)
			}
			return
		}
	}
	if req.isMultiSelect() {
		y := startY + len(req.options)*rowH
		if g.consumeLeftClickIn(x+16, y-2, x+w-16, y-2+rowH) {
			req.selection = req.confirmRowIndex()
			g.confirmLevelUpSelections()
		}
	}
}

func (ih *InputHandler) handleEncounterMouseInput() {
	npc := ih.game.dialogNPC
	if npc == nil || npc.DialogueData == nil {
		return
	}
	ih.consumeEncounterMouseInput(npc, ih.game.visibleNPCChoices(npc))
}

func (ih *InputHandler) consumeEncounterMouseInput(npc *character.NPC, choices []*character.NPCDialogueChoice) bool {
	// Mouse: clicking a choice row selects it; a second click on the same row
	// (double-click, like every other dialog list) executes it.
	dlg := npcDialogLayout(ih.game)
	for i := range choices {
		x, y, w, h := ih.game.dialogueChoiceRect(npc, i, dlg.x, dlg.y, dlg.w)
		if h == 0 {
			continue
		}
		if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
			ih.game.selectedChoice = i
			if ih.dialogDoubleClick("encounter_choice", i) {
				ih.executeEncounterChoice()
				ih.resetDialogDoubleClick()
			}
			return true
		}
	}

	return false
}
