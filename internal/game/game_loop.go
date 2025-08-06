package game

import (
	"fmt"
	"math"
	"math/rand"
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

	// Update projectiles in parallel
	gl.updateProjectilesParallel()
	
	// Update slash effects
	gl.updateSlashEffects()

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

	// Check for projectile-monster collisions BEFORE movement to prevent tunneling
	gl.combat.CheckProjectileMonsterCollisions()

	// Convert all projectiles to wrappers and update in parallel
	allProjectiles := gl.game.ConvertProjectilesToWrappers()
	gl.game.threading.EntityUpdater.UpdateProjectilesParallel(allProjectiles, gl.game.world.CanMoveTo)

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

// updateSlashEffects updates all active slash effects
func (gl *GameLoop) updateSlashEffects() {
	activeSlashEffects := make([]SlashEffect, 0, len(gl.game.slashEffects))
	
	for i := range gl.game.slashEffects {
		slash := &gl.game.slashEffects[i]
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
		
		activeSlashEffects = append(activeSlashEffects, *slash)
	}
	
	gl.game.slashEffects = activeSlashEffects
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

// updateMonstersTurnBased handles monster updates in turn-based mode
func (gl *GameLoop) updateMonstersTurnBased() {
	if gl.game.currentTurn != 1 { // Not monster turn
		return
	}
	
	// Only monsters within 6-tile vision range participate in turn-based combat
	tileSize := float64(gl.game.config.GetTileSize())
	visionRange := tileSize * 6.0 // 6 tiles
	turnBasedAttackDistance := tileSize * 1.1 // Allow attacks from neighboring tiles
	
	// Process each monster's turn (only those in vision range)
	monstersActed := 0
	for _, monster := range gl.game.world.Monsters {
		if !monster.IsAlive() {
			continue
		}
		
		// Calculate distance to player
		dx := monster.X - gl.game.camera.X
		dy := monster.Y - gl.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)
		
		// Skip monsters outside vision range - they don't participate in turn-based combat
		if distance > visionRange {
			continue
		}
		
		// Monster can either move 1 tile OR attack if close enough
		if distance < turnBasedAttackDistance {
			// Attack the current character
			gl.monsterAttackTurnBased(monster)
		} else {
			// Move 1 tile towards player (or preferred direction)
			gl.monsterMoveTurnBased(monster)
		}
		
		monstersActed++
	}
	
	// Always end monster turn and start party turn
	// Even if no monsters acted, we need to return control to the party
	gl.endMonsterTurn()
}

// monsterAttackTurnBased handles a monster attack in turn-based mode
func (gl *GameLoop) monsterAttackTurnBased(monster *monster.Monster3D) {
	// Attack a random party character
	targetIndex := rand.Intn(len(gl.game.party.Members))
	target := gl.game.party.Members[targetIndex]
	
	damage := monster.GetAttackDamage()
	
	// Apply armor damage reduction
	finalDamage := gl.combat.ApplyArmorDamageReduction(damage, target)
	target.HitPoints -= finalDamage
	if target.HitPoints < 0 {
		target.HitPoints = 0
	}
	
	// Trigger damage blink effect
	gl.game.TriggerDamageBlink(targetIndex)
	
	gl.game.AddCombatMessage(fmt.Sprintf("%s attacks %s for %d damage!", monster.Name, target.Name, finalDamage))
}

// monsterMoveTurnBased handles a monster move in turn-based mode
func (gl *GameLoop) monsterMoveTurnBased(monster *monster.Monster3D) {
	tileSize := float64(gl.game.config.GetTileSize())
	
	// Calculate direction to player
	dx := gl.game.camera.X - monster.X
	dy := gl.game.camera.Y - monster.Y
	distance := math.Sqrt(dx*dx + dy*dy)
	
	if distance == 0 {
		return // Already at player position
	}
	
	// Normalize direction and move 1 tile
	dirX := dx / distance
	dirY := dy / distance
	
	newX := monster.X + dirX*tileSize
	newY := monster.Y + dirY*tileSize
	
	// Check if the monster can move to the new position
	if gl.game.collisionSystem.CanMoveTo(monster.ID, newX, newY) {
		monster.X = newX
		monster.Y = newY
		gl.game.collisionSystem.UpdateEntity(monster.ID, newX, newY)
	}
}

// endMonsterTurn ends the monster turn and starts party turn
func (gl *GameLoop) endMonsterTurn() {
	gl.game.currentTurn = 0 // Party turn
	gl.game.partyActionsUsed = 0
	// Don't spam combat log with turn messages
}
