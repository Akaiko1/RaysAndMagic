package game

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/highscore"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
	"unicode/utf8"

	"ugataima/internal/game/keytracker"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputHandler handles all user input for the game
type InputHandler struct {
	game                 *MMGame
	slashKeyTracker      keytracker.KeyStateTracker
	apostropheKeyTracker keytracker.KeyStateTracker
	enterKeyTracker      keytracker.KeyStateTracker
	tabKeyTracker        keytracker.KeyStateTracker
	escapeKeyTracker     keytracker.KeyStateTracker
	upKeyTracker         keytracker.KeyStateTracker
	downKeyTracker       keytracker.KeyStateTracker
	leftKeyTracker       keytracker.KeyStateTracker
	rightKeyTracker      keytracker.KeyStateTracker
	wKeyTracker          keytracker.KeyStateTracker
	aKeyTracker          keytracker.KeyStateTracker
	sKeyTracker          keytracker.KeyStateTracker
	dKeyTracker          keytracker.KeyStateTracker
	qKeyTracker          keytracker.KeyStateTracker
	eKeyTracker          keytracker.KeyStateTracker
	spaceKeyTracker      keytracker.KeyStateTracker
	fKeyTracker          keytracker.KeyStateTracker
	hKeyTracker          keytracker.KeyStateTracker
	rKeyTracker          keytracker.KeyStateTracker
	cKeyTracker          keytracker.KeyStateTracker
	attackHoldFrames     int // frames an RT attack key has been held (tap vs hold-repeat)
	menuKeyTracker       keytracker.KeyStateTracker
	inventoryKeyTracker  keytracker.KeyStateTracker
	charactersKeyTracker keytracker.KeyStateTracker
	questsKeyTracker     keytracker.KeyStateTracker
	cardsKeyTracker      keytracker.KeyStateTracker
	interactKeyTracker   keytracker.KeyStateTracker
	newGameKeyTracker    keytracker.KeyStateTracker
	loadKeyTracker       keytracker.KeyStateTracker
}

// NewInputHandler creates a new input handler
func NewInputHandler(game *MMGame) *InputHandler {
	return &InputHandler{game: game}
}

// inputDebounceCooldown is a minimal cooldown to prevent key repeat issues
const inputDebounceCooldown = 10

// rtActionStagger is the short global gap (frames) between any two real-time
// combat actions. The big gate is each character's own RTCooldown; this small
// stagger only spaces out a held-key volley so the party visibly fires in turn
// (~0.07s apart at 120 TPS) instead of several members acting on one frame.
const rtActionStagger = 8

// rtHoldRepeatDelay is how long (frames) an attack key must be HELD before it
// starts auto-repeating (cycling the party). Set well above a normal tap (even a
// slow ~0.3s one) so single presses fire exactly once — only a deliberate hold
// cycles. ~0.45s at 120 TPS.
const rtHoldRepeatDelay = 54

// actionCooldown returns the number of frames to wait before the next action.
// In turn-based mode, returns a minimal debounce value since actions are limited by turns.
// In real-time mode, uses Speed-based scaling: Speed 5 => ~60 frames, Speed 50 => ~30 frames.
func (ih *InputHandler) actionCooldown(_ int) int {
	if ih == nil || ih.game == nil || ih.game.combat == nil {
		return inputDebounceCooldown
	}
	selected := ih.game.party.Members[ih.game.selectedChar]
	return ih.game.combat.CalculateActionCooldownFrames(selected)
}

// HandleInput processes all input for the current frame
func (ih *InputHandler) HandleInput() {
	// Game over: the on-screen buttons (New Game / Load / Main Menu / Quit) are
	// handled in drawGameOverOverlay via consumeLeftClickIn; these keys mirror them.
	if ih.game.gameOver {
		if ih.newGameKeyTracker.IsKeyJustPressed(ebiten.KeyN) {
			ih.restartNewGame()
			return
		}
		if ih.loadKeyTracker.IsKeyJustPressed(ebiten.KeyL) {
			ih.game.returnToMainMenu()
			ih.game.entryMenuMode = EntryMenuLoad
			ih.game.slotSelection = 0
			ih.game.savePage = 0 // open Load on page 1, matching every other Load entry
			return
		}
		return
	}

	// Handle victory screen input
	if ih.game.gameVictory {
		ih.handleVictoryInput()
		return
	}

	// Handle high scores overlay
	if ih.game.showHighScores {
		if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
			ih.game.showHighScores = false
		}
		return
	}

	if ih.game.combatLogOpen {
		ih.handleCombatLogInput()
		return
	}

	// Handle level-up choice overlay
	if ih.game.currentLevelUpChoice() != nil {
		ih.handleLevelUpChoiceInput()
		return
	}

	// Revival potion target picker: clicks are consumed inside the popup's
	// own Draw call (it lives in ui_dialogs.go). Just suppress gameplay input
	// so the player can't move/attack/cast while choosing a revive target.
	if ih.game.revivalPickerOpen || ih.game.healPickerOpen {
		return
	}

	// Promotion picker: same deal — clicks handled inside its Draw; just suppress
	// gameplay input while the player chooses who to promote.
	if ih.game.promotionPickerOpen {
		return
	}

	// Tavern roster screen: clicks handled inside its Draw; suppress gameplay.
	// ESC closes the screen here (edge-tracked + consumed) so it can't leak to the
	// menu-open handler below on the next frame.
	if ih.game.rosterScreenOpen {
		if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
			ih.game.rosterScreenOpen = false
			ih.game.rosterSelectedActive = -1
		}
		return
	}

	// Tavern stash screen: drag + clicks handled inside its Draw; suppress gameplay.
	if ih.game.stashScreenOpen {
		if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
			ih.game.stashScreenOpen = false
			ih.game.clearStashDrag()
		}
		return
	}

	// Close map overlay with ESC before other UI handling
	if ih.game.mapOverlayOpen && ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
		ih.game.mapOverlayOpen = false
		return
	}
	if ih.game.mapOverlayOpen {
		return
	}
	// ESC handling: close current overlay before opening menu
	if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
		// If main menu is open, back out of submenus or close it
		if ih.game.mainMenuOpen {
			if ih.game.mainMenuMode != MenuMain {
				ih.game.mainMenuMode = MenuMain
			} else {
				ih.game.mainMenuOpen = false
			}
			return
		}
		// Close stat popup if open
		if ih.game.statPopupOpen {
			ih.game.statPopupOpen = false
			return
		}
		// Close dialog if open. The skill-trainer mastery popup is a sub-
		// modal on top of the dialog, so ESC peels it off first instead
		// of closing the whole trader.
		if ih.game.dialogActive {
			if ih.game.skillTrainerPopup {
				ih.game.skillTrainerPopup = false
			} else {
				ih.game.dialogActive = false
				ih.game.dialogNPC = nil
				ih.game.skillTrainerPopup = false
				ih.game.dialogTab = 0 // never leak the Quests tab into the next dialog
			}
			return
		}
		// Close tabbed menu if open
		if ih.game.menuOpen {
			ih.game.menuOpen = false
			return
		}
		// Otherwise open main menu
		ih.game.mainMenuOpen = true
		ih.game.mainMenuSelection = 0
		ih.game.slotSelection = 0
		ih.game.mainMenuMode = MenuMain
		return
	}

	// When main menu is open, handle only its input
	if ih.game.mainMenuOpen {
		ih.handleMainMenuInput()
		return
	}
	// Handle dialog UI (blocks movement when open)
	if ih.game.dialogActive {
		ih.handleDialogInput()
		return
	}

	// Handle tabbed menu UI (blocks movement when open, but allows UI input)
	if ih.game.menuOpen {
		ih.handleTabbedMenuInput()
		ih.handleUIInput()    // Allow UI input to close the panel
		ih.handleMouseInput() // Allow party character clicking when menu is open
		return
	}

	if ih.handleCombatLogOpenInput() {
		return
	}

	// Handle normal gameplay input
	if ih.game.turnBasedMode {
		ih.handleTurnBasedInput()
	} else {
		ih.handleMovementInput()
		ih.handleCombatInput()
	}
	ih.handleCharacterSelectionInput()
	ih.handleUIInput()
	ih.handleMouseInput()
}

// restartNewGame resets to a fresh game with the default config roster (used by
// the game-over screen's New Game shortcut).
func (ih *InputHandler) restartNewGame() {
	ih.game.startNewGameWithParty(character.NewParty(ih.game.config))
}

// startNewGameWithParty resets all world/combat/UI state for a fresh game and
// drops the player into gameplay with the given party. Shared by restartNewGame
// (default roster) and the party-creation screen (player-picked roster).
func (g *MMGame) startNewGameWithParty(party *character.Party) {
	g.party = party
	g.selectedChar = 0
	g.parkSelection = false

	// Reset victory/high score state and session timer
	g.gameOver = false
	g.gameVictory = false
	g.victoryScoreSaved = false
	g.victoryNameInput = ""
	g.victoryTime = time.Time{}
	g.sessionStartTime = time.Now()
	g.showHighScores = false
	g.frameCount = 0

	// Clear combat/projectile state (shared cleaner: also unregisters
	// projectile collision entities and drops impact lights)
	g.clearTransientCombatState()
	g.groundContainers = g.groundContainers[:0]
	g.traps = nil
	g.combatLogHistory = g.combatLogHistory[:0]
	g.combatLogOpen = false
	g.combatLogScroll = 0
	g.cardFxTimers = [cardFxCount][4]int{}

	// Reset spell/UI selections and cooldowns
	g.selectedSchool = 0
	g.selectedSpell = 0
	g.spellInputCooldown = 0
	g.collapsedSpellSchools = make(map[character.MagicSchoolID]bool)
	g.utilitySpellStatuses = make(map[spells.SpellID]*UtilitySpellStatus)
	g.lastSpellClickTime = 0
	g.lastClickedSpell = -1
	g.lastClickedSchool = -1
	g.lastSchoolClickTime = 0
	g.lastSchoolClickedIdx = -1
	g.dialogLastClickTime = 0
	g.dialogLastClickedIdx = -1

	// Reset dialog/menu states
	g.dialogActive = false
	g.dialogNPC = nil
	g.dialogSelectedChar = 0
	g.dialogSelectedSpell = 0
	g.selectedCharIdx = 0
	g.selectedSpellKey = ""
	g.selectedChoice = 0
	g.menuOpen = false
	g.mapOverlayOpen = false
	g.currentTab = TabInventory
	g.mainMenuOpen = false
	g.mainMenuMode = MenuMain
	g.mainMenuSelection = 0
	g.slotSelection = 0
	g.saveRenameOpen = false
	g.saveRenameSlot = -1
	g.saveRenameInput = ""
	g.exitRequested = false

	// Reset every timed effect family (buffs, zones, utility flags)
	g.resetTimedEffects()

	// Fresh party must not inherit the old run's card effects.
	g.resetCardCollection()

	// Reset turn-based state
	g.turnBasedMode = false
	g.currentTurn = 0
	g.partyActionsUsed = 0
	g.turnBasedMoveCooldown = 0
	g.turnBasedRotCooldown = 0
	g.monsterTurnResolved = false
	g.turnBasedSpRegenCount = 0

	// Clear pending level-up choices
	g.levelUpChoiceQueue = nil
	g.levelUpChoiceOpen = false
	g.levelUpChoiceIdx = 0

	// Reset quest progress so victory doesn't immediately re-trigger.
	if quests.GlobalQuestManager != nil {
		quests.GlobalQuestManager.Reset()
	}
	g.questManager = quests.GlobalQuestManager

	// Reset maps to a fresh state with monsters and NPCs.
	if wm := world.GlobalWorldManager; wm != nil {
		if err := wm.Reset(); err != nil {
			fmt.Printf("Warning: Failed to reset maps during restart: %v\n", err)
		}
		g.world = wm.GetCurrentWorld()
	}

	// Move player to start position (fallback to nearest walkable tile if map has no '+')
	if currentWorld := g.GetCurrentWorld(); currentWorld != nil {
		tileSize := float64(g.config.GetTileSize())
		startX, startY := 0.0, 0.0
		if currentWorld.StartX >= 0 && currentWorld.StartY >= 0 {
			startX = float64(currentWorld.StartX) * tileSize
			startY = float64(currentWorld.StartY) * tileSize
		} else {
			centerX := float64(currentWorld.Width) * tileSize * 0.5
			centerY := float64(currentWorld.Height) * tileSize * 0.5
			startX, startY = g.FindNearestWalkableTileMustSucceed(centerX, centerY)
		}
		g.camera.X = startX
		g.camera.Y = startY
		g.camera.Angle = 0
		g.camera.FOV = g.config.GetCameraFOV()
		g.camera.ViewDist = g.config.GetViewDistance()

		// Rebuild collision system and register entities
		g.collisionSystem = collision.NewCollisionSystem(currentWorld, float64(g.config.World.TileSize))
		playerEntity := collision.NewEntity("player", startX, startY, 16, 16, collision.CollisionTypePlayer, false)
		g.collisionSystem.RegisterEntity(playerEntity)
		currentWorld.RegisterMonstersWithCollisionSystem(g.collisionSystem)

		g.UpdateSkyAndGroundColors()
		if g.collisionSystem != nil {
			g.collisionSystem.UpdateTileChecker(currentWorld)
		}
		if g.gameLoop != nil && g.gameLoop.renderer != nil {
			g.gameLoop.renderer.precomputeFloorColorCache()
			g.gameLoop.renderer.buildTransparentSpriteCache()
		}
	}

	// Hand control to the gameplay loop.
	g.appScreen = AppScreenInGame
}

// handleVictoryInput processes input on the victory screen
func (ih *InputHandler) handleVictoryInput() {
	// If high scores overlay is open during victory, allow ESC to close it first.
	if ih.game.showHighScores {
		if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
			ih.game.showHighScores = false
		}
		return
	}

	// If score already saved, handle post-save options
	if ih.game.victoryScoreSaved {
		// H to view high scores
		if ih.hKeyTracker.IsKeyJustPressed(ebiten.KeyH) {
			ih.game.showHighScores = true
		}
		// ESC to return to main menu
		if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
			ih.restartNewGame()
			ih.game.gameVictory = false
			ih.game.victoryScoreSaved = false
			ih.game.victoryNameInput = ""
		}
		return
	}

	// Handle text input for name
	ih.handleVictoryNameInput()

	// Enter to save score
	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		ih.saveVictoryScore()
	}

	// ESC to skip saving and return to main menu
	if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
		ih.restartNewGame()
		ih.game.gameVictory = false
		ih.game.victoryNameInput = ""
	}
}

// handleVictoryNameInput handles text input for the player name
func (ih *InputHandler) handleVictoryNameInput() {
	// Get input characters
	inputChars := ebiten.AppendInputChars(nil)
	for _, char := range inputChars {
		if char == '\n' || char == '\r' || char == '\t' {
			continue // same control-char guard as handleSaveRenameInput
		}
		if len(ih.game.victoryNameInput) < 20 {
			ih.game.victoryNameInput += string(char)
		}
	}

	// Handle backspace
	if repeatingKeyPressed(ebiten.KeyBackspace) {
		if len(ih.game.victoryNameInput) > 0 {
			ih.game.victoryNameInput = ih.game.victoryNameInput[:len(ih.game.victoryNameInput)-1]
		}
	}
}

