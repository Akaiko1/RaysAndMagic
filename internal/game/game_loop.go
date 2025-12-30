package game

import (
	"fmt"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// GameLoop manages the main game update and render cycle
type GameLoop struct {
	game         *MMGame
	inputHandler *InputHandler
	combat       *CombatSystem
	ui           *UISystem
	renderer     *Renderer
}

// NewGameLoop creates a new game loop manager
func NewGameLoop(game *MMGame) *GameLoop {
	inputHandler := NewInputHandler(game)
	combat := NewCombatSystem(game)
	ui := NewUISystem(game)
	renderer := NewRenderer(game)

	return &GameLoop{
		game:         game,
		inputHandler: inputHandler,
		combat:       combat,
		ui:           ui,
		renderer:     renderer,
	}
}

// Update handles all game logic updates for one frame
func (gl *GameLoop) Update() error {
	frameTimer := gl.game.threading.PerformanceMonitor.StartFrame()
	defer frameTimer.EndFrame()

	// Handle exit request from main menu
	if gl.game.exitRequested {
		return ErrExit
	}

	// Update per-frame mouse state before input handling and Draw
	gl.ui.updateMouseState()

	gl.updateExploration()

	return nil
}

// updateExploration handles the main exploration gameplay loop
func (gl *GameLoop) updateExploration() {
	// Handle party updates (pass turn-based mode to disable timer-based regeneration)
	gl.game.party.UpdateWithMode(gl.game.turnBasedMode)

	// Update damage blink timers
	gl.game.UpdateDamageBlinkTimers()

	// Update all special effects and timers
	gl.updateSpecialEffects()

	// Handle all input
	gl.inputHandler.HandleInput()

	// Update monsters (turn-based or real-time)
	if gl.game.turnBasedMode {
		gl.updateMonstersTurnBased()
	} else {
		// Update monsters in parallel with performance monitoring
		gl.game.threading.PerformanceMonitor.ProfiledFunction("entity_update", func() {
			gl.updateMonstersParallel()
		})
	}

	// Handle combat interactions (only in real-time mode)
	if !gl.game.turnBasedMode {
		gl.combat.HandleMonsterInteractions()
	}

	// Update projectiles - skip if no active projectiles to save CPU
	if gl.hasActiveProjectiles() {
		gl.updateProjectilesParallel()
	}

	// Update slash effects - skip if none active
	if len(gl.game.slashEffects) > 0 {
		gl.updateSlashEffects()
	}

	// Remove dead monsters - only if there are any to remove
	if len(gl.game.deadMonsterIDs) > 0 {
		gl.removeDeadMonstersByID()
	}

	// Update performance metrics
	gl.updatePerformanceMetrics()
}

// Draw handles all rendering for one frame
func (gl *GameLoop) Draw(screen *ebiten.Image) {
	// Clear with forest background color
	// forestBg := gl.game.config.Graphics.Colors.ForestBg
	// screen.Fill(color.RGBA{uint8(forestBg[0]), uint8(forestBg[1]), uint8(forestBg[2]), 255})

	// Render the 3D first-person view
	gl.renderer.RenderFirstPersonView(screen)

	// Draw UI elements
	gl.ui.Draw(screen)
}

// Layout returns the screen dimensions
func (gl *GameLoop) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return gl.game.config.GetScreenWidth(), gl.game.config.GetScreenHeight()
}

// updateMonstersParallel updates all monsters using parallel processing
func (gl *GameLoop) updateMonstersParallel() {
	monsters := gl.game.ConvertMonstersToWrappers()
	gl.game.threading.EntityUpdater.UpdateMonstersParallel(monsters)
}

// updateProjectilesParallel updates all projectiles using parallel processing
func (gl *GameLoop) updateProjectilesParallel() {
	gl.game.projectileMutex.Lock()
	defer gl.game.projectileMutex.Unlock()

	// Check for projectile-monster collisions BEFORE movement to prevent tunneling
	gl.combat.CheckProjectilePlayerCollisions()
	gl.combat.CheckProjectileMonsterCollisions()

	// Convert all projectiles to wrappers and update in parallel
	allProjectiles := gl.game.ConvertProjectilesToWrappers()
	gl.game.threading.EntityUpdater.UpdateProjectilesParallel(allProjectiles, gl.game.world.CanMoveTo)

	// Remove inactive projectiles
	gl.game.RemoveInactiveEntities()
}

