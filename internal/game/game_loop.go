package game

import (
	"fmt"
	"math"
	"time"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// GameLoop manages the main game update and render cycle
type GameLoop struct {
	game               *MMGame
	inputHandler       *InputHandler
	combat             *CombatSystem
	ui                 *UISystem
	renderer           *Renderer
	lastUpdateDuration time.Duration
	lastDrawDuration   time.Duration
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
	updateStart := time.Now()
	defer func() {
		gl.lastUpdateDuration = time.Since(updateStart)
	}()
	frameTimer := gl.game.threading.PerformanceMonitor.StartFrame()
	defer frameTimer.EndFrame()

	gl.game.frameCount++

	// Handle exit request from main menu
	if gl.game.exitRequested {
		return ErrExit
	}

	// Update per-frame mouse state before input handling and Draw
	gl.ui.updateMouseState()

	// Top-level screens replace the gameplay loop entirely. Their click handling
	// lives in the matching Draw call (roster-screen convention); update only
	// processes keyboard/back navigation here.
	switch gl.game.appScreen {
	case AppScreenMainMenu:
		gl.game.updateEntryMenu()
		return nil
	case AppScreenPartyCreate:
		gl.game.updatePartyCreate()
		return nil
	}

	gl.updateExploration()

	return nil
}

// updateExploration handles the main exploration gameplay loop
func (gl *GameLoop) updateExploration() {
	// In turn-based, snap selectedChar to a living member if the current one
	// died from delayed sources (in-flight projectiles, poison ticks). Has
	// to run before HandleInput so Space/F on a corpse advances selection
	// rather than silently no-opping for one frame.
	gl.game.ensureSelectedCharCanAct()

	// Handle all input first (menus/panels may pause gameplay)
	gl.inputHandler.HandleInput()

	// Ease the rendered view angle toward the (snapped) camera angle — smooth TB
	// turns. No-op in real time. Cheap; fine to run before the pause check.
	gl.game.advanceViewTurn()

	// Pause gameplay updates while menus/panels are open
	if gl.game.mainMenuOpen || gl.game.combatLogOpen || gl.game.statPopupOpen || gl.game.revivalPickerOpen || gl.game.currentLevelUpChoice() != nil {
		return
	}

	// Handle party updates (pass turn-based mode to disable timer-based regeneration)
	gl.game.party.UpdateWithMode(gl.game.turnBasedMode)

	// Update damage blink timers
	gl.game.UpdateDamageBlinkTimers()

	// Update all special effects and timers
	gl.updateSpecialEffects()

	// Refresh the party's active "traits" once per frame so passive monsters
	// that hate a trait (hates.yaml) know whether to turn hostile on sight.
	monster.PartyTraits["lich"] = gl.game.party.HasLich()

	// Cache bound undead so the AI-target lookup (bound-undead seek / mob
	// retaliation) stays cheap when none exist — the overwhelmingly common case.
	gl.game.refreshBoundUndeadCache()

	// Update monsters (turn-based or real-time)
	if gl.game.turnBasedMode {
		// Evasive bosses react in real time even in TB — see tickEvasiveBossesTB.
		gl.combat.tickEvasiveBossesTB()
		gl.updateMonstersTurnBased()
	} else {
		// Update monsters in parallel with performance monitoring
		gl.game.threading.PerformanceMonitor.ProfiledFunction("entity_update", func() {
			gl.updateMonstersParallel()
		})
	}

	// Gently push overlapping monsters apart. Overlap is reachable two ways:
	// non-engaged monsters deliberately pass through each other (pathfinding
	// deadlock prevention), and the parallel update can move two monsters into
	// the same spot in one tick. Once engaged while overlapped they deadlock —
	// each vetoes the other's every move — so resolve it here instead.
	gl.separateOverlappingMonsters()

	// Update monster hit tint timers
	gl.game.UpdateMonsterHitTintTimers()

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

	// Update hit effects (arrow bursts, stuck arrows, spell particles). Stuck
	// arrows outlive the burst, so they must keep the updater alive too.
	if len(gl.game.spellHitEffects) > 0 {
		gl.game.UpdateHitEffects()
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
	drawStart := time.Now()
	defer func() {
		gl.lastDrawDuration = time.Since(drawStart)
	}()
	// Clear with forest background color
	// forestBg := gl.game.config.Graphics.Colors.ForestBg
	// screen.Fill(color.RGBA{uint8(forestBg[0]), uint8(forestBg[1]), uint8(forestBg[2]), 255})

	// Top-level menu screens render instead of the 3D scene + gameplay UI.
	switch gl.game.appScreen {
	case AppScreenMainMenu:
		gl.ui.drawEntryMenuScreen(screen)
		return
	case AppScreenPartyCreate:
		gl.ui.drawPartyCreateScreen(screen)
		return
	}

	// Render the 3D scene, then composite to the screen. During a turn-based turn
	// the scene goes through a horizontal motion-blur shader (camera blur — the
	// view pans sideways) whose length tracks the turn speed; otherwise it's a
	// straight blit. Either way the UI is drawn last, directly to the screen, so it
	// never blurs.
	g := gl.game
	screenBounds := screen.Bounds()
	blurPx := g.turnBlurPixels(screenBounds.Dx()) // blur length scales with the real draw width
	if blurPx >= 0.75 {
		if scene := g.ensureTurnSceneBuffer(screenBounds); scene != nil {
			if shader, err := g.ensureBlurShader(); err == nil {
				scene.Clear()
				gl.renderer.RenderFirstPersonView(scene)
				b := scene.Bounds()
				op := &ebiten.DrawRectShaderOptions{}
				op.Images[0] = scene
				op.Uniforms = map[string]any{"BlurPx": float32(blurPx)}
				screen.DrawRectShader(b.Dx(), b.Dy(), shader, op)
			} else {
				gl.renderer.RenderFirstPersonView(screen) // shader failed to compile — no blur
			}
		} else {
			gl.renderer.RenderFirstPersonView(screen)
		}
	} else if turnBlurStrength > 0 && !g.turnBlurWarm {
		if scene := g.ensureTurnSceneBuffer(screenBounds); scene != nil {
			if shader, err := g.ensureBlurShader(); err == nil {
				// Prewarm the exact blur pipeline on an idle frame. BlurPx=0 is a
				// visually identical blit, but it creates the scene buffer and lets
				// Ebiten/Metal compile the shader pipeline before the first TB turn.
				scene.Clear()
				gl.renderer.RenderFirstPersonView(scene)
				b := scene.Bounds()
				op := &ebiten.DrawRectShaderOptions{}
				op.Images[0] = scene
				op.Uniforms = map[string]any{"BlurPx": float32(0)}
				screen.DrawRectShader(b.Dx(), b.Dy(), shader, op)
				g.turnBlurWarm = true
			} else {
				g.turnBlurWarm = true
				gl.renderer.RenderFirstPersonView(screen)
			}
		} else {
			g.turnBlurWarm = true
			gl.renderer.RenderFirstPersonView(screen)
		}
	} else {
		gl.renderer.RenderFirstPersonView(screen)
	}

	// Draw UI elements (straight to the screen — never blurred)
	gl.ui.Draw(screen)
}

// Layout returns the actual outside window dimensions, mutating runtime
// screen size + reallocating screen-sized buffers when the viewport changes
// (e.g. fullscreen on first frame). Returning fixed config dims would
// letterbox the game; returning outside dims renders at native resolution
// and lets UI anchors stick to actual screen edges.
func (gl *GameLoop) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	if outsideWidth > 0 && outsideHeight > 0 {
		gl.game.handleResize(outsideWidth, outsideHeight)
	}
	return outsideWidth, outsideHeight
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
	// Projectiles fly over floor-level obstacles (chasms, water) and only stop at
	// solid walls — unlike entity movement, which uses CanMoveTo.
	gl.game.threading.EntityUpdater.UpdateProjectilesParallel(allProjectiles, gl.game.world.CanProjectileMoveTo)

	// Remove inactive projectiles
	gl.game.RemoveInactiveEntities()
}

// separateOverlappingMonsters softly resolves monster-monster overlap: each
// overlapping pair is pushed apart a few pixels per tick along their least
// penetrated axis, so glued pairs un-merge smoothly instead of teleporting
// (the old unstuck ring-search) or freezing (engaged-while-overlapped pairs
// veto each other's every normal move). Terrain still wins: a push that would
// enter a blocked tile is skipped for that monster.
func (gl *GameLoop) separateOverlappingMonsters() {
	monsters := gl.game.world.Monsters
	if len(monsters) < 2 || gl.game.collisionSystem == nil {
		return
	}
	const pushPerTick = 2.0
	// Mirror of the collision rule: two CALM monsters pass through each other
	// by design (pathfinding deadlock prevention) — separating them turned
	// every crossing into a push-fight (measured: 1850 one-tick shove episodes
	// per 2 sim-minutes on the forest map). Only pairs where at least one side
	// is engaged actually collide, and only those can glue.
	engaged := func(m *monster.Monster3D) bool {
		return m.IsEngagingPlayer || m.State == monster.StateAttacking
	}
	// Tile-checked half-push; also refuses to shove a monster into the PLAYER's
	// box — entity collision is deliberately skipped (the overlapped partner
	// would veto every push), but landing on the player would deadlock the
	// monster against player collision instead.
	camX, camY := gl.game.camera.X, gl.game.camera.Y
	pushOne := func(m *monster.Monster3D, px, py float64) bool {
		nx, ny := m.X+px, m.Y+py
		mw, mh := m.GetSize()
		if math.Abs(nx-camX) < mw/2+8 && math.Abs(ny-camY) < mh/2+8 {
			return false
		}
		if !gl.game.collisionSystem.CanOccupyTilesWithHabitat(m.ID, nx, ny, m.HabitatPrefs, m.Flying) {
			return false
		}
		m.X, m.Y = nx, ny
		gl.game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		return true
	}
	for i := 0; i < len(monsters); i++ {
		a := monsters[i]
		if !a.IsAlive() {
			continue
		}
		aw, ah := a.GetSize()
		for j := i + 1; j < len(monsters); j++ {
			b := monsters[j]
			if !b.IsAlive() {
				continue
			}
			if !engaged(a) && !engaged(b) {
				continue // calm pair: pass-through is intended, no shoving
			}
			bw, bh := b.GetSize()
			dx := b.X - a.X
			dy := b.Y - a.Y
			sepX := (aw+bw)/2 - math.Abs(dx)
			sepY := (ah+bh)/2 - math.Abs(dy)
			if sepX <= 0 || sepY <= 0 {
				continue // no overlap
			}
			// Signed pushes per axis (b gets the positive direction); perfectly
			// stacked pairs get a deterministic tiebreak.
			sx := pushPerTick
			if dx < 0 || (dx == 0 && i%2 == 0) {
				sx = -sx
			}
			sy := pushPerTick
			if dy < 0 || (dy == 0 && i%2 == 0) {
				sy = -sy
			}
			// Prefer the axis of least penetration (standard AABB resolve), but
			// fall back to the other one when terrain blocks it: in a one-wide
			// gap between trees the cross-corridor push hits a trunk on both
			// sides, and the pair could only ever separate ALONG the corridor.
			var prim, sec [2]float64
			if sepX < sepY {
				prim, sec = [2]float64{sx, 0}, [2]float64{0, sy}
			} else {
				prim, sec = [2]float64{0, sy}, [2]float64{sx, 0}
			}
			// a moves opposite to b.
			if !pushOne(a, -prim[0], -prim[1]) {
				pushOne(a, -sec[0], -sec[1])
			}
			if !pushOne(b, prim[0], prim[1]) {
				pushOne(b, sec[0], sec[1])
			}
		}
	}
}

// removeDeadMonstersByID removes specific dead monsters by their IDs
// This is O(n) where n = number of dead monsters, not O(m) where m = all monsters
func (gl *GameLoop) removeDeadMonstersByID() {
	// Build a set of dead monster IDs for O(1) lookup (map reused across frames)
	deadSet := gl.game.reusableDeadSet
	for k := range deadSet {
		delete(deadSet, k)
	}
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
				gl.game.grantSharedXP(questRewards.Experience)
			}
			gl.game.addTreasureChestsFromRewards(rewards)
			gl.freeCaptivesFromRewards(rewards)
			return // Quest handled rewards, don't double-award
		}
	}

	// Fallback for encounters without quest integration
	// Show configurable completion message, or default if not set
	if rewards.CompletionMessage != "" {
		gl.game.AddCombatMessage(rewards.CompletionMessage)
	}
	gl.game.addTreasureChestsFromRewards(rewards)

	// Award gold to party
	if rewards.Gold > 0 {
		gl.game.party.Gold += rewards.Gold
		gl.game.AddCombatMessage(fmt.Sprintf("Party found %d gold!", rewards.Gold))
	}

	// Award experience to all party members
	if rewards.Experience > 0 {
		gl.game.grantSharedXP(rewards.Experience)
		gl.game.AddCombatMessage(fmt.Sprintf("Party gains %d experience!", rewards.Experience))
	}
	gl.freeCaptivesFromRewards(rewards)
}