// saveVictoryScore saves the current score to high scores
func (ih *InputHandler) saveVictoryScore() {
	name := ih.game.victoryNameInput
	if name == "" {
		name = "Anonymous"
	}

	scoreData := ih.game.GetScoreData()
	finalScore := highscore.Calculate(scoreData)
	playTimeStr := highscore.FormatPlayTime(scoreData.PlayTime)

	entry := highscore.Entry{
		PlayerName: name,
		Score:      finalScore,
		Gold:       scoreData.Gold,
		Experience: scoreData.TotalExperience,
		AvgLevel:   scoreData.AverageLevel,
		PlayTime:   playTimeStr,
		Date:       ih.game.victoryTime,
	}

	scores, _ := highscore.Load()
	highscore.Add(scores, entry)
	_ = highscore.Save(scores)

	ih.game.victoryScoreSaved = true
}

// repeatingKeyPressed returns true if key is pressed with repeat handling
func repeatingKeyPressed(key ebiten.Key) bool {
	const (
		delay    = 30
		interval = 3
	)
	d := inpututil.KeyPressDuration(key)
	if d == 1 {
		return true
	}
	if d >= delay && (d-delay)%interval == 0 {
		return true
	}
	return false
}

// activateMainMenuSelection runs the action for the highlighted MenuMain option,
// shared by Enter-key and mouse-click activation.
func (ih *InputHandler) activateMainMenuSelection() {
	switch ih.game.mainMenuSelection {
	case 0: // Continue
		ih.game.mainMenuOpen = false
	case 1: // Save
		ih.game.mainMenuMode = MenuSaveSelect
		ih.game.slotSelection = 0
		ih.game.savePage = 0
	case 2: // Load
		ih.game.mainMenuMode = MenuLoadSelect
		ih.game.slotSelection = 0
		ih.game.savePage = 0
	case 3: // High Scores
		ih.game.showHighScores = true
	case 4: // Main Menu (return to title, not quit the app)
		ih.game.returnToMainMenu()
	}
}

// handleMainMenuInput processes input for the main menu (opened with ESC)
func (ih *InputHandler) handleMainMenuInput() {
	// Mouse position for hover/click
	mouseX, mouseY := ebiten.CursorPosition()
	panelW, panelH := saveMenuPanelW, saveMenuPanelH
	w := ih.game.config.GetScreenWidth()
	h := ih.game.config.GetScreenHeight()

	switch ih.game.mainMenuMode {
	case MenuMain:
		panelW, panelH = 360, 320
		px := (w - panelW) / 2
		py := (h - panelH) / 2
		// Navigate options (debounced)
		if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) {
			if ih.game.mainMenuSelection > 0 {
				ih.game.mainMenuSelection--
			}
		}
		if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) {
			if ih.game.mainMenuSelection < len(mainMenuOptions)-1 {
				ih.game.mainMenuSelection++
			}
		}

		// Mouse hover selection
		ih.mainMenuHoverSelect(mouseX, mouseY, len(mainMenuOptions), panelW, panelH, 56)

		// Activate selection with Enter or a mouse click on the panel
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			ih.activateMainMenuSelection()
		}
		if ih.game.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			ih.activateMainMenuSelection()
		}
	case MenuSaveSelect:
		px := (w - panelW) / 2
		py := (h - panelH) / 2
		if ih.game.saveRenameOpen {
			ih.handleSaveRenameInput()
			return
		}
		ih.navigateSavePage(px, py, panelW, panelH)
		for i := 0; i < saveRowsPerPage; i++ {
			row := ih.game.savePage*saveRowsPerPage + i
			y := py + saveMenuListTopY + i*saveMenuRowPitch
			if ih.game.consumeRightClickIn(px+16, y-4, px+panelW-16, y+24) {
				if saveRowIsAutosave(row) {
					ih.game.AddCombatMessage("The Autosave slot cannot be renamed")
				} else if sum := GetSaveRowSummary(row); !sum.Exists {
					ih.game.AddCombatMessage("No save in slot to rename")
				} else {
					ih.game.slotSelection = i
					ih.game.saveRenameOpen = true
					ih.game.saveRenameSlot = row
					ih.game.saveRenameInput = sum.Name
				}
				return
			}
		}
		// Mouse hover selection (row within page).
		ih.mainMenuHoverSelect(mouseX, mouseY, saveRowsPerPage, panelW, panelH, saveMenuListTopY)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			ih.doSaveToSelectedRow()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			row := ih.game.selectedSaveRow()
			if saveRowIsAutosave(row) {
				ih.game.AddCombatMessage("The Autosave slot cannot be renamed")
			} else if sum := GetSaveRowSummary(row); !sum.Exists {
				ih.game.AddCombatMessage("No save in slot to rename")
			} else {
				ih.game.saveRenameOpen = true
				ih.game.saveRenameSlot = row
				ih.game.saveRenameInput = sum.Name
			}
		}
		// Mouse click activation
		if ih.game.consumeLeftClickIn(px, py+saveMenuListTopY-6, px+panelW, py+saveMenuListTopY-6+saveRowsPerPage*saveMenuRowPitch) {
			ih.doSaveToSelectedRow()
		}
	case MenuLoadSelect:
		px := (w - panelW) / 2
		py := (h - panelH) / 2
		ih.navigateSavePage(px, py, panelW, panelH)
		ih.mainMenuHoverSelect(mouseX, mouseY, saveRowsPerPage, panelW, panelH, saveMenuListTopY)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			ih.doLoadFromSelectedRow()
		}
		if ih.game.consumeLeftClickIn(px, py+saveMenuListTopY-6, px+panelW, py+saveMenuListTopY-6+saveRowsPerPage*saveMenuRowPitch) {
			ih.doLoadFromSelectedRow()
		}
	}
}

// navigateSavePage handles nav in the save/load menus: Up/Down moves the cursor
// within the current page, Left/Right (keys) or the on-screen Prev/Next buttons
// (a strip below the panel) flip between pages. The button rects are shared with
// the draw side via savePagerButtonRects.
func (ih *InputHandler) navigateSavePage(px, py, panelW, panelH int) {
	g := ih.game
	if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) && g.slotSelection > 0 {
		g.slotSelection--
	}
	if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) && g.slotSelection < saveRowsPerPage-1 {
		g.slotSelection++
	}
	pl, pr := savePagerButtonRects(px, py, panelW, panelH)
	prev := inpututil.IsKeyJustPressed(ebiten.KeyLeft) ||
		(g.savePage > 0 && g.consumeLeftClickIn(pl.x1, pl.y1, pl.x2, pl.y2))
	next := inpututil.IsKeyJustPressed(ebiten.KeyRight) ||
		(g.savePage < savePageCount-1 && g.consumeLeftClickIn(pr.x1, pr.y1, pr.x2, pr.y2))
	if prev && g.savePage > 0 {
		g.savePage--
	}
	if next && g.savePage < savePageCount-1 {
		g.savePage++
	}
}

// doSaveToSelectedRow writes the manual slot under the cursor. The Autosave slot
// is load-only and refuses a manual write.
func (ih *InputHandler) doSaveToSelectedRow() {
	g := ih.game
	row := g.selectedSaveRow()
	if saveRowIsAutosave(row) {
		g.AddCombatMessage("Autosave is written automatically - pick another slot")
		return
	}
	if err := g.SaveGameToFile(saveRowPath(row)); err != nil {
		g.AddCombatMessage("Save failed")
	} else {
		g.AddCombatMessage("Saved to " + saveRowLabel(row))
		g.mainMenuMode = MenuMain
	}
}

// doLoadFromSelectedRow loads the slot under the cursor (Autosave included).
func (ih *InputHandler) doLoadFromSelectedRow() {
	g := ih.game
	row := g.selectedSaveRow()
	if sum := GetSaveRowSummary(row); !sum.Exists {
		g.AddCombatMessage("No save in that slot")
		return
	}
	if err := g.LoadGameFromFile(saveRowPath(row)); err != nil {
		g.AddCombatMessage("Load failed")
	} else {
		g.AddCombatMessage("Loaded " + saveRowLabel(row))
		g.mainMenuOpen = false
		g.mainMenuMode = MenuMain
	}
}

func (ih *InputHandler) handleSaveRenameInput() {
	inputChars := ebiten.AppendInputChars(nil)
	for _, char := range inputChars {
		if char == '\n' || char == '\r' || char == '\t' {
			continue
		}
		if len([]rune(ih.game.saveRenameInput)) < 24 {
			ih.game.saveRenameInput += string(char)
		}
	}
	if repeatingKeyPressed(ebiten.KeyBackspace) {
		if len(ih.game.saveRenameInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(ih.game.saveRenameInput)
			if size > 0 && size <= len(ih.game.saveRenameInput) {
				ih.game.saveRenameInput = ih.game.saveRenameInput[:len(ih.game.saveRenameInput)-size]
			}
		}
	}
	if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
		name := strings.TrimSpace(ih.game.saveRenameInput)
		if err := RenameSaveSlot(ih.game.saveRenameSlot, name); err != nil {
			ih.game.AddCombatMessage("Rename failed")
		} else {
			ih.game.AddCombatMessage("Save renamed")
		}
		ih.game.saveRenameOpen = false
		ih.game.saveRenameSlot = -1
		ih.game.saveRenameInput = ""
	}
	if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
		ih.game.saveRenameOpen = false
		ih.game.saveRenameSlot = -1
		ih.game.saveRenameInput = ""
	}
}

// mainMenuHoverSelect updates selection based on mouse hover over the menu panel
// panelW/H must match ui.drawMainMenu sizes (300x220), startY is the first option baseline.
func (ih *InputHandler) mainMenuHoverSelect(mouseX, mouseY, count, panelW, panelH, startY int) {
	w := ih.game.config.GetScreenWidth()
	h := ih.game.config.GetScreenHeight()
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	// Option rectangles: x in [px+16, px+panelW-16], y baseline at startY+i*32; highlight box spans y-4..y-4+28
	for i := 0; i < count; i++ {
		y := py + startY + i*32
		x1 := px + 16
		x2 := px + panelW - 16
		y1 := y - 4
		y2 := y - 4 + 28
		if mouseX >= x1 && mouseX < x2 && mouseY >= y1 && mouseY < y2 {
			if ih.game.mainMenuMode == MenuMain {
				ih.game.mainMenuSelection = i
			} else {
				ih.game.slotSelection = i
			}
		}
	}
}

// handleLevelUpChoiceInput processes input for the level-up choice popup.
func (ih *InputHandler) handleLevelUpChoiceInput() {
	req := ih.game.currentLevelUpChoice()
	if req == nil {
		return
	}
	if ih.escapeKeyTracker.IsKeyJustPressed(ebiten.KeyEscape) {
		ih.game.closeLevelUpChoice()
		return
	}
	optionCount := len(req.options)
	if optionCount == 0 {
		return
	}

	// Cursor range: single-select spans the options; multi-select adds a trailing
	// Confirm row at index == optionCount.
	maxSel := optionCount - 1
	if req.isMultiSelect() {
		maxSel = req.confirmRowIndex()
	}

	// Keyboard navigation
	if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) && req.selection > 0 {
		req.selection--
	}
	if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) && req.selection < maxSel {
		req.selection++
	}

	// Mouse hover selection (rows + confirm row for multi-select)
	mouseX, mouseY := ebiten.CursorPosition()
	screenW := ih.game.config.GetScreenWidth()
	screenH := ih.game.config.GetScreenHeight()
	popupX, _, popupW, _, startY, rowH := levelUpChoiceLayout(req, screenW, screenH)

	for i := 0; i <= maxSel; i++ {
		y := startY + i*rowH
		if mouseX >= popupX+16 && mouseX < popupX+popupW-16 && mouseY >= y-2 && mouseY < y-2+rowH {
			req.selection = i
			break
		}
	}

	if req.isMultiSelect() {
		ih.handleMultiSelectInput(req, popupX, popupW, startY, rowH)
		return
	}

	// Single-select: Enter or click applies immediately.
	if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
		ih.game.consumeLevelUpChoice(req.selection)
		return
	}
	for i := 0; i < optionCount; i++ {
		y := startY + i*rowH
		if ih.game.consumeLeftClickIn(popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			req.selection = i
			ih.game.consumeLevelUpChoice(req.selection)
			return
		}
	}
}

// handleMultiSelectInput drives the "pick K of N" picker: Space/Enter on an
// option toggles it, Enter/click on the Confirm row applies all picks.
func (ih *InputHandler) handleMultiSelectInput(req *levelUpChoiceRequest, popupX, popupW, startY, rowH int) {
	optionCount := len(req.options)
	confirmIdx := req.confirmRowIndex()

	if ih.spaceKeyTracker.IsKeyJustPressed(ebiten.KeySpace) && req.selection < optionCount {
		ih.game.toggleLevelUpSelection(req.selection)
	}
	if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
		if req.selection == confirmIdx {
			ih.game.confirmLevelUpSelections()
		} else {
			ih.game.toggleLevelUpSelection(req.selection)
		}
		return
	}
	// Clicks: option rows toggle, the Confirm row confirms.
	for i := 0; i < optionCount; i++ {
		y := startY + i*rowH
		if ih.game.consumeLeftClickIn(popupX+16, y-2, popupX+popupW-16, y-2+rowH) {
			req.selection = i
			ih.game.toggleLevelUpSelection(i)
			return
		}
	}
	cy := startY + optionCount*rowH
	if ih.game.consumeLeftClickIn(popupX+16, cy-2, popupX+popupW-16, cy-2+rowH) {
		req.selection = confirmIdx
		ih.game.confirmLevelUpSelections()
	}
}

// handleMovementInput processes movement and camera controls
func (ih *InputHandler) handleMovementInput() {
	moveScale := ih.movementScale()
	// Rotation
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		ih.game.camera.Angle -= ih.game.config.GetRotSpeed() * moveScale
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		ih.game.camera.Angle += ih.game.config.GetRotSpeed() * moveScale
	}

	// Forward/backward movement
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		ih.moveForward()
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		ih.moveBackward()
	}

	// Strafe left/right
	if ebiten.IsKeyPressed(ebiten.KeyQ) {
		ih.strafeLeft()
	}
	if ebiten.IsKeyPressed(ebiten.KeyE) {
		ih.strafeRight()
	}
}

