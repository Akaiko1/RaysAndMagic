package game

import "ugataima/internal/character"

// These adapters consume only buffered clicks. Draw registers the visible
// surface; Update runs it before discarding unmatched modal events. Keyboard
// and wheel handlers never invoke these adapters.
func (ih *InputHandler) handleMainMenuMouseInput() {
	g := ih.game
	panel := mainMenuPanelRect(g.config.GetScreenWidth(), g.config.GetScreenHeight(), g.mainMenuMode)
	px, py, panelW, panelH := panel.x, panel.y, panel.w, panel.h
	switch g.mainMenuMode {
	case MenuMain:
		x, y, ok := g.leftClickPosition()
		if ok && g.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			ih.mainMenuHoverSelect(x, y, len(mainMenuOptions), panelW, panelH, mainMenuListTopY, mainMenuRowPitch)
			ih.activateMainMenuSelection()
		}
	case MenuSaveSelect:
		ih.handleSaveLoadMouseInput(escSaveListLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight()), true, ih.doSaveToSelectedRow)
	case MenuLoadSelect:
		ih.handleSaveLoadMouseInput(escSaveListLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight()), false, ih.doLoadFromSelectedRow)
	}
}

func (ih *InputHandler) handleSaveLoadMouseInput(l saveListLayout, allowRename bool, activate func()) bool {
	g := ih.game
	if g.saveRenameOpen {
		return false
	}
	if ih.navigateSavePageMouse(l) {
		return true
	}
	// Rename exists only on the Save surface, never on Load or Autosave.
	if allowRename && ih.handleSaveRowRename(l) {
		return true
	}
	for i, r := range l.rows {
		if g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			g.slotSelection = i
			activate()
			return true
		}
	}
	return false
}

func (ih *InputHandler) navigateSavePageMouse(l saveListLayout) bool {
	g := ih.game
	switch {
	case g.consumeLeftClickIn(l.prev.x, l.prev.y, l.prev.right(), l.prev.bottom()):
		g.savePage = (g.savePage + savePageCount - 1) % savePageCount
	case g.consumeLeftClickIn(l.next.x, l.next.y, l.next.right(), l.next.bottom()):
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
	p := makeLevelUpChoicePanel(req, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	for i := range req.options {
		r := p.target(i)
		if g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
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
		r := p.confirm
		if g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
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
