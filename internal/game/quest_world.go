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
// an older save — maps are NOT reloaded from disk on load).
func (g *MMGame) ensureQuestTileOriginals() {
	if g.questTileOriginals != nil || g.questManager == nil {
		return
	}
	g.questTileOriginals = make(map[string]world.TileType3D)
	for _, def := range g.questManager.Definitions() {
		for _, tc := range def.OnCompleteTiles {
			w := g.worldByKey(tc.Map)
			if w == nil || tc.Y < 0 || tc.Y >= len(w.Tiles) || tc.X < 0 || tc.X >= len(w.Tiles[tc.Y]) {
				continue
			}
			g.questTileOriginals[questTileKey(tc)] = w.Tiles[tc.Y][tc.X]
		}
	}
}

// syncQuestTiles makes the loaded maps MATCH quest state, both ways: a
// completed quest's on_complete_tiles are applied, and a NOT-completed quest's
// positions revert to their pristine originals. The revert half is what keeps
// world changes save-safe — loading a save where the quest isn't done (or
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
			if w == nil || tc.Y < 0 || tc.Y >= len(w.Tiles) || tc.X < 0 || tc.X >= len(w.Tiles[tc.Y]) {
				continue
			}
			if w.Tiles[tc.Y][tc.X] != target {
				w.Tiles[tc.Y][tc.X] = target
				if w == g.world {
					changedCurrent = true
				}
			}
		}
	}
	// The floor renderer bakes tile colors/textures into per-map images at map
	// entry — a swapped tile keeps DRAWING as the old one (walkable water!)
	// until the bake reruns. Other maps re-bake on switch.
	if changedCurrent && g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
	}
}

// applyCompletedQuestTiles is the historical name for syncQuestTiles — kept so
// call sites read naturally at quest-completion triggers.
func (g *MMGame) applyCompletedQuestTiles() { g.syncQuestTiles() }

// worldByKey resolves a map key against the loaded maps, falling back to the
// current world when no world manager exists (minimal test setups).
func (g *MMGame) worldByKey(mapKey string) *world.World3D {
	if wm := world.GlobalWorldManager; wm != nil {
		return wm.LoadedMaps[mapKey]
	}
	return g.world
}

// completeExterminationQuests finishes map-scoped kill quests the moment their
// last living target dies, regardless of the kill counter — "clear the map"
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
		if living < 0 { // not an exterminate quest — sync didn't scan, so do it here
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
			if wm != nil && wm.LoadedMaps[tc.Map] == nil {
				return fmt.Errorf("quest %q on_complete_tiles: unknown map %q", id, tc.Map)
			}
		}
	}
	return nil
}
