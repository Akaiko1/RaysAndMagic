package game

import (
	"fmt"

	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// applyCompletedQuestTiles applies the on_complete_tiles of every completed
// quest to the loaded maps (idempotent). Called whenever a quest may have just
// completed and after a save restores quest state, so world changes always
// match quest status.
func (g *MMGame) applyCompletedQuestTiles() {
	if g.questManager == nil || world.GlobalTileManager == nil {
		return
	}
	changedCurrent := false
	for _, q := range g.questManager.GetAllQuests() {
		if !q.Completed {
			continue
		}
		for _, tc := range q.Definition.OnCompleteTiles {
			tileType, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.Tile)
			if !ok {
				continue // validated at startup; unknown key here means a test stub
			}
			w := g.worldByKey(tc.Map)
			if w == nil || tc.Y < 0 || tc.Y >= len(w.Tiles) || tc.X < 0 || tc.X >= len(w.Tiles[tc.Y]) {
				continue
			}
			if w.Tiles[tc.Y][tc.X] != tileType {
				w.Tiles[tc.Y][tc.X] = tileType
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
