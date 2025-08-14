package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"ugataima/internal/game/inpututil"

	"github.com/hajimehoshi/ebiten/v2"
)

// InputHandler handles all user input for the game
type InputHandler struct {
	game                 *MMGame
	slashKeyTracker      inpututil.KeyStateTracker
	apostropheKeyTracker inpututil.KeyStateTracker
	enterKeyTracker      inpututil.KeyStateTracker
	escapeKeyTracker     inpututil.KeyStateTracker
	upKeyTracker         inpututil.KeyStateTracker
	downKeyTracker       inpututil.KeyStateTracker
}

// NewInputHandler creates a new input handler
func NewInputHandler(game *MMGame) *InputHandler {
    return &InputHandler{game: game}
}

// actionCooldown returns the number of frames to wait before the next action,
// using Speed-based anchors: Speed 5 => ~60 frames (1 sec), Speed 50 => ~30 frames (0.5 sec).
// Scales linearly between/around anchors and clamps to [15, 90] frames. The `base` arg is ignored.
func (ih *InputHandler) actionCooldown(_ int) int {
    selected := ih.game.party.Members[ih.game.selectedChar]
    // Use effective stats to include buffs/items
    _, _, _, _, _, speed, _ := selected.GetEffectiveStats(ih.game.statBonus)
    // Linear fit through points: (5,60) and (50,30)
    frames := 63.333333 - (2.0/3.0)*float64(speed)
    cd := int(math.Round(frames))
    if cd < 15 {
        cd = 15
    }
    if cd > 90 {
        cd = 90
    }
    return cd
}