// removeDeadMonstersByID removes specific dead monsters by their IDs
// This is O(n) where n = number of dead monsters, not O(m) where m = all monsters
func (gl *GameLoop) removeDeadMonstersByID() {
	// Build a set of dead monster IDs for O(1) lookup
	deadSet := make(map[string]bool, len(gl.game.deadMonsterIDs))
	for _, id := range gl.game.deadMonsterIDs {
		deadSet[id] = true
	}

	// Clear encounter rewards map for reuse
	for k := range gl.game.reusableEncounterRewardsMap {
		delete(gl.game.reusableEncounterRewardsMap, k)
	}

	// In-place filtering - only check against known dead IDs
	writeIdx := 0
	for readIdx := range gl.game.world.Monsters {
		m := gl.game.world.Monsters[readIdx]
		if !deadSet[m.ID] {
			if writeIdx != readIdx {
				gl.game.world.Monsters[writeIdx] = m
			}
			writeIdx++
		} else {
			// Check if this was an encounter monster for rewards
			if gl.isEncounterMonster(m) {
				gl.game.reusableEncounterRewardsMap[m.EncounterRewards]++
			}
			// Unregister dead monster from collision system
			gl.game.collisionSystem.UnregisterEntity(m.ID)
		}
	}
	gl.game.world.Monsters = gl.game.world.Monsters[:writeIdx]

	// Award encounter rewards once per encounter type
	for rewards := range gl.game.reusableEncounterRewardsMap {
		if gl.countRemainingEncounterMonsters(gl.game.world.Monsters, rewards) == 0 {
			gl.awardEncounterRewards(rewards)
		}
	}

	// Clear the dead monster IDs list for next frame
	gl.game.deadMonsterIDs = gl.game.deadMonsterIDs[:0]
}

// awardEncounterRewards gives the party gold and experience for completing an encounter
func (gl *GameLoop) awardEncounterRewards(rewards *monster.EncounterRewards) {
	if rewards == nil {
		return
	}

	// Auto-complete linked encounter quest (rewards are handled by quest system)
	if rewards.QuestID != "" && quests.GlobalQuestManager != nil {
		questRewards := quests.GlobalQuestManager.CompleteEncounterQuest(rewards.QuestID)
		if questRewards != nil {
			// Show completion message
			if rewards.CompletionMessage != "" {
				gl.game.AddCombatMessage(rewards.CompletionMessage)
			}
			gl.game.AddCombatMessage(fmt.Sprintf("Quest Completed: Received %d gold and %d experience!", questRewards.Gold, questRewards.Experience))

			// Award gold to party
			if questRewards.Gold > 0 {
				gl.game.party.Gold += questRewards.Gold
			}

			// Award experience to all party members
			if questRewards.Experience > 0 {
				for _, member := range gl.game.party.Members {
					if member.HitPoints > 0 {
						member.Experience += questRewards.Experience
						gl.game.combat.checkLevelUp(member)
					}
				}
			}
			return // Quest handled rewards, don't double-award
		}
	}

	// Fallback for encounters without quest integration
	// Show configurable completion message, or default if not set
	if rewards.CompletionMessage != "" {
		gl.game.AddCombatMessage(rewards.CompletionMessage)
	}

	// Award gold to party
	if rewards.Gold > 0 {
		gl.game.party.Gold += rewards.Gold
		gl.game.AddCombatMessage(fmt.Sprintf("Party found %d gold!", rewards.Gold))
	}

	// Award experience to all party members
	if rewards.Experience > 0 {
		for _, member := range gl.game.party.Members {
			if member.HitPoints > 0 { // Only living members get experience
				member.Experience += rewards.Experience

				// Check for level up using the combat system's level up logic
				gl.game.combat.checkLevelUp(member)
			}
		}
		gl.game.AddCombatMessage(fmt.Sprintf("Party gains %d experience!", rewards.Experience))
	}
}

// updateSlashEffects updates all active slash effects using in-place filtering
// This avoids allocating new slices each frame to reduce GC pressure
func (gl *GameLoop) updateSlashEffects() {
	writeIdx := 0
	for readIdx := range gl.game.slashEffects {
		slash := &gl.game.slashEffects[readIdx]
		if !slash.Active {
			continue
		}

		// Advance animation frame
		slash.AnimationFrame++

		// Check if animation is complete
		if slash.AnimationFrame >= slash.MaxFrames {
			slash.Active = false
			continue
		}

		// Keep this effect (in-place move if needed)
		if writeIdx != readIdx {
			gl.game.slashEffects[writeIdx] = *slash
		}
		writeIdx++
	}
	gl.game.slashEffects = gl.game.slashEffects[:writeIdx]
}

// updatePerformanceMetrics updates game-specific performance metrics
func (gl *GameLoop) updatePerformanceMetrics() {
	monstersUpdated := uint64(len(gl.game.world.Monsters))
	projectilesActive := int32(len(gl.game.magicProjectiles) + len(gl.game.meleeAttacks))
	collisionsDetected := uint64(0) // Could be updated by collision detection system

	gl.game.threading.PerformanceMonitor.UpdateGameMetrics(monstersUpdated, projectilesActive, collisionsDetected)
}

// updateSpecialEffects updates all special effects and input cooldowns
func (gl *GameLoop) updateSpecialEffects() {
	// Update spellbook input cooldown
	if gl.game.spellInputCooldown > 0 {
		gl.game.spellInputCooldown--
	}

	// Update torch light effect
	gl.updateTorchLightEffect()

	// Update wizard eye effect
	gl.updateWizardEyeEffect()

	// Update walk on water effect
	gl.updateWalkOnWaterEffect()

	// Update bless effect
	gl.updateBlessEffect()

	// Update water breathing effect
	gl.updateWaterBreathingEffect()
}

