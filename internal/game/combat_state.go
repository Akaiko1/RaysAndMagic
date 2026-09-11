package game

import ()

// clearTransientCombatState drops every in-flight transient tied to the
// CURRENT world: projectiles, swings, VFX and pending deaths. Must run on any
// world swap (map switch, save load) or leftovers keep updating against the
// new map and can hit monsters there.
func (g *MMGame) clearTransientCombatState() {
	// Door state is per-map: entities unregister and closed-ness resets, so the
	// "portcullises rise" transition can't fire on the destination map.
	// NOTE: per-card summon cooldowns deliberately survive here - they are
	// persisted balance timers, and clearing them would let a map switch or a
	// quick reload bypass the authored delay.
	g.clearDoorState()
	g.clearBuildingEntities()
	g.clearLockedDoorEntities()
	g.projectileMutex.Lock()
	if g.collisionSystem != nil {
		// applySave rebuilds the collision system anyway; switchToMap keeps it,
		// so stale projectile entities must be dropped explicitly.
		for i := range g.magicProjectiles {
			g.collisionSystem.UnregisterEntity(g.magicProjectiles[i].ID)
		}
		for i := range g.arrows {
			g.collisionSystem.UnregisterEntity(g.arrows[i].ID)
		}
	}
	g.magicProjectiles = g.magicProjectiles[:0]
	g.arrows = g.arrows[:0]
	g.projectileMutex.Unlock()
	// Stone Blossom blooms scheduled mid-flight belong to the map they were
	// aimed on - a map switch/load must not detonate them at the same
	// coordinates of the destination map.
	g.pendingMortars = g.pendingMortars[:0]
	g.slashEffects = g.slashEffects[:0]
	g.hitEffectsMu.Lock()
	g.spellHitEffects = g.spellHitEffects[:0]
	g.impactLights = g.impactLights[:0]
	g.hitEffectsMu.Unlock()
	g.deadMonsterIDs = g.deadMonsterIDs[:0]
	g.clearPartyScaleStacks()
	// The Brood Mother's field is map-local. Save loading restores the loaded
	// field after this cleaner; an ordinary map switch must leave no old tiles
	// to render, detonate, or leak into the destination autosave.
	g.bossFireTraps = nil
	g.bossFireTrapsOwner = ""
	// A suspended TB turn belongs to the old world. applySave restores the
	// suspension from its own snapshot after this cleaner returns.
	g.turnBasedTurnSuspended = false
}