// HandleInput processes all input for the current frame
func (ih *InputHandler) HandleInput() {
    // When game over, only allow New Game or Load
    if ih.game.gameOver {
        if ebiten.IsKeyPressed(ebiten.KeyN) {
            ih.restartNewGame()
            return
        }
        if ebiten.IsKeyPressed(ebiten.KeyL) {
            ih.game.mainMenuOpen = true
            ih.game.mainMenuMode = MenuLoadSelect
            return
        }
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
		// Close dialog if open
		if ih.game.dialogActive {
			ih.game.dialogActive = false
			ih.game.dialogNPC = nil
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
    // Recreate party
    ih.game.party = character.NewParty(ih.game.config)
    // Reset game over state
    ih.game.gameOver = false
    // Reset dialog/menu states
    ih.game.dialogActive = false
    ih.game.menuOpen = false
    // Move player to start position
    if ih.game.GetCurrentWorld() != nil {
        startX, startY := ih.game.GetCurrentWorld().GetStartingPosition()
        ih.game.camera.X = startX
        ih.game.camera.Y = startY
        ih.game.collisionSystem.UpdateEntity("player", startX, startY)
    }
}

// handleMainMenuInput processes input for the main menu (opened with ESC)
func (ih *InputHandler) handleMainMenuInput() {
	// Mouse position for hover/click
	mouseX, mouseY := ebiten.CursorPosition()

	switch ih.game.mainMenuMode {
	case MenuMain:
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
		ih.mainMenuHoverSelect(mouseX, mouseY, len(mainMenuOptions), 300, 220, 56)

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
			case 3: // Exit
				ih.game.exitRequested = true
			}
		}

		// Mouse click activation
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ih.game.mousePressed {
			ih.game.mousePressed = true
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
				ih.game.exitRequested = true
			}
		}
	case MenuSaveSelect:
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
		ih.mainMenuHoverSelect(mouseX, mouseY, 5, 300, 220, 56)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			if err := ih.game.SaveGameToFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Save failed")
			} else {
				ih.game.AddCombatMessage("Saved to slot")
				ih.game.mainMenuMode = MenuMain
			}
		}
		// Mouse click activation
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ih.game.mousePressed {
			ih.game.mousePressed = true
			if err := ih.game.SaveGameToFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Save failed")
			} else {
				ih.game.AddCombatMessage("Saved to slot")
				ih.game.mainMenuMode = MenuMain
			}
		}
	case MenuLoadSelect:
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
		ih.mainMenuHoverSelect(mouseX, mouseY, 5, 300, 220, 56)
		if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
			if err := ih.game.LoadGameFromFile(slotPath(ih.game.slotSelection)); err != nil {
				ih.game.AddCombatMessage("Load failed")
			} else {
				ih.game.AddCombatMessage("Loaded from slot")
				ih.game.mainMenuOpen = false
				ih.game.mainMenuMode = MenuMain
			}
		}
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ih.game.mousePressed {
			ih.game.mousePressed = true
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

// handleMovementInput processes movement and camera controls
func (ih *InputHandler) handleMovementInput() {
	// Rotation
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		ih.game.camera.Angle -= ih.game.config.GetRotSpeed()
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		ih.game.camera.Angle += ih.game.config.GetRotSpeed()
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

// handleCombatInput processes combat-related input
func (ih *InputHandler) handleCombatInput() {
    // Magic attack (F key) - single press with cooldown to prevent spam
    if ebiten.IsKeyPressed(ebiten.KeyF) && ih.game.spellInputCooldown == 0 {
        ih.game.combat.CastEquippedSpell()
        ih.game.spellInputCooldown = ih.actionCooldown(ih.game.config.UI.SpellInputCooldown)
    }

    // Melee attack (Space key) - with cooldown to prevent spam
    if ebiten.IsKeyPressed(ebiten.KeySpace) && ih.game.spellInputCooldown == 0 {
        ih.game.combat.EquipmentMeleeAttack()
        ih.game.spellInputCooldown = ih.actionCooldown(15) // base ~0.25s at 60 FPS
    }

	// Note: H key healing is handled in handleMouseInput for proper targeting
}

// handleCharacterSelectionInput processes party character selection
func (ih *InputHandler) handleCharacterSelectionInput() {
	if ebiten.IsKeyPressed(ebiten.Key1) {
		ih.game.selectedChar = 0
	}
	if ebiten.IsKeyPressed(ebiten.Key2) {
		ih.game.selectedChar = 1
	}
	if ebiten.IsKeyPressed(ebiten.Key3) {
		ih.game.selectedChar = 2
	}
	if ebiten.IsKeyPressed(ebiten.Key4) {
		ih.game.selectedChar = 3
	}
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
	if ebiten.IsKeyPressed(ebiten.KeyM) && ih.game.spellInputCooldown == 0 {
		if ih.game.menuOpen {
			if ih.game.currentTab == TabSpellbook {
				ih.game.menuOpen = false // Close menu if already on Spellbook tab
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
			} else {
				ih.game.currentTab = TabSpellbook
			}
		} else {
			ih.openTabbedMenu(TabSpellbook)
		}
	}
	if ebiten.IsKeyPressed(ebiten.KeyI) && ih.game.spellInputCooldown == 0 {
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
	if ebiten.IsKeyPressed(ebiten.KeyC) && ih.game.spellInputCooldown == 0 {
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

	// Handle NPC interaction with T key
    if ebiten.IsKeyPressed(ebiten.KeyT) && ih.game.spellInputCooldown == 0 {
        ih.handleNPCInteraction()
        ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
    }

	// Toggle turn-based mode with Enter key
	if ih.enterKeyTracker.IsKeyJustPressed(ebiten.KeyEnter) {
		ih.toggleTurnBasedMode()
	}
}

// handleSpellbookInput processes spellbook navigation and casting
// Movement helper methods
func (ih *InputHandler) moveForward() {
	newX := ih.game.camera.X + ih.game.camera.GetForwardX()*ih.game.config.GetMoveSpeed()
	newY := ih.game.camera.Y + ih.game.camera.GetForwardY()*ih.game.config.GetMoveSpeed()
	if ih.game.collisionSystem.CanMoveTo("player", newX, newY) {
		ih.game.camera.X = newX
		ih.game.camera.Y = newY
		ih.game.collisionSystem.UpdateEntity("player", newX, newY)
		ih.checkTeleporter()
		ih.checkDeepWater()
	}
}

func (ih *InputHandler) moveBackward() {
	newX := ih.game.camera.X - ih.game.camera.GetForwardX()*ih.game.config.GetMoveSpeed()
	newY := ih.game.camera.Y - ih.game.camera.GetForwardY()*ih.game.config.GetMoveSpeed()
	if ih.game.collisionSystem.CanMoveTo("player", newX, newY) {
		ih.game.camera.X = newX
		ih.game.camera.Y = newY
		ih.game.collisionSystem.UpdateEntity("player", newX, newY)
		ih.checkTeleporter()
		ih.checkDeepWater()
	}
}

func (ih *InputHandler) strafeLeft() {
	newX := ih.game.camera.X + ih.game.camera.GetRightX()*-ih.game.config.GetMoveSpeed()
	newY := ih.game.camera.Y + ih.game.camera.GetRightY()*-ih.game.config.GetMoveSpeed()
	if ih.game.collisionSystem.CanMoveTo("player", newX, newY) {
		ih.game.camera.X = newX
		ih.game.camera.Y = newY
		ih.game.collisionSystem.UpdateEntity("player", newX, newY)
		ih.checkTeleporter()
		ih.checkDeepWater()
	}
}

func (ih *InputHandler) strafeRight() {
	newX := ih.game.camera.X + ih.game.camera.GetRightX()*ih.game.config.GetMoveSpeed()
	newY := ih.game.camera.Y + ih.game.camera.GetRightY()*ih.game.config.GetMoveSpeed()
	if ih.game.collisionSystem.CanMoveTo("player", newX, newY) {
		ih.game.camera.X = newX
		ih.game.camera.Y = newY
		ih.game.collisionSystem.UpdateEntity("player", newX, newY)
		ih.checkTeleporter()
		ih.checkDeepWater()
	}
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
	}

	ih.game.camera.X = newX
	ih.game.camera.Y = newY
	ih.game.collisionSystem.UpdateEntity("player", newX, newY)
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
	var ttype string
	switch tile {
	case world.TileVioletTeleporter:
		ttype = "violet"
	case world.TileRedTeleporter:
		ttype = "red"
	default:
		return "", x, y, false
	}
	reg := world.GlobalWorldManager.GlobalTeleporterRegistry
	if time.Since(reg.LastUsedTime) < reg.CooldownPeriod {
		return "", x, y, false
	}
	count := 0
	for _, tel := range reg.Teleporters {
		if tel.Type == ttype {
			count++
		}
	}
	if count < 2 {
		return "", x, y, false
	}
	var dests []world.TeleporterLocation
	for _, tel := range reg.Teleporters {
		if tel.Type == ttype && (tel.MapKey != world.GlobalWorldManager.CurrentMapKey || tel.X != tx || tel.Y != ty) {
			dests = append(dests, tel)
		}
	}
	if len(dests) == 0 {
		return "", x, y, false
	}
	d := dests[rand.Intn(len(dests))]
	reg.LastUsedTime = time.Now()
	nx := float64(d.X)*tileSize + tileSize/2
	ny := float64(d.Y)*tileSize + tileSize/2
	return d.MapKey, nx, ny, true
}

// switchToMap handles common map switching logic for teleporters and spell effects
func (ih *InputHandler) switchToMap(targetMapKey string) {
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
		centerX := float64(25)*tileSize + tileSize/2
		centerY := float64(25)*tileSize + tileSize/2
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

func (ih *InputHandler) navigateSpellbookUp(schools []character.MagicSchool) {
	currentChar := ih.game.party.Members[ih.game.selectedChar]

	if ih.game.selectedSpell > 0 {
		ih.game.selectedSpell--
	} else if ih.game.selectedSchool > 0 {
		// Move to previous school
		ih.game.selectedSchool--
		ih.game.selectedSpell = len(currentChar.GetSpellsForSchool(schools[ih.game.selectedSchool])) - 1
	}
}

func (ih *InputHandler) navigateSpellbookDown(schools []character.MagicSchool) {
	currentChar := ih.game.party.Members[ih.game.selectedChar]
	currentSpells := currentChar.GetSpellsForSchool(schools[ih.game.selectedSchool])

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
	// Get mouse position
	mouseX, mouseY := ebiten.CursorPosition()

	// Handle heal targeting when H key is pressed (only when menu is not open and cooldown is ready)
    if !ih.game.menuOpen && ebiten.IsKeyPressed(ebiten.KeyH) && ih.game.spellInputCooldown == 0 {
		caster := ih.game.party.Members[ih.game.selectedChar]

		// Check if character has a heal spell equipped
		spell, hasSpell := caster.Equipment[items.SlotSpell]
		if hasSpell && (spell.SpellEffect == items.SpellEffectHealSelf || spell.SpellEffect == items.SpellEffectHealOther) {
			targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)

			if targetCharIndex >= 0 {
				// Check if the spell can target others or if targeting self
                if spell.SpellEffect == items.SpellEffectHealOther || targetCharIndex == ih.game.selectedChar {
                    ih.game.combat.CastEquippedHealOnTarget(targetCharIndex)
                    ih.game.spellInputCooldown = ih.actionCooldown(ih.game.config.UI.SpellInputCooldown)
                } else {
					// Self-only spell (First Aid) but targeting someone else - fallback to self-heal
                    ih.game.combat.EquipmentHeal()
                    ih.game.spellInputCooldown = ih.actionCooldown(ih.game.config.UI.SpellInputCooldown)
                }
            } else {
                // No target under mouse, heal self (original behavior)
                ih.game.combat.EquipmentHeal()
                ih.game.spellInputCooldown = ih.actionCooldown(ih.game.config.UI.SpellInputCooldown)
            }
        }
    }

	// Handle party character selection clicks (works both in and out of menu)
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ih.game.mousePressed {
		targetCharIndex := ih.getPartyMemberUnderMouse(mouseX, mouseY)
		if targetCharIndex >= 0 {
			ih.game.selectedChar = targetCharIndex
			ih.game.mousePressed = true // Prevent multiple clicks
		}
	}

	// Reset mouse state when button is released
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ih.game.mousePressed = false
	}
}

// getPartyMemberUnderMouse returns the index of the party member under the mouse cursor
// Returns -1 if no party member is under the cursor
func (ih *InputHandler) getPartyMemberUnderMouse(mouseX, mouseY int) int {
	if !ih.game.showPartyStats {
		return -1
	}

	// Calculate party UI layout (same as UI system)
	portraitWidth := ih.game.config.GetScreenWidth() / 4
	portraitHeight := ih.game.config.UI.PartyPortraitHeight
	startY := ih.game.config.GetScreenHeight() - portraitHeight

	// Check if mouse is in party UI area
	if mouseY < startY || mouseY >= startY+portraitHeight {
		return -1
	}

	// Determine which character portrait the mouse is over
	charIndex := mouseX / portraitWidth
	if charIndex >= 0 && charIndex < len(ih.game.party.Members) {
		// Check if the click is on the + button area (exclude it from character selection)
		member := ih.game.party.Members[charIndex]
		if member.FreeStatPoints > 0 {
			x := charIndex * portraitWidth
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
		return charIndex
	}

	return -1
}

// openTabbedMenu opens the tabbed menu with the specified tab
func (ih *InputHandler) openTabbedMenu(tab MenuTab) {
	ih.game.menuOpen = true
	ih.game.currentTab = tab
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
	schools := currentChar.GetAvailableSchools()

	if len(schools) == 0 {
		return
	}

	// Navigation
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		ih.navigateSpellbookUp(schools)
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		ih.navigateSpellbookDown(schools)
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Cast spell
	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		ih.game.combat.EquipSelectedSpell()
		ih.game.menuOpen = false
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
}

// handleNPCInteraction handles talking to nearby NPCs
func (ih *InputHandler) handleNPCInteraction() {
	// Find nearby NPCs within interaction distance
	const interactionDistance = 128.0 // 2 tiles

	for _, npc := range ih.game.GetCurrentWorld().NPCs {
		dx := npc.X - ih.game.camera.X
		dy := npc.Y - ih.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= interactionDistance {
			// Start dialog with this NPC
			ih.game.dialogActive = true
			ih.game.dialogNPC = npc
			ih.game.selectedCharIdx = 0     // Default to first character
			ih.game.dialogSelectedChar = 0  // Ensure dialog selection is also set
			ih.game.dialogSelectedSpell = 0 // Default to first spell
			ih.game.selectedSpellKey = ""   // No spell selected initially
			ih.game.selectedChoice = 0      // Reset encounter choice selection

			// If NPC has spells, select the first one (deterministic order)
			if npc.Type == "spell_trader" && len(npc.SpellData) > 0 {
				spellKeys := ih.getAvailableSpellKeys() // Use deterministic ordering
				if len(spellKeys) > 0 {
					ih.game.selectedSpellKey = spellKeys[0]
				}
			}
			return
		}
	}
}

// handleDialogInput handles input when in dialog mode
func (ih *InputHandler) handleDialogInput() {
	// Handle mouse input for character selection
	ih.handleDialogMouseInput()

	// Close dialog with Escape
	if ebiten.IsKeyPressed(ebiten.KeyEscape) && ih.game.spellInputCooldown == 0 {
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
		return
	}

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

	// Handle different NPC types
	if ih.game.dialogNPC != nil {
		switch ih.game.dialogNPC.Type {
		case "spell_trader":
			ih.handleSpellTraderInput()
		case "encounter":
			ih.handleEncounterInput()
		}
	}

	// Reset mouse state when button is released
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ih.game.mousePressed = false
	}
}

// getAvailableSpellKeys returns the list of spell keys available from the current NPC in deterministic order
func (ih *InputHandler) getAvailableSpellKeys() []string {
	if ih.game.dialogNPC == nil || ih.game.dialogNPC.SpellData == nil {
		return []string{}
	}

	keys := make([]string, 0, len(ih.game.dialogNPC.SpellData))
	for key := range ih.game.dialogNPC.SpellData {
		keys = append(keys, key)
	}

	// Sort keys to ensure deterministic ordering and prevent UI blinking
	sort.Strings(keys)

	return keys
}

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
	if ih.characterKnowsSpell(selectedChar, spellData.Name) {
		ih.game.AddCombatMessage(fmt.Sprintf("%s already knows %s!", selectedChar.Name, spellData.Name))
		return
	}

	// Check if character has enough gold
	if ih.game.party.Gold < spellData.Cost {
		ih.game.AddCombatMessage(fmt.Sprintf("Need %d gold to learn %s", spellData.Cost, spellData.Name))
		return
	}

	// Check if character can learn this spell (class restrictions) - reuse UI logic
	if !ih.game.gameLoop.ui.characterCanLearnSpell(selectedChar, spellData) {
		ih.game.AddCombatMessage(fmt.Sprintf("%s cannot learn %s (wrong class/school)", selectedChar.Name, spellData.Name))
		return
	}

	// Purchase the spell
	ih.game.party.Gold -= spellData.Cost

	// Add spell to character's spellbook
	ih.addSpellToCharacter(selectedChar, spellData)

	ih.game.AddCombatMessage(fmt.Sprintf("%s learned %s!", selectedChar.Name, spellData.Name))
}

// characterKnowsSpell checks if a character already knows a spell
func (ih *InputHandler) characterKnowsSpell(char *character.MMCharacter, spellName string) bool {
	for _, magicSkill := range char.MagicSchools {
		for _, spellID := range magicSkill.KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.Name == spellName {
				return true
			}
		}
	}
	return false
}