// handleCombatInput processes real-time combat input. Keys (all modes):
//
//	R     — melee/ranged WEAPON attack
//	Space — smart attack: cast the slotted spell if it's offensive, else weapon
//	F     — cast the slotted spell (heal spells target via mouse)
//	C     — cast the strongest known heal from the spellbook
//
// Every action gates on the SELECTED character being off their own cooldown
// (RTCooldown) plus a short global stagger, then advances selection to the next
// ready member — so holding a key fires the party in turn, not one machine-gun.
func (ih *InputHandler) handleCombatInput() {
	// Edge state for every action key — read EVERY frame so tracker state stays
	// synced (must not be short-circuited by an early return below).
	rJust := ih.rKeyTracker.IsKeyJustPressed(ebiten.KeyR)
	spaceJust := ih.spaceKeyTracker.IsKeyJustPressed(ebiten.KeySpace)
	fJust := ih.fKeyTracker.IsKeyJustPressed(ebiten.KeyF)
	cJust := ih.cKeyTracker.IsKeyJustPressed(ebiten.KeyC)
	hJust := ih.hKeyTracker.IsKeyJustPressed(ebiten.KeyH)
	rHeld := ebiten.IsKeyPressed(ebiten.KeyR)
	spaceHeld := ebiten.IsKeyPressed(ebiten.KeySpace)
	fHeld := ebiten.IsKeyPressed(ebiten.KeyF)
	cHeld := ebiten.IsKeyPressed(ebiten.KeyC)
	hHeld := ebiten.IsKeyPressed(ebiten.KeyH)

	// No attacks/casts/shots while running — you must stop sprinting to act.
	running := ih.isRunning()

	// Space also picks up ground loot — works while sprinting, cooldown-independent.
	if spaceHeld && ih.game.spellInputCooldown == 0 {
		if ih.game.tryPickupNearestGroundContainer(ih.game.groundContainerPickupRange()) {
			return
		}
	}
	if running {
		ih.attackHoldFrames = 0
		return
	}

	// Tap vs hold: a fresh press (Just) fires ONCE; a sustained hold only
	// auto-repeats after rtHoldRepeatDelay, so a single tap can't fire twice.
	anyHeld := rHeld || spaceHeld || fHeld || cHeld || hHeld
	if anyHeld {
		ih.attackHoldFrames++
	} else {
		ih.attackHoldFrames = 0
	}
	repeat := ih.attackHoldFrames >= rtHoldRepeatDelay

	kind := rtActNone
	switch {
	case rJust:
		kind = rtActWeapon
	case spaceJust:
		kind = rtActSmart
	case fJust:
		kind = rtActCast
	case cJust || hJust:
		kind = rtActHeal
	case repeat && rHeld:
		kind = rtActWeapon
	case repeat && spaceHeld:
		kind = rtActSmart
	case repeat && fHeld:
		kind = rtActCast
	case repeat && (cHeld || hHeld):
		kind = rtActHeal
	}
	if kind == rtActNone {
		return
	}

	// Off a corpse first, then onto a member who can actually do THIS action:
	// holding F only visits casters, C only healers, R only the armed.
	ih.game.ensureSelectedCanActRT()
	// (1) If the selected member can't do this action AT ALL, park on a capable
	// one (preferring a ready one) — a single move so the waiting frame sits on a
	// real actor, not a per-frame churn.
	if !ih.game.rtActionCapable(ih.game.selectedChar, kind) {
		// Explicit F with nothing castable: say WHY once per fresh press (the
		// TB path announces through the cast itself; holds stay silent so a
		// held key can't spam). Space keeps its silent weapon fallback.
		if kind == rtActCast && fJust {
			ih.announceCastShortfall(ih.game.selectedChar)
		}
		ih.game.advanceRTActor(kind)
	}
	// (2) Selected is capable. If it's on cooldown, jump to another member who is
	// ready RIGHT NOW; if nobody is ready, hold still and wait quietly — do NOT
	// shuffle the selection among on-cooldown members (that was the jitter).
	if !ih.game.rtActionReady(ih.game.selectedChar, kind) {
		if i := ih.game.nextReadyRTActor(kind); i >= 0 {
			ih.game.selectedChar = i
		}
	}

	// Gate: short global stagger AND the selected member ready+capable. A capable
	// member on cooldown lands here and simply waits (no fire, no chat spam).
	if ih.game.spellInputCooldown != 0 || !ih.game.rtActionReady(ih.game.selectedChar, kind) {
		return
	}
	sel := ih.game.party.Members[ih.game.selectedChar]

	switch kind {
	case rtActWeapon:
		if ih.game.combat.EquipmentMeleeAttack() {
			ih.commitRTAction(rtActWeapon, ih.game.combat.WeaponCooldownFrames(sel))
		} else {
			// No weapon after all (capability raced an unequip): pass the turn
			// on without a cooldown so a held key doesn't stick here.
			ih.game.advanceRTActor(rtActWeapon)
		}
	case rtActSmart:
		acted, spellID := ih.game.combat.SmartAttack()
		switch {
		case spellID != "":
			ih.commitRTAction(rtActSmart, ih.game.combat.SpellCooldownFrames(sel, spellID))
		case acted:
			ih.commitRTAction(rtActSmart, ih.game.combat.WeaponCooldownFrames(sel))
		default:
			// Nothing this hero could do (no weapon, no castable spell, no one
			// to heal): hand the selection on instead of looping on them.
			ih.game.advanceRTActor(rtActSmart)
		}
	case rtActCast:
		ih.castSlottedSpell(sel)
	case rtActHeal:
		ih.castBestHeal(sel)
	}
}

// commitRTAction puts the just-acted character on cooldown, applies the short
// global stagger, and hands selection to the next member who can do the SAME
// action (so a held key keeps cycling among capable members of that action).
func (ih *InputHandler) commitRTAction(kind rtActionKind, cooldownFrames int) {
	sel := ih.game.party.Members[ih.game.selectedChar]
	sel.RTCooldown = cooldownFrames
	ih.game.spellInputCooldown = rtActionStagger
	ih.game.advanceRTActor(kind)
}

// announceCastShortfall explains a refused explicit cast (F) for the member
// the player had selected: not enough SP for the slotted spell/trap. Uses the
// LIVE trap cost (saved items may carry stale SpellCost).
func (ih *InputHandler) announceCastShortfall(idx int) {
	if idx < 0 || idx >= len(ih.game.party.Members) {
		return
	}
	m := ih.game.party.Members[idx]
	if m == nil || !m.CanAct() {
		return
	}
	slot, ok := m.Equipment[items.SlotSpell]
	if !ok {
		return
	}
	cost := slot.SpellCost
	if slot.Type == items.ItemTrap {
		if def, defOk := config.GetTrapDefinition(string(slot.SpellEffect)); defOk {
			cost = def.SPCost
		}
	}
	cost = ih.game.combat.effectiveSpellCost(m, cost)
	if m.SpellPoints < cost {
		ih.game.AddCombatMessage(fmt.Sprintf("%s's %s fizzles! (Not enough SP: %d/%d)",
			m.Name, slot.Name, m.SpellPoints, cost))
	}
}

// castSlottedSpellResolved performs the slotted-spell cast the F key triggers in
// both modes: a heal spell aims at the mouse-picked party member, anything else
// casts directly. Reports whether it fired and the spell's ID for cooldown lookup.
// The caller commits the result per mode (RT cooldown vs TB action slot).
func (ih *InputHandler) castSlottedSpellResolved(sel *character.MMCharacter) (bool, spells.SpellID) {
	spell, hasSpell := sel.Equipment[items.SlotSpell]
	if !hasSpell {
		return false, ""
	}
	spellID := spells.SpellID(spell.SpellEffect)
	if spell.SpellEffect == items.SpellEffectHealSelf || spell.SpellEffect == items.SpellEffectHealOther {
		mouseX, mouseY := ebiten.CursorPosition()
		targetCharIndex := ih.resolveHealTarget(spell, mouseX, mouseY)
		return ih.game.combat.CastEquippedHealOnTarget(targetCharIndex), spellID
	}
	return ih.game.combat.CastEquippedSpell(), spellID
}

// castSlottedSpell casts the selected character's slotted spell (F key) and, in
// real-time mode, applies the cooldown only if the cast actually fired.
func (ih *InputHandler) castSlottedSpell(sel *character.MMCharacter) {
	if fired, spellID := ih.castSlottedSpellResolved(sel); fired {
		ih.commitRTAction(rtActCast, ih.game.combat.SpellCooldownFrames(sel, spellID))
	}
}

// castBestHealResolved performs the best-heal cast the C/H key triggers in both
// modes: aim at the party member under the mouse, falling back to the selected
// character. Reports whether it fired and the heal spell's ID for cooldown lookup.
func (ih *InputHandler) castBestHealResolved() (bool, spells.SpellID) {
	mouseX, mouseY := ebiten.CursorPosition()
	targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)
	if targetCharIndex < 0 {
		targetCharIndex = ih.game.selectedChar
	}
	return ih.game.combat.CastBestHealOnTarget(targetCharIndex)
}

// castBestHeal casts the selected character's strongest known heal (C key),
// aimed at the party member under the mouse (or self). No-op if they know none.
func (ih *InputHandler) castBestHeal(sel *character.MMCharacter) {
	if cast, spellID := ih.castBestHealResolved(); cast {
		ih.commitRTAction(rtActHeal, ih.game.combat.SpellCooldownFrames(sel, spellID))
	}
}

// handleCharacterSelectionInput processes party character selection via 1-4
// keys. Selection is a UI/inventory focus, not permission to act: turn-based
// action keys still gate on canSelectChar before attacking/casting.
func (ih *InputHandler) handleCharacterSelectionInput() {
	target := -1
	switch {
	case ebiten.IsKeyPressed(ebiten.Key1):
		target = 0
	case ebiten.IsKeyPressed(ebiten.Key2):
		target = 1
	case ebiten.IsKeyPressed(ebiten.Key3):
		target = 2
	case ebiten.IsKeyPressed(ebiten.Key4):
		target = 3
	}
	if target < 0 || target >= len(ih.game.party.Members) {
		return
	}
	ih.game.selectedChar = target
	ih.game.parkSelection = true // park here even if KO (use their potions)
}

// handleUIInput processes UI-related input
func (ih *InputHandler) handleUIInput() {
	// Toggle FPS counter with '/' key (slash)
	if ih.slashKeyTracker.IsKeyJustPressed(ebiten.KeySlash) {
		ih.game.showFPS = !ih.game.showFPS
	}
	// Toggle collision box visibility with apostrophe key (')
	if ih.apostropheKeyTracker.IsKeyJustPressed(ebiten.KeyApostrophe) {
		ih.game.showCollisionBoxes = !ih.game.showCollisionBoxes
	}
	if ih.menuKeyTracker.IsKeyJustPressed(ebiten.KeyM) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabSpellbook {
				ih.game.menuOpen = false // Close menu if already on Spellbook tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				// Switching into the spellbook: clear highlight until user picks one.
				ih.game.currentTab = TabSpellbook
				ih.game.selectedSpell = -1
			}
		} else {
			ih.openTabbedMenu(TabSpellbook)
		}
	}
	if ih.inventoryKeyTracker.IsKeyJustPressed(ebiten.KeyI) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabInventory {
				ih.game.menuOpen = false // Close menu if already on Inventory tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				ih.game.currentTab = TabInventory
			}
		} else {
			ih.openTabbedMenu(TabInventory)
		}
	}
	// Characters sheet is on P ('party') — C now casts the best heal in combat.
	if ih.charactersKeyTracker.IsKeyJustPressed(ebiten.KeyP) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabCharacters {
				ih.game.menuOpen = false // Close menu if already on Characters tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				ih.game.currentTab = TabCharacters
			}
		} else {
			ih.openTabbedMenu(TabCharacters)
		}
	}
	if ih.questsKeyTracker.IsKeyJustPressed(ebiten.KeyJ) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabQuests {
				ih.game.menuOpen = false // Close menu if already on Quests tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				ih.game.currentTab = TabQuests
			}
		} else {
			ih.openTabbedMenu(TabQuests)
		}
	}
	if ih.cardsKeyTracker.IsKeyJustPressed(ebiten.KeyK) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabCards {
				ih.game.menuOpen = false // Close menu if already on Cards tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				ih.game.currentTab = TabCards
			}
		} else {
			ih.openTabbedMenu(TabCards)
		}
	}

	// Handle NPC interaction with T key
	if ih.interactKeyTracker.IsKeyJustPressed(ebiten.KeyT) && ih.game.spellInputCooldown == 0 {
		ih.handleNPCInteraction()
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Toggle turn-based mode with Tab key
	if ih.tabKeyTracker.IsKeyJustPressed(ebiten.KeyTab) {
		ih.toggleTurnBasedMode()
	}
}

// handleSpellbookInput processes spellbook navigation and casting
// Movement helper methods
// movePlayer translates the party by (dx, dy) world units with wall-sliding: if
// the combined move is blocked, it slides along whichever single axis is still
// clear, so grazing a corner/obstacle no longer stops the party dead. Only one
// axis slides (sliding both would just recreate the blocked diagonal and clip).
func (ih *InputHandler) movePlayer(dx, dy float64) {
	cam := ih.game.camera
	cs := ih.game.collisionSystem
	moved := false
	switch {
	case cs.CanMoveTo("player", cam.X+dx, cam.Y+dy):
		cam.X += dx
		cam.Y += dy
		moved = true
	case dx != 0 && cs.CanMoveTo("player", cam.X+dx, cam.Y):
		cam.X += dx
		moved = true
	case dy != 0 && cs.CanMoveTo("player", cam.X, cam.Y+dy):
		cam.Y += dy
		moved = true
	}
	if !moved {
		return
	}
	cs.UpdateEntity("player", cam.X, cam.Y)
	ih.game.maybeCardMoveBurst() // Gorilla Titan Card: chance to burst nearby foes on a step
	ih.checkTeleporter()
	ih.checkDeepWater()
}

func (ih *InputHandler) moveForward() {
	speed := ih.moveSpeed()
	ih.movePlayer(ih.game.camera.GetForwardX()*speed, ih.game.camera.GetForwardY()*speed)
}

func (ih *InputHandler) moveBackward() {
	speed := ih.moveSpeed()
	ih.movePlayer(-ih.game.camera.GetForwardX()*speed, -ih.game.camera.GetForwardY()*speed)
}

func (ih *InputHandler) strafeLeft() {
	speed := ih.moveSpeed()
	ih.movePlayer(ih.game.camera.GetRightX()*-speed, ih.game.camera.GetRightY()*-speed)
}

func (ih *InputHandler) strafeRight() {
	speed := ih.moveSpeed()
	ih.movePlayer(ih.game.camera.GetRightX()*speed, ih.game.camera.GetRightY()*speed)
}

func (ih *InputHandler) movementScale() float64 {
	tps := ih.game.config.GetTPS()
	if tps <= 0 {
		return 1.0
	}
	return 60.0 / float64(tps)
}

