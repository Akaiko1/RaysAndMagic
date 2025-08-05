package game

import (
	"ugataima/internal/monster"

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

	gl.updateExploration()
	return nil
}

// updateExploration handles the main exploration gameplay loop
func (gl *GameLoop) updateExploration() {
	// Handle party updates
	gl.game.party.Update()

	// Update damage blink timers
	gl.game.UpdateDamageBlinkTimers()

	// Update all special effects and timers
	gl.updateSpecialEffects()

	// Handle all input
	gl.inputHandler.HandleInput()

	// Update monsters in parallel with performance monitoring
	gl.game.threading.PerformanceMonitor.ProfiledFunction("entity_update", func() {
		gl.updateMonstersParallel()
	})

	// Handle combat interactions
	gl.combat.HandleMonsterInteractions()

	// Update projectiles in parallel
	gl.updateProjectilesParallel()

	// Remove dead monsters from the world
	gl.removeDeadMonsters()

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

	// Convert all projectiles to wrappers and update in parallel
	allProjectiles := gl.game.ConvertProjectilesToWrappers()
	gl.game.threading.EntityUpdater.UpdateProjectilesParallel(allProjectiles, gl.game.world.CanMoveTo)

	// Check for projectile-monster collisions after movement
	gl.combat.CheckProjectileMonsterCollisions()

	// Remove inactive projectiles
	gl.game.RemoveInactiveEntities()
}

// removeDeadMonsters removes dead monsters from the world to improve performance
func (gl *GameLoop) removeDeadMonsters() {
	aliveMonsters := make([]*monster.Monster3D, 0, len(gl.game.world.Monsters))
	for _, monster := range gl.game.world.Monsters {
		if monster.IsAlive() {
			aliveMonsters = append(aliveMonsters, monster)
		} else {
			// Unregister dead monster from collision system
			gl.game.collisionSystem.UnregisterEntity(monster.ID)
		}
	}
	gl.game.world.Monsters = aliveMonsters
}

// updatePerformanceMetrics updates game-specific performance metrics
func (gl *GameLoop) updatePerformanceMetrics() {
	monstersUpdated := uint64(len(gl.game.world.Monsters))
	projectilesActive := int32(len(gl.game.fireballs) + len(gl.game.swordAttacks))
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

	// Sync the walk on water state with the world
	gl.game.world.SetWalkOnWaterActive(gl.game.walkOnWaterActive)
}

// updateBlessEffect updates the bless stat bonus effect
func (gl *GameLoop) updateBlessEffect() {
	if gl.game.blessActive {
		gl.game.blessDuration--
		if gl.game.blessDuration <= 0 {
			gl.game.blessActive = false
			gl.game.blessDuration = 0
			gl.game.statBonus -= 20  // Subtract the +20 Bless bonus from total stat bonus
		}
	}
}
