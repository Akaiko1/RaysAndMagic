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
	ui                 *UISystem
	renderer           *Renderer
	lastUpdateDuration time.Duration
	lastDrawDuration   time.Duration

	// timedBuffRegistry caches the timedBuffs() registry: its pointers are
	// stable for the MMGame lifetime, so it is built once on first use.
	timedBuffRegistry []timedBuff

	// Per-tick scratch buffers, reset with [:0]/clear instead of reallocating.
	monsterFrameBuf       []monsterFramePosition
	sepEngagedBuf         []int
	bandersBuf            []*monster.Monster3D
	bandSinglesBuf        []*monster.Monster3D
	bandIDsBuf            []int
	bandParentBuf         []int
	bandHandled           map[*monster.Monster3D]bool
	bandUsedSingles       map[*monster.Monster3D]bool
	attackPostBuf         []*monster.Monster3D
	combatTransitStackBuf []*monster.Monster3D
}

type monsterFramePosition struct {
	monster *monster.Monster3D
	x, y    float64
}

// NewGameLoop creates a new game loop manager. Combat is shared via
// game.combat (a stateless back-pointer holder) - no second instance.
func NewGameLoop(game *MMGame) *GameLoop {
	return &GameLoop{
		game:         game,
		inputHandler: NewInputHandler(game),
		ui:           NewUISystem(game),
		renderer:     NewRenderer(game),
	}
}