// freeCaptivesFromRewards moves the party's imprisoned heroes into the reserve
// when an encounter that frees them is cleared. The captives have been leveling
// alongside the party all along, so they arrive at the right level with their
// stat points and owed level-up choices banked for the player to distribute.
func (gl *GameLoop) freeCaptivesFromRewards(rewards *monster.EncounterRewards) {
	if rewards == nil || !rewards.FreesCaptives {
		return
	}
	for _, c := range gl.game.party.FreeCaptives() {
		gl.game.AddCombatMessage(fmt.Sprintf("%s the %s is freed — they'll wait at the tavern.", c.Name, c.Class.String()))
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
	projectilesActive := int32(len(gl.game.magicProjectiles) + len(gl.game.arrows))
	collisionsDetected := uint64(0) // Could be updated by collision detection system

	gl.game.threading.PerformanceMonitor.UpdateGameMetrics(monstersUpdated, projectilesActive, collisionsDetected)
	gl.maybeLogPerfDrop()
}

// updateSpecialEffects updates all special effects and input cooldowns
func (gl *GameLoop) updateSpecialEffects() {
	// Update spellbook input cooldown
	if gl.game.spellInputCooldown > 0 {
		gl.game.spellInputCooldown--
	}

	// Screen shake decays exponentially toward rest.
	if gl.game.screenShake > 0 {
		gl.game.screenShake *= 0.88
		if gl.game.screenShake < 0.05 {
			gl.game.screenShake = 0
		}
	}

	// Impact light flashes burn down and expire.
	if len(gl.game.impactLights) > 0 {
		gl.game.hitEffectsMu.Lock()
		dst := gl.game.impactLights[:0]
		for _, il := range gl.game.impactLights {
			il.Life--
			if il.Life > 0 {
				dst = append(dst, il)
			}
		}
		gl.game.impactLights = dst
		gl.game.hitEffectsMu.Unlock()
	}

	// Tick down each party member's real-time action cooldown. Off in
	// turn-based mode (which gates on action slots, not frame cooldowns).
	if !gl.game.turnBasedMode {
		for _, m := range gl.game.party.Members {
			if m != nil && m.RTCooldown > 0 {
				m.RTCooldown--
			}
		}
	}

	// Tick every timed party buff and refresh its HUD status from ONE registry.
	for _, b := range gl.game.timedBuffs() {
		tickBuff(b.active, b.duration, b.onExpire)
		gl.game.updateUtilityStatus(b.id, *b.duration, *b.active)
	}
	// Stacking combat buffs (Day of the Gods, Hour of Power, Stone Skin, Heroism)
	// tick from their own list — see combat_buffs.go.
	gl.game.tickCombatBuffs()
	gl.game.tickStatBuffs()
	// Persistent damage zones (Hot Steam): lifetime + real-time damage cadence.
	gl.updateSteamZonesRT()
	// Armed traps: ambient swirl VFX (both modes) + RT trigger sweep.
	gl.updateTraps()

	// Walk-on-water / water-breathing drive world flags every frame.
	if gl.game.world != nil {
		gl.game.world.SetWalkOnWaterActive(gl.game.walkOnWaterActive)
		gl.game.world.SetWaterBreathingActive(gl.game.waterBreathingActive)
	}

	// Bind_undead charm timers are per-monster, not a party buff.
	gl.updateControlledMonsters()
}

// timedBuff is one duration-based party buff: its active/duration pointers are
// ticked each frame and surfaced as a HUD status; onExpire (optional) undoes the
// buff's effect when it runs out.
type timedBuff struct {
	id       spells.SpellID
	active   *bool
	duration *int
	onExpire func()
}

// timedBuffs is the SINGLE registry of duration-based buffs. To add a new timed
// buff, add one entry here — it then ticks, shows its HUD icon, and is restored
// on load automatically, with no other code changes.
func (g *MMGame) timedBuffs() []timedBuff {
	return []timedBuff{
		{"torch_light", &g.torchLightActive, &g.torchLightDuration, nil},
		{"wizard_eye", &g.wizardEyeActive, &g.wizardEyeDuration, nil},
		{"walk_on_water", &g.walkOnWaterActive, &g.walkOnWaterDuration, nil},
		{"water_breathing", &g.waterBreathingActive, &g.waterBreathingDuration, func() {
			// If still underwater when it lapses, surface the party.
			if g.gameLoop != nil && world.GlobalWorldManager != nil && world.GlobalWorldManager.CurrentMapKey == "water" {
				g.gameLoop.returnFromUnderwater()
			}
		}},
	}
}

// tickBuff decrements the duration of an active timed buff and runs onExpire
// when it hits zero. Shared by all utility spells with active/duration pairs.
// Returns true if the buff was active this tick (regardless of expiration).
func tickBuff(active *bool, duration *int, onExpire func()) bool {
	if !*active {
		return false
	}
	*duration--
	if *duration <= 0 {
		*active = false
		*duration = 0
		if onExpire != nil {
			onExpire()
		}
	}
	return true
}

// updateControlledMonsters ticks Bind Undead and Charm timers in real-time. When a
// bind expires the undead turns hostile again; when a charm expires the living
// mob re-aggros. (TB control persists the encounter — no real-frame countdown.)
func (gl *GameLoop) updateControlledMonsters() {
	if gl.game.world == nil {
		return
	}
	for _, m := range gl.game.world.Monsters {
		if m.Bound && m.BoundFramesRemaining > 0 {
			m.BoundFramesRemaining--
			if m.BoundFramesRemaining == 0 {
				m.Bound = false
				gl.game.AddCombatMessage(fmt.Sprintf("%s breaks free of your binding!", m.Name))
			}
		}
		if m.Pacified && m.PacifiedFramesRemaining > 0 {
			m.PacifiedFramesRemaining--
			if m.PacifiedFramesRemaining == 0 {
				m.Pacified = false
				m.WasAttacked = true // re-aggros when the charm wears off
				gl.game.AddCombatMessage(fmt.Sprintf("The charm on %s wears off!", m.Name))
			}
		}
	}
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
	return len(gl.game.magicProjectiles) > 0 || len(gl.game.arrows) > 0
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
