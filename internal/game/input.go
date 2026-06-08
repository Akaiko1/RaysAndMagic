package game

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
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
	pKeyTracker          keytracker.KeyStateTracker
	attackHoldFrames     int // frames an RT attack key has been held (tap vs hold-repeat)
	menuKeyTracker       keytracker.KeyStateTracker
	inventoryKeyTracker  keytracker.KeyStateTracker
	charactersKeyTracker keytracker.KeyStateTracker
	questsKeyTracker     keytracker.KeyStateTracker
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
	// When game over, only allow New Game or Load
	if ih.game.gameOver {
		if ih.game.mainMenuOpen && ih.game.mainMenuMode == MenuLoadSelect {
			ih.handleMainMenuInput()
			return
		}
		if ih.newGameKeyTracker.IsKeyJustPressed(ebiten.KeyN) {
			ih.restartNewGame()
			return
		}
		if ih.loadKeyTracker.IsKeyJustPressed(ebiten.KeyL) {
			ih.game.mainMenuOpen = true
			ih.game.mainMenuMode = MenuLoadSelect
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

	// Handle level-up choice overlay
	if ih.game.currentLevelUpChoice() != nil {
		ih.handleLevelUpChoiceInput()
		return
	}

	// Revival potion target picker: clicks are consumed inside the popup's
	// own Draw call (it lives in ui_dialogs.go). Just suppress gameplay input
	// so the player can't move/attack/cast while choosing a revive target.
	if ih.game.revivalPickerOpen {
		return
	}

	// Promotion picker: same deal — clicks handled inside its Draw; just suppress
	// gameplay input while the player chooses who to promote.
	if ih.game.promotionPickerOpen {
		return
	}

	// Tavern roster screen: clicks handled inside its Draw; suppress gameplay.
	if ih.game.rosterScreenOpen {
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

// restartNewGame resets party and state for a fresh start
func (ih *InputHandler) restartNewGame() {
	g := ih.game
	// Recreate party
	g.party = character.NewParty(g.config)
	g.selectedChar = 0

	// Reset victory/high score state and session timer
	g.gameOver = false
	g.gameVictory = false
	g.victoryScoreSaved = false
	g.victoryNameInput = ""
	g.victoryTime = time.Time{}
	g.sessionStartTime = time.Now()
	g.showHighScores = false
	g.frameCount = 0

	// Clear combat/projectile state
	g.magicProjectiles = g.magicProjectiles[:0]
	g.meleeAttacks = g.meleeAttacks[:0]
	g.arrows = g.arrows[:0]
	g.groundContainers = g.groundContainers[:0]
	g.slashEffects = g.slashEffects[:0]
	g.spellHitEffects = g.spellHitEffects[:0]
	g.deadMonsterIDs = g.deadMonsterIDs[:0]
	g.combatMessages = g.combatMessages[:0]
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

	// Reset utility effects and bonuses
	g.torchLightActive = false
	g.torchLightDuration = 0
	g.torchLightRadius = 0
	g.wizardEyeActive = false
	g.wizardEyeDuration = 0
	g.walkOnWaterActive = false
	g.walkOnWaterDuration = 0
	g.blessActive = false
	g.blessDuration = 0
	g.blessStatBonus = 0
	g.waterBreathingActive = false
	g.waterBreathingDuration = 0
	g.underwaterReturnX = 0
	g.underwaterReturnY = 0
	g.underwaterReturnMap = ""
	g.statBonus = 0

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

// handleMainMenuInput processes input for the main menu (opened with ESC)
func (ih *InputHandler) handleMainMenuInput() {
	// Mouse position for hover/click
	mouseX, mouseY := ebiten.CursorPosition()
	panelW, panelH := 300, 220
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

		// Activate selection with Enter
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			switch ih.game.mainMenuSelection {
			case 0: // Continue
				ih.game.mainMenuOpen = false
			case 1: // Save
				ih.game.mainMenuMode = MenuSaveSelect
				ih.game.slotSelection = 0
			case 2: // Load
				ih.game.mainMenuMode = MenuLoadSelect
				ih.game.slotSelection = 0
			case 3: // High Scores
				ih.game.showHighScores = true
			case 4: // Exit
				ih.game.exitRequested = true
			}
		}

		// Mouse click activation
		if ih.game.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			switch ih.game.mainMenuSelection {
			case 0:
				ih.game.mainMenuOpen = false
			case 1:
				ih.game.mainMenuMode = MenuSaveSelect
				ih.game.slotSelection = 0
			case 2:
				ih.game.mainMenuMode = MenuLoadSelect
				ih.game.slotSelection = 0
			case 3:
				ih.game.showHighScores = true
			case 4:
				ih.game.exitRequested = true
			}
		}
	case MenuSaveSelect:
		px := (w - panelW) / 2
		py := (h - panelH) / 2
		if ih.game.saveRenameOpen {
			ih.handleSaveRenameInput()
			return
		}
		for i := 0; i < 5; i++ {
			y := py + 56 + i*32
			if ih.game.consumeRightClickIn(px+16, y-4, px+panelW-16, y+24) {
				sum := GetSaveSlotSummary(i)
				if !sum.Exists {
					ih.game.AddCombatMessage("No save in slot to rename")
				} else {
					ih.game.slotSelection = i
					ih.game.saveRenameOpen = true
					ih.game.saveRenameSlot = i
					ih.game.saveRenameInput = sum.Name
				}
				return
			}
		}
		// Navigate slots 0..4
		if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) {
			if ih.game.slotSelection > 0 {
				ih.game.slotSelection--
			}
		}
		if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) {
			if ih.game.slotSelection < 4 {
				ih.game.slotSelection++
			}
		}
		// Mouse hover selection
		ih.mainMenuHoverSelect(mouseX, mouseY, 5, panelW, panelH, 56)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			if err := ih.game.SaveGameToFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Save failed")
			} else {
				ih.game.AddCombatMessage("Saved to slot")
				ih.game.mainMenuMode = MenuMain
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			sum := GetSaveSlotSummary(ih.game.slotSelection)
			if !sum.Exists {
				ih.game.AddCombatMessage("No save in slot to rename")
			} else {
				ih.game.saveRenameOpen = true
				ih.game.saveRenameSlot = ih.game.slotSelection
				ih.game.saveRenameInput = sum.Name
			}
		}
		// Mouse click activation
		if ih.game.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			if err := ih.game.SaveGameToFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Save failed")
			} else {
				ih.game.AddCombatMessage("Saved to slot")
				ih.game.mainMenuMode = MenuMain
			}
		}
	case MenuLoadSelect:
		px := (w - panelW) / 2
		py := (h - panelH) / 2
		if ih.upKeyTracker.IsKeyJustPressed(ebiten.KeyUp) {
			if ih.game.slotSelection > 0 {
				ih.game.slotSelection--
			}
		}
		if ih.downKeyTracker.IsKeyJustPressed(ebiten.KeyDown) {
			if ih.game.slotSelection < 4 {
				ih.game.slotSelection++
			}
		}
		ih.mainMenuHoverSelect(mouseX, mouseY, 5, panelW, panelH, 56)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			if err := ih.game.LoadGameFromFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Load failed")
			} else {
				ih.game.AddCombatMessage("Loaded from slot")
				ih.game.mainMenuOpen = false
				ih.game.mainMenuMode = MenuMain
			}
		}
		if ih.game.consumeLeftClickIn(px, py, px+panelW, py+panelH) {
			if err := ih.game.LoadGameFromFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Load failed")
			} else {
				ih.game.AddCombatMessage("Loaded from slot")
				ih.game.mainMenuOpen = false
				ih.game.mainMenuMode = MenuMain
			}
		}
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
		ih.game.combat.EquipmentMeleeAttack()
		ih.commitRTAction(rtActWeapon, ih.game.combat.WeaponCooldownFrames(sel))
	case rtActSmart:
		if cast, spellID := ih.game.combat.SmartAttack(); cast {
			ih.commitRTAction(rtActSmart, ih.game.combat.SpellCooldownFrames(sel, spellID))
		} else {
			ih.commitRTAction(rtActSmart, ih.game.combat.WeaponCooldownFrames(sel))
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

// castSlottedSpell casts the selected character's slotted spell (F key). Heal
// spells are aimed with the mouse; everything else casts directly. Applies the
// real-time cooldown only if the cast actually fired.
func (ih *InputHandler) castSlottedSpell(sel *character.MMCharacter) {
	spell, hasSpell := sel.Equipment[items.SlotSpell]
	if !hasSpell {
		return
	}
	spellID := spells.SpellID(spell.SpellEffect)
	if spell.SpellEffect == items.SpellEffectHealSelf || spell.SpellEffect == items.SpellEffectHealOther {
		mouseX, mouseY := ebiten.CursorPosition()
		targetCharIndex := ih.resolveHealTarget(spell, mouseX, mouseY)
		if ih.game.combat.CastEquippedHealOnTarget(targetCharIndex) {
			ih.commitRTAction(rtActCast, ih.game.combat.SpellCooldownFrames(sel, spellID))
		}
		return
	}
	if ih.game.combat.CastEquippedSpell() {
		ih.commitRTAction(rtActCast, ih.game.combat.SpellCooldownFrames(sel, spellID))
	}
}

// castBestHeal casts the selected character's strongest known heal (C key),
// aimed at the party member under the mouse (or self). No-op if they know none.
func (ih *InputHandler) castBestHeal(sel *character.MMCharacter) {
	mouseX, mouseY := ebiten.CursorPosition()
	targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)
	if targetCharIndex < 0 {
		targetCharIndex = ih.game.selectedChar
	}
	if cast, spellID := ih.game.combat.CastBestHealOnTarget(targetCharIndex); cast {
		ih.commitRTAction(rtActHeal, ih.game.combat.SpellCooldownFrames(sel, spellID))
	}
}

