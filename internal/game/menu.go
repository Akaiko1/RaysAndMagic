package game

import (
	"errors"
	"fmt"
)

// ErrExit is returned from the game loop to request a clean exit
var ErrExit = errors.New("exit game")

// Save-slot menu layout. The menus show saveRowsPerPage rows across savePageCount
// pages. Global row 0 is the shared Autosave (written automatically on map change
// and stash use; load-only - never manually overwritten). Rows 1..N are manual
// slots and map to save1.json.. unchanged, so existing saves stay reachable.
const (
	saveRowsPerPage = 7
	savePageCount   = 3
	saveRowCount    = saveRowsPerPage * savePageCount
	autosaveFile    = "autosave.json"

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
	mainMenuPanelW     = 360
	mainMenuPanelH     = 380
	mainMenuListTopY   = 56
	mainMenuRowPitch   = 32
	settingsMenuPanelW = 480
	settingsMenuPanelH = 300

	// menuRowHeight is the highlight/hitbox height of one vertical-menu row,
	// shared by Main-menu options and save/load slots (see menuRowRect).
	menuRowHeight = 28
)

// menuPanelSize returns the panel dimensions for a main-menu mode. Shared by the
// draw code (drawMainMenu) and the input hit-testing (handleMainMenuInput) so
// the drawn panel and its click regions can't drift.
func menuPanelSize(mode MainMenuMode) (w, h int) {
	if mode == MenuMain {
		return mainMenuPanelW, mainMenuPanelH
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

// saveRowIsAutosave reports whether a row is the load-only autosave slot.
func saveRowIsAutosave(row int) bool { return row == 0 }

// selectedSaveRow is the global save-row index the save/load menu cursor points
// at: the row-within-page (slotSelection) offset by the current page.
func (g *MMGame) selectedSaveRow() int {
	return g.savePage*saveRowsPerPage + g.slotSelection
}

// saveRowLabel is the slot's display name ("Autosave" or "Slot N").
func saveRowLabel(row int) string {
	if row == 0 {
		return "Autosave"
	}
	return fmt.Sprintf("Slot %d", row)
}

type mainMenuOption struct {
	key    string
	label  string
	action func(*MMGame)
}

// mainMenuOptions owns each ESC-menu label and its action. "Main Menu" returns
// to the title screen rather than quitting the application.
var mainMenuOptions = []mainMenuOption{
	{key: "continue", label: "Continue", action: func(g *MMGame) { g.mainMenuOpen = false }},
	{key: "save", label: "Save", action: func(g *MMGame) { g.openSaveLoad(MenuSaveSelect) }},
	{key: "load", label: "Load", action: func(g *MMGame) { g.openSaveLoad(MenuLoadSelect) }},
	{key: "scores", label: "High Scores", action: func(g *MMGame) { g.showHighScores = true }},
	{key: "settings", label: "Settings", action: func(g *MMGame) {
		g.mainMenuMode = MenuSettings
		g.beginAudioSettings()
	}},
	{key: "main_menu", label: "Main Menu", action: func(g *MMGame) { g.returnToMainMenu() }},
}

var mainMenuControlTips = []string{
	"Controls:",
	"WASD: Move  QE: Strafe",
	"Space: Smart Attack  R: Weapon  F: Cast  C: Heal",
	"I: Inventory  P: Characters  M: Spellbook",
	"1-4: Select",
	"Tab: Toggle Mode (TB/RT)",
}

func mainMenuTipsTopY() int {
	return mainMenuListTopY + len(mainMenuOptions)*mainMenuRowPitch + 10
}
