package game

import "ugataima/internal/monster"

// runMonsterFrame owns the ordering shared by RT and TB. Worker execution has
// a synchronous barrier; all post arbitration, cross-actor actions and rewards
// run on the game goroutine after it. Projectiles and physical removal follow
// this method in updateExploration.
func (gl *GameLoop) runMonsterFrame() {
	gl.prepareMonsterFrame()
	monsterFrameStart := gl.captureMonsterFramePositions()
	gl.simulateMonsterFrame()
	gl.faceMonstersAlongFrameMotion(monsterFrameStart)
	// Parallel RT updates can nominate the same logical post from one frozen
	// snapshot. Serial arbitration runs before combat so only one can strike.
	gl.reconcileMonsterAttackPosts()

	gl.resolveMonsterFrameActions()
	// Catch autonomous kills the normal combat paths never saw: RT poison/ignite
	// ticks inside the parallel Monster3D.Update (monster_ai.go TickPoison) and TB
	// TickPoisonTurn can zero a monster's HP with no CombatSystem in scope to run
	// finishMonsterKill itself. Anything left in world.Monsters with IsAlive()
	// false and not already queued in deadMonsterIDs died this way - finish it
	// here so XP/loot/quest-kill-count/band-scatter/collision cleanup still run
	// (steam zones and traps already self-finish via finishIndirectKill).
	gl.finalizeIndirectKills()

}

func (gl *GameLoop) prepareMonsterFrame() {
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

}

func (gl *GameLoop) simulateMonsterFrame() {
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

}

func (gl *GameLoop) resolveMonsterFrameActions() {
	// Banding: stack calm same-key flockers onto their leader (or scatter a band
	// whose member just engaged/was hit). Runs after movement so it has the final
	// positions to snap/fan.
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

}