// moveSpeed is the per-frame real-time translation speed: base move speed × the
// tick-rate scale, sprinted by the run multiplier while Shift is held. Sprint is
// real-time only (turn-based movement is tile-stepped) and applies to translation,
// not rotation.
func (ih *InputHandler) moveSpeed() float64 {
	speed := ih.game.config.GetMoveSpeed() * ih.movementScale()
	if ih.isRunning() {
		speed *= ih.game.config.GetRunMultiplier()
	}
	// Monster cards (e.g. the Thief Bug Card) speed up party travel.
	if pct := ih.game.cardMoveSpeedPct(); pct != 0 {
		speed *= 1 + float64(pct)/100
	}
	return speed
}

// isRunning reports whether the party is sprinting (run key held) in real time.
// All attacks/casts/shots are disabled while running — you must stop to act.
// Always false in turn-based mode (movement there is tile-stepped, not sprinted).
func (ih *InputHandler) isRunning() bool {
	return !ih.game.turnBasedMode &&
		(ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight))
}

// checkTeleporter checks if player is on a teleporter and handles teleportation
func (ih *InputHandler) checkTeleporter() {
	targetMapKey, newX, newY, teleported := ih.tryTeleportation()
	if !teleported {
		return // No teleportation occurred
	}

	// Cross-map teleport: switch, then land north-facing + autosave (same arrival
	// path as enter_map portals).
	if targetMapKey != "" && world.GlobalWorldManager != nil && targetMapKey != world.GlobalWorldManager.CurrentMapKey {
		ih.switchToMap(targetMapKey)
		ih.finishMapArrival(newX, newY, AngleNorth)
		return
	}

	// Same-map teleport: keep the party's heading, no map-change autosave.
	ih.game.camera.X = newX
	ih.game.camera.Y = newY
	ih.game.collisionSystem.UpdateEntity("player", newX, newY)
	if ih.game.turnBasedMode {
		ih.game.snapToCardinalDirection()
	}
}

// switchToMap performs a world switch and refreshes render caches for the new map
// This ensures environment sprites and floor colors reflect the active world

// tryTeleportation checks if the player is on a teleporter and attempts teleportation using the global registry
func (ih *InputHandler) tryTeleportation() (string, float64, float64, bool) {
	x, y := ih.game.camera.X, ih.game.camera.Y
	worldInst := ih.game.GetCurrentWorld()
	if worldInst == nil || world.GlobalWorldManager == nil {
		return "", x, y, false
	}
	tileSize := float64(ih.game.config.GetTileSize())
	tx, ty := int(x/tileSize), int(y/tileSize)
	if tx < 0 || tx >= worldInst.Width || ty < 0 || ty >= worldInst.Height {
		return "", x, y, false
	}
	tile := worldInst.Tiles[ty][tx]
	if world.GlobalTileManager == nil {
		return "", x, y, false
	}
	tileData := world.GlobalTileManager.GetTileData(tile)
	if tileData == nil || strings.ToLower(tileData.Type) != "teleporter" {
		return "", x, y, false
	}
	reg := world.GlobalWorldManager.GlobalTeleporterRegistry
	source, ok := reg.FindTeleporter(world.GlobalWorldManager.CurrentMapKey, tx, ty)
	if !ok || !source.AutoActivate {
		return "", x, y, false
	}
	if reg.LastUsedByGroup == nil {
		reg.LastUsedByGroup = make(map[string]time.Time)
	}
	cooldown := time.Duration(source.CooldownSeconds * float64(time.Second))
	if cooldown > 0 {
		if last, exists := reg.LastUsedByGroup[source.Group]; exists {
			if time.Since(last) < cooldown {
				return "", x, y, false
			}
		}
	}

	dest, ok := reg.GetRandomDestinationTeleporter(source)
	if !ok {
		return "", x, y, false
	}
	reg.LastUsedByGroup[source.Group] = time.Now()
	nx, ny := TileCenterFromTile(dest.X, dest.Y, tileSize)
	return dest.MapKey, nx, ny, true
}

// switchToMap handles common map switching logic for teleporters and spell effects
func (ih *InputHandler) switchToMap(targetMapKey string) {
	// Bound UNDEAD can't follow across maps: they crumble as the party departs,
	// granting their XP but no loot or gold. (Pacified mobs aren't yours — they're
	// simply left behind.) Card-collection allies (SummonedBy == cardSummonOwner)
	// are exempt: they're permanent summons that persist on their map (re-summoned
	// fresh on the new one via the proc), not disposable spell-bound undead.
	if ih.game.world != nil && ih.game.combat != nil {
		for _, m := range ih.game.world.Monsters {
			if m != nil && m.Bound && m.IsAlive() && m.SummonedBy != cardSummonOwner {
				ih.game.combat.awardExperienceOnly(m)
				ih.game.AddCombatMessage(fmt.Sprintf("Your bound %s crumbles as you leave.", m.Name))
				m.HitPoints = 0
			}
		}
	}

	err := world.GlobalWorldManager.SwitchToMap(targetMapKey)
	if err != nil {
		fmt.Printf("Failed to switch to map %s: %v\n", targetMapKey, err)
		return
	}

	// Update world reference and collision system
	oldWorld := ih.game.world
	ih.game.world = ih.game.GetCurrentWorld()
	ih.game.clearTransientCombatState()
	if ih.game.collisionSystem != nil {
		ih.game.collisionSystem.UpdateTileChecker(ih.game.world)
		// Unregister old world monsters
		if oldWorld != nil {
			for _, monster := range oldWorld.Monsters {
				ih.game.collisionSystem.UnregisterEntity(monster.ID)
			}
		}
		// Register new world monsters
		ih.game.world.RegisterMonstersWithCollisionSystem(ih.game.collisionSystem)
	}

	// Update visual systems
	ih.game.UpdateSkyAndGroundColors()
	if ih.game.gameLoop != nil && ih.game.gameLoop.renderer != nil {
		// Refresh renderer caches that depend on world tiles
		ih.game.gameLoop.renderer.precomputeFloorColorCache()
		ih.game.gameLoop.renderer.buildTransparentSpriteCache()
	}

	// NOTE: do NOT autosave here. The player's position on the new map is set by
	// finishMapArrival AFTER this returns; autosaving here would snapshot the OLD
	// map's coordinates on the new map (e.g. the party jammed into a border wall).
}

// finishMapArrival is the single "arrived on a new map" path: it places the
// party at (x,y,angle), re-registers collision, snaps to a cardinal heading in
// turn-based mode, and autosaves. Keeping position + autosave together here is
// what guarantees the autosave can't capture stale pre-switch coordinates — the
// ordering invariant lives in one place instead of being copy-pasted per caller.
func (ih *InputHandler) finishMapArrival(x, y, angle float64) {
	ih.game.camera.X = x
	ih.game.camera.Y = y
	ih.game.camera.Angle = angle
	if ih.game.collisionSystem != nil {
		ih.game.collisionSystem.UpdateEntity("player", x, y)
	}
	// Turn-based facing must be cardinal; a restored return-pose / free RT heading
	// would otherwise leave the party at 45° on the new map.
	if ih.game.turnBasedMode {
		ih.game.snapToCardinalDirection()
	}
	ih.game.Autosave()
}

// checkDeepWater checks if player stepped on deep water and handles Water Breathing teleportation
func (ih *InputHandler) checkDeepWater() {
	x, y := ih.game.camera.X, ih.game.camera.Y
	worldInst := ih.game.GetCurrentWorld()
	if worldInst == nil || world.GlobalWorldManager == nil {
		return
	}

	tileSize := float64(ih.game.config.GetTileSize())
	tx, ty := int(x/tileSize), int(y/tileSize)

	// Check bounds
	if tx < 0 || tx >= worldInst.Width || ty < 0 || ty >= worldInst.Height {
		return
	}

	tile := worldInst.Tiles[ty][tx]

	// Only handle deep water tiles
	if tile != world.TileDeepWater {
		return
	}

	// If Water Breathing is active, teleport to underwater map center
	if ih.game.waterBreathingActive {
		// Find safe return position before going underwater - MUST succeed for safety
		currentX, currentY := ih.game.camera.X, ih.game.camera.Y
		safeX, safeY := ih.game.FindNearestWalkableTileMustSucceed(currentX, currentY)

		// Store safe return position and current map
		ih.game.underwaterReturnX = safeX
		ih.game.underwaterReturnY = safeY
		if world.GlobalWorldManager != nil {
			ih.game.underwaterReturnMap = world.GlobalWorldManager.CurrentMapKey
		}

		// Switch to underwater map
		ih.switchToMap("water")

		// Teleport to center of water map
		centerX, centerY := TileCenterFromTile(25, 25, tileSize)
		ih.finishMapArrival(centerX, centerY, ih.game.camera.Angle)

		fmt.Println("Entered underwater realm with Water Breathing active!")
	} else {
		// No Water Breathing - player drowns or takes damage
		fmt.Println("You cannot survive in deep water without Water Breathing!")
		// TODO: Add drowning damage or teleport back to safe position
	}
}

// Spellbook helper methods

func (ih *InputHandler) navigateSpellbookUp(schools []character.MagicSchoolID) {
	currentChar := ih.game.party.Members[ih.game.selectedChar]

	// Selection cleared (e.g. after switching schools): keyboard nav starts at the last spell.
	if ih.game.selectedSpell < 0 {
		currentSpells := currentChar.GetSpellsForSchool(schools[ih.game.selectedSchool])
		if len(currentSpells) > 0 {
			ih.game.selectedSpell = len(currentSpells) - 1
		}
		return
	}
	if ih.game.selectedSpell > 0 {
		ih.game.selectedSpell--
	} else if ih.game.selectedSchool > 0 {
		// Move to previous school
		ih.game.selectedSchool--
		ih.game.selectedSpell = len(currentChar.GetSpellsForSchool(schools[ih.game.selectedSchool])) - 1
	}
}

func (ih *InputHandler) navigateSpellbookDown(schools []character.MagicSchoolID) {
	currentChar := ih.game.party.Members[ih.game.selectedChar]
	currentSpells := currentChar.GetSpellsForSchool(schools[ih.game.selectedSchool])

	// Selection cleared: keyboard nav starts at the first spell.
	if ih.game.selectedSpell < 0 {
		if len(currentSpells) > 0 {
			ih.game.selectedSpell = 0
		}
		return
	}
	if ih.game.selectedSpell < len(currentSpells)-1 {
		ih.game.selectedSpell++
	} else if ih.game.selectedSchool < len(schools)-1 {
		// Move to next school
		ih.game.selectedSchool++
		ih.game.selectedSpell = 0
	}
}

// handleMouseInput processes mouse input for targeting and UI interaction
func (ih *InputHandler) handleMouseInput() {
	// Heal targeting (H/C key) is handled in the combat input handlers
	// (handleCombatInput / handleTurnBasedInput) so it shares the new
	// per-character cooldown + auto-advance, instead of a separate path here.

	// Handle party character selection clicks (works both in and out of menu).
	// Selection is allowed even when the member is out of TB actions; combat
	// actions remain gated separately.
	if clickX, clickY, ok := ih.game.leftClickPosition(); ok {
		targetCharIndex := ih.getPartyMemberUnderMouse(clickX, clickY)
		if targetCharIndex >= 0 {
			if ih.game.consumeLeftClick() {
				ih.game.selectedChar = targetCharIndex
				ih.game.parkSelection = true // park here even if KO (use their potions)
			}
		}
	}

	// Ground container pickup (only during gameplay, no overlays)
	if !ih.game.menuOpen && !ih.game.mainMenuOpen && !ih.game.showHighScores && !ih.game.mapOverlayOpen && !ih.game.dialogActive && !ih.game.statPopupOpen && ih.game.currentLevelUpChoice() == nil {
		pickupRange := ih.game.groundContainerPickupRange()
		if clickX, clickY, ok := ih.game.leftClickPosition(); ok {
			if idx := ih.game.findGroundContainerIndexAtScreen(clickX, clickY, pickupRange); idx >= 0 {
				ih.game.consumeLeftClick()
				ih.game.pickupGroundContainerAt(idx)
				return
			}
		}
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// getPartyMemberUnderMouse returns the index of the party member under the mouse cursor
// Returns -1 if no party member is under the cursor
func (ih *InputHandler) getPartyMemberUnderMouse(mouseX, mouseY int) int {
	if !ih.game.showPartyStats {
		return -1
	}

	// Calculate party UI layout (same as UI system)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(ih.game)

	// Check if mouse is in party UI area
	if mouseY < startY || mouseY >= startY+portraitHeight {
		return -1
	}
	if mouseX < baseLeft || mouseX >= baseLeft+portraitWidth*4 {
		return -1
	}

	// Determine which character portrait the mouse is over
	charIndex := (mouseX - baseLeft) / portraitWidth
	if charIndex >= 0 && charIndex < len(ih.game.party.Members) {
		// Check if the click is on the + button area (exclude it from character selection)
		member := ih.game.party.Members[charIndex]
		if member.FreeStatPoints > 0 {
			x := baseLeft + charIndex*portraitWidth
			plusBtnX := x + 20
			plusBtnY := startY + portraitHeight - 28
			plusBtnW := 24
			plusBtnH := 24

			// If clicking on + button area, don't select character
			if mouseX >= plusBtnX && mouseX < plusBtnX+plusBtnW &&
				mouseY >= plusBtnY && mouseY < plusBtnY+plusBtnH {
				return -1
			}
		}
		if charIndex == 0 {
			hasFreeStats := false
			for _, partyMember := range ih.game.party.Members {
				if partyMember != nil && partyMember.FreeStatPoints > 0 {
					hasFreeStats = true
					break
				}
			}
			autoBtnX := baseLeft + 72
			autoBtnY := startY + portraitHeight - 28
			if hasFreeStats && mouseX >= autoBtnX && mouseX < autoBtnX+58 &&
				mouseY >= autoBtnY && mouseY < autoBtnY+24 {
				return -1
			}
		}
		if ih.game.hasLevelUpChoiceForChar(charIndex) {
			x := baseLeft + charIndex*portraitWidth
			caretX := x + portraitWidth - 28
			caretY := startY + portraitHeight - 28
			caretW := 24
			caretH := 24
			if mouseX >= caretX && mouseX < caretX+caretW &&
				mouseY >= caretY && mouseY < caretY+caretH {
				return -1
			}
		}
		return charIndex
	}

	return -1
}

// openTabbedMenu opens the tabbed menu with the specified tab
func (ih *InputHandler) openTabbedMenu(tab MenuTab) {
	ih.game.menuOpen = true
	ih.game.currentTab = tab
	if tab == TabSpellbook {
		// No spell highlighted until the user clicks or navigates.
		ih.game.selectedSpell = -1
	}
	ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
}

// handleTabbedMenuInput processes input when the tabbed menu is open
func (ih *InputHandler) handleTabbedMenuInput() {
	if ih.game.spellInputCooldown > 0 {
		return
	}

	// Close menu with Escape
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		ih.game.menuOpen = false
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
		return
	}

	// Allow character selection in menu with 1-4 keys
	if ebiten.IsKeyPressed(ebiten.Key1) && len(ih.game.party.Members) > 0 {
		ih.game.selectedChar = 0
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.Key2) && len(ih.game.party.Members) > 1 {
		ih.game.selectedChar = 1
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.Key3) && len(ih.game.party.Members) > 2 {
		ih.game.selectedChar = 2
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.Key4) && len(ih.game.party.Members) > 3 {
		ih.game.selectedChar = 3
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Handle spellbook navigation when in spellbook tab
	if ih.game.currentTab == TabSpellbook {
		ih.handleSpellbookNavigation()
	}
}

// handleSpellbookNavigation handles navigation within the spellbook tab
func (ih *InputHandler) handleSpellbookNavigation() {
	currentChar := ih.game.party.Members[ih.game.selectedChar]

	// Trap book (thief): spell-like controls — Up/Down browse, Enter/F equips
	// the selection into the quick slot. MUST run before the magic-school
	// checks: a trapper has no schools and would bail out early.
	if hasTrapBook(currentChar) {
		keys := availableTraps(currentChar)
		if len(keys) == 0 {
			return
		}
		if ih.game.selectedTrap >= len(keys) || ih.game.selectedTrap < 0 {
			ih.game.selectedTrap = 0
		}
		if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) || ih.wKeyTracker.IsKeyJustPressed(ebiten.KeyW) {
			ih.game.selectedTrap = (ih.game.selectedTrap - 1 + len(keys)) % len(keys)
		}
		if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) || ih.sKeyTracker.IsKeyJustPressed(ebiten.KeyS) {
			ih.game.selectedTrap = (ih.game.selectedTrap + 1) % len(keys)
		}
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) || ih.fKeyTracker.IsKeyJustPressed(ebiten.KeyF) {
			equipTrap(currentChar, keys[ih.game.selectedTrap])
			ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
		}
		return
	}

	schools := spellbookSchoolsWithSpells(currentChar)
	if len(schools) == 0 {
		return
	}

	// The school list is PER CHARACTER: switching members (keys 1-4, mouse)
	// can shrink it under a stale index — clamp before any schools[...] access.
	if ih.game.selectedSchool >= len(schools) || ih.game.selectedSchool < 0 {
		ih.game.selectedSchool = 0
		ih.game.selectedSpell = -1
	}

	// Navigation: step one spell per key press so the user can't overshoot.
	// No cooldown needed — IsKeyJustPressed already debounces to one step per press.
	if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) {
		ih.navigateSpellbookUp(schools)
	}

	if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) {
		ih.navigateSpellbookDown(schools)
	}

	// Equip spell to the fast spell slot. Keep the spellbook open so the player
	// can keep browsing after binding the fast slot.
	if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) || ih.fKeyTracker.IsKeyJustPressed(ebiten.KeyF) {
		ih.game.combat.EquipSelectedSpell()
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
}