// handleCharacterSelectionInput processes party character selection via 1-4
// keys. In turn-based mode, characters that have already spent all their
// action slots this round (or are KO) are not selectable.
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
	if ih.game.turnBasedMode && !ih.game.canSelectChar(target) {
		return
	}
	ih.game.selectedChar = target
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

	// Handle map transition if needed
	if targetMapKey != "" && world.GlobalWorldManager != nil && targetMapKey != world.GlobalWorldManager.CurrentMapKey {
		ih.switchToMap(targetMapKey)
		// Cross-location teleport: face north on arrival for a consistent
		// orientation (same rule as enter_map portals). Same-map teleporters
		// keep the party's current heading.
		ih.game.camera.Angle = AngleNorth
	}

	ih.game.camera.X = newX
	ih.game.camera.Y = newY
	ih.game.collisionSystem.UpdateEntity("player", newX, newY)
	// Keep turn-based facing cardinal after a teleport.
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
	// Bound undead can't follow across maps: they crumble as the party departs,
	// granting their XP but no loot or gold. (Pacified mobs aren't yours — they're
	// simply left behind.)
	if ih.game.world != nil && ih.game.combat != nil {
		for _, m := range ih.game.world.Monsters {
			if m != nil && m.Bound && m.IsAlive() {
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
		mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
		if mapConfig != nil {
			fmt.Printf("Switched to %s map with %s colors: %v\n", targetMapKey, mapConfig.Biome, mapConfig.DefaultFloorColor)
		}
		// Refresh renderer caches that depend on world tiles
		ih.game.gameLoop.renderer.precomputeFloorColorCache()
		ih.game.gameLoop.renderer.buildTransparentSpriteCache()
	}
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
		ih.game.camera.X = centerX
		ih.game.camera.Y = centerY
		ih.game.collisionSystem.UpdateEntity("player", centerX, centerY)

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
	// In turn-based mode, skip portraits whose owner can't act this round
	// (KO or already spent all their slots).
	if clickX, clickY, ok := ih.game.leftClickPosition(); ok {
		targetCharIndex := ih.getPartyMemberUnderMouse(clickX, clickY)
		if targetCharIndex >= 0 {
			if ih.game.turnBasedMode && !ih.game.canSelectChar(targetCharIndex) {
				// Eat the click anyway so we don't fall through to other
				// handlers thinking the click is unused.
				ih.game.consumeLeftClick()
			} else if ih.game.consumeLeftClick() {
				ih.game.selectedChar = targetCharIndex
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
	schools := spellbookSchoolsWithSpells(currentChar)

	if len(schools) == 0 {
		return
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
	// A Lich in the party can't even speak with quest-giving wards (the Mage
	// Tower). The wards reject the undead outright.
	if npcIsQuestGiver(npc) && ih.game.party.HasLich() {
		ih.game.AddCombatMessage("The tower's wards flare against the undead — it will not answer a Lich.")
		return
	}
	ih.game.dialogActive = true
	ih.game.dialogNPC = npc
	ih.buildStatueChoices(npc)      // statues offer held statuettes as choices
	ih.game.selectedCharIdx = 0     // Default to first character
	ih.game.dialogSelectedChar = 0  // Ensure dialog selection is also set
	ih.game.dialogSelectedSpell = 0 // Default to first spell
	ih.game.selectedSpellKey = ""   // No spell selected initially
	ih.game.skillTrainerPopup = false
	ih.game.selectedChoice = 0 // Reset encounter choice selection

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
		switch {
		case npcHasSpellTrading(ih.game.dialogNPC):
			ih.handleSpellTraderInput()
		case npcHasSkillTraining(ih.game.dialogNPC):
			ih.handleSkillTrainerInput()
		case npcHasChoiceDialog(ih.game.dialogNPC):
			ih.handleEncounterInput()
		}
	}

	// Mouse state is updated once per frame in updateMouseState().
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

	// Check if character already knows this spell
	if characterKnowsSpellByName(selectedChar, spellData.Name) {
		msg := "Already known."
		if ih.game.dialogNPC != nil && ih.game.dialogNPC.DialogueData != nil && ih.game.dialogNPC.DialogueData.AlreadyKnown != "" {
			msg = formatNPCDialogue(ih.game.dialogNPC.DialogueData.AlreadyKnown, npcDialogVars{
				Name:  selectedChar.Name,
				Spell: spellData.Name,
				Cost:  spellData.Cost,
			})
		} else {
			msg = fmt.Sprintf("%s already knows %s!", selectedChar.Name, spellData.Name)
		}
		ih.game.AddCombatMessage(msg)
		return
	}

	// Check if character has enough gold
	if ih.game.party.Gold < spellData.Cost {
		msg := "Not enough gold."
		if ih.game.dialogNPC != nil && ih.game.dialogNPC.DialogueData != nil && ih.game.dialogNPC.DialogueData.InsufficientGold != "" {
			msg = formatNPCDialogue(ih.game.dialogNPC.DialogueData.InsufficientGold, npcDialogVars{
				Name:  selectedChar.Name,
				Spell: spellData.Name,
				Cost:  spellData.Cost,
			})
		} else {
			msg = fmt.Sprintf("Need %d gold to learn %s", spellData.Cost, spellData.Name)
		}
		ih.game.AddCombatMessage(msg)
		return
	}

	// Check if character can learn this spell (class restrictions) - reuse UI logic
	if !canCharacterLearnNPCSpell(selectedChar, spellData) {
		ih.game.AddCombatMessage(fmt.Sprintf("%s cannot learn %s (requirements not met)", selectedChar.Name, spellData.Name))
		return
	}

	// Purchase the spell
	ih.game.party.Gold -= spellData.Cost

	// Add spell to character's spellbook
	ih.addSpellToCharacter(selectedChar, spellData)

	msg := fmt.Sprintf("%s learned %s!", selectedChar.Name, spellData.Name)
	if ih.game.dialogNPC != nil && ih.game.dialogNPC.DialogueData != nil && ih.game.dialogNPC.DialogueData.Success != "" {
		msg = formatNPCDialogue(ih.game.dialogNPC.DialogueData.Success, npcDialogVars{
			Name:  selectedChar.Name,
			Spell: spellData.Name,
			Cost:  spellData.Cost,
		})
	}
	ih.game.AddCombatMessage(msg)
}

// characterKnowsSpell checks if a character already knows a spell
// addSpellToCharacter adds a spell to a character's spellbook
func (ih *InputHandler) addSpellToCharacter(char *character.MMCharacter, spellData *character.NPCSpell) {
	targetSchool := character.MagicSchoolID(spellData.School)

	// Ensure the character has the magic school
	if char.MagicSchools[targetSchool] == nil {
		char.MagicSchools[targetSchool] = &character.MagicSkill{
			Mastery:     character.MasteryNovice,
			KnownSpells: make([]spells.SpellID, 0),
		}
	}

	// Convert spell name to SpellID using centralized mapping
	spellIDToAdd, err := spells.GetSpellIDByName(spellData.Name)
	if err != nil {
		return // Spell not found
	}

	// Check if character already has this spell
	for _, existingSpell := range char.MagicSchools[targetSchool].KnownSpells {
		if existingSpell == spellIDToAdd {
			return // Already knows this spell
		}
	}

	// Add the spell ID to the school
	char.MagicSchools[targetSchool].KnownSpells = append(char.MagicSchools[targetSchool].KnownSpells, spellIDToAdd)
}

// handleDialogMouseInput handles mouse input in dialog mode
func (ih *InputHandler) handleDialogMouseInput() {
	if _, _, ok := ih.game.leftClickPosition(); !ok {
		return
	}

	// Calculate dialog coordinates (same as in UI)
	screenWidth := ih.game.config.GetScreenWidth()
	screenHeight := ih.game.config.GetScreenHeight()
	dialogWidth := 600
	dialogHeight := 400
	dialogX := (screenWidth - dialogWidth) / 2
	dialogY := (screenHeight - dialogHeight) / 2

	// Spell trader — portrait strip + icon grid.
	if ih.game.dialogNPC != nil && npcHasSpellTrading(ih.game.dialogNPC) {
		for i := range ih.game.party.Members {
			x, y, w, h := spellTraderPortraitRect(dialogX, dialogY, i)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				ih.game.selectedCharIdx = i
				return
			}
		}
		spellKeys := npcSpellKeys(ih.game.dialogNPC)
		for i, spellKey := range spellKeys {
			x, y, w, h := spellTraderIconRect(dialogX, dialogY, i)
			if ih.game.consumeLeftClickIn(x, y, x+w, y+h) {
				if ih.dialogDoubleClick(i) {
					ih.purchaseSelectedSpell()
				} else {
					ih.game.dialogSelectedSpell = i
					ih.game.selectedSpellKey = spellKey
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
					if ih.dialogDoubleClick(i) {
						ih.purchaseSelectedTraining()
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

	// Check if clicking to buy/sell items (if NPC is merchant)
	if ih.game.dialogNPC != nil && npcHasMerchant(ih.game.dialogNPC) {
		_, _, _, _, listY, leftX, rightX, colW, rowH := merchantDialogLayout(screenWidth, screenHeight)
		maxItems := 12

		// Buy from merchant (left list)
		for i := 0; i < len(ih.game.dialogNPC.MerchantStock) && i < maxItems; i++ {
			y := listY + i*rowH
			if ih.game.consumeLeftClickIn(leftX-2, y-2, leftX+colW+1, y-2+rowH+1) {
				if ih.dialogDoubleClick(i) {
					entry := ih.game.dialogNPC.MerchantStock[i]
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
					return
				}
				return
			}
		}

		// Sell to merchant (right list)
		if ih.game.dialogNPC.SellAvailable {
			for i := 0; i < len(ih.game.party.Inventory) && i < maxItems; i++ {
				y := listY + i*rowH
				if ih.game.consumeLeftClickIn(rightX-2, y-2, rightX+colW+1, y-2+rowH+1) {
					if ih.dialogDoubleClick(i) {
						item := ih.game.party.Inventory[i]
						base := item.Attributes["value"]
						if base <= 0 {
							ih.game.AddCombatMessage("This item has no value.")
							return
						}
						price := ih.game.merchantSellPrice(base) // Merchant skill markup
						ih.game.party.Gold += price
						ih.game.party.RemoveItem(i)
						ih.game.AddCombatMessage(fmt.Sprintf("Sold %s for %d gold.", item.Name, price))
						return
					}
					return
				}
			}
		}
		return
	}

	// Check if clicking on encounter choices (if NPC is encounter type)
	if ih.game.dialogNPC != nil && npcHasChoiceDialog(ih.game.dialogNPC) {
		npc := ih.game.dialogNPC
		// Use the SAME state-filtered choices + body text as drawEncounterDialog so
		// click positions line up with what's actually drawn.
		choices := ih.game.visibleNPCChoices(npc)
		if npc.DialogueData != nil && len(choices) > 0 {
			{
				lines := wrapText(ih.game.npcDialogueText(npc), 70)
				choicesY := dialogY + 50 + len(lines)*16 + 20

				// Add space for choice prompt if it exists
				if npc.DialogueData.ChoicePrompt != "" {
					choicesY += 20
				}

				for i := range choices {
					choiceY := choicesY + i*25

					// Check if mouse is over this choice entry (clickable from text start)
					if ih.game.consumeLeftClickIn(dialogX-20, choiceY-2, dialogX+dialogWidth+1, choiceY+22+1) {

						// Check for double-click to execute choice immediately (neutral tracking)
						currentTime := time.Now().UnixMilli()
						delta := currentTime - ih.game.dialogLastClickTime
						doubleClick := ih.game.selectedChoice == i &&
							delta < doubleClickWindowMs
						if doubleClick {
							// Double-click detected - execute the choice
							ih.executeEncounterChoice()
						} else {
							// Single click - just select the choice
							ih.game.selectedChoice = i
						}

						// Update click tracking
						ih.game.dialogLastClickTime = currentTime
						return
					}
				}
			}
		}
	}
}

func (ih *InputHandler) dialogDoubleClick(index int) bool {
	currentTime := time.Now().UnixMilli()
	delta := currentTime - ih.game.dialogLastClickTime
	doubleClick := ih.game.dialogLastClickedIdx == index && delta < doubleClickWindowMs
	ih.game.dialogLastClickTime = currentTime
	ih.game.dialogLastClickedIdx = index
	return doubleClick
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
			ih.game.turnBasedMoveCooldown = 18 // 0.3 second at 60 FPS
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
			ih.game.turnBasedRotCooldown = 18 // 0.3 second at 60 FPS
		}
	}

	if moved {
		// Movement is a party-wide commitment: it spends EVERY remaining
		// action slot and ends the round immediately.
		for _, m := range ih.game.party.Members {
			m.ActionsRemaining = 0
		}
		ih.game.endPartyTurn()
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
		ih.game.combat.EquipmentMeleeAttack()
		ih.game.consumeSelectedCharAction()
		ih.game.spellInputCooldown = ih.actionCooldown(15)
	case ih.spaceKeyTracker.IsKeyJustPressed(ebiten.KeySpace): // smart attack
		if ih.game.tryPickupNearestGroundContainer(ih.game.groundContainerPickupRange()) {
			return
		}
		ih.game.combat.SmartAttack()
		ih.game.consumeSelectedCharAction()
		ih.game.spellInputCooldown = ih.actionCooldown(15)
	case ih.fKeyTracker.IsKeyJustPressed(ebiten.KeyF): // cast slotted spell
		spell, hasSpell := selected.Equipment[items.SlotSpell]
		if hasSpell && (spell.SpellEffect == items.SpellEffectHealSelf || spell.SpellEffect == items.SpellEffectHealOther) {
			mouseX, mouseY := ebiten.CursorPosition()
			targetCharIndex := ih.resolveHealTarget(spell, mouseX, mouseY)
			if ih.game.combat.CastEquippedHealOnTarget(targetCharIndex) {
				ih.game.consumeSelectedCharAction()
			}
			ih.game.spellInputCooldown = ih.actionCooldown(15)
		} else if ih.game.combat.CastEquippedSpell() {
			ih.game.consumeSelectedCharAction()
			ih.game.spellInputCooldown = ih.actionCooldown(15)
		}
	case ih.cKeyTracker.IsKeyJustPressed(ebiten.KeyC) || ih.hKeyTracker.IsKeyJustPressed(ebiten.KeyH): // cast best known heal (H = legacy alias)
		mouseX, mouseY := ebiten.CursorPosition()
		targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)
		if targetCharIndex < 0 {
			targetCharIndex = ih.game.selectedChar
		}
		if cast, _ := ih.game.combat.CastBestHealOnTarget(targetCharIndex); cast {
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
	spellKeys := npcSpellKeys(ih.game.dialogNPC)

	if ebiten.IsKeyPressed(ebiten.KeyUp) && ih.game.spellInputCooldown == 0 {
		ih.navigateSpellSelectionUp(spellKeys)
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) && ih.game.spellInputCooldown == 0 {
		ih.navigateSpellSelectionDown(spellKeys)
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

	case "summon_dragon":
		ih.summonDragonFromStatue(npc, choice.SummonIndex)

	case "open_roster":
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		ih.game.rosterScreenOpen = true
		ih.game.rosterSelectedActive = -1

	default:
		// Unknown action - just close dialog
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
	}
}

// npcIsQuestGiver reports whether an NPC offers any give_quest/turn_in_quest
// choice — used to reject Lich party members from those NPCs (the Mage Tower).
func npcIsQuestGiver(npc *character.NPC) bool {
	if npc == nil || npc.DialogueData == nil {
		return false
	}
	for _, c := range npc.DialogueData.Choices {
		if c.Action == "give_quest" || c.Action == "turn_in_quest" {
			return true
		}
	}
	return false
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
			g.AddCombatMessage("The trial is already underway — return when the Lich King is slain.")
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
	}
	g.AddCombatMessage(fmt.Sprintf("Quest accepted: %s", name))
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
		g.AddCombatMessage("That task isn't finished yet — return when it's done.")
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
		g.AddCombatMessage("The valve won't budge — no reason to shut it yet.")
		return
	}
	completed := g.questManager.OnInteract(q.Definition.TargetMonster)
	npc.Visited = true // this valve stays shut and can't be re-counted
	g.AddCombatMessage(fmt.Sprintf("You heave the valve shut. (%s)", q.GetProgressString()))
	for _, cq := range completed {
		g.AddCombatMessage(fmt.Sprintf("Quest '%s' complete! The flood drains from the lair.", cq.Definition.Name))
	}
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
	ih.game.camera.X = x
	ih.game.camera.Y = y
	ih.game.camera.Angle = angle
	if ih.game.collisionSystem != nil {
		ih.game.collisionSystem.UpdateEntity("player", x, y)
	}
	// Turn-based facing must be cardinal. A restored return-pose angle can be a
	// free real-time heading (diagonal), which would leave the party at 45° on
	// the new map — snap it to the nearest cardinal in TB.
	if ih.game.turnBasedMode {
		ih.game.snapToCardinalDirection()
	}
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
				fmt.Printf("Spawned %s at walkable position (%.1f, %.1f) with AI enabled\n", monsterDef.Type, spawnX, spawnY)
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

		// Only return if the exact candidate position is walkable
		if ih.isPositionWalkable(candidateX, candidateY) {
			return candidateX, candidateY
		}
	}

	// No fallback - if we can't find a good spot, don't spawn the monster
	fmt.Printf("Warning: Could not find walkable spawn location near NPC at (%.1f, %.1f)\n", npcX, npcY)
	return 0, 0 // Return invalid coordinates
}

// isPositionWalkable checks if a specific position is walkable (DRY helper)
func (ih *InputHandler) isPositionWalkable(x, y float64) bool {
	worldInst := ih.game.GetCurrentWorld()
	if worldInst == nil {
		return false
	}

	tileSize := float64(ih.game.config.GetTileSize())
	// Treat negative positions as out of bounds
	if x < 0 || y < 0 {
		return false
	}
	tileX := int(x / tileSize)
	tileY := int(y / tileSize)

	if tileX >= 0 && tileX < worldInst.Width && tileY >= 0 && tileY < worldInst.Height {
		tile := worldInst.Tiles[tileY][tileX]
		return world.GlobalTileManager != nil && world.GlobalTileManager.IsWalkable(tile)
	}
	return false
}
