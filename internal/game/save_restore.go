package game

import (
	"fmt"
	"time"
	"ugataima/internal/world"
)

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
	g.cancelCampPresentation()
	g.profileKilled = nil
	g.restoreSavedTimeline(wm, save, targetWorld)
	g.restoreSavedParty(save)
	g.updatePartyLevelUnlocks()
	legacyRewards := g.restoreSavedMonsters(wm, save)
	g.restoreSavedNPCs(wm, save)
	g.restoreSavedTurnState(save)
	g.restoreSavedEffects(save)
	g.restoreSavedContainers(wm, save)
	// Containers must exist before migrated rewards can suppress duplicate chests.
	if legacyRewards != nil {
		g.addTreasureChestsFromRewards(legacyRewards)
	}
	g.restoreSavedEffectPresentation(wm, save)
	g.restoreSavedQuests(save)
	// A restored journal is not news, and the loaded run must not inherit the old
	// one's heading or focus identity (loading does NOT reload maps, so NPC
	// pointers survive). One reset, after the journal has settled.
	g.resetScreenBanners()

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
	g.camera.X = playerX
	g.camera.Y = playerY
	g.snapFacing(wm.ProjectAngle(save.MapKey, save.PlayerAngle))
	g.collisionSystem.UpdateEntity("player", playerX, playerY)
}