// handleNPCInteraction starts a dialog with the nearest NPC within
// interaction range. Mirrors the HUD hint (GetNearestInteractableNPC) so the
// player always talks to the same NPC they see prompted.
func (ih *InputHandler) handleNPCInteraction() {
	npc := ih.game.GetNearestInteractableNPC()
	if npc == nil {
		return
	}
	// A Light-aligned ward that flags rejects_lich (the Mage Tower) won't speak to
	// a party containing a Lich. Gated on the NPC's own flag, NOT "is a quest
	// giver" — other quest givers (e.g. the Dragon Cliffs hermits) are unaffected.
	if npc.RejectsLich && ih.game.party.HasLich() {
		ih.game.AddCombatMessage("The tower's wards flare against the undead - it will not answer a Lich.")
		return
	}
	// Credit any of this NPC's kill quests whose targets are already gone, so the
	// turn-in option appears (e.g. the Lich King slain before the Archmage's Trial
	// was taken, or the cliff trolls thinned below the quota).
	ih.game.creditClearedKillQuests(npc)
	ih.game.applyCompletedQuestTiles() // dialogue-time credit can complete a world-changing quest
	ih.game.dialogActive = true
	ih.game.dialogNPC = npc
	ih.game.dialogTab = 0           // spell traders with quests open on the Spells tab
	ih.buildStatueChoices(npc)      // statues offer held statuettes as choices
	ih.game.selectedCharIdx = 0     // Default to first character
	ih.game.dialogSelectedChar = 0  // Ensure dialog selection is also set
	ih.game.dialogSelectedSpell = 0 // Default to first spell
	ih.game.selectedSpellKey = ""   // No spell selected initially
	ih.game.skillTrainerPopup = false
	ih.game.selectedChoice = 0   // Reset encounter choice selection
	ih.game.dialogNodePath = nil // Start every conversation at the greeting
	ih.game.merchantBuyPage = 0
	ih.game.merchantSellPage = 0
	ih.game.spellTraderPage = 0
	ih.game.cardCollectorInvPage = 0

	// If NPC has spells, select the first one (deterministic order)
	if npcHasSpellTrading(npc) {
		spellKeys := npcSpellKeys(npc) // Use deterministic ordering
		if len(spellKeys) > 0 {
			ih.game.selectedSpellKey = spellKeys[0]
		}
	}
}

// handleDialogInput handles input when in dialog mode
func (ih *InputHandler) handleDialogInput() {
	// Handle mouse input for character selection
	ih.handleDialogMouseInput()

	// ESC is handled at the top-level input dispatcher (sees the
	// skillTrainerPopup flag and peels off the popup before the dialog).

	// Navigate characters with Left/Right arrows
	if ebiten.IsKeyPressed(ebiten.KeyLeft) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedCharIdx > 0 {
			ih.game.selectedCharIdx--
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedCharIdx < len(ih.game.party.Members)-1 {
			ih.game.selectedCharIdx++
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Handle different NPC capabilities
	if ih.game.dialogNPC != nil {
		switch npcDialogKindFor(ih.game.dialogNPC) {
		case dialogKindSpellTrader:
			ih.handleSpellTraderInput()
		case dialogKindSkillTrainer:
			ih.handleSkillTrainerInput()
		case dialogKindChoices:
			ih.handleEncounterInput()
		}
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// syncSpellTraderPageToSelection keeps the highlight index and the visible page
// aligned with selectedSpellKey after keyboard navigation, so the selected spell
// is always on-screen and an Enter purchase matches what's shown.
func (ih *InputHandler) syncSpellTraderPageToSelection(spellKeys []string) {
	for i, key := range spellKeys {
		if key == ih.game.selectedSpellKey {
			ih.game.dialogSelectedSpell = i
			ih.game.spellTraderPage = i / spellTraderPerPage
			return
		}
	}
}

// getAvailableSpellKeys returns the list of spell keys available from the current NPC in deterministic order
// navigateSpellSelectionUp moves spell selection up
func (ih *InputHandler) navigateSpellSelectionUp(spellKeys []string) {
	if len(spellKeys) == 0 {
		return
	}

	if ih.game.selectedSpellKey == "" {
		ih.game.selectedSpellKey = spellKeys[len(spellKeys)-1]
		return
	}

	for i, key := range spellKeys {
		if key == ih.game.selectedSpellKey {
			if i > 0 {
				ih.game.selectedSpellKey = spellKeys[i-1]
			} else {
				ih.game.selectedSpellKey = spellKeys[len(spellKeys)-1]
			}
			return
		}
	}
}

// navigateSpellSelectionDown moves spell selection down
func (ih *InputHandler) navigateSpellSelectionDown(spellKeys []string) {
	if len(spellKeys) == 0 {
		return
	}

	if ih.game.selectedSpellKey == "" {
		ih.game.selectedSpellKey = spellKeys[0]
		return
	}

	for i, key := range spellKeys {
		if key == ih.game.selectedSpellKey {
			if i < len(spellKeys)-1 {
				ih.game.selectedSpellKey = spellKeys[i+1]
			} else {
				ih.game.selectedSpellKey = spellKeys[0]
			}
			return
		}
	}
}

// npcShopLine returns the dialog NPC's configured line for a shop event (with the
// buyer/spell vars filled in), or `fallback` when the NPC defines none. `pick`
// selects the relevant field from the NPC's dialogue data.
func (ih *InputHandler) npcShopLine(pick func(*character.NPCDialogue) string, vars npcDialogVars, fallback string) string {
	if ih.game.dialogNPC != nil && ih.game.dialogNPC.DialogueData != nil {
		if custom := pick(ih.game.dialogNPC.DialogueData); custom != "" {
			return formatNPCDialogue(custom, vars)
		}
	}
	return fallback
}

// purchaseSelectedSpell attempts to purchase the selected spell for the selected character
func (ih *InputHandler) purchaseSelectedSpell() {
	if ih.game.dialogNPC == nil || ih.game.selectedSpellKey == "" {
		return
	}

	selectedChar := ih.game.party.Members[ih.game.selectedCharIdx]
	spellData := ih.game.dialogNPC.SpellData[ih.game.selectedSpellKey]

	if spellData == nil {
		return
	}

	vars := npcDialogVars{Name: selectedChar.Name, Spell: spellData.Name, Cost: spellData.Cost}

	// Check if character already knows this spell
	if characterKnowsSpellByName(selectedChar, spellData.Name) {
		ih.game.AddCombatMessage(ih.npcShopLine(
			func(d *character.NPCDialogue) string { return d.AlreadyKnown }, vars,
			fmt.Sprintf("%s already knows %s!", selectedChar.Name, spellData.Name)))
		return
	}

	// Check if character has enough gold
	if ih.game.party.Gold < spellData.Cost {
		ih.game.AddCombatMessage(ih.npcShopLine(
			func(d *character.NPCDialogue) string { return d.InsufficientGold }, vars,
			fmt.Sprintf("Need %d gold to learn %s", spellData.Cost, spellData.Name)))
		return
	}

	// Check if character can learn this spell (class restrictions) - reuse UI logic
	if !canCharacterLearnNPCSpell(selectedChar, spellData) {
		ih.game.AddCombatMessage(fmt.Sprintf("%s cannot learn %s (requirements not met)", selectedChar.Name, spellData.Name))
		return
	}

	// Teach FIRST, charge after: a spell that fails to resolve must not eat
	// the gold (and must not leave an empty school behind).
	if !ih.addSpellToCharacter(selectedChar, spellData) {
		ih.game.AddCombatMessage(fmt.Sprintf("%s cannot be taught right now.", spellData.Name))
		return
	}
	ih.game.party.Gold -= spellData.Cost

	ih.game.AddCombatMessage(ih.npcShopLine(
		func(d *character.NPCDialogue) string { return d.Success }, vars,
		fmt.Sprintf("%s learned %s!", selectedChar.Name, spellData.Name)))
}

// addSpellToCharacter teaches a shop spell; reports whether the spellbook
// actually changed (false: unresolvable spell or already known — the caller
// must not charge for it). Resolution and learning go through the ONE path
// (LearnSpell), which also picks the school from spells.yaml rather than the
// trader catalog.
func (ih *InputHandler) addSpellToCharacter(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	spellIDToAdd, err := spells.GetSpellIDByName(spellData.Name)
	if err != nil {
		return false // Spell not found
	}
	return char.LearnSpell(spellIDToAdd)
}

// handleDialogMouseInput handles mouse input in dialog mode
func (ih *InputHandler) handleDialogMouseInput() {
	if _, _, ok := ih.game.leftClickPosition(); !ok {
		return
	}

	// Dialog coordinates (single source: npcDialogLayout, same as the UI)
	dlg := npcDialogLayout(ih.game)
	dialogX, dialogY, dialogWidth, dialogHeight := dlg.x, dlg.y, dlg.w, dlg.h

	// Card collector — double-click a slotted card to take it back, a loose card
	// to slot it (matches the inventory's equip/unequip double-click). Reset after
	// each action since the lists shift underneath the cursor.
	if npcIsCardCollector(ih.game.dialogNPC) {
		for slot := 0; slot < MaxCardSlots; slot++ {
			x, y, w, h := cardCollectorSlotRect(dialogX, dialogY, slot)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				if ih.dialogDoubleClick("card_slot", slot) {
					ih.game.removeCardToInventory(slot)
					ih.resetDialogDoubleClick()
				}
				return
			}
		}
		cardIdx := ih.game.inventoryCardIndices()
		// Match the renderer's pagination: only the current page's loose cards
		// are clickable, mapping page-slot -> inventory index.
		invStart := ih.game.cardCollectorInvPage * cardInvMaxShown
		for slot := 0; slot < cardInvMaxShown; slot++ {
			i := invStart + slot
			if i >= len(cardIdx) {
				break
			}
			inv := cardIdx[i]
			x, y, w, h := cardCollectorInvRect(dialogX, dialogY, slot)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				if ih.dialogDoubleClick("card_inv", inv) {
					if !ih.game.placeCardFromInventory(inv) {
						ih.game.AddCombatMessage("The collection is full (8 cards).")
					}
					ih.resetDialogDoubleClick()
				}
				return
			}
		}
		return
	}

	// Spell trader — portrait strip + icon grid.
	if ih.game.dialogNPC != nil && npcHasSpellTrading(ih.game.dialogNPC) {
		// On the Quests tab the shop widgets aren't drawn — don't let their
		// hidden rects swallow clicks meant for the quest choices/tabs.
		if ih.game.dialogTab == 1 && npcHasChoiceDialog(ih.game.dialogNPC) {
			return
		}
		for i := range ih.game.party.Members {
			x, y, w, h := spellTraderPortraitRect(dialogX, dialogY, i)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				ih.game.selectedCharIdx = i
				return
			}
		}
		spellKeys := npcSpellKeys(ih.game.dialogNPC)
		// Match the renderer's pagination: only the current page's icons are
		// clickable, mapping page-slot → global spell index.
		pageStart := ih.game.spellTraderPage * spellTraderPerPage
		for slot := 0; slot < spellTraderPerPage; slot++ {
			i := pageStart + slot
			if i >= len(spellKeys) {
				break
			}
			x, y, w, h := spellTraderIconRect(dialogX, dialogY, slot)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				if ih.dialogDoubleClick("trader_spell", i) {
					ih.purchaseSelectedSpell()
					ih.resetDialogDoubleClick()
				} else {
					ih.game.dialogSelectedSpell = i
					ih.game.selectedSpellKey = spellKeys[i]
				}
				return
			}
		}
		return
	}

	// Skill trainer — portrait click opens the per-character popup with
	// trainable masteries. Popup option clicks select/purchase. Clicks
	// outside the popup close it (back to portrait grid).
	if ih.game.dialogNPC != nil && npcHasSkillTraining(ih.game.dialogNPC) {
		if ih.game.skillTrainerPopup &&
			ih.game.selectedCharIdx >= 0 &&
			ih.game.selectedCharIdx < len(ih.game.party.Members) {
			px, py, pw, ph := skillTrainerPopupRect(dialogX, dialogY, dialogWidth, dialogHeight)
			options := trainerOptions(ih.game.party.Members[ih.game.selectedCharIdx])
			for i := range options {
				x, y, w, h := skillTrainerOptionRect(px, py, i)
				if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
					ih.game.dialogSelectedSpell = i
					if ih.dialogDoubleClick("trainer_option", i) {
						ih.purchaseSelectedTraining()
						ih.resetDialogDoubleClick()
					}
					return
				}
			}
			// Click outside popup → close.
			if ih.game.consumeLeftClick() {
				_ = px
				_ = py
				_ = pw
				_ = ph
				ih.game.skillTrainerPopup = false
			}
			return
		}
		// No popup → portrait click opens it.
		for i := range ih.game.party.Members {
			x, y, w, h := skillTrainerPortraitRect(dialogX, dialogY, dialogWidth, i)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				ih.game.selectedCharIdx = i
				ih.game.dialogSelectedSpell = 0
				ih.game.skillTrainerPopup = true
				return
			}
		}
		return
	}

	// Buy/sell clicks on the merchant icon grids. Cell rects + page state are
	// shared with the renderer (merchantGridLayout/merchantCellRect); the pager
	// buttons are consumed in the draw pass, so a click that misses every cell
	// here falls through to flip the page. idx (absolute list position) keys the
	// double-click so the same item keeps its identity across pages.
	if ih.game.dialogNPC != nil && npcHasMerchant(ih.game.dialogNPC) {
		leftX, rightX, gridTop, _ := merchantGridLayout(dialogX, dialogY)

		// Buy from merchant (left grid).
		stock := ih.game.dialogNPC.MerchantStock
		buyStart := ih.game.merchantBuyPage * merchantPageSize
		for slot := 0; slot < merchantPageSize; slot++ {
			idx := buyStart + slot
			if idx >= len(stock) {
				break
			}
			x, y, w, h := merchantCellRect(leftX, gridTop, slot)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				if ih.dialogDoubleClick("merchant_buy", idx) {
					entry := stock[idx]
					if entry.Quantity <= 0 {
						ih.game.AddCombatMessage("That item is sold out.")
						return
					}
					cost := ih.game.merchantBuyPrice(entry.Cost) // Merchant skill discount
					if cost > ih.game.party.Gold {
						ih.game.AddCombatMessage(fmt.Sprintf("Need %d gold to buy %s.", cost, entry.Item.Name))
						return
					}
					ih.game.party.Gold -= cost
					ih.game.party.AddItem(entry.Item)
					entry.Quantity--
					ih.game.AddCombatMessage(fmt.Sprintf("Bought %s for %d gold.", entry.Item.Name, cost))
					ih.resetDialogDoubleClick()
				}
				return
			}
		}

		// Sell to merchant (right grid).
		if ih.game.dialogNPC.SellAvailable {
			inv := ih.game.party.Inventory
			sellStart := ih.game.merchantSellPage * merchantPageSize
			for slot := 0; slot < merchantPageSize; slot++ {
				idx := sellStart + slot
				if idx >= len(inv) {
					break
				}
				x, y, w, h := merchantCellRect(rightX, gridTop, slot)
				if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
					if ih.dialogDoubleClick("merchant_sell", idx) {
						item := inv[idx]
						base := item.Attributes["value"]
						if base <= 0 {
							ih.game.AddCombatMessage("This item has no value.")
							return
						}
						price := ih.game.merchantSellPrice(base) // Merchant skill markup
						ih.game.party.Gold += price
						ih.game.party.RemoveItem(idx)
						ih.game.AddCombatMessage(fmt.Sprintf("Sold %s for %d gold.", item.Name, price))
						ih.resetDialogDoubleClick()
					}
					return
				}
			}
		}
		return
	}

	// Encounter choices are handled in handleEncounterInput (shared
	// dialogueChoiceRect geometry; works for plain encounters AND the spell
	// trader's Quests tab).
}

