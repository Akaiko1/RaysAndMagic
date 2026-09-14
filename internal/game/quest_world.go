package game

import (
	"fmt"
	"sort"
	"strings"

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

// dialogueQuestChoices lists a dialogue tree's quest-bearing choices at any
// nesting depth - a give_quest often sits two "info" branches deep. It reuses
// the shared WalkChoices traversal, so runtime chain selection and boot-time
// validation always see the same choices in the same order.
func dialogueQuestChoices(d *character.NPCDialogue) []*character.NPCDialogueChoice {
	if d == nil {
		return nil
	}
	var out []*character.NPCDialogueChoice
	_ = d.WalkChoices(func(c *character.NPCDialogueChoice) error {
		if (c.Action == "give_quest" || c.Action == "turn_in_quest") && c.QuestID != "" {
			out = append(out, c)
		}
		return nil
	})
	return out
}

// questChoicesOf lists a live NPC's quest-bearing choices.
func questChoicesOf(npc *character.NPC) []*character.NPCDialogueChoice {
	if npc == nil {
		return nil
	}
	return dialogueQuestChoices(npc.DialogueData)
}

// npcChainQuestIDs is the set of quests one giver actually hands out or takes
// in - the only quests activeChainQuestID can ever select for that NPC.
func npcChainQuestIDs(d *character.NPCDialogue) map[string]bool {
	ids := map[string]bool{}
	for _, c := range dialogueQuestChoices(d) {
		ids[c.QuestID] = true
	}
	return ids
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
				if wm.OpenWorldRegionAtTile(TileIndex(m.X, ts), TileIndex(m.Y, ts)) != r {
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

// questIDLess is THE stable order for quests collected out of the manager's map
// (it walks a Go map, so every consumer that prints or queues them has to impose
// one, or the output differs run to run). Consumers that carry the quest inside
// a bigger value (the banner events) compare with this rather than restating it.
func questIDLess(a, b *quests.Quest) bool { return a.ID < b.ID }

func sortQuestsByID(qs []*quests.Quest) {
	sort.Slice(qs, func(i, j int) bool { return questIDLess(qs[i], qs[j]) })
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
	g.announceQuestCompletionWithMessage(q, "")
}

func (g *MMGame) announceQuestCompletionWithMessage(q *quests.Quest, message string) {
	if q == nil || q.Definition == nil {
		return
	}
	g.playSound(soundQuestComplete)
	if message != "" {
		g.AddCombatMessage(message)
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

// placedNPCKeys is the set of NPC keys actually standing in a loaded world. The
// catalog holds far more entries than any run spawns - an NPC dropped from every
// map still parses fine - so this is what turns "authored" into "reachable".
// Nil means no trustworthy world census is available. A non-nil empty map means
// worlds loaded successfully and contain no NPCs.
func placedNPCKeys(wm *world.WorldManager) map[string]bool {
	if wm == nil || len(wm.FailedMaps) > 0 {
		// A map that failed to load takes its NPCs with it (LoadAllMaps only
		// warns and continues). Judging placement off a partial world turns one
		// broken map into an unbootable game, and blames the content for it.
		return nil
	}
	placed := make(map[string]bool)
	loadedWorld := false
	wm.EachWorld(func(_ string, w *world.World3D) {
		if w == nil {
			return
		}
		loadedWorld = true
		for _, npc := range w.NPCs {
			if npc != nil && npc.Key != "" {
				placed[npc.Key] = true
			}
		}
	})
	if !loadedWorld {
		return nil
	}
	return placed
}

// questIsObtainable reports whether the party can ever hold this quest: it starts
// active, or an NPC that is really out there OFFERS it on a choice the party can
// reach. A gate on anything else is a permanent lock.
//
// placed is the live spawn set. Nil means placement is unavailable and authoring
// alone counts; an empty non-nil set is a trustworthy world with no givers.
func questIsObtainable(qm *quests.QuestManager, questID string, catalog map[string]*character.NPCData, placed map[string]bool) bool {
	return questIsObtainableVia(qm, questID, catalog, placed, map[string]bool{}, map[string]bool{})
}

// questIsObtainableVia is the recursive body. An offer can carry its own
// requires_quest, so "somebody gives it" is only true if that somebody's offer is
// itself reachable - and a chain that loops back on itself (pending marks the
// quests already being resolved) reaches nothing.
//
// Only requires_quest is followed. quest_step scopes a choice to a step of the
// giver's OWN chain, which is a sequencing detail rather than a lock, and
// judging it here would abort boots over authoring that works.
func questIsObtainableVia(qm *quests.QuestManager, questID string, catalog map[string]*character.NPCData,
	placed, pending, obtainable map[string]bool) bool {
	if def := qm.Definitions()[questID]; def != nil && def.IsStartingQuest {
		return true
	}
	if obtainable[questID] {
		return true // answered already: two offers gated on one quest resolve it once
	}
	if pending[questID] {
		// Already being resolved further up: this branch offers nothing, but that
		// is a fact about the CYCLE, not about the quest - another giver may still
		// reach it, so the answer must not be memoized.
		return false
	}
	pending[questID] = true
	defer delete(pending, questID)

	for npcKey, npc := range catalog {
		if npc == nil || (placed != nil && !placed[npcKey]) {
			continue
		}
		if dialogueOffersObtainableQuest(qm, npc.Dialogue, questID, catalog, placed, pending, obtainable) {
			obtainable[questID] = true
			return true
		}
	}
	// Only POSITIVE answers are remembered. A negative can be the product of a
	// cycle cut anywhere below this walk, and another branch may still reach the
	// quest - memoizing it would answer a later branch with a wrong "no".
	return false
}

// dialogueOffersObtainableQuest follows the same navigation topology as the
// runtime: only an info row opens its children, and every requires_quest on the
// path must itself be obtainable. A flat WalkChoices scan loses both facts and
// can certify a quest hidden behind an unreachable parent.
func dialogueOffersObtainableQuest(qm *quests.QuestManager, dialogue *character.NPCDialogue, questID string,
	catalog map[string]*character.NPCData, placed, pending, obtainable map[string]bool) bool {
	if dialogue == nil {
		return false
	}
	var walk func([]*character.NPCDialogueChoice) bool
	walk = func(choices []*character.NPCDialogueChoice) bool {
		for _, c := range choices {
			if c == nil {
				continue
			}
			if c.RequiresQuest != "" &&
				!questIsObtainableVia(qm, c.RequiresQuest, catalog, placed, pending, obtainable) {
				continue
			}
			if c.Action == "give_quest" && c.QuestID == questID {
				return true
			}
			if c.Action == "info" && walk(c.Choices) {
				return true
			}
		}
		return false
	}
	return walk(dialogue.Choices)
}

// ValidateInteractTagProducers fails when an interact quest cannot be finished:
// its tag is credited by nothing, or fewer props carrying that tag are PLACED in
// the world than the errand asks for. Both lock any service gated behind it, and
// the shipped errands run with zero slack - 3 lamps for 3, 7 valves for 7, 5
// racks for 5 - so one prop lost off a map is a dead run.
//
// Called from BOOT (after the maps load), not from validateQuestWorldReferences:
// it needs both real catalogs plus the live spawn set, and that validator is also
// run by fixtures that install a handful of NPCs.
func ValidateInteractTagProducers(qm *quests.QuestManager) error {
	if qm == nil || character.NPCConfigInstance == nil {
		return nil
	}
	producers := interactTagProducers(character.NPCConfigInstance.NPCs)
	placedPerTag, censusErr := placedPropTagCounts(world.GlobalWorldManager)
	if censusErr != nil {
		return censusErr
	}
	ids := sortedMapKeys(qm.Definitions()) // one error first, the same one every run
	for _, id := range ids {
		def := qm.Definitions()[id]
		if def == nil || def.Type != quests.QuestTypeInteract {
			continue
		}
		source, credited := producers[def.TargetMonster]
		if !credited {
			return fmt.Errorf("quest %q counts interact tag %q, which nothing credits: no prop declares it and no code path produces it",
				id, def.TargetMonster)
		}
		// A code-produced tag is repeatable (a bout can be fought again) and has
		// no props to count - but its code path must still be able to fire.
		if source != interactTagFromProp {
			if ready := codeInteractTagReady[def.TargetMonster]; ready != nil && !ready(world.GlobalWorldManager) {
				return fmt.Errorf("quest %q counts code tag %q, but nothing in the loaded world can produce it (the arena tag needs a duel block and a placed duel starter on the same map)",
					id, def.TargetMonster)
			}
			continue
		}
		if placedPerTag == nil {
			// Nothing to judge: no loaded world, or a map failed to load. An empty
			// non-nil census is trustworthy and must reject an impossible quest.
			fmt.Printf("[WARN] quest %q needs %d props tagged %q; the prop census was unavailable and was skipped (failed maps: %v)\n",
				id, def.TargetCount, def.TargetMonster, failedMapsOf(world.GlobalWorldManager))
			continue
		}
		if placed := placedPerTag[def.TargetMonster]; placed < def.TargetCount {
			return fmt.Errorf("quest %q asks for %d of tag %q but only %d such props are placed in the world - it could never be finished",
				id, def.TargetCount, def.TargetMonster, placed)
		}
	}
	return nil
}

// placedPropTagCounts counts distinct one-shot NPC props actually standing in
// loaded worlds. A physical NPC counts once; multiple prop actions on one NPC
// are ambiguous authoring rather than multiple usable objects. A nil map means
// the census is unavailable. A non-nil empty map is a trustworthy count of zero.
func placedPropTagCounts(wm *world.WorldManager) (map[string]int, error) {
	if wm == nil || len(wm.FailedMaps) > 0 {
		// Same rule as placedNPCKeys: a map that failed to load took its props
		// with it, and counting off a partial world turns one broken map into an
		// unbootable game that blames the quest data for it.
		return nil, nil
	}
	counts := map[string]int{}
	seenNPCs := make(map[*character.NPC]struct{})
	loadedWorld := false
	var censusErr error
	wm.EachWorld(func(_ string, w *world.World3D) {
		if w == nil || censusErr != nil {
			return
		}
		loadedWorld = true
		for _, npc := range w.NPCs {
			if npc == nil || npc.DialogueData == nil {
				continue
			}
			if _, duplicate := seenNPCs[npc]; duplicate {
				continue
			}
			seenNPCs[npc] = struct{}{}
			propTag := ""
			propChoices := 0
			if err := npc.DialogueData.WalkChoices(func(c *character.NPCDialogueChoice) error {
				if c.Prop == nil {
					return nil
				}
				propChoices++
				if propChoices > 1 {
					return fmt.Errorf("NPC %q declares multiple one-shot prop actions; one physical NPC can credit only one prop", npc.Key)
				}
				propTag = c.Prop.Tag
				return nil
			}); err != nil {
				censusErr = err
				return
			}
			if propTag != "" {
				counts[propTag]++
			}
		}
	})
	if censusErr != nil {
		return nil, censusErr
	}
	if !loadedWorld {
		return nil, nil
	}
	return counts, nil
}

// failedMapsOf reports the maps LoadAllMaps could not read, for diagnostics.
func failedMapsOf(wm *world.WorldManager) []string {
	if wm == nil {
		return nil
	}
	return wm.FailedMaps
}

// interactTagSource is WHO credits an interact tag. The distinction is not
// cosmetic: a prop is a one-shot object that has to be standing in the world,
// a code tag is an event that can happen again, so only prop tags get counted.
type interactTagSource int

const (
	interactTagFromProp interactTagSource = iota // an authored prop:/tag block
	interactTagFromCode                          // produced by a code path (the arena bout)
)

// codeInteractTagReady names every tag no prop declares, WITH the world condition
// that makes its code path able to fire. Naming a code tag is not evidence: the
// arena bout credits only while some map authors a `duel:` block, so deleting
// that block silently locks pit_standing - and with it Nadira's training - while
// the validator reports the tag as credited.
var codeInteractTagReady = map[string]func(*world.WorldManager) bool{
	quests.ArenaDuelTag: func(wm *world.WorldManager) bool {
		// Nothing to judge: no manager, a catalog-less fixture, or a run where a
		// map failed to load - the same stand-down the prop census takes, so a
		// broken map file is never reported as a quest-data fault.
		if wm == nil || len(wm.MapConfigs) == 0 || len(wm.FailedMaps) > 0 {
			return true
		}
		loadedWorld := len(wm.LoadedMaps) > 0 || wm.OpenWorld != nil
		if !loadedWorld {
			return true // a catalog-only fixture has no placement census to judge
		}
		tileSize := 0.0
		if config.GlobalConfig != nil {
			tileSize = config.GlobalConfig.GetTileSize()
		}
		for mapKey, mc := range wm.MapConfigs {
			if mc == nil || mc.Duel == nil {
				continue
			}
			if placedNPCWithActionOnMap(wm, mapKey, "start_arena_duel", tileSize) != nil {
				return true
			}
		}
		return false
	},
}

// interactTagProducers is every tag something in the game can actually credit:
// the authored props declare theirs, the code tags declare their own.
func interactTagProducers(catalog map[string]*character.NPCData) map[string]interactTagSource {
	tags := map[string]interactTagSource{}
	for tag := range codeInteractTagReady {
		tags[tag] = interactTagFromCode
	}
	for _, npc := range catalog {
		if npc == nil {
			continue
		}
		_ = npc.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			// A code tag stays a code tag even if a prop also declares it (a
			// trophy rack beside the pit that credits a bout): the code path still
			// produces it, so demanding one placed prop per target would abort the
			// boot on content that works.
			if c.Prop != nil && c.Prop.Tag != "" {
				if _, code := codeInteractTagReady[c.Prop.Tag]; !code {
					tags[c.Prop.Tag] = interactTagFromProp
				}
			}
			return nil
		})
	}
	return tags
}

// validateQuestWorldReferences fails fast on every cross-catalog reference
// used by quest-driven world behavior.
func (g *MMGame) validateQuestWorldReferences(qm *quests.QuestManager) error {
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
		catalog := character.NPCConfigInstance.NPCs
		placed := placedNPCKeys(wm)
		for _, npcKey := range sortedMapKeys(catalog) {
			npc := catalog[npcKey]
			if npc == nil {
				continue
			}
			// Per-step copy and per-step choices are selected by
			// activeChainQuestID, which only ever returns a quest THIS NPC hands
			// out or takes in. A globally valid id from another giver would load
			// fine and then never appear, so both fields are checked against the
			// giver's own chain.
			chainQuests := npcChainQuestIDs(npc.Dialogue)
			// A service gate has to be OPENABLE, not merely well-spelled. Three
			// ways it can be authored into a permanent lock, all rejected here.
			if npc.RequiresQuest != "" {
				if qm.Definitions()[npc.RequiresQuest] == nil {
					return fmt.Errorf("NPC %q references unknown requires_quest %q", npcKey, npc.RequiresQuest)
				}
				hasService, err := g.npcDataHasGatedService(npcKey)
				if err != nil {
					return fmt.Errorf("NPC %q gates on %q but cannot be built: %w", npcKey, npc.RequiresQuest, err)
				}
				if !hasService {
					return fmt.Errorf("NPC %q sets requires_quest %q but owns no service to gate", npcKey, npc.RequiresQuest)
				}
				// The gate quest must be reachable at all: a typo that lands on
				// another real quest id - or a giver that no map spawns any more -
				// passes the existence check above and then shuts the shop forever.
				if !questIsObtainable(qm, npc.RequiresQuest, catalog, placed) {
					return fmt.Errorf("NPC %q gates on quest %q that no PLACED NPC gives and that does not start active - the service could never open",
						npcKey, npc.RequiresQuest)
				}
				// And the gated NPC must be able to say something about it: either
				// it hands the errand out itself, or it authors the greeting that
				// explains why the shop is shut.
				if !chainQuests[npc.RequiresQuest] && (npc.Dialogue == nil || npc.Dialogue.QuestGreeting == "") {
					return fmt.Errorf("NPC %q gates on quest %q it does not give: it needs a quest_greeting explaining the closed service", npcKey, npc.RequiresQuest)
				}
			}
			if npc.Dialogue != nil {
				for questID := range npc.Dialogue.QuestMessages {
					if qm.Definitions()[questID] == nil {
						return fmt.Errorf("NPC %q dialogue quest_messages references unknown quest %q", npcKey, questID)
					}
					if !chainQuests[questID] {
						return fmt.Errorf("NPC %q dialogue quest_messages references quest %q that this NPC never gives or takes in", npcKey, questID)
					}
				}
			}
			if err := npc.Dialogue.WalkChoices(func(choice *character.NPCDialogueChoice) error {
				isQuestProp := choice.Prop != nil
				// The reserved action and the prop block travel together: a block on
				// any other action would be silently overridden by the prop route, and
				// the action alone would fall through the switch and do nothing.
				if isQuestProp != (choice.Action == questPropAction) {
					return fmt.Errorf("NPC %q dialogue action %q: a quest prop must declare action %q and nothing else may (prop block present: %v)",
						npcKey, choice.Action, questPropAction, isQuestProp)
				}
				switch {
				case choice.Action == "give_quest", choice.Action == "turn_in_quest", isQuestProp:
					// Prop actions carry the quest they count toward: without it the
					// prop would consume itself and advance nothing.
					if choice.QuestID == "" {
						return fmt.Errorf("NPC %q dialogue action %q has empty quest_id", npcKey, choice.Action)
					}
					def := qm.Definitions()[choice.QuestID]
					if def == nil {
						return fmt.Errorf("NPC %q dialogue action %q references unknown quest %q", npcKey, choice.Action, choice.QuestID)
					}
					// A prop credits its quest through OnInteract, which only advances
					// INTERACT quests carrying a tag. Point one at a kill quest (a
					// plausible copy-paste) and the prop still consumes itself, prints
					// "(0/3)" and can never be undone - the soft-lock this branch
					// exists to prevent.
					if isQuestProp {
						if def.Type != quests.QuestTypeInteract {
							return fmt.Errorf("NPC %q prop action %q counts toward quest %q of type %q, want %q",
								npcKey, choice.Action, choice.QuestID, def.Type, quests.QuestTypeInteract)
						}
						// An interact quest with no tag at all: LoadQuestConfig refuses one,
						// but a definition built in code does not pass through it. Named
						// separately from the mismatch below because the fix is different -
						// author target_monster, rather than align two tags.
						// (target_monsters is deliberately NOT accepted here: an interact
						// quest counts ONE tag, and LoadQuestConfig rejects the list form.)
						if def.TargetMonster == "" {
							return fmt.Errorf("NPC %q prop action %q counts toward quest %q, which carries no interact tag (target_monster)",
								npcKey, choice.Action, choice.QuestID)
						}
						// The prop's own tag must be the quest's: quest_id alone routes the
						// credit, so a copy-pasted id would let a valve advance the lamps.
						if choice.Prop.Tag == "" || def.TargetMonster != choice.Prop.Tag {
							return fmt.Errorf("NPC %q prop action %q credits tag %q but quest %q counts %q",
								npcKey, choice.Action, choice.Prop.Tag, choice.QuestID, def.TargetMonster)
						}
						if choice.Prop.NotYet == "" || choice.Prop.Took == "" || choice.Prop.Completed == "" {
							return fmt.Errorf("NPC %q prop action %q is missing prop copy (not_yet/took/completed)", npcKey, choice.Action)
						}
						// A spent prop must either LEAVE the world (hide_when_visited, like
						// a lifted lamp) or be able to say it is spent - otherwise the
						// dialog closes in silence when the player tries it again.
						if !npc.HideWhenVisited && (npc.Dialogue == nil || npc.Dialogue.VisitedMessage == "") {
							return fmt.Errorf("NPC %q carries prop action %q but neither hides when visited nor authors a visited_message",
								npcKey, choice.Action)
						}
						if (choice.Prop.LootTable == "") != (choice.Prop.LootLine == "") {
							return fmt.Errorf("NPC %q prop action %q must author loot_table and loot_line together", npcKey, choice.Action)
						}
						if choice.Prop.LootTable != "" {
							if _, ok := config.GetWeightedLootTable(choice.Prop.LootTable); !ok {
								return fmt.Errorf("NPC %q prop action %q rolls unknown loot table %q", npcKey, choice.Action, choice.Prop.LootTable)
							}
							// The line is printed with fmt.Sprintf and the item list: any
							// other count prints %!(EXTRA ...) into the combat log.
							if strings.Count(choice.Prop.LootLine, "%s") != 1 ||
								strings.Count(choice.Prop.LootLine, "%") != 1 {
								return fmt.Errorf("NPC %q prop action %q loot_line %q must contain exactly one %%s",
									npcKey, choice.Action, choice.Prop.LootLine)
							}
						}
					}
				}
				if choice.RequiresQuest != "" && qm.Definitions()[choice.RequiresQuest] == nil {
					return fmt.Errorf("NPC %q dialogue choice references unknown requires_quest %q", npcKey, choice.RequiresQuest)
				}
				if choice.QuestStep != "" {
					if qm.Definitions()[choice.QuestStep] == nil {
						return fmt.Errorf("NPC %q dialogue choice references unknown quest_step %q", npcKey, choice.QuestStep)
					}
					if !chainQuests[choice.QuestStep] {
						return fmt.Errorf("NPC %q dialogue choice pins quest_step %q that this NPC never gives or takes in", npcKey, choice.QuestStep)
					}
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
