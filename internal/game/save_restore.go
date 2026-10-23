package game

import (
	"fmt"
	"time"
	"ugataima/internal/world"
)

// Saves from stackLayoutSaveVersion on keep the player's separate stacks of
// one item; an older save (canonical bags, or loose pre-stacking units) is
// folded once by MergeStacks on load, after the stash strip.
const stackLayoutSaveVersion = 1

// applySave keeps the restore phases in dependency order. Travel deliberately
// uses a different lifecycle: loading replaces the timeline and never autosaves
// an intermediate state or consumes travel-only quest spawns.
func (g *MMGame) applySave(wm *world.WorldManager, source *GameSave) error {
	save, targetWorld, err := prepareSaveRestore(wm, source)
	if err != nil {
		return err
	}
	if save.MapKey != wm.CurrentMapKey {
		if err := wm.SwitchToMap(save.MapKey); err != nil {
			return err
		}
	}
	g.applyTerrainChanges(g.terrainChanges, true)
	g.terrainChanges = nil
	g.cancelDayNightSkip()
	g.cancelCampPresentation()
	g.closeStoryScreen()
	g.profileKilled = nil
	g.discardFish()
	wm.RandomizeEnvironmentSprites()
	g.adventure = cloneAdventureState(save.Adventure)
	for key, v := range g.adventure.Visits {
		v.migrateLegacy(g.adventureConfig(key), g.config.GetTPS())
	}
	g.restoreSavedTimeline(wm, save, targetWorld)
	g.restoreSavedParty(save)
	g.updatePartyLevelUnlocks()
	legacyRewards := g.restoreSavedMonsters(wm, save)
	g.restoreSavedNPCs(wm, save)
	g.restoreSavedTurnState(save)
	g.restoreSavedEffects(save)
	g.restoreSavedContainers(wm, save)
	g.reconcilePartyAgainstStash()
	if save.StackLayoutVersion < stackLayoutSaveVersion {
		g.party.MergeStacks()
	}
	g.warmPartyWeaponModels()
	// The aggregate is DERIVED from the restored registry (never trusted from
	// the save) - a drifted legacy save can't turn a buff expiry into a
	// permanent debuff. Also re-derives members' MaxHP/MaxSP under the buffs.
	g.recomputeStatBonuses()
	// Containers must exist before migrated rewards can suppress duplicate chests.
	if legacyRewards != nil {
		g.addTreasureChestsFromRewards(legacyRewards)
	}
	g.restoreSavedEffectPresentation(wm, save)
	g.restoreStory(save) // before the journal: its reconciliation reads this save's reveals
	g.restoreSavedQuests(save)
	g.resyncLegacyStory(save)
	g.restoreWatcher(save)
	g.restoreTerrainChanges(save.TerrainChanges)
	g.settleStrandedMonsters() // after ice and terrain are restored
	// A position saved on an older map layout can sit inside what is now a
	// wall; clamp it to walkable ground. Runs here, after the buff restore
	// above, so water/Fly saves keep their legal mid-lake or airborne spot.
	if sx, sy := g.safePartyDestination(g.camera.X, g.camera.Y); sx != g.camera.X || sy != g.camera.Y {
		g.setPartyPosition(sx, sy)
	}
	g.arenaBarrierMessageAfter = 0
	g.syncAdventureArenaBounds()
	g.ecology = cloneEcologyState(save.Ecology)
	g.ecology.migrateLegacy(g.config.GetTPS())
	g.ecologyViews = nil
	g.caravanAttackAlertUntil = time.Time{}
	g.syncCaravanStock()
	// A restored journal is not news, and the loaded run must not inherit the old
	// one's heading or focus identity (loading does NOT reload maps, so NPC
	// pointers survive). One reset, after the journal has settled.
	g.resetScreenBanners()
	g.reconcileRestoredPursuit(wm)
	// Gear the party restore moved is reported in the loaded game's own log.
	for _, notice := range g.loadNotices {
		g.AddCombatMessage(notice)
	}
	g.loadNotices = nil
	// A fight resumed from a save is not a fresh fight: no new opener. The
	// flag reads the restored monsters.
	g.partyWasInFight = g.partyInCombat()

	// Restore played time by adjusting session start
	if save.PlayedTimeNs > 0 {
		g.sessionStartTime = time.Now().Add(-time.Duration(save.PlayedTimeNs))
	}

	return nil
}