// dialogDoubleClick reports whether this click completes a double-click on the
// same list entry. zone keys the tracker per list (buy/sell/spell/...), so two
// fast clicks on the same index of DIFFERENT lists never count.
func (ih *InputHandler) dialogDoubleClick(zone string, index int) bool {
	currentTime := time.Now().UnixMilli()
	delta := currentTime - ih.game.dialogLastClickTime
	doubleClick := ih.game.dialogLastClickZone == zone &&
		ih.game.dialogLastClickedIdx == index && delta < doubleClickWindowMs
	ih.game.dialogLastClickTime = currentTime
	ih.game.dialogLastClickedIdx = index
	ih.game.dialogLastClickZone = zone
	return doubleClick
}

// resetDialogDoubleClick clears the tracker after a completed action, so a
// follow-up click can't pair with the consumed one — crucial when the action
// shifted the list (selling removes the row; the next row slides under the
// cursor and a stray click would sell it too).
func (ih *InputHandler) resetDialogDoubleClick() {
	ih.game.resetDialogClickTracker()
}

// resetDialogClickTracker clears the dialog double-click tracker. Shared with
// the renderer so a merchant page flip (consumed in the Draw pass) also breaks
// the chain — otherwise click-item → flip page → click same index could pair as
// a double-click buy/sell despite the navigation in between.
func (g *MMGame) resetDialogClickTracker() {
	g.dialogLastClickTime = 0
	g.dialogLastClickedIdx = -1
	g.dialogLastClickZone = ""
}

// toggleTurnBasedMode switches between real-time and turn-based modes
func (ih *InputHandler) toggleTurnBasedMode() {
	ih.game.ToggleTurnBasedMode()
}

// handleTurnBasedInput processes input during turn-based mode
func (ih *InputHandler) handleTurnBasedInput() {
	if ih.game.currentTurn != 0 { // Not party's turn
		return
	}

	// Update cooldowns
	if ih.game.turnBasedMoveCooldown > 0 {
		ih.game.turnBasedMoveCooldown--
	}
	if ih.game.turnBasedRotCooldown > 0 {
		ih.game.turnBasedRotCooldown--
	}

	// A TB rotation is an inspection sweep through the diagonal sector, not just a
	// cosmetic snap. While the rendered view is still turning, ignore party actions
	// so the player actually gets those intermediate frames before acting again.
	if ih.game.viewTurnFramesLeft > 0 {
		return
	}

	// Each alive+conscious character has Speed-derived attack slots
	// (ActionsRemaining). Round ends when all are 0 OR the party moves.
	if ih.game.partyAllExhausted() {
		return // Turn is over
	}

	// Handle movement (counts as using the turn) - only if cooldown is ready
	moved := false
	if ih.game.turnBasedMoveCooldown == 0 {
		moveForward := ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) || ih.wKeyTracker.IsKeyJustPressed(ebiten.KeyW)
		moveBackward := ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) || ih.sKeyTracker.IsKeyJustPressed(ebiten.KeyS)
		strafeLeft := ih.qKeyTracker.IsKeyJustPressed(ebiten.KeyQ)
		strafeRight := ih.eKeyTracker.IsKeyJustPressed(ebiten.KeyE)

		if moveForward {
			moved = ih.moveTurnBasedForward()
		} else if moveBackward {
			moved = ih.moveTurnBasedBackward()
		} else if strafeLeft {
			moved = ih.moveTurnBasedStrafeLeft()
		} else if strafeRight {
			moved = ih.moveTurnBasedStrafeRight()
		}

		if moved {
			ih.game.turnBasedMoveCooldown = int(TurnBasedInputCooldownSeconds * float64(ih.game.config.GetTPS()))
		}
	}

	// 90-degree rotation with cooldown (doesn't use turn)
	if ih.game.turnBasedRotCooldown == 0 {
		rotated := false
		rotateLeft := ih.leftKeyTracker.IsKeyJustPressed(ebiten.KeyLeft) || ih.aKeyTracker.IsKeyJustPressed(ebiten.KeyA)
		rotateRight := ih.rightKeyTracker.IsKeyJustPressed(ebiten.KeyRight) || ih.dKeyTracker.IsKeyJustPressed(ebiten.KeyD)
		if rotateLeft {
			ih.rotateTurnBased(-1) // Counter-clockwise
			rotated = true
		} else if rotateRight {
			ih.rotateTurnBased(1) // Clockwise
			rotated = true
		}

		if rotated {
			ih.game.turnBasedRotCooldown = int(TurnBasedInputCooldownSeconds * float64(ih.game.config.GetTPS()))
			return
		}
	}

	if moved {
		ih.game.endPartyTurnAfterMovement()
		return
	}

	// Selected character can attack/spell if they're still selectable this
	// round (alive + conscious + has an action slot). Same key scheme as
	// real-time (R/Space/F/C); turn-based gates on action slots (not frame
	// cooldowns) and consumes a slot per action via consumeSelectedCharAction.
	selected := ih.game.party.Members[ih.game.selectedChar]
	canAct := ih.game.canSelectChar(ih.game.selectedChar)
	if !canAct || ih.game.spellInputCooldown != 0 {
		return
	}

	switch {
	case ih.rKeyTracker.IsKeyJustPressed(ebiten.KeyR): // melee/ranged weapon attack
		if ih.game.combat.EquipmentMeleeAttack() {
			ih.game.consumeSelectedCharAction()
		}
		ih.game.spellInputCooldown = ih.actionCooldown(15)
	case ih.spaceKeyTracker.IsKeyJustPressed(ebiten.KeySpace): // smart attack
		if ih.game.tryPickupNearestGroundContainer(ih.game.groundContainerPickupRange()) {
			return
		}
		if acted, _ := ih.game.combat.SmartAttack(); acted {
			ih.game.consumeSelectedCharAction()
		}
		ih.game.spellInputCooldown = ih.actionCooldown(15)
	case ih.fKeyTracker.IsKeyJustPressed(ebiten.KeyF): // cast slotted spell
		if fired, _ := ih.castSlottedSpellResolved(selected); fired {
			ih.game.consumeSelectedCharAction()
		}
		ih.game.spellInputCooldown = ih.actionCooldown(15) // debounce even on a failed cast, like the other action keys
	case ih.cKeyTracker.IsKeyJustPressed(ebiten.KeyC) || ih.hKeyTracker.IsKeyJustPressed(ebiten.KeyH): // cast best known heal (H = legacy alias)
		if cast, _ := ih.castBestHealResolved(); cast {
			ih.game.consumeSelectedCharAction()
		}
		ih.game.spellInputCooldown = ih.actionCooldown(15)
	}
}

// Movement functions for turn-based mode (snap to tile centers)
func (ih *InputHandler) moveTurnBasedForward() bool {
	// Move in the direction the camera is facing
	deltaX, deltaY := ih.getDirectionFromAngle(ih.game.camera.Angle)
	return ih.moveTurnBasedInDirection(deltaX, deltaY)
}

func (ih *InputHandler) moveTurnBasedBackward() bool {
	// Move opposite to the direction the camera is facing
	deltaX, deltaY := ih.getDirectionFromAngle(ih.game.camera.Angle)
	return ih.moveTurnBasedInDirection(-deltaX, -deltaY)
}

func (ih *InputHandler) moveTurnBasedStrafeLeft() bool {
	// Move 90 degrees counter-clockwise from camera direction
	leftAngle := ih.game.camera.Angle - math.Pi/2
	deltaX, deltaY := ih.getDirectionFromAngle(leftAngle)
	return ih.moveTurnBasedInDirection(deltaX, deltaY)
}

func (ih *InputHandler) moveTurnBasedStrafeRight() bool {
	// Move 90 degrees clockwise from camera direction
	rightAngle := ih.game.camera.Angle + math.Pi/2
	deltaX, deltaY := ih.getDirectionFromAngle(rightAngle)
	return ih.moveTurnBasedInDirection(deltaX, deltaY)
}

// getDirectionFromAngle converts an angle to grid movement direction
func (ih *InputHandler) getDirectionFromAngle(angle float64) (int, int) {
	// Normalize angle to 0-2π range
	for angle < 0 {
		angle += 2 * math.Pi
	}
	for angle >= 2*math.Pi {
		angle -= 2 * math.Pi
	}

	// Convert angle to grid direction
	// East (0): +X (1, 0)
	// South (π/2): +Y (0, 1)
	// West (π): -X (-1, 0)
	// North (3π/2): -Y (0, -1)

	if angle < math.Pi/4 || angle >= 7*math.Pi/4 {
		return 1, 0 // East
	} else if angle < 3*math.Pi/4 {
		return 0, 1 // South
	} else if angle < 5*math.Pi/4 {
		return -1, 0 // West
	} else {
		return 0, -1 // North
	}
}

// moveTurnBasedInDirection handles grid-based movement with tile center snapping
func (ih *InputHandler) moveTurnBasedInDirection(deltaX, deltaY int) bool {
	tileSize := float64(ih.game.config.GetTileSize())

	// Get current tile coordinates
	currentTileX := int(ih.game.camera.X / tileSize)
	currentTileY := int(ih.game.camera.Y / tileSize)

	// Calculate target tile
	targetTileX := currentTileX + deltaX
	targetTileY := currentTileY + deltaY

	// First, check if the target tile itself is passable (not blocked by terrain)
	if !ih.canMoveToTile(targetTileX, targetTileY) {
		return false // Target tile is impassable (tree, wall, etc.)
	}

	// Calculate exact center of target tile
	targetX, targetY := TileCenterFromTile(targetTileX, targetTileY, tileSize)

	// In turn-based mode, if the tile is passable, we should always be able to move there
	// This fixes getting stuck issues by prioritizing tile passability over entity collision
	ih.game.camera.X = targetX
	ih.game.camera.Y = targetY
	ih.game.collisionSystem.UpdateEntity("player", targetX, targetY)
	ih.game.maybeCardMoveBurst() // Gorilla Titan Card: chance to burst nearby foes on a step (parity with RT)
	ih.checkTeleporter()
	ih.checkDeepWater()
	return true
}

// canMoveToTile checks if a tile coordinate is passable (ignores entity collisions)
// Uses centralized collision system for DRY compliance
func (ih *InputHandler) canMoveToTile(tileX, tileY int) bool {
	// Convert tile coordinates to world coordinates (center of tile)
	tileSize := float64(ih.game.config.World.TileSize)
	worldX, worldY := TileCenterFromTile(tileX, tileY, tileSize)

	// Use collision system which handles bounds checking and centralized tile logic
	return ih.game.collisionSystem.CanMoveTo("player", worldX, worldY)
}

// rotateTurnBased rotates the camera in 90-degree increments
func (ih *InputHandler) rotateTurnBased(direction int) {
	// Rotate by 90 degrees (π/2 radians)
	rotationAmount := math.Pi / 2
	if direction < 0 {
		rotationAmount = -rotationAmount
	}

	ih.game.camera.Angle += rotationAmount

	// Normalize angle to keep it between 0 and 2π
	for ih.game.camera.Angle < 0 {
		ih.game.camera.Angle += 2 * math.Pi
	}
	for ih.game.camera.Angle >= 2*math.Pi {
		ih.game.camera.Angle -= 2 * math.Pi
	}

	// The logical angle snapped above (gameplay reads it immediately); ease the
	// RENDERED view toward it over the next frames so the turn glides (advanceViewTurn).
	ih.game.viewTurnFramesLeft = ih.game.turnViewFrames()
}