// Update handles all game logic updates for one frame
func (gl *GameLoop) Update() error {
	updateStart := time.Now()
	defer func() {
		gl.lastUpdateDuration = time.Since(updateStart)
	}()
	defer func() {
		if gl.renderer != nil {
			gl.renderer.prewarmPendingTreeStandeeResources()
		}
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

	// Resolve the Space-to-interact focus target before input reads it. Uses
	// the camera as rendered last frame - exactly what the player is seeing.
	gl.game.updateFocusedNPC()

	// Handle all input first (menus/panels may pause gameplay)
	gl.inputHandler.HandleInput()

	// Ease the rendered view angle toward the (snapped) camera angle - smooth TB
	// turns. No-op in real time. Cheap; fine to run before the pause check.
	gl.game.advanceViewTurn()

	// Pause gameplay updates while menus/panels are open
	if gl.game.mainMenuOpen || gl.game.combatLogOpen || gl.game.statPopupOpen || gl.game.revivalPickerOpen || gl.game.healPickerOpen || gl.game.townPortalPickerOpen || gl.game.currentLevelUpChoice() != nil {
		return
	}

	// Track the party's region on the unified open world BEFORE anything below
	// reads the current map key (sky, packs, quest scoping).
	gl.game.syncOpenWorldRegion()

	// Handle party updates (pass turn-based mode to disable timer-based regeneration)
	gl.game.party.UpdateWithMode(gl.game.turnBasedMode)
	gl.game.combat.knockOutLethalDoTVictims()

	// Day/night clock: runs in both RT and TB, pauses with menus (above).
	gl.game.updateDayNight()

	// Update damage blink timers
	gl.game.UpdateDamageBlinkTimers()

	// Card-summon proc cooldown ticks in real time in both modes; it silences
	// only the proc, so nothing else waits on it.
	if gl.game.cardSummonCDFrames > 0 {
		gl.game.cardSummonCDFrames--
	}

	// Update all special effects and timers
	gl.updateSpecialEffects()

	// Refresh the party's active "traits" once per frame so passive monsters
	// that hate a trait (hates.yaml) know whether to turn hostile on sight.
	monster.PartyTraits["lich"] = gl.game.party.HasLich()

	// Cache bound undead so the AI-target lookup (bound-undead seek / mob
	// retaliation) stays cheap when none exist - the overwhelmingly common case.
	gl.game.refreshMonsterAIState()
	// Reconcile restored or redirected combat attack posts before the next RT
	// snapshot/TB action can use them.
	gl.reconcileMonsterAttackPosts()
	// Calm solo mobs that can see an unclaimed crate or spell lectern reserve up
	// to two guard slots before movement. RT carries the prepared patrol tile into
	// its AI pass; TB keeps calm guards stationary and uses only their normal
	// direct-sight engagement rule.
	gl.prepareLootPropGuards()

	// Reconcile door state (closed iff a living champion is on this map) and the
	// solid collision entities behind it, before either monster update runs.
	gl.game.refreshDoors()

	// The facing pass must see only the movement pass's displacement.
	monsterFrameStart := gl.captureMonsterFramePositions()

	// Update monsters (turn-based or real-time)
	if gl.game.turnBasedMode {
		// A stun can remove the final party actor after slots were assigned. Do
		// this in the scheduler, rather than input, so keyboard, spellbook, trap,
		// and delayed-projectile paths all hand the empty turn to monsters alike.
		gl.game.skipTurnBasedPartyTurnWithoutActor()
		// Evasive bosses react in real time even in TB - see tickEvasiveBossesTB.
		gl.game.combat.tickEvasiveBossesTB()
		gl.updateMonstersTurnBased()
	} else {
		// Update monsters in parallel with performance monitoring
		gl.game.threading.PerformanceMonitor.ProfiledFunction("entity_update", func() {
			gl.updateMonstersParallel()
		})
	}

	gl.faceMonstersAlongFrameMotion(monsterFrameStart)
	// Parallel RT updates can nominate the same logical post from one frozen
	// snapshot. Serial arbitration runs before combat so only one can strike.
	gl.reconcileMonsterAttackPosts()

	// Non-combat overlaps still get a gentle resolution. Active combatants remain
	// pass-through and can share a transit tile; their logical post reservation,
	// rather than a physical shove, decides who can attack.
	gl.separateOverlappingMonsters()

	// Banding: stack calm same-key flockers onto their leader (or scatter a band
	// whose member just engaged/was hit). Runs after movement+separation so it has
	// the final positions to snap/fan.
	gl.updateMonsterBands()
	// Guard pairs use the same stack/fan presentation but admit mixed monster
	// keys and cap at two. Reconcile after movement so sight aggro scatters the
	// pair before the combat pass and calm followers rejoin their leader.
	gl.reconcileLootPropGuardBands()

	// Alarm bells: an engaged rally monster wakes its neighbours (serial pass -
	// the parallel update must not mutate other monsters).
	gl.game.rallyAggroedAlarms()
	// Transit stacks are cosmetic only: they reuse the band fan without changing
	// band membership or physical positions.
	gl.updateCombatTransitVisualStacks()

	// Update monster hit tint timers
	gl.game.UpdateMonsterHitTintTimers()

	// Handle combat interactions (only in real-time mode)
	if !gl.game.turnBasedMode {
		gl.game.combat.HandleMonsterInteractions()
	}

	// Catch autonomous kills the normal combat paths never saw: RT poison/ignite
	// ticks inside the parallel Monster3D.Update (monster_ai.go TickPoison) and TB
	// TickPoisonTurn can zero a monster's HP with no CombatSystem in scope to run
	// finishMonsterKill itself. Anything left in world.Monsters with IsAlive()
	// false and not already queued in deadMonsterIDs died this way - finish it
	// here so XP/loot/quest-kill-count/band-scatter/collision cleanup still run
	// (steam zones and traps already self-finish via finishIndirectKill).
	gl.finalizeIndirectKills()

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

// faceMonstersAlongFrameMotion is the single source of truth for movement-facing:
// each monster faces its accumulated walk displacement. The capture->face window
// spans only the movement pass, so separation shoves, band snaps and combat
// blinks can never flip a walker. Accumulation (FaceAcc) lets sub-threshold
// walkers still turn while back-and-forth jitter cancels out; standing still
// drops the momentum. Movement helpers don't set m.Direction themselves - only
// no-move state transitions (idle/alert/flee) set an intent facing.
func (gl *GameLoop) captureMonsterFramePositions() []monsterFramePosition {
	if gl.game == nil || gl.game.world == nil || len(gl.game.world.Monsters) == 0 {
		return nil
	}
	positions := gl.monsterFrameBuf[:0]
	for _, m := range gl.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		positions = append(positions, monsterFramePosition{monster: m, x: m.X, y: m.Y})
	}
	gl.monsterFrameBuf = positions
	return positions
}

func (gl *GameLoop) faceMonstersAlongFrameMotion(start []monsterFramePosition) {
	const minMoveForFacing = 1.5
	const minMoveSq = minMoveForFacing * minMoveForFacing
	for _, pos := range start {
		m := pos.monster
		if m == nil || !m.IsAlive() {
			continue
		}
		dx := m.X - pos.x
		dy := m.Y - pos.y
		if dx == 0 && dy == 0 {
			m.FaceAccX, m.FaceAccY = 0, 0
			continue
		}
		m.FaceAccX += dx
		m.FaceAccY += dy
		if m.FaceAccX*m.FaceAccX+m.FaceAccY*m.FaceAccY < minMoveSq {
			continue
		}
		m.Direction = math.Atan2(m.FaceAccY, m.FaceAccX)
		m.FaceAccX, m.FaceAccY = 0, 0
	}
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
	// the scene goes through a horizontal motion-blur shader (camera blur - the
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
				g.drawTurnBlur(screen, scene, shader, float32(blurPx))
			} else {
				gl.renderer.RenderFirstPersonView(screen) // shader failed to compile - no blur
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
				g.drawTurnBlur(screen, scene, shader, 0)
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

	// Draw UI elements (straight to the screen - never blurred)
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
	gl.game.combat.CheckProjectilePlayerCollisions()
	gl.game.combat.CheckProjectileMonsterCollisions()

	// Convert all projectiles to wrappers and update in parallel
	allProjectiles := gl.game.ConvertProjectilesToWrappers()
	// Projectiles fly over floor-level obstacles (chasms, water) and only stop at
	// solid walls - unlike entity movement, which uses CanMoveTo.
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
	// by design (pathfinding deadlock prevention) - separating them turned
	// every crossing into a push-fight (measured: 1850 one-tick shove episodes
	// per 2 sim-minutes on the forest map). Only non-party fights still need
	// physical separation; party-targeting mobs deliberately overlap in transit.
	requiresSeparation := func(m *monster.Monster3D) bool {
		if m.TargetsParty() {
			return false
		}
		return m.IsInCombat()
	}
	// Tile-checked half-push; also refuses to shove a monster into the PLAYER's
	// box - entity collision is deliberately skipped (the overlapped partner
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
	resolvePair := func(i int, a, b *monster.Monster3D, aw, ah float64) {
		if a.TargetsParty() || b.TargetsParty() {
			return
		}
		bw, bh := b.GetSize()
		dx := b.X - a.X
		dy := b.Y - a.Y
		sepX := (aw+bw)/2 - math.Abs(dx)
		sepY := (ah+bh)/2 - math.Abs(dy)
		if sepX <= 0 || sepY <= 0 {
			return // no overlap
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
	// Every processed pair has a non-party combat side, so collect that alive
	// subset once (reusable buffer): the common calm case exits without any
	// pair scan, and the scan walks allxengaged instead of allxall.
	engagedIdx := gl.sepEngagedBuf[:0]
	for i, m := range monsters {
		if m.IsAlive() && requiresSeparation(m) {
			engagedIdx = append(engagedIdx, i)
		}
	}
	gl.sepEngagedBuf = engagedIdx
	if len(engagedIdx) == 0 {
		return
	}
	// Pairs run in the original (i,j) order: an engaged a pairs with every
	// alive j>i; a calm a pairs only with the engaged mobs after it. Pushes
	// change positions only, so engagement/liveness are constant mid-pass.
	nextEngaged := 0
	for i := 0; i < len(monsters); i++ {
		for nextEngaged < len(engagedIdx) && engagedIdx[nextEngaged] <= i {
			nextEngaged++
		}
		a := monsters[i]
		if !a.IsAlive() {
			continue
		}
		aw, ah := a.GetSize()
		if requiresSeparation(a) {
			for j := i + 1; j < len(monsters); j++ {
				b := monsters[j]
				if !b.IsAlive() {
					continue
				}
				resolvePair(i, a, b, aw, ah)
			}
		} else {
			for _, j := range engagedIdx[nextEngaged:] {
				resolvePair(i, a, monsters[j], aw, ah)
			}
		}
	}
}

// finalizeIndirectKills sweeps for monsters that died from an autonomous tick
// with no CombatSystem in scope to finish the kill itself (RT poison/ignite via
// Monster3D.Update's TickPoison, TB's TickPoisonTurn) - anything left in
// world.Monsters with IsAlive() false and not already queued in deadMonsterIDs
// died this way this frame. Reuses the same scratch set removeDeadMonstersByID
// rebuilds right after, so this costs one extra O(n) pass, no new allocation.
func (gl *GameLoop) finalizeIndirectKills() {
	queued := gl.game.reusableDeadSet
	for k := range queued {
		delete(queued, k)
	}
	for _, id := range gl.game.deadMonsterIDs {
		queued[id] = true
	}
	for _, m := range gl.game.world.Monsters {
		if m == nil || m.IsAlive() || queued[m.ID] {
			continue
		}
		gl.game.combat.finishIndirectKill(m)
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
			gl.game.AddCombatMessage("Quest Completed: Received " + questRewardSummary(questRewards.Gold, questRewards.ArenaPoints, questRewards.Experience) + "!")

			// Award gold to party
			if questRewards.Gold > 0 {
				gl.game.awardGold(questRewards.Gold)
			}
			if questRewards.ArenaPoints > 0 {
				gl.game.awardArenaPoints(questRewards.ArenaPoints)
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
		gl.game.awardGold(rewards.Gold)
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
		gl.game.AddCombatMessage(fmt.Sprintf("%s the %s is freed - they'll wait at the tavern.", c.Name, c.Class.String()))
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

	// Buff-cast overlay animations age out.
	gl.game.tickBuffFx()

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
			if m == nil {
				continue
			}
			if m.RTCooldown > 0 {
				m.RTCooldown--
			}
			if m.OffHandRTCooldown > 0 {
				m.OffHandRTCooldown--
			}
		}
	}

	// Tick every timed party buff and refresh its HUD status from ONE registry.
	for _, b := range gl.game.timedBuffs() {
		tickBuff(b.active, b.duration, b.onExpire)
		gl.game.updateUtilityStatus(b.id, *b.duration, *b.active)
	}
	// Stacking combat buffs (Day of the Gods, Hour of Power, Stone Skin, Heroism)
	// tick from their own list - see combat_buffs.go.
	gl.game.tickCombatBuffs()
	gl.game.tickStatBuffs()
	// Persistent damage zones (Hot Steam): lifetime + real-time damage cadence.
	gl.updateSteamZonesRT()
	// Stone Blossom mortars in flight (detonate on landing; flies in RT and TB).
	gl.game.tickPendingMortars()
	// Armed traps: ambient swirl VFX (both modes) + RT trigger sweep.
	gl.updateTraps()

	// Walk-on-water / water-breathing / fly drive world flags every frame.
	if gl.game.world != nil {
		// The Medusa Card grants permanent walk-on-water on top of the spell.
		gl.game.world.SetWalkOnWaterActive(gl.game.walkOnWaterActive || gl.game.hasCardWalkOnWater())
		gl.game.world.SetWaterBreathingActive(gl.game.waterBreathingActive)
		gl.game.world.SetFlyActive(gl.game.flyActive)
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

// timedBuffs returns the SINGLE registry of duration-based buffs. Its pointers
// are stable for the MMGame lifetime, so the registry is built once and cached
// on the game loop.
func (g *MMGame) timedBuffs() []timedBuff {
	gl := g.gameLoop
	if gl == nil {
		return g.buildTimedBuffs()
	}
	if gl.timedBuffRegistry == nil {
		gl.timedBuffRegistry = g.buildTimedBuffs()
	}
	return gl.timedBuffRegistry
}

// buildTimedBuffs assembles the registry. To add a new timed buff, add one
// entry here - it then ticks, shows its HUD icon, and is restored on load
// automatically, with no other code changes.
func (g *MMGame) buildTimedBuffs() []timedBuff {
	return []timedBuff{
		{"torch_light", &g.torchLightActive, &g.torchLightDuration, nil},
		{"wizard_eye", &g.wizardEyeActive, &g.wizardEyeDuration, nil},
		{"walk_on_water", &g.walkOnWaterActive, &g.walkOnWaterDuration, func() {
			// Lapsing mid-lake strands the party on a blocking water tile;
			// wade ashore unless another effect still handles the water.
			g.settleAfterWalkOnWater()
		}},
		{"fly", &g.flyActive, &g.flyDuration, func() {
			// Fly let the party pass through walls; if it lapses while they hover
			// inside solid terrain, surface them or movement stays wall-locked.
			g.ejectFromWallAfterFly()
		}},
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
// mob re-aggros. (TB control persists the encounter - no real-frame countdown.)
func (gl *GameLoop) updateControlledMonsters() {
	if gl.game.world == nil {
		return
	}
	for _, m := range gl.game.world.Monsters {
		if m.Bound && m.BoundFramesRemaining > 0 {
			m.BoundFramesRemaining--
			if m.BoundFramesRemaining == 0 {
				m.Bound = false
				m.WasAttacked = true // sticky: a freed undead immediately turns hostile
				m.BeginPlayerEngagement()
				gl.game.AddCombatMessage(fmt.Sprintf("%s breaks free of your binding!", m.Name))
			}
		}
		if m.Pacified && m.PacifiedFramesRemaining > 0 {
			m.PacifiedFramesRemaining--
			if m.PacifiedFramesRemaining == 0 {
				m.Pacified = false
				m.WasAttacked = true // sticky: Charm expiry restores hostility immediately
				m.BeginPlayerEngagement()
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

	// Teleport to the safe position (single arrival path: position + autosave).
	gl.inputHandler.finishMapArrival(returnX, returnY, gl.game.camera.Angle)

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
