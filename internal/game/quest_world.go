package game

import (
	"fmt"

	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// questTileKey identifies one quest-changed tile position across maps.
func questTileKey(tc quests.QuestTileChange) string {
	return fmt.Sprintf("%s:%d:%d", tc.Map, tc.X, tc.Y)
}

// ensureQuestTileOriginals captures, once, the pristine tile at every position
// any quest's on_complete_tiles targets. Maps always load pristine from disk
// (boot, WorldManager.Reset), and this runs before the first quest change is
// applied, so what it sees IS the original. Needed to make syncQuestTiles
// reversible: without the originals, a bridge laid in one run could never be
// taken back off a world instance that outlives the quest state (e.g. loading
// an older save - maps are NOT reloaded from disk on load).
func (g *MMGame) ensureQuestTileOriginals() {
	if g.questTileOriginals != nil || g.questManager == nil {
		return
	}
	g.questTileOriginals = make(map[string]world.TileType3D)
	for _, def := range g.questManager.Definitions() {
		for _, tc := range def.OnCompleteTiles {
			w := g.worldByKey(tc.Map)
			// Quest tiles are authored map-local; a merged region projects into
			// the unified grid. The originals map stays keyed by LOCAL coords.
			tx, ty := projectTileToCurrentWorld(tc.Map, tc.X, tc.Y)
			if w == nil || ty < 0 || ty >= len(w.Tiles) || tx < 0 || tx >= len(w.Tiles[ty]) {
				continue
			}
			g.questTileOriginals[questTileKey(tc)] = w.Tiles[ty][tx]
		}
	}
}

// syncQuestTiles makes the loaded maps MATCH quest state, both ways: a
// completed quest's on_complete_tiles are applied, and a NOT-completed quest's
// positions revert to their pristine originals. The revert half is what keeps
// world changes save-safe - loading a save where the quest isn't done (or
// resetting quests for a new run) must take the bridge back out of the shared
// loaded-map instances, which persist across loads. Idempotent; called
// whenever a quest may have just completed and after a save restores quest
// state.
func (g *MMGame) syncQuestTiles() {
	if g.questManager == nil || world.GlobalTileManager == nil {
		return
	}
	g.ensureQuestTileOriginals()

	completed := make(map[string]bool)
	for _, q := range g.questManager.GetAllQuests() {
		if q.Completed {
			completed[q.ID] = true
		}
	}

	changedCurrent := false
	for id, def := range g.questManager.Definitions() {
		for _, tc := range def.OnCompleteTiles {
			var target world.TileType3D
			if completed[id] {
				t, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.Tile)
				if !ok {
					continue // validated at startup; unknown key here means a test stub
				}
				target = t
			} else {
				t, ok := g.questTileOriginals[questTileKey(tc)]
				if !ok {
					continue
				}
				target = t
			}
			w := g.worldByKey(tc.Map)
			tx, ty := projectTileToCurrentWorld(tc.Map, tc.X, tc.Y)
			if w == nil || ty < 0 || ty >= len(w.Tiles) || tx < 0 || tx >= len(w.Tiles[ty]) {
				continue
			}
			if w.Tiles[ty][tx] != target {
				w.Tiles[ty][tx] = target
				if w == g.world {
					changedCurrent = true
				}
			}
		}
	}
	if changedCurrent {
		g.refreshWorldTileRenderCaches()
	}
}

// dropPropTilesFromRenderCaches is the CHEAP half of a tile change: tiles that
// LOST their prop are filtered out of the sprite caches in place. Use it instead
// of refreshWorldTileRenderCaches when nothing was added, since the full rebuild
// rescans the world, drops the processed-sprite cache and re-arms the prewarm.
func (g *MMGame) dropPropTilesFromRenderCaches(tiles map[[2]int]bool) {
	if g.gameLoop == nil || len(tiles) == 0 {
		return
	}
	if r := g.gameLoop.renderer; r != nil {
		r.transparentSpritesCache = filterOutPropTiles(r.transparentSpritesCache, tiles)
		r.treeTilesCache = filterOutPropTiles(r.treeTilesCache, tiles)
		r.clearCanopyShadeCache() // canopy shade is derived from tree tiles; rebuilt lazily
		r.precomputeFloorColorCache()
	}
	if g.gameLoop.ui != nil {
		g.gameLoop.ui.invalidateCompassTileLayer()
	}
}

func filterOutPropTiles(cache []TransparentSpriteData, drop map[[2]int]bool) []TransparentSpriteData {
	kept := cache[:0]
	for _, sprite := range cache {
		if drop[[2]int{sprite.tileX, sprite.tileY}] {
			continue
		}
		kept = append(kept, sprite)
	}
	return kept
}

// refreshWorldTileRenderCaches re-bakes every renderer cache keyed off world
// TILES - floor colour bake, the transparent-sprite/prop inventory, and the
// compass tile layer. Call it after ANY runtime tile change (quest world change,
// Earthquake toppling props) or the old tiles keep being drawn.
func (g *MMGame) refreshWorldTileRenderCaches() {
	if g.gameLoop == nil {
		return
	}
	if g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}
	if g.gameLoop.ui != nil {
		g.gameLoop.ui.invalidateCompassTileLayer()
	}
}

// applyCompletedQuestTiles is the historical name for syncQuestTiles - kept so
// call sites read naturally at quest-completion triggers.
func (g *MMGame) applyCompletedQuestTiles() { g.syncQuestTiles() }

// worldByKey resolves a map key against the loaded maps (merged region keys
// resolve to the unified world), falling back to the current world when no
// world manager exists (minimal test setups).
func (g *MMGame) worldByKey(mapKey string) *world.World3D {
	if wm := world.GlobalWorldManager; wm != nil {
		return wm.WorldByKey(mapKey)
	}
	return g.world
}

// completeExterminationQuests finishes map-scoped kill quests the moment their
// last living target dies, regardless of the kill counter - "clear the map"
// semantics (the counter can drift: targets slain before the quest was taken,
// or killed on other maps). Mirrors creditClearedKillQuests, which grants the
// same credit at dialogue time.
func (g *MMGame) completeExterminationQuests(monsterType string) {
	if g.questManager == nil {
		return
	}
	for _, q := range g.questManager.GetActiveQuests() {
		if q.Definition.Type != quests.QuestTypeKill ||
			q.Definition.TargetMap == "" || q.Definition.TargetMonster != monsterType {
			continue
		}
		living := g.syncExterminationQuestProgress(q.ID)
		if living < 0 { // not an exterminate quest - sync didn't scan, so do it here
			living = g.countLivingQuestTargets(q.Definition.TargetMonster, q.Definition.TargetMap)
		}
		if living == 0 {
			g.questManager.MarkCompleted(q.ID)
			g.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Open Quests (J) to claim reward.", q.Definition.Name))
		}
	}
}

// validateQuestTileChanges fails fast at startup on a quest tile change that
// names an unknown tile key or map, instead of silently never appearing.
func validateQuestTileChanges(qm *quests.QuestManager) error {
	if qm == nil || world.GlobalTileManager == nil {
		return nil
	}
	wm := world.GlobalWorldManager
	for id, def := range qm.Definitions() {
		for _, tc := range def.OnCompleteTiles {
			if !world.GlobalTileManager.HasTileKey(tc.Tile) {
				return fmt.Errorf("quest %q on_complete_tiles: unknown tile key %q", id, tc.Tile)
			}
			if wm != nil && wm.WorldByKey(tc.Map) == nil {
				return fmt.Errorf("quest %q on_complete_tiles: unknown map %q", id, tc.Map)
			}
		}
	}
	return nil
}