func (ih *InputHandler) resolveHealTarget(spell items.Item, mouseX, mouseY int) int {
	targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)
	if targetCharIndex >= 0 && spell.SpellEffect == items.SpellEffectHealOther {
		return targetCharIndex
	}
	if targetCharIndex == ih.game.selectedChar {
		return targetCharIndex
	}
	return ih.game.selectedChar
}

// handleSpellTraderInput handles input for spell trader NPCs
func (ih *InputHandler) handleSpellTraderInput() {
	// Quest-giving traders (e.g. Aldric) carry a second "Quests" tab; Tab
	// switches, and on that tab the encounter-style choice input takes over.
	if npcHasChoiceDialog(ih.game.dialogNPC) {
		if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
			ih.game.dialogTab = 1 - ih.game.dialogTab
			ih.game.selectedChoice = 0
		}
		if ih.game.dialogTab == 1 {
			ih.handleEncounterInput()
			return
		}
	}

	spellKeys := npcSpellKeys(ih.game.dialogNPC)

	if ebiten.IsKeyPressed(ebiten.KeyUp) && ih.game.spellInputCooldown == 0 {
		ih.navigateSpellSelectionUp(spellKeys)
		ih.syncSpellTraderPageToSelection(spellKeys)
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) && ih.game.spellInputCooldown == 0 {
		ih.navigateSpellSelectionDown(spellKeys)
		ih.syncSpellTraderPageToSelection(spellKeys)
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Purchase spell with Enter
	if ebiten.IsKeyPressed(ebiten.KeyEnter) && ih.game.spellInputCooldown == 0 {
		ih.purchaseSelectedSpell()
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
}

func (ih *InputHandler) handleSkillTrainerInput() {
	if ih.game.selectedCharIdx < 0 || ih.game.selectedCharIdx >= len(ih.game.party.Members) {
		return
	}
	options := trainerOptions(ih.game.party.Members[ih.game.selectedCharIdx])
	if len(options) == 0 {
		ih.game.dialogSelectedSpell = 0
		return
	}
	if ih.game.dialogSelectedSpell >= len(options) {
		ih.game.dialogSelectedSpell = len(options) - 1
	}

	if ebiten.IsKeyPressed(ebiten.KeyUp) && ih.game.spellInputCooldown == 0 {
		if ih.game.dialogSelectedSpell > 0 {
			ih.game.dialogSelectedSpell--
		} else {
			ih.game.dialogSelectedSpell = len(options) - 1
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) && ih.game.spellInputCooldown == 0 {
		if ih.game.dialogSelectedSpell < len(options)-1 {
			ih.game.dialogSelectedSpell++
		} else {
			ih.game.dialogSelectedSpell = 0
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyEnter) && ih.game.spellInputCooldown == 0 {
		ih.purchaseSelectedTraining()
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
}

func (ih *InputHandler) purchaseSelectedTraining() {
	if ih.game.dialogNPC == nil || !npcHasSkillTraining(ih.game.dialogNPC) {
		return
	}
	if ih.game.selectedCharIdx < 0 || ih.game.selectedCharIdx >= len(ih.game.party.Members) {
		return
	}
	selectedChar := ih.game.party.Members[ih.game.selectedCharIdx]
	options := trainerOptions(selectedChar)
	if ih.game.dialogSelectedSpell < 0 || ih.game.dialogSelectedSpell >= len(options) {
		return
	}
	option := options[ih.game.dialogSelectedSpell]
	if ih.game.party.Gold < option.Cost {
		ih.game.AddCombatMessage(fmt.Sprintf("Need %d gold to train %s.", option.Cost, option.Label))
		return
	}

	trained := false
	if option.IsMagic {
		if skill := selectedChar.MagicSchools[option.School]; skill != nil {
			trained = skill.IncreaseMastery()
		}
	} else {
		// trainSkill bumps mastery AND refreshes derived stats (Bodybuilding → Max HP).
		trained = ih.game.trainSkill(selectedChar, option.SkillType)
	}
	if !trained {
		ih.game.AddCombatMessage(fmt.Sprintf("%s is already at maximum mastery.", option.Label))
		return
	}

	ih.game.party.Gold -= option.Cost
	ih.game.AddCombatMessage(fmt.Sprintf("%s trained %s to %s for %d gold.", selectedChar.Name, option.Label, option.Next.String(), option.Cost))
}

// handleEncounterInput handles input for encounter NPCs
func (ih *InputHandler) handleEncounterInput() {
	npc := ih.game.dialogNPC
	if npc == nil || npc.DialogueData == nil {
		return
	}

	// Only the choices valid in the NPC's current state. None → concluded /
	// non-actionable; ESC leaves.
	choices := ih.game.visibleNPCChoices(npc)
	if len(choices) == 0 {
		return
	}
	// State may have shrunk the list since the dialog opened — keep the cursor valid.
	if ih.game.selectedChoice >= len(choices) {
		ih.game.selectedChoice = len(choices) - 1
	}

	// Mouse: clicking a choice row selects it; a second click on the same row
	// (double-click, like every other dialog list) executes it.
	dlg := npcDialogLayout(ih.game)
	for i := range choices {
		x, y, w, h := ih.game.dialogueChoiceRect(npc, i, dlg.x, dlg.y, dlg.w)
		if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
			ih.game.selectedChoice = i
			if ih.dialogDoubleClick("encounter_choice", i) {
				ih.executeEncounterChoice()
				ih.resetDialogDoubleClick()
			}
			return
		}
	}

	// Navigate choices with Up/Down arrows
	if ebiten.IsKeyPressed(ebiten.KeyUp) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedChoice > 0 {
			ih.game.selectedChoice--
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedChoice < len(choices)-1 {
			ih.game.selectedChoice++
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Select choice with Enter or number keys
	if ebiten.IsKeyPressed(ebiten.KeyEnter) && ih.game.spellInputCooldown == 0 {
		ih.executeEncounterChoice()
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Number key shortcuts (1-9)
	for i := 0; i < len(choices) && i < 9; i++ {
		var key ebiten.Key
		switch i {
		case 0:
			key = ebiten.Key1
		case 1:
			key = ebiten.Key2
		case 2:
			key = ebiten.Key3
		case 3:
			key = ebiten.Key4
		case 4:
			key = ebiten.Key5
		case 5:
			key = ebiten.Key6
		case 6:
			key = ebiten.Key7
		case 7:
			key = ebiten.Key8
		case 8:
			key = ebiten.Key9
		}
		if ebiten.IsKeyPressed(key) && ih.game.spellInputCooldown == 0 {
			ih.game.selectedChoice = i
			ih.executeEncounterChoice()
			ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			break
		}
	}
}

// executeEncounterChoice executes the selected encounter choice
func (ih *InputHandler) executeEncounterChoice() {
	npc := ih.game.dialogNPC
	if npc == nil || npc.DialogueData == nil {
		return
	}
	choices := ih.game.visibleNPCChoices(npc)
	if ih.game.selectedChoice >= len(choices) {
		return
	}

	choice := choices[ih.game.selectedChoice]

	switch choice.Action {
	case "info":
		// Branch deeper: show this choice's reply + its follow-up choices. The
		// conversation stays open (no quest taken) until the player picks a
		// terminal action inside the branch.
		ih.game.dialogNodePath = append(ih.game.dialogNodePath, choice)
		ih.game.selectedChoice = 0

	case "back":
		// Pop one conversation level (back toward the greeting).
		if n := len(ih.game.dialogNodePath); n > 0 {
			ih.game.dialogNodePath = ih.game.dialogNodePath[:n-1]
		}
		ih.game.selectedChoice = 0

	case "leave":
		// Close dialog and leave
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil

	case "combat":
		// Start encounter combat
		ih.startEncounter()

	case "enter_map":
		ih.enterEncounterMap(choice.Map)

	case "give_quest":
		ih.handleGiveQuest(choice.QuestID)

	case "turn_in_quest":
		ih.handleTurnInQuest(choice.QuestID)

	case "close_valve":
		ih.handleCloseValve(choice.QuestID)

	case "take_swords":
		ih.handleOpenSwordRack(choice.QuestID)

	case "tavern_rest":
		ih.handleTavernRest(choice)

	case "buy_food":
		ih.handleBuyFood(choice)

	case "summon_dragon":
		ih.summonDragonFromStatue(npc, choice.SummonIndex)

	case "open_roster":
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		ih.game.rosterScreenOpen = true
		ih.game.rosterSelectedActive = -1

	case "manage_stash":
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		ih.game.openStash()

	default:
		// Unknown action - just close dialog
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
	}
}

// countLivingQuestTargets returns living, quest-eligible monsters whose name maps
// to target (the same name→key normalization quest kills use). Dead monsters are
// dropped from the world slice, so HP>0 means alive. targetMap scopes the search
// to one map; empty scans every loaded map (suits a unique boss). Runtime/ad-hoc
// summons can opt out so they do not distort map-clear quest progress.
func (g *MMGame) countLivingQuestTargets(target, targetMap string) int {
	scan := func(w *world.World3D) int {
		if w == nil {
			return 0
		}
		count := 0
		for _, m := range w.Monsters {
			if m == nil || m.HitPoints <= 0 || m.QuestProgressIgnored {
				continue
			}
			if strings.ToLower(strings.ReplaceAll(m.Name, " ", "_")) == target {
				count++
			}
		}
		return count
	}
	wm := world.GlobalWorldManager
	if wm == nil {
		return scan(g.world)
	}
	if targetMap != "" {
		return scan(wm.LoadedMaps[targetMap])
	}
	total := 0
	for _, w := range wm.LoadedMaps {
		total += scan(w)
	}
	return total
}

// syncExterminationQuestProgress refreshes an active exterminate quest's counter
// to (target - living) and returns the living target count, so callers can reuse
// it for the completion check instead of scanning the world a second time.
// Returns -1 when the quest isn't an active exterminate kill (no sync done).
func (g *MMGame) syncExterminationQuestProgress(questID string) int {
	if g.questManager == nil {
		return -1
	}
	q := g.questManager.GetQuest(questID)
	if q == nil || q.Completed || q.Definition.Type != quests.QuestTypeKill || !q.Definition.Exterminate {
		return -1
	}
	living := g.countLivingQuestTargets(q.Definition.TargetMonster, q.Definition.TargetMap)
	g.questManager.SetCurrentCount(q.ID, q.Target()-living)
	return living
}

func (g *MMGame) syncExterminationQuestProgressForTarget(target string) {
	if g.questManager == nil || target == "" {
		return
	}
	for _, q := range g.questManager.GetActiveQuests() {
		if q.Definition.Type == quests.QuestTypeKill &&
			q.Definition.Exterminate &&
			q.Definition.TargetMonster == target {
			g.syncExterminationQuestProgress(q.ID)
		}
	}
}

// creditQuestIfCleared marks an active kill quest completed when none of its
// targets remain alive — a quest taken (or held) after the killing was already
// done can be turned in instead of showing 0/N forever. Returns whether it
// completed the quest just now.
func (g *MMGame) creditQuestIfCleared(questID string) bool {
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(questID)
	if q == nil || q.Completed ||
		q.Definition.Type != quests.QuestTypeKill || q.Definition.TargetMonster == "" {
		return false
	}
	living := g.syncExterminationQuestProgress(questID)
	if living < 0 { // not an exterminate quest — sync didn't scan, so do it here
		living = g.countLivingQuestTargets(q.Definition.TargetMonster, q.Definition.TargetMap)
	}
	if living > 0 {
		return false
	}
	g.questManager.MarkCompleted(questID)
	return true
}

// creditClearedKillQuests completes any of the NPC's active kill quests whose
// targets are all already dead — so a quest taken after its targets were slain
// (or one whose remaining targets number fewer than its quota) can still be
// turned in. Only kill quests with a target qualify; other types complete through
// their own progress.
func (g *MMGame) creditClearedKillQuests(npc *character.NPC) {
	if npc == nil || npc.DialogueData == nil || g.questManager == nil {
		return
	}
	for _, c := range npc.DialogueData.Choices {
		if c == nil || c.QuestID == "" ||
			(c.Action != "give_quest" && c.Action != "turn_in_quest") {
			continue
		}
		g.creditQuestIfCleared(c.QuestID)
	}
}

// handleGiveQuest activates a quest offered by an NPC (e.g. the Archmage trial).
func (ih *InputHandler) handleGiveQuest(questID string) {
	g := ih.game
	g.dialogActive = false
	g.dialogNPC = nil
	if questID == "" || quests.GlobalQuestManager == nil {
		return
	}
	// The Archmage's Trial is special: it has promotion prerequisites (no lich in
	// the party, an eligible promotable member) and its own Lich-King flavor. Those
	// checks/messages must NOT leak onto ordinary quests (e.g. the Dragon Cliffs
	// kill quests), which just activate generically.
	if questID == "archmage_trial" {
		if g.party.HasLich() {
			g.AddCombatMessage("The tower's wards reject the undead.")
			return
		}
		if len(g.eligibleArchmageIndices()) == 0 {
			g.AddCombatMessage("No one in your party can walk the Archmage's path.")
			return
		}
		if err := quests.GlobalQuestManager.ActivateQuest(questID); err != nil {
			g.AddCombatMessage("The trial is already underway - return when the Lich King is slain.")
			return
		}
		g.AddCombatMessage("Trial accepted: slay the Lich King, then return to the tower.")
		return
	}

	// Generic quest activation.
	if err := quests.GlobalQuestManager.ActivateQuest(questID); err != nil {
		g.AddCombatMessage("You are already on that quest.")
		return
	}
	name := questID
	if q := quests.GlobalQuestManager.GetQuest(questID); q != nil && q.Definition.Name != "" {
		name = q.Definition.Name
		// Exterminate quests count the map's live target population at accept time,
		// so the goal tracks the real census instead of a hand-maintained number
		// (and an already-empty map completes instantly via creditQuestIfCleared).
		if q.Definition.Exterminate {
			census := g.countLivingQuestTargets(q.Definition.TargetMonster, q.Definition.TargetMap)
			quests.GlobalQuestManager.SetDynamicTarget(questID, census)
		}
	}
	g.AddCombatMessage(fmt.Sprintf("Quest accepted: %s", name))

	// Targets already wiped out before the quest was taken? Credit it on the
	// spot (and apply any world changes) instead of showing 0/N until the next
	// chat — the journal should never say "0/21" on a finished job.
	if g.creditQuestIfCleared(questID) {
		g.applyCompletedQuestTiles()
		g.AddCombatMessage(fmt.Sprintf("'%s' is already done! Return to claim your reward.", name))
	}
}

// handleTurnInQuest turns a completed quest in at the NPC. The Archmage's Trial
// is special (promotes a member instead of paying gold/XP); every other quest
// claims its reward on the spot. Either way the NPC concludes (Visited) so it
// stops re-offering.
func (ih *InputHandler) handleTurnInQuest(questID string) {
	g := ih.game
	npc := g.dialogNPC // capture before clearing — we conclude it on success
	g.dialogActive = false
	g.dialogNPC = nil
	if questID == "" || g.questManager == nil {
		return
	}

	if questID == "archmage_trial" {
		if g.party.HasLich() {
			g.AddCombatMessage("The tower's wards reject the undead.")
			return
		}
		quest := g.questManager.GetQuest(questID)
		if quest == nil || !quest.Completed {
			g.AddCombatMessage("The Lich King still draws breath. Return when the deed is done.")
			return
		}
		if !g.promoteEligibleMember(character.PromotionArchmage, -1) {
			g.AddCombatMessage("No one in your party can walk the Archmage's path.")
			return
		}
		g.questManager.RemoveQuest(questID) // can't be turned in twice
		if npc != nil {
			npc.Visited = true
		}
		return
	}

	// Generic turn-in: must be done, then pay out and conclude the NPC.
	quest := g.questManager.GetQuest(questID)
	if quest == nil || !quest.Completed {
		g.AddCombatMessage("That task isn't finished yet - return when it's done.")
		return
	}
	if g.claimQuestReward(questID) && npc != nil {
		npc.Visited = true
	}
}

// handleCloseValve shuts a sluice valve: advances the interact-quest by one (only
// while it's active) and marks this valve Visited so it stays shut and can't be
// re-counted. The quest's tag is its TargetMonster ("valve").
func (ih *InputHandler) handleCloseValve(questID string) {
	g := ih.game
	npc := g.dialogNPC
	g.dialogActive = false
	g.dialogNPC = nil
	if g.questManager == nil || npc == nil {
		return
	}
	q := g.questManager.GetQuest(questID)
	if q == nil || q.Status != quests.QuestStatusActive {
		g.AddCombatMessage("The valve won't budge - no reason to shut it yet.")
		return
	}
	completed := g.questManager.OnInteract(q.Definition.TargetMonster)
	npc.Visited = true // this valve stays shut and can't be re-counted
	g.AddCombatMessage(fmt.Sprintf("You heave the valve shut. (%s)", q.GetProgressString()))
	for _, cq := range completed {
		g.AddCombatMessage(fmt.Sprintf("Quest '%s' complete! The flood drains from the lair.", cq.Definition.Name))
	}
}

// handleOpenSwordRack loots a katana rack hidden behind a shoji: rolls the zone
// loot table into the party (random zone gear + a little gold, never a unique),
// advances the interact-quest, and marks the rack Visited so it can't be
// re-looted. Gating on an ACTIVE quest avoids consuming a rack that wouldn't
// count (no soft-lock). Gathering all of them completes castle_armory, which
// wakes the dormant Samurai Warlord (passive_until_quest).
func (ih *InputHandler) handleOpenSwordRack(questID string) {
	g := ih.game
	npc := g.dialogNPC
	g.dialogActive = false
	g.dialogNPC = nil
	if g.questManager == nil || npc == nil {
		return
	}
	if npc.Visited {
		g.AddCombatMessage("The rack stands empty.")
		return
	}
	q := g.questManager.GetQuest(questID)
	if q == nil || q.Status != quests.QuestStatusActive {
		g.AddCombatMessage("No reason to disturb the armoury yet.")
		return
	}
	npc.Visited = true
	loot, gold := rollWeightedLootTable("castle_armory")
	for _, it := range loot {
		g.party.AddItem(it)
	}
	if gold > 0 {
		g.party.Gold += gold
	}
	parts := make([]string, 0, len(loot)+1)
	if gold > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", gold))
	}
	for _, it := range loot {
		parts = append(parts, it.Name)
	}
	if len(parts) > 0 {
		g.AddCombatMessage(fmt.Sprintf("You take from the rack: %s.", strings.Join(parts, ", ")))
	}
	completed := g.questManager.OnInteract(q.Definition.TargetMonster)
	g.AddCombatMessage(fmt.Sprintf("Sword rack cleared. (%s)", q.GetProgressString()))
	for _, cq := range completed {
		g.AddCombatMessage(fmt.Sprintf("Quest '%s' complete! A cold wind stirs the keep - the Warlord wakes.", cq.Definition.Name))
	}
}

// handleTavernRest charges the room price and fully restores the party (the
// dead stay dead), then closes the dialog — the night passes.
func (ih *InputHandler) handleTavernRest(choice *character.NPCDialogueChoice) {
	g := ih.game
	if g.party.Gold < choice.Cost {
		g.AddCombatMessage(fmt.Sprintf("A night here costs %d gold - you cannot afford it.", choice.Cost))
		return
	}
	g.party.Gold -= choice.Cost
	g.restParty()
	g.dialogActive = false
	g.dialogNPC = nil
	g.AddCombatMessage(fmt.Sprintf("The party sleeps soundly (-%d gold). HP and spell points restored.", choice.Cost))
}

// handleBuyFood sells Amount food for Cost gold; the dialog stays open so the
// player can stock up or rest afterwards.
func (ih *InputHandler) handleBuyFood(choice *character.NPCDialogueChoice) {
	g := ih.game
	if g.party.Gold < choice.Cost {
		g.AddCombatMessage(fmt.Sprintf("Rations cost %d gold - you cannot afford them.", choice.Cost))
		return
	}
	g.party.Gold -= choice.Cost
	g.party.Food += choice.Amount
	g.AddCombatMessage(fmt.Sprintf("Bought %d rations for %d gold (food: %d).", choice.Amount, choice.Cost, g.party.Food))
}

// buildStatueChoices rebuilds a dragon statue's dialogue choices at open time:
// one "summon" option per dragon statuette the party currently holds (unless the
// statue is spent), plus a trailing Leave. No-op for non-statue NPCs.
func (ih *InputHandler) buildStatueChoices(npc *character.NPC) {
	if npc == nil || len(npc.Summons) == 0 || npc.DialogueData == nil {
		return
	}
	var choices []*character.NPCDialogueChoice
	if !npc.Visited {
		for i, s := range npc.Summons {
			for _, it := range ih.game.party.Inventory {
				if it.Name == s.Statuette {
					choices = append(choices, &character.NPCDialogueChoice{
						Text:        fmt.Sprintf("Offer the %s Dragon Statuette", s.Label),
						Action:      "summon_dragon",
						SummonIndex: i,
					})
					break
				}
			}
		}
	}
	choices = append(choices, &character.NPCDialogueChoice{Text: "Leave", Action: "leave"})
	npc.DialogueData.Choices = choices
}

// summonDragonFromStatue consumes the chosen statuette, spawns its dragon next
// to the statue (flagged so it — and only it — counts toward the win quest),
// and removes the spent statue from the world.
func (ih *InputHandler) summonDragonFromStatue(npc *character.NPC, summonIdx int) {
	g := ih.game
	g.dialogActive = false
	g.dialogNPC = nil
	if npc == nil || summonIdx < 0 || summonIdx >= len(npc.Summons) {
		return
	}
	s := npc.Summons[summonIdx]
	// Re-find the statuette now (inventory may have shifted since the dialog opened).
	itemIdx := -1
	for i, it := range g.party.Inventory {
		if it.Name == s.Statuette {
			itemIdx = i
			break
		}
	}
	if itemIdx < 0 {
		return
	}
	g.party.RemoveItem(itemIdx)

	spawnX, spawnY := ih.findEncounterSpawnLocation(npc.X, npc.Y)
	if spawnX == 0 && spawnY == 0 {
		spawnX, spawnY = npc.X, npc.Y // walled corner: spawn on the statue's tile
	}
	m := monster.NewMonster3DFromConfig(spawnX, spawnY, s.Monster, g.config)
	// Flag so updateQuestProgress credits dragon_slayer for THIS dragon only.
	m.IsEncounterMonster = true
	m.EncounterRewards = &monster.EncounterRewards{QuestID: "dragon_slayer"}
	g.registerSpawnedMonster(m)

	// Mark spent (hide_when_visited makes it vanish from render + interaction).
	// We keep the NPC in the world so its Visited=true is saved and the statue
	// stays spent across reloads — dropping it from the world would lose that.
	npc.Visited = true
	g.AddCombatMessage(fmt.Sprintf("The %s Dragon erupts from the shattering statue!", s.Label))
}

func (ih *InputHandler) enterEncounterMap(targetMapKey string) {
	if targetMapKey == "" || world.GlobalWorldManager == nil || !world.GlobalWorldManager.IsValidMap(targetMapKey) {
		ih.game.AddCombatMessage("You cannot enter from here.")
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		return
	}

	// Remember where the player is on the current map so a return trip
	// drops them back at the doorway, not at the map's spawn tile.
	if ih.game.mapReturnPoses == nil {
		ih.game.mapReturnPoses = make(map[string]MapPose)
	}
	if currentKey := world.GlobalWorldManager.CurrentMapKey; currentKey != "" {
		ih.game.mapReturnPoses[currentKey] = MapPose{
			X:     ih.game.camera.X,
			Y:     ih.game.camera.Y,
			Angle: ih.game.camera.Angle,
		}
	}

	ih.game.dialogActive = false
	ih.game.dialogNPC = nil
	ih.switchToMap(targetMapKey)

	currentWorld := ih.game.GetCurrentWorld()
	if currentWorld == nil {
		return
	}

	// Prefer the previously-stored pose for this map if we've been here before;
	// fall back to the map's spawn tile on first visit.
	var x, y, angle float64
	if pose, ok := ih.game.mapReturnPoses[targetMapKey]; ok {
		x, y, angle = pose.X, pose.Y, pose.Angle
	} else {
		// First arrival: drop the party on the centre of the spawn tile facing
		// north, so every fresh location entry is consistent and predictable.
		x, y = currentWorld.GetStartingPosition()
		angle = AngleNorth
	}
	ih.finishMapArrival(x, y, angle)
}

// startEncounter initiates combat encounter with bandits
func (ih *InputHandler) startEncounter() {
	npc := ih.game.dialogNPC
	if npc.EncounterData == nil {
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		return
	}

	// Mark as visited
	npc.Visited = true

	// Close dialog
	ih.game.dialogActive = false
	ih.game.dialogNPC = nil

	// Create encounter quest if quest details are provided
	if npc.EncounterData.QuestID != "" && quests.GlobalQuestManager != nil {
		gold := 0
		exp := 0
		if npc.EncounterData.Rewards != nil {
			gold = npc.EncounterData.Rewards.Gold
			exp = npc.EncounterData.Rewards.Experience
			// Link quest ID to rewards for auto-completion
			npc.EncounterData.Rewards.QuestID = npc.EncounterData.QuestID
		}
		quests.GlobalQuestManager.CreateEncounterQuest(
			npc.EncounterData.QuestID,
			npc.EncounterData.QuestName,
			npc.EncounterData.QuestDescription,
			gold,
			exp,
		)
		ih.game.AddCombatMessage(fmt.Sprintf("Quest Started: %s", npc.EncounterData.QuestName))
	}

	// Spawn monsters near the encounter location
	ih.spawnEncounterMonsters(npc)

	// Add combat message (optional, data-driven)
	if npc.EncounterData != nil && npc.EncounterData.StartMessage != "" {
		ih.game.AddCombatMessage(npc.EncounterData.StartMessage)
	}
}

// spawnEncounterMonsters spawns monsters defined in the encounter
func (ih *InputHandler) spawnEncounterMonsters(npc *character.NPC) {
	if npc.EncounterData == nil || len(npc.EncounterData.Monsters) == 0 {
		return
	}

	for _, monsterDef := range npc.EncounterData.Monsters {
		// Calculate number of monsters to spawn
		count := monsterDef.CountMin
		if monsterDef.CountMax > monsterDef.CountMin {
			count += rand.Intn(monsterDef.CountMax - monsterDef.CountMin + 1)
		}

		// Spawn the monsters near the NPC location
		for i := 0; i < count; i++ {
			// Find a suitable spawn location near the NPC
			spawnX, spawnY := ih.findEncounterSpawnLocation(npc.X, npc.Y)

			// Skip spawning if no valid location found
			if spawnX == 0 && spawnY == 0 {
				fmt.Printf("Skipping monster spawn %d/%d - no walkable location found\n", i+1, count)
				continue
			}

			// Create monster from config
			monster := monster.NewMonster3DFromConfig(spawnX, spawnY, monsterDef.Type, ih.game.config)
			if monster != nil {
				// Mark this monster as part of an encounter for reward tracking
				monster.IsEncounterMonster = true
				monster.EncounterRewards = npc.EncounterData.Rewards
				// Encounter monsters are hostile from the start (the player
				// initiated the fight) — this also wakes passive_until_attacked
				// types like the prison's elf swordsmen.
				monster.WasAttacked = true
				monster.IsEngagingPlayer = true
				ih.game.registerSpawnedMonster(monster)
			}
		}
	}
}

// findEncounterSpawnLocation finds a suitable spawn location near the encounter using DRY helper
func (ih *InputHandler) findEncounterSpawnLocation(npcX, npcY float64) (float64, float64) {
	tileSize := float64(ih.game.config.GetTileSize())

	// Try to spawn within 3-5 tiles of the NPC
	maxAttempts := 15
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Random offset 3-5 tiles away
		offsetRange := 3.0 + rand.Float64()*2.0 // 3-5 tiles
		angle := rand.Float64() * 2 * 3.14159   // Random angle

		offsetX := math.Cos(angle) * offsetRange * tileSize
		offsetY := math.Sin(angle) * offsetRange * tileSize

		candidateX := npcX + offsetX
		candidateY := npcY + offsetY

		// Only return if the exact candidate position is walkable. Uses raw tile
		// walkability (not the player's CanMoveTo) on purpose: a spawn must land on
		// terrain-walkable ground regardless of the player's transient walk-on-water.
		if ih.isPositionWalkable(candidateX, candidateY) {
			return candidateX, candidateY
		}
	}

	// No fallback - if we can't find a good spot, don't spawn the monster
	fmt.Printf("Warning: Could not find walkable spawn location near NPC at (%.1f, %.1f)\n", npcX, npcY)
	return 0, 0 // Return invalid coordinates
}

// isPositionWalkable reports whether a world position sits on a terrain-walkable
// tile. Deliberately a raw tile-walkability check (not the player's CanMoveTo):
// encounter spawns must land on ground that is walkable on its own, independent
// of the player's transient walk-on-water / water-breathing buffs.
func (ih *InputHandler) isPositionWalkable(x, y float64) bool {
	worldInst := ih.game.GetCurrentWorld()
	if worldInst == nil {
		return false
	}
	if x < 0 || y < 0 {
		return false // negative positions are out of bounds
	}
	tileSize := float64(ih.game.config.GetTileSize())
	tileX := int(x / tileSize)
	tileY := int(y / tileSize)
	if tileX >= 0 && tileX < worldInst.Width && tileY >= 0 && tileY < worldInst.Height {
		tile := worldInst.Tiles[tileY][tileX]
		return world.GlobalTileManager != nil && world.GlobalTileManager.IsWalkable(tile)
	}
	return false
}