func prepareSaveRestore(wm *world.WorldManager, save *GameSave) (*GameSave, *world.World3D, error) {
	if wm == nil || save == nil {
		return nil, nil, fmt.Errorf("cannot restore a save without a world manager and save data")
	}
	// Resolve map identity before touching either timeline. Old saves without a
	// key deliberately inherit the current map, including its coordinate frame.
	restored := *save
	if restored.MapKey == "" {
		restored.MapKey = wm.CurrentMapKey
	}
	save = &restored
	targetWorld := wm.LoadedMaps[save.MapKey]
	if targetWorld == nil && wm.IsOpenWorldRegion(save.MapKey) {
		targetWorld = wm.OpenWorld
	}
	if targetWorld == nil {
		return nil, nil, fmt.Errorf("saved map is not loaded: %s", save.MapKey)
	}
	migrateRespawnDays(save)
	return save, targetWorld, nil
}

func (g *MMGame) restoreSavedTimeline(wm *world.WorldManager, save *GameSave, targetWorld *world.World3D) {
	// Loading replaces the timeline, including any carried split and picker.
	g.closeConversation()
	g.closeMainMenu()
	g.clearFocusMode()
	// Update world reference and visuals
	g.world = targetWorld

	// Deferred quest spawns belong to the timeline being replaced. Unlike a
	// normal map switch, loading another slot must discard them before the
	// loaded quest snapshot can enqueue its own completion spawns.
	g.pendingQuestSpawns = nil
	g.clearTransientCombatState()

	// Restore the day/night clock BEFORE the sky refresh below so the panorama
	// resolves to the saved phase. Recomputed silently (no flip side effects):
	// the save's pack monsters are restored as part of MapMonsters.
	g.maxPartyLevel = save.MaxPartyLevel
	g.dayNightFrames = save.DayNightFrames
	g.dayNightDay = save.DayNightDay
	g.calendarDay, g.calendarWeek, g.calendarMonth = calendarFromSave(save.CalendarDay, save.CalendarWeek, save.CalendarMonth, save.DayNightDay, g.config.DayNight)
	g.arenaTierFoughtDay = save.ArenaTierFoughtDay
	{
		names := make([]string, 0, len(save.Party.Members))
		for _, m := range save.Party.Members {
			names = append(names, m.Name)
		}
		g.playthroughID = adoptPlaythroughID(save.ArenaRunID, names)
	}
	g.dayNightIsNight = dayNightIsNightAt(g.dayNightFrac())

	// A loaded game starts with a clean combat log - the previous slot's history
	// must not bleed into it (clearTransientCombatState runs on every map switch
	// too, so the log reset lives here, on the load path, not in it).
	g.combatLogHistory = g.combatLogHistory[:0]
	g.combatLogVersion++
	g.combatLogScroll = 0
	g.combatLogOpen = false

	g.UpdateSkyAndGroundColors()
	g.collisionSystem.UpdateTileChecker(g.world)
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}

	// Restore player. Saves hold map-local coordinates and heading (local
	// canon): a merged region key projects into the unified grid via the
	// current layout and placement orientation.
	playerX, playerY := wm.ProjectWorldPos(save.MapKey, save.PlayerX, save.PlayerY)
	g.setPartyPosition(playerX, playerY)
	g.snapFacing(wm.ProjectAngle(save.MapKey, save.PlayerAngle))
}
