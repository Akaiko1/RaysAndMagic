package game

import (
	"fmt"
	"math"
	"sort"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/items"
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
}

// NewInputHandler creates a new input handler
func NewInputHandler(game *MMGame) *InputHandler {
	return &InputHandler{game: game}
}

// HandleInput processes all input for the current frame
func (ih *InputHandler) HandleInput() {
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
	ih.handleMovementInput()
	ih.handleCombatInput()
	ih.handleCharacterSelectionInput()
	ih.handleUIInput()
	ih.handleMouseInput()
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
		ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
	}

	// Melee attack (Space key) - with cooldown to prevent spam
	if ebiten.IsKeyPressed(ebiten.KeySpace) && ih.game.spellInputCooldown == 0 {
		ih.game.combat.EquipmentMeleeAttack()
		ih.game.spellInputCooldown = 15 // 0.25 second cooldown at 60 FPS
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
	}
}

// checkTeleporter checks if player is on a teleporter and handles teleportation
func (ih *InputHandler) checkTeleporter() {
	// Try global (cross-map) teleportation first
	if world.GlobalWorldManager != nil {
		targetMapKey, newX, newY, teleported := world.GlobalWorldManager.TryGlobalTeleport(ih.game.camera.X, ih.game.camera.Y)
		if teleported {
			// Handle map transition if needed
			if targetMapKey != world.GlobalWorldManager.CurrentMapKey {
				err := world.GlobalWorldManager.SwitchToMap(targetMapKey)
				if err != nil {
					fmt.Printf("Failed to switch to map %s: %v\n", targetMapKey, err)
					return
				}
				// Update game world reference after map switch
				oldWorld := ih.game.world
				ih.game.world = ih.game.GetCurrentWorld()

				// Update collision system to use new world
				if ih.game.collisionSystem != nil {
					ih.game.collisionSystem.UpdateTileChecker(ih.game.world)

					// Unregister old monsters from collision system
					if oldWorld != nil {
						for _, monster := range oldWorld.Monsters {
							ih.game.collisionSystem.UnregisterEntity(monster.ID)
						}
					}

					// Register new world's monsters with collision system
					ih.game.world.RegisterMonstersWithCollisionSystem(ih.game.collisionSystem)
				}

				// Update sky and ground colors for new map
				ih.game.UpdateSkyAndGroundColors()

				// Regenerate floor color cache for new map with explicit map config
				if ih.game.gameLoop != nil && ih.game.gameLoop.renderer != nil {
					// Ensure we're using the new map's config by getting it directly
					mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
					if mapConfig != nil {
						fmt.Printf("Regenerating floor cache with %s map colors: %v\n", mapConfig.Biome, mapConfig.DefaultFloorColor)
					}
					ih.game.gameLoop.renderer.precomputeFloorColorCache()
				}
			}

			// Update player position
			ih.game.camera.X = newX
			ih.game.camera.Y = newY
			ih.game.collisionSystem.UpdateEntity("player", newX, newY)
			return
		}
	}

	// Fallback to local teleportation within current map
	newX, newY, teleported := ih.game.GetCurrentWorld().TryTeleport(ih.game.camera.X, ih.game.camera.Y)
	if teleported {
		ih.game.camera.X = newX
		ih.game.camera.Y = newY
		ih.game.collisionSystem.UpdateEntity("player", newX, newY)
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
					ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
				} else {
					// Self-only spell (First Aid) but targeting someone else - fallback to self-heal
					ih.game.combat.EquipmentHeal()
					ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
				}
			} else {
				// No target under mouse, heal self (original behavior)
				ih.game.combat.EquipmentHeal()
				ih.game.spellInputCooldown = ih.game.config.UI.SpellInputCooldown
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

	// Navigate spells with Up/Down arrows (if NPC is a spell trader)
	if ih.game.dialogNPC != nil && ih.game.dialogNPC.Type == "spell_trader" {
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

	// Check requirements (simplified for now)
	// TODO: Add proper requirement checking

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
			def := spells.GetSpellDefinitionByID(spellID)
			if def.Name == spellName {
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
	spellIDToAdd := spells.GetSpellIDByName(spellData.Name)

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

	// Check if clicking on spells (if NPC is spell trader)
	if ih.game.dialogNPC != nil && ih.game.dialogNPC.Type == "spell_trader" {
		spellsY := dialogY + 100 + (len(ih.game.party.Members) * 25) + 20
		spellKeys := ih.getAvailableSpellKeys()

		for spellIndex, spellKey := range spellKeys {
			spellY := spellsY + 20 + (spellIndex * 25)

			// Check if mouse is over this spell entry
			if mouseX >= dialogX+20 && mouseX <= dialogX+370 &&
				mouseY >= spellY-2 && mouseY <= spellY+22 {

				// Check for double-click to purchase spell directly
				currentTime := time.Now().UnixMilli()
				if ih.game.lastClickedDialogSpell == spellIndex &&
					currentTime-ih.game.lastDialogSpellClickTime < 500 {
					// Double-click detected - purchase the spell
					ih.purchaseSelectedSpell()
				} else {
					// Single click - just select the spell
					ih.game.dialogSelectedSpell = spellIndex
					ih.game.selectedSpellKey = spellKey
				}

				// Update click tracking for dialog spells
				ih.game.lastDialogSpellClickTime = currentTime
				ih.game.lastClickedDialogSpell = spellIndex
				ih.game.mousePressed = true
				return
			}
		}
	}
}