// updateTorchLightEffect updates the torch light illumination effect
func (gl *GameLoop) updateTorchLightEffect() {
	if gl.game.torchLightActive {
		gl.game.torchLightDuration--
		if gl.game.torchLightDuration <= 0 {
			gl.game.torchLightActive = false
			gl.game.torchLightDuration = 0
		}
	}
	gl.game.updateUtilityStatus(spells.SpellID("torch_light"), gl.game.torchLightDuration, gl.game.torchLightActive)
}

// updateWizardEyeEffect updates the wizard eye enemy detection effect
func (gl *GameLoop) updateWizardEyeEffect() {
	if gl.game.wizardEyeActive {
		gl.game.wizardEyeDuration--
		if gl.game.wizardEyeDuration <= 0 {
			gl.game.wizardEyeActive = false
			gl.game.wizardEyeDuration = 0
		}
	}
	gl.game.updateUtilityStatus(spells.SpellID("wizard_eye"), gl.game.wizardEyeDuration, gl.game.wizardEyeActive)
}

// updateWalkOnWaterEffect updates the walk on water effect
func (gl *GameLoop) updateWalkOnWaterEffect() {
	if gl.game.walkOnWaterActive {
		gl.game.walkOnWaterDuration--
		if gl.game.walkOnWaterDuration <= 0 {
			gl.game.walkOnWaterActive = false
			gl.game.walkOnWaterDuration = 0
		}
	}

	gl.game.updateUtilityStatus(spells.SpellID("walk_on_water"), gl.game.walkOnWaterDuration, gl.game.walkOnWaterActive)

	// Sync the walk on water state with the world
	gl.game.world.SetWalkOnWaterActive(gl.game.walkOnWaterActive)

	// Sync the water breathing state with the world
	gl.game.world.SetWaterBreathingActive(gl.game.waterBreathingActive)
}

// updateBlessEffect updates the bless stat bonus effect
func (gl *GameLoop) updateBlessEffect() {
	if gl.game.blessActive {
		gl.game.blessDuration--
		if gl.game.blessDuration <= 0 {
			gl.game.blessActive = false
			gl.game.blessDuration = 0
			gl.game.statBonus -= gl.game.blessStatBonus // Subtract the stored Bless bonus
			gl.game.blessStatBonus = 0
		}
	}
	gl.game.updateUtilityStatus(spells.SpellID("bless"), gl.game.blessDuration, gl.game.blessActive)
}

// updateWaterBreathingEffect updates the water breathing effect
func (gl *GameLoop) updateWaterBreathingEffect() {
	if gl.game.waterBreathingActive {
		gl.game.waterBreathingDuration--
		if gl.game.waterBreathingDuration <= 0 {
			gl.game.waterBreathingActive = false
			gl.game.waterBreathingDuration = 0

			// If currently underwater (water map), teleport back to surface
			if world.GlobalWorldManager != nil && world.GlobalWorldManager.CurrentMapKey == "water" {
				gl.returnFromUnderwater()
			}
		}
	}
	gl.game.updateUtilityStatus(spells.SpellID("water_breathing"), gl.game.waterBreathingDuration, gl.game.waterBreathingActive)
}

// returnFromUnderwater teleports the player back to the surface when Water Breathing expires
func (gl *GameLoop) returnFromUnderwater() {
	// First switch back to the original map
	returnMapKey := gl.game.underwaterReturnMap
	if returnMapKey == "" {
		returnMapKey = "main" // Fallback to main map
	}

	// Use input handler's common map switching logic
	if gl.inputHandler != nil {
		gl.inputHandler.switchToMap(returnMapKey)
	} else {
		fmt.Printf("Error: Input handler not available for map switching\n")
		return
	}

	// Find nearest walkable tile to the stored return position - MUST succeed for safety
	returnX, returnY := gl.game.FindNearestWalkableTileMustSucceed(gl.game.underwaterReturnX, gl.game.underwaterReturnY)

	// Teleport to the safe position
	gl.game.camera.X = returnX
	gl.game.camera.Y = returnY
	gl.game.collisionSystem.UpdateEntity("player", returnX, returnY)

	fmt.Println("Water Breathing expired! Returned to surface.")
}

// Turn-based combat helpers moved to tb_combat.go

// hasActiveProjectiles checks if there are any active projectiles to update
func (gl *GameLoop) hasActiveProjectiles() bool {
	return len(gl.game.magicProjectiles) > 0 || len(gl.game.meleeAttacks) > 0 || len(gl.game.arrows) > 0
}

// isEncounterMonster checks if a monster is part of an encounter with rewards
func (gl *GameLoop) isEncounterMonster(m *monster.Monster3D) bool {
	return m.IsEncounterMonster && m.EncounterRewards != nil
}

// countRemainingEncounterMonsters counts how many monsters of a specific encounter type are still alive
func (gl *GameLoop) countRemainingEncounterMonsters(monsters []*monster.Monster3D, rewards *monster.EncounterRewards) int {
	count := 0
	for _, monster := range monsters {
		if monster.IsEncounterMonster && monster.EncounterRewards == rewards {
			count++
		}
	}
	return count
}