// addSpellToCharacter adds a spell to a character's spellbook
func (ih *InputHandler) addSpellToCharacter(char *character.MMCharacter, spellData *character.NPCSpell) {
	// Find the appropriate magic school for the spell
	var targetSchool character.MagicSchool

	// Dynamic school string to enum conversion (no more hardcoded switches!)
	schoolID := character.MagicSchoolID(spellData.School)
	targetSchool = character.MagicSchoolIDToLegacy(schoolID)

	// Ensure the character has the magic school
	if char.MagicSchools[targetSchool] == nil {
		char.MagicSchools[targetSchool] = &character.MagicSkill{
			Level:       1,
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
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || ih.game.mousePressed {
		return
	}

	// Get mouse position
	mouseX, mouseY := ebiten.CursorPosition()

	// Calculate dialog coordinates (same as in UI)
	screenWidth := ih.game.config.GetScreenWidth()
	screenHeight := ih.game.config.GetScreenHeight()
	dialogWidth := 600
	dialogHeight := 400
	dialogX := (screenWidth - dialogWidth) / 2
	dialogY := (screenHeight - dialogHeight) / 2

    // Check if clicking on spells (if NPC is spell trader)
    if ih.game.dialogNPC != nil && ih.game.dialogNPC.Type == "spell_trader" {

		// Check if clicking on character list
		for i := range ih.game.party.Members {
			charY := dialogY + 100 + (i * 25)

			// Check if mouse is over this character entry
			if mouseX >= dialogX+20 && mouseX <= dialogX+320 &&
				mouseY >= charY-2 && mouseY <= charY+22 {
				ih.game.selectedCharIdx = i
				ih.game.mousePressed = true
				return
			}
		}

		spellsY := dialogY + 100 + (len(ih.game.party.Members) * 25) + 20
		spellKeys := ih.getAvailableSpellKeys()

		for spellIndex, spellKey := range spellKeys {
			spellY := spellsY + 20 + (spellIndex * 25)

			// Check if mouse is over this spell entry
			if mouseX >= dialogX+20 && mouseX <= dialogX+370 &&
				mouseY >= spellY-2 && mouseY <= spellY+22 {

        // Check for double-click to purchase spell directly (neutral dialog tracking)
        currentTime := time.Now().UnixMilli()
        if ih.game.dialogLastClickedIdx == spellIndex &&
            currentTime-ih.game.dialogLastClickTime < 500 {
            // Double-click detected - purchase the spell
            ih.purchaseSelectedSpell()
        } else {
            // Single click - just select the spell
            ih.game.dialogSelectedSpell = spellIndex
            ih.game.selectedSpellKey = spellKey
        }

        // Update click tracking for dialog spells
        ih.game.dialogLastClickTime = currentTime
        ih.game.dialogLastClickedIdx = spellIndex
        ih.game.mousePressed = true
        return
        }
        }
    }

    // Check if clicking to sell items (if NPC is merchant)
    if ih.game.dialogNPC != nil && ih.game.dialogNPC.Type == "merchant" {
        dialogWidth := 600
        dialogHeight := 400
        dialogX := (screenWidth - dialogWidth) / 2
        dialogY := (screenHeight - dialogHeight) / 2

        // Inventory list region mirrors drawMerchantDialog
        listY := dialogY + 90 + 20
        maxItems := 15
        for i := 0; i < len(ih.game.party.Inventory) && i < maxItems; i++ {
            y := listY + i*25
            if mouseX >= dialogX+18 && mouseX <= dialogX+dialogWidth-18 && mouseY >= y-2 && mouseY <= y-2+20 {
                if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ih.game.mousePressed {
                    currentTime := time.Now().UnixMilli()
                    // Use neutral dialog click tracking to detect double-click per index
                    if ih.game.dialogLastClickedIdx == i && currentTime-ih.game.dialogLastClickTime < 500 {
                        ih.game.mousePressed = true
                        // Double-click detected - sell the item for its value
                        item := ih.game.party.Inventory[i]
                        price := item.Attributes["value"]
                        if price <= 0 {
                            ih.game.AddCombatMessage("This item has no value.")
                            return
                        }
                        ih.game.party.Gold += price
                        ih.game.party.RemoveItem(i)
                        ih.game.AddCombatMessage(fmt.Sprintf("Sold %s for %d gold.", item.Name, price))
                        return
                    }
                    // Single click - select (no-op visual for now), store click tracking
                    ih.game.dialogLastClickTime = currentTime
                    ih.game.dialogLastClickedIdx = i
                    ih.game.mousePressed = true
                    return
                }
            }
        }
        return
    }

    // Check if clicking on encounter choices (if NPC is encounter type)
    if ih.game.dialogNPC != nil && ih.game.dialogNPC.Type == "encounter" {
		npc := ih.game.dialogNPC
		if npc.DialogueData != nil && len(npc.DialogueData.Choices) > 0 {
			// Skip if already visited and encounter is first-visit-only
			if !(npc.Visited && npc.EncounterData != nil && npc.EncounterData.FirstVisitOnly) {
				// Calculate position of choices (matching drawEncounterDialog exactly)
				greeting := npc.DialogueData.Greeting
				lines := ih.wrapTextForDialog(greeting, 70)
				choicesY := dialogY + 50 + len(lines)*16 + 20

				// Add space for choice prompt if it exists
				if npc.DialogueData.ChoicePrompt != "" {
					choicesY += 20
				}

				for i := range npc.DialogueData.Choices {
					choiceY := choicesY + i*25

					// Check if mouse is over this choice entry (clickable from text start)
					if mouseX >= dialogX-20 && mouseX <= dialogX+dialogWidth &&
						mouseY >= choiceY-2 && mouseY <= choiceY+22 {

                    // Check for double-click to execute choice immediately (neutral tracking)
                    currentTime := time.Now().UnixMilli()
                    if ih.game.selectedChoice == i &&
                        currentTime-ih.game.dialogLastClickTime < 500 {
                        // Double-click detected - execute the choice
                        ih.executeEncounterChoice()
                    } else {
                        // Single click - just select the choice
                        ih.game.selectedChoice = i
                    }

                    // Update click tracking
                    ih.game.dialogLastClickTime = currentTime
                    ih.game.mousePressed = true
                    return
                }
            }
            }
		}
	}
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

	// Safeguard: if partyActionsUsed is somehow > 2, reset it to prevent soft-lock
	if ih.game.partyActionsUsed > 2 {
		ih.game.partyActionsUsed = 2
	}

	// Party can move 1 tile OR 2 characters can attack
	if ih.game.partyActionsUsed >= 2 {
		return // Turn is over
	}

	// Handle movement (counts as using the turn) - only if cooldown is ready
	moved := false
	if ih.game.turnBasedMoveCooldown == 0 {
		if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
			moved = ih.moveTurnBasedForward()
		} else if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
			moved = ih.moveTurnBasedBackward()
		} else if ebiten.IsKeyPressed(ebiten.KeyQ) {
			moved = ih.moveTurnBasedStrafeLeft()
		} else if ebiten.IsKeyPressed(ebiten.KeyE) {
			moved = ih.moveTurnBasedStrafeRight()
		}

		if moved {
			ih.game.turnBasedMoveCooldown = 18 // 0.3 second at 60 FPS
		}
	}

	// 90-degree rotation with cooldown (doesn't use turn)
	if ih.game.turnBasedRotCooldown == 0 {
		rotated := false
		if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
			ih.rotateTurnBased(-1) // Counter-clockwise
			rotated = true
		} else if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
			ih.rotateTurnBased(1) // Clockwise
			rotated = true
		}

		if rotated {
			ih.game.turnBasedRotCooldown = 18 // 0.3 second at 60 FPS
		}
	}

	if moved {
		ih.endPartyTurn()
		return
	}

	// Handle attacks (any 2 attacks from any party members)
    // Only allow actions if selected character is conscious
    selected := ih.game.party.Members[ih.game.selectedChar]
    canAct := !(selected.HitPoints <= 0)
    for _, cond := range selected.Conditions { if cond == character.ConditionUnconscious { canAct = false; break } }

    if canAct && ebiten.IsKeyPressed(ebiten.KeySpace) && ih.game.spellInputCooldown == 0 {
        ih.game.combat.EquipmentMeleeAttack()
        ih.game.partyActionsUsed++
        ih.game.spellInputCooldown = ih.actionCooldown(15)

		if ih.game.partyActionsUsed >= 2 {
			ih.endPartyTurn()
		}
	}

    if canAct && ebiten.IsKeyPressed(ebiten.KeyF) && ih.game.spellInputCooldown == 0 {
        ih.game.combat.CastEquippedSpell()
        ih.game.partyActionsUsed++
        ih.game.spellInputCooldown = ih.actionCooldown(15)

		if ih.game.partyActionsUsed >= 2 {
			ih.endPartyTurn()
		}
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
	targetX := float64(targetTileX)*tileSize + tileSize/2
	targetY := float64(targetTileY)*tileSize + tileSize/2

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
	worldX := float64(tileX)*tileSize + tileSize/2
	worldY := float64(tileY)*tileSize + tileSize/2

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

// endPartyTurn ends the party's turn and starts monster turn
func (ih *InputHandler) endPartyTurn() {
	// Regenerate 1 mana for all party members at end of turn
	for _, member := range ih.game.party.Members {
		if member.SpellPoints < member.MaxSpellPoints {
			member.SpellPoints++
		}
	}

	ih.game.currentTurn = 1 // Monster turn
	// Don't spam combat log with turn messages
}

// handleSpellTraderInput handles input for spell trader NPCs
func (ih *InputHandler) handleSpellTraderInput() {
	spellKeys := ih.getAvailableSpellKeys()

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

// handleEncounterInput handles input for encounter NPCs
func (ih *InputHandler) handleEncounterInput() {
	npc := ih.game.dialogNPC
	if npc.DialogueData == nil || len(npc.DialogueData.Choices) == 0 {
		return
	}

	// Skip interaction if already visited and encounter is first-visit-only
	if npc.Visited && npc.EncounterData != nil && npc.EncounterData.FirstVisitOnly {
		return
	}

	// Navigate choices with Up/Down arrows
	if ebiten.IsKeyPressed(ebiten.KeyUp) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedChoice > 0 {
			ih.game.selectedChoice--
		}
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) && ih.game.spellInputCooldown == 0 {
		if ih.game.selectedChoice < len(npc.DialogueData.Choices)-1 {
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
	for i := 0; i < len(npc.DialogueData.Choices) && i < 9; i++ {
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
	if npc.DialogueData == nil || ih.game.selectedChoice >= len(npc.DialogueData.Choices) {
		return
	}

	choice := npc.DialogueData.Choices[ih.game.selectedChoice]

	switch choice.Action {
	case "leave":
		// Close dialog and leave
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil

	case "combat":
		// Start encounter combat
		ih.startEncounter()

	default:
		// Unknown action - just close dialog
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
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

	// Spawn monsters near the encounter location
	ih.spawnEncounterMonsters(npc)

	// Add combat message
	ih.game.AddCombatMessage("Bandits emerge from the shipwreck to attack!")
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
				ih.game.world.Monsters = append(ih.game.world.Monsters, monster)

				// Register monster with collision system for proper AI movement
				width, height := monster.GetSize()
				entity := collision.NewEntity(monster.ID, monster.X, monster.Y, width, height, collision.CollisionTypeMonster, true)
				ih.game.collisionSystem.RegisterEntity(entity)

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

// wrapTextForDialog wraps text to fit within specified width (same as UI wrapText)
func (ih *InputHandler) wrapTextForDialog(text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		// Check if adding this word would exceed maxWidth
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	// Add the last line
	lines = append(lines, currentLine)
	return lines
}
