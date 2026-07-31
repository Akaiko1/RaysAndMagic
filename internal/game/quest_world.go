package game

import (
	"fmt"
	"sort"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// refreshRepeatableQuests clears every finished repeatable errand at nightfall
// so its giver offers the same task again. The quest is dropped rather than
// rewound: an absent quest is exactly the "never taken" state the NPC dialogue
// machine already reads as an offer. The giver's Visited flag is cleared with
// it - turn-in sets Visited to conclude a giver, and a nightly errand must not
// stay concluded.
func (g *MMGame) refreshRepeatableQuests() {
	if g.questManager == nil {
		return
	}
	refreshed := make(map[string]bool)
	for id, def := range g.questManager.Definitions() {
		if def == nil || !def.Repeatable {
			continue
		}
		q := g.questManager.GetQuest(id)
		if q == nil || !q.Completed || !q.RewardsClaimed {
			continue
		}
		g.questManager.RemoveQuest(id)
		refreshed[id] = true
	}
	if len(refreshed) == 0 {
		return
	}
	for _, npc := range g.allLoadedNPCs() {
		if npc == nil || !npc.Visited {
			continue
		}
		for _, c := range questChoicesOf(npc) {
			if refreshed[c.QuestID] {
				npc.Visited = false
				break
			}
		}
	}
}

// allLoadedNPCs walks every loaded world's NPCs (split maps and the unified
// world alike), so a state sweep cannot miss the map the party is not on.
func (g *MMGame) allLoadedNPCs() []*character.NPC {
	wm := world.GlobalWorldManager
	if wm == nil {
		return nil
	}
	var out []*character.NPC
	seen := make(map[*character.NPC]bool)
	add := func(w *world.World3D) {
		if w == nil {
			return
		}
		for _, npc := range w.NPCs {
			if npc != nil && !seen[npc] {
				seen[npc] = true
				out = append(out, npc)
			}
		}
	}
	for _, w := range wm.LoadedMaps {
		add(w)
	}
	add(wm.OpenWorld)
	return out
}

// questChoicesOf lists an NPC's quest-bearing choices at any nesting depth -
// a give_quest often sits two "info" branches deep.
func questChoicesOf(npc *character.NPC) []*character.NPCDialogueChoice {
	if npc == nil || npc.DialogueData == nil {
		return nil
	}
	var out []*character.NPCDialogueChoice
	var walk func(cs []*character.NPCDialogueChoice)
	walk = func(cs []*character.NPCDialogueChoice) {
		for _, c := range cs {
			if c == nil {
				continue
			}
			if (c.Action == "give_quest" || c.Action == "turn_in_quest") && c.QuestID != "" {
				out = append(out, c)
			}
			walk(c.Choices)
		}
	}
	walk(npc.DialogueData.Choices)
	return out
}

func questMonsterTag(m *monster.Monster3D) string {
	if m == nil {
		return ""
	}
	return quests.NormalizeTarget(m.Name)
}

// countLivingQuestTargets returns living, quest-eligible monsters whose name
// maps to the same normalized target key used by kill progress. TargetMap
// scopes the census to one map or merged-world region; an empty target scans
// every loaded world.
func (g *MMGame) countLivingQuestTargets(def *quests.QuestDefinition) int {
	if def == nil {
		return 0
	}
	targetMap := def.TargetMap
	matches := func(m *monster.Monster3D) bool {
		return def.MatchesTarget(questMonsterTag(m))
	}
	scan := func(w *world.World3D) int {
		if w == nil {
			return 0
		}
		count := 0
		for _, m := range w.Monsters {
			if m == nil || m.HitPoints <= 0 || m.QuestProgressIgnored {
				continue
			}
			if matches(m) {
				count++
			}
		}
		return count
	}

	wm := world.GlobalWorldManager
	if wm == nil {
		return scan(g.world)
	}
	if targetMap != "" {
		// A merged region scopes the census to its rect of the unified world.
		if r := wm.OpenWorldRegionByKey(targetMap); r != nil {
			ts := g.config.GetTileSize()
			count := 0
			for _, m := range wm.OpenWorld.Monsters {
				if m == nil || m.HitPoints <= 0 || m.QuestProgressIgnored {
					continue
				}
				if wm.OpenWorldRegionAtTile(int(m.X/ts), int(m.Y/ts)) != r {
					continue
				}
				if matches(m) {
					count++
				}
			}
			return count
		}
		return scan(wm.LoadedMaps[targetMap])
	}

	total := 0
	wm.EachWorld(func(_ string, w *world.World3D) {
		total += scan(w)
	})
	return total
}

// syncKillQuestCensus returns the live target count for an active kill quest.
// Exterminate counters are derived from that same census, keeping the world
// roster as their single source of truth.
func (g *MMGame) syncKillQuestCensus(q *quests.Quest) (int, bool) {
	if g.questManager == nil || q == nil || q.Completed || q.Definition == nil ||
		q.Definition.Type != quests.QuestTypeKill ||
		q.Definition.EncounterOnly ||
		(q.Definition.TargetMonster == "" && len(q.Definition.TargetMonsters) == 0) {
		return 0, false
	}

	living := g.countLivingQuestTargets(q.Definition)
	if q.Definition.Exterminate {
		if q.DynamicTarget == 0 {
			g.questManager.SetDynamicTarget(q.ID, living+q.CurrentCount)
		}
		g.questManager.SetCurrentCount(q.ID, q.Target()-living)
	}
	return living, true
}

// completeKillQuestIfCleared performs the one census-backed transition from an
// active kill quest to completed and owns its completion announcement.
func (g *MMGame) completeKillQuestIfCleared(q *quests.Quest) bool {
	living, ok := g.syncKillQuestCensus(q)
	if !ok || living > 0 {
		return false
	}
	g.questManager.MarkCompleted(q.ID)
	g.announceQuestCompletion(q)
	return true
}

// Journal ordering ranks. The player's next action decides the order: first
// what can be HANDED IN (there is a reward waiting), then what is still being
// worked on, then what is finished and closed.
const (
	questRankTurnIn = iota // completed, reward not yet claimed
	questRankActive        // still in progress
	questRankDone          // completed and claimed (auto-claimed quests land here)
)

// questJournalRank is THE ordering policy for the quest journal.
func questJournalRank(q *quests.Quest) int {
	switch {
	case q == nil:
		return questRankDone
	case q.Completed && !q.RewardsClaimed:
		return questRankTurnIn
	case !q.Completed:
		return questRankActive
	default:
		return questRankDone
	}
}

// sortQuestJournal orders quests in place for display: by rank, then by name so
// the list is stable frame to frame.
func sortQuestJournal(qs []*quests.Quest) {
	sort.SliceStable(qs, func(i, j int) bool {
		ri, rj := questJournalRank(qs[i]), questJournalRank(qs[j])
		if ri != rj {
			return ri < rj
		}
		return qs[i].Definition.Name < qs[j].Definition.Name
	})
}

// creditQuestIfCleared credits an active kill quest when its targets were
// already gone before the current kill or dialogue interaction.
func (g *MMGame) creditQuestIfCleared(questID string) bool {
	if g.questManager == nil {
		return false
	}
	return g.completeKillQuestIfCleared(g.questManager.GetQuest(questID))
}

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

// applyCompletedQuestTiles runs at every "a quest may have just completed"
// touchpoint: syncs quest world-tiles and fires any pending completion spawns.
func (g *MMGame) applyCompletedQuestTiles() {
	g.syncQuestTiles()
	g.spawnQuestCompletionMonsters(false)
}

func (g *MMGame) announceQuestCompletion(q *quests.Quest) {
	if q == nil || q.Definition == nil {
		return
	}
	if q.RewardsClaimed {
		g.AddCombatMessage(fmt.Sprintf("Quest '%s' completed!", q.Definition.Name))
		return
	}
	g.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Open Quests (J) to claim reward.", q.Definition.Name))
}

// spawnQuestCompletionMonsters places each completed quest's on_complete_spawns
// exactly once per playthrough (the Brood Mother materializing in the emptied
// crater). Unlike quest tiles this is an EVENT, not synced state: each fired
// spawn is recorded in questSpawnsDone under its stable YAML ID. Legacy
// "questID#index" and bare "questID" entries remain accepted. The monster then
// lives or dies through the normal per-map monster save. An on_entry spawn
// waits out the party standing on its map: arrival=true marks a fresh map
// arrival, the only moment such a spawn may fire on the CURRENT map.
func (g *MMGame) spawnQuestCompletionMonsters(arrival bool) {
	if g.questManager == nil {
		return
	}
	if g.questSpawnsDone == nil {
		g.questSpawnsDone = make(map[string]bool)
	}
	ts := float64(g.config.GetTileSize())
	for id, def := range g.questManager.Definitions() {
		if len(def.OnCompleteSpawns) == 0 {
			continue
		}
		q := g.questManager.GetQuest(id)
		if q == nil || !q.Completed {
			continue
		}
		legacyDone := g.questSpawnsDone[id]
		for i, sp := range def.OnCompleteSpawns {
			key := fmt.Sprintf("%s#%s", id, sp.ID)
			legacyIndexKey := fmt.Sprintf("%s#%d", id, i)
			if legacyDone || g.questSpawnsDone[key] || g.questSpawnsDone[legacyIndexKey] {
				continue
			}
			w := g.worldByKey(sp.Map)
			if w == nil {
				continue
			}
			if sp.OnEntry && w == g.world && !arrival {
				continue // deferred: fires on the next arrival here (or once the party leaves)
			}
			tx, ty := projectTileToCurrentWorld(sp.Map, sp.X, sp.Y)
			x, y := TileCenterFromTile(tx, ty, ts)
			m := monster.NewMonster3DFromConfig(x, y, sp.Monster, g.config)
			if w == g.world {
				// Defer to the frame boundary: completion fires MID-ATTACK
				// (finishMonsterKill runs inside projectile/AoE processing), and
				// a boss appended to the live slice here joins the very splash
				// pass that unlocked it - one Fireball could wake (WasAttacked)
				// and burn the passive Brood Mother on arrival. The entry pins
				// its TARGET world: a map switch before the flush must not
				// carry the boss to the destination map.
				g.pendingQuestSpawns = append(g.pendingQuestSpawns, pendingQuestSpawn{world: w, monster: m})
			} else {
				// Collision registers when the map is entered (switchToMap
				// re-registers the destination world's monsters).
				w.Monsters = append(w.Monsters, m)
			}
			g.questSpawnsDone[key] = true
		}
	}
}

// worldByKey resolves a map key against the loaded maps (merged region keys
// resolve to the unified world), falling back to the current world when no
// world manager exists (minimal test setups).
func (g *MMGame) worldByKey(mapKey string) *world.World3D {
	if wm := world.GlobalWorldManager; wm != nil {
		return wm.WorldByKey(mapKey)
	}
	return g.world
}

// reconcileExterminationQuests re-anchors every ACTIVE exterminate quest to the
// live world. Starting quests never pass through handleGiveQuest - the only
// DynamicTarget assigner - so on a fresh run the journal would show the static
// nominal count, and on a loaded save whose targets are ALREADY all dead no
// future kill would ever run the completion check (the Brood Mother could
// never spawn). Must run AFTER the monster rosters exist: new game (post
// world reset) and save load (post monster restore).
func (g *MMGame) reconcileExterminationQuests() {
	if g.questManager == nil {
		return
	}
	completed := false
	for _, q := range g.questManager.GetActiveQuests() {
		if q.Completed || q.Definition == nil ||
			q.Definition.Type != quests.QuestTypeKill || !q.Definition.Exterminate {
			continue
		}
		if g.completeKillQuestIfCleared(q) {
			completed = true
		}
	}
	if completed {
		g.applyCompletedQuestTiles()
	}
}

// pendingQuestSpawn is one deferred quest spawn, pinned to its target world.
type pendingQuestSpawn struct {
	world   *world.World3D
	monster *monster.Monster3D
}

// flushPendingQuestSpawns lands the deferred quest spawns at the frame
// boundary, safely outside any in-flight attack resolution. Each entry lands
// on ITS OWN world: if the party switched maps between the queue and the
// flush, the boss still spawns where it was authored (collision registers on
// the next entry of that map).
func (g *MMGame) flushPendingQuestSpawns() {
	if len(g.pendingQuestSpawns) == 0 {
		return
	}
	for _, sp := range g.pendingQuestSpawns {
		if sp.world == g.world {
			g.registerSpawnedMonster(sp.monster)
			g.AddCombatMessage(fmt.Sprintf("%s has appeared!", sp.monster.Name))
		} else if sp.world != nil {
			sp.world.Monsters = append(sp.world.Monsters, sp.monster)
		}
	}
	g.pendingQuestSpawns = nil
}

// completeClearedKillQuestsForTarget finishes map-scoped kill quests the moment
// their last living target dies, regardless of the kill counter. The shared
// census helper also keeps Exterminate counters aligned with the live roster.
func (g *MMGame) completeClearedKillQuestsForTarget(monsterType string) {
	if g.questManager == nil {
		return
	}
	for _, q := range g.questManager.GetActiveQuests() {
		if q.Definition == nil || q.Definition.Type != quests.QuestTypeKill ||
			q.Definition.TargetMap == "" || !q.Definition.MatchesTarget(monsterType) {
			continue
		}
		g.completeKillQuestIfCleared(q)
	}
}

// validateQuestWorldReferences fails fast on every cross-catalog reference
// used by quest-driven world behavior.
func validateQuestWorldReferences(qm *quests.QuestManager) error {
	if qm == nil {
		return nil
	}
	wm := world.GlobalWorldManager
	for id, def := range qm.Definitions() {
		if def == nil {
			return fmt.Errorf("quest %q has empty definition", id)
		}
		if wm != nil && def.TargetMap != "" && wm.WorldByKey(def.TargetMap) == nil {
			return fmt.Errorf("quest %q references unknown target_map %q", id, def.TargetMap)
		}
		if wm != nil && def.MarkerMap != "" && wm.WorldByKey(def.MarkerMap) == nil {
			return fmt.Errorf("quest %q references unknown marker_map %q", id, def.MarkerMap)
		}
		for _, tc := range def.OnCompleteTiles {
			if world.GlobalTileManager != nil && !world.GlobalTileManager.HasTileKey(tc.Tile) {
				return fmt.Errorf("quest %q on_complete_tiles: unknown tile key %q", id, tc.Tile)
			}
			if wm != nil && wm.WorldByKey(tc.Map) == nil {
				return fmt.Errorf("quest %q on_complete_tiles: unknown map %q", id, tc.Map)
			}
		}
		for _, sp := range def.OnCompleteSpawns {
			if monster.MonsterConfig != nil {
				if _, ok := monster.MonsterConfig.Monsters[sp.Monster]; !ok {
					return fmt.Errorf("quest %q on_complete_spawns: unknown monster key %q", id, sp.Monster)
				}
			}
			if wm != nil && wm.WorldByKey(sp.Map) == nil {
				return fmt.Errorf("quest %q on_complete_spawns: unknown map %q", id, sp.Map)
			}
		}
		for i, itemKey := range def.Rewards.ItemPool {
			if itemKey == "" {
				return fmt.Errorf("quest %q rewards.item_pool[%d] is empty", id, i)
			}
			if _, err := items.TryCreateItemFromYAML(itemKey); err != nil {
				return fmt.Errorf("quest %q rewards.item_pool[%d]: %w", id, i, err)
			}
		}
	}
	if character.NPCConfigInstance != nil {
		for npcKey, npc := range character.NPCConfigInstance.NPCs {
			if npc == nil {
				continue
			}
			if err := npc.Dialogue.WalkChoices(func(choice *character.NPCDialogueChoice) error {
				switch choice.Action {
				case "give_quest", "turn_in_quest":
					if choice.QuestID == "" {
						return fmt.Errorf("NPC %q dialogue action %q has empty quest_id", npcKey, choice.Action)
					}
					if qm.Definitions()[choice.QuestID] == nil {
						return fmt.Errorf("NPC %q dialogue action %q references unknown quest %q", npcKey, choice.Action, choice.QuestID)
					}
				}
				if choice.RequiresQuest != "" && qm.Definitions()[choice.RequiresQuest] == nil {
					return fmt.Errorf("NPC %q dialogue choice references unknown requires_quest %q", npcKey, choice.RequiresQuest)
				}
				return nil
			}); err != nil {
				return err
			}
			for i, summon := range npc.Summons {
				if summon == nil {
					return fmt.Errorf("NPC %q summons[%d] has empty definition", npcKey, i)
				}
				if _, _, ok := config.GetItemDefinitionByName(summon.Statuette); !ok {
					return fmt.Errorf("NPC %q summons[%d] references unknown statuette %q", npcKey, i, summon.Statuette)
				}
				if monster.MonsterConfig != nil {
					if _, ok := monster.MonsterConfig.Monsters[summon.Monster]; !ok {
						return fmt.Errorf("NPC %q summons[%d] references unknown monster %q", npcKey, i, summon.Monster)
					}
				}
				if summon.QuestID != "" && qm.Definitions()[summon.QuestID] == nil {
					return fmt.Errorf("NPC %q summons[%d] references unknown quest %q", npcKey, i, summon.QuestID)
				}
				if summon.QuestID != "" && summon.LockedResponse == "" {
					return fmt.Errorf("NPC %q summons[%d] with quest_id requires locked_response", npcKey, i)
				}
			}
		}
	}
	if monster.MonsterConfig != nil {
		for monsterKey, def := range monster.MonsterConfig.Monsters {
			if def.PassiveUntilQuest != "" && qm.Definitions()[def.PassiveUntilQuest] == nil {
				return fmt.Errorf("monster %q references unknown passive_until_quest %q", monsterKey, def.PassiveUntilQuest)
			}
		}
	}
	return nil
}
