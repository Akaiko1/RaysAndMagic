package game

import (
	"errors"
	"fmt"
	uitext "ugataima/assets/text"
)

// ErrExit is returned from the game loop to request a clean exit
var ErrExit = errors.New("exit game")

// Save-slot menu layout. The menus show saveRowsPerPage rows across savePageCount
// pages. Global row 0 is the shared Autosave (written on map change and stash
// use) and row 1 the Quicksave (F5); the game writes both and the menus only
// load them. Rows 2.. are the manual slots: row r is "Slot r-1" in
// save{r-1}.json, so existing saves keep their slot numbers.
const (
	saveRowsPerPage = 7
	savePageCount   = 4
	saveRowCount    = saveRowsPerPage * savePageCount
	autosaveRow     = 0
	quicksaveRow    = 1
	firstManualRow  = 2
	autosaveFile    = "autosave.json"
	quicksaveFile   = "quicksave.json"

	// Shared geometry for the save/load menus, used by both the draw code and
	// the layout-collision test so the two never drift. Panels are sized to fit
	// saveRowsPerPage rows (the pager sits on a strip just below the in-game panel).
	saveMenuPanelW = 340
	saveMenuPanelH = 320
	// saveMenuListTopY centers the saveRowsPerPage-row block vertically between
	// the header text (ends ~py+48) and the panel bottom (py+saveMenuPanelH):
	// the highlight block is (rows-1)*pitch + 28 tall, so equal top/bottom gaps
	// put the first row at py+78. Recompute if panelH / row count / pitch change.
	saveMenuListTopY = 78
	saveMenuRowPitch = 32 // vertical pitch between rows
	entryLoadPanelW  = 460
	entryLoadPanelH  = 480
	entryLoadRowH    = 44

	// Main-menu panel + option-list layout (its own size, distinct from the
	// save/load panel). Shared by the draw code and the input hit-testing.
	mainMenuPanelW     = 280
	mainMenuPanelH     = 310
	mainMenuListTopY   = 56
	mainMenuRowPitch   = 32
	settingsMenuPanelW = 720
	settingsMenuPanelH = 510

	// menuRowHeight is the highlight/hitbox height of one vertical-menu row,
	// shared by Main-menu options and save/load slots (see menuRowRect).
	menuRowHeight = 28
)

// menuPanelSize returns the panel dimensions for a main-menu mode. Shared by the
// draw code (drawMainMenu) and the input hit-testing (handleMainMenuInput) so
// the drawn panel and its click regions can't drift.
func menuPanelSize(mode MainMenuMode, screenW, screenH int) (w, h int) {
	if mode == MenuMain {
		return mainMenuPanelW, mainMenuPanelH
	}
	if mode == MenuControlTips {
		return controlTipsPanelSize(screenW, screenH)
	}
	if mode == MenuSettings {
		return settingsMenuPanelW, settingsMenuPanelH
	}
	return saveMenuPanelW, saveMenuPanelH
}

// menuRowRect returns the highlight/click box and the text baseline for row i of
// a vertical menu list (Main-menu options, save/load slots). ONE geometry so the
// draw highlight, hover-select and click hit-tests never drift (cf.
// savePagerButtonRects). startY is the first row's baseline offset from py; pitch
// is the row spacing.
func menuRowRect(px, py, panelW, startY, pitch, i int) (box pagerRect, textX, textY int) {
	y := py + startY + i*pitch
	return pagerRect{px + 16, y - 4, px + panelW - 16, y - 4 + menuRowHeight}, px + 28, y
}

// saveRowIsLoadOnly reports whether a row is one the game writes itself (the
// Autosave or the Quicksave): the menus load it but never save, rename,
// archive or restore into it.
func saveRowIsLoadOnly(row int) bool { return row == autosaveRow || row == quicksaveRow }

// saveRowIsSlot reports whether row is a row of the save menus.
func saveRowIsSlot(row int) bool { return row >= 0 && row < saveRowCount }

// selectedSaveRow is the global save-row index the save/load menu cursor points
// at: the row-within-page (slotSelection) offset by the current page.
func (g *MMGame) selectedSaveRow() int {
	return g.savePage*saveRowsPerPage + g.slotSelection
}

// saveRowLabel is the slot's display name ("Autosave", "Quicksave" or "Slot N").
func saveRowLabel(row int) string {
	switch row {
	case autosaveRow:
		return "Autosave"
	case quicksaveRow:
		return "Quicksave"
	}
	return fmt.Sprintf("Slot %d", row-firstManualRow+1)
}

type mainMenuOption struct {
	key    string
	label  string
	action func(*MMGame)
}

// mainMenuOptions owns each ESC-menu label and its action. "Main Menu" returns
// to the title screen rather than quitting the application.
var mainMenuOptions = []mainMenuOption{
	{key: "continue", label: "Continue", action: func(g *MMGame) { g.closeMainMenu() }},
	{key: "save", label: "Save", action: func(g *MMGame) { g.openSaveLoad(MenuSaveSelect) }},
	{key: "load", label: "Load", action: func(g *MMGame) { g.openSaveLoad(MenuLoadSelect) }},
	{key: "scores", label: "High Scores", action: func(g *MMGame) { g.showHighScores = true }},
	{key: "settings", label: "Settings", action: func(g *MMGame) {
		g.mainMenuMode = MenuSettings
		g.beginAudioSettings()
	}},
	{key: "control_tips", label: uitext.Text("ui.control_tips"), action: func(g *MMGame) {
		g.mainMenuMode = MenuControlTips
		g.controlTipsScroll = 0
	}},
	{key: "main_menu", label: "Main Menu", action: func(g *MMGame) { g.returnToMainMenu() }},
}
