package game

import "ugataima/internal/character"

// menuState owns the pause menu, its settings and the shared save/load picker.
// It is transient and is not part of GameSave. MMGame embeds it so rendering
// and input read the same state without compatibility copies.
type menuState struct {
	mainMenuOpen           bool
	mainMenuSelection      int
	mainMenuMode           MainMenuMode
	audioSettingsSelection int
	audioSliderDrag        int
	audioSettingsDirty     bool
	slotSelection          int
	savePage               int
	saveRenameOpen         bool
	saveRenameSlot         int
	saveRenameInput        string
}

func newMenuState() menuState {
	return menuState{saveRenameSlot: -1, audioSliderDrag: -1}
}

func (g *MMGame) openMainMenu() {
	g.closeMainMenu()
	g.mainMenuOpen = true
}

func (g *MMGame) closeMainMenu() {
	if g.mainMenuMode == MenuSettings {
		g.closeAudioSettings()
	}
	g.menuState = newMenuState()
}

// dialogState owns one conversation and every page, pending service and
// double-click identity beneath it. Modal priority and redraw gating remain in
// topModalLayerFor/topModalSnapshot; this owner only controls its own lifetime.
type dialogState struct {
	dialogLastClickTime  int64
	dialogLastClickedIdx int
	dialogLastClickZone  string
	dialogActive         bool
	dialogNPC            *character.NPC
	dialogSelectedSpell  int
	selectedCharIdx      int
	skillTrainerPopup    bool
	skillTrainerPage     int
	selectedSpellKey     string
	selectedChoice       int
	dialogNodePath       []*character.NPCDialogueChoice
	dialogTab            int
	pendingBuffService   *character.NPCDialogueChoice
	pendingTavernAction  *character.NPCDialogueChoice
	merchantBuyPage      int
	merchantSellPage     int
	spellTraderPage      int
	cardCollectorInvPage int
}

func newDialogState() dialogState {
	return dialogState{dialogLastClickedIdx: -1}
}

func (g *MMGame) beginConversation(npc *character.NPC) {
	g.closeConversation()
	g.dialogNPC = npc
	g.dialogActive = npc != nil
	g.switchDialogTab(0)
}

// closeConversation retires the entire conversation, whether closed by input,
// a completed service, travel, loading or a party wipe. A trainer popup's Escape
// still closes only that child in the modal dispatcher.
func (g *MMGame) closeConversation() {
	g.cancelStackSplitInteraction()
	g.clearStashDrag()
	g.rosterSelectedActive = -1
	g.dialogState = newDialogState()
}

// closeSaveRename dismisses the save-rename modal and clears its scratch state.
func (m *menuState) closeSaveRename() {
	m.saveRenameOpen = false
	m.saveRenameSlot = -1
	m.saveRenameInput = ""
}

// openSaveLoad switches the main menu into a save/load slot list, resetting the
// cursor to the first row of the first page. Single source for the "open a slot
// list" state so a new reset field is added in one place.
func (m *menuState) openSaveLoad(mode MainMenuMode) {
	m.mainMenuMode = mode
	m.slotSelection = 0
	m.savePage = 0
}
