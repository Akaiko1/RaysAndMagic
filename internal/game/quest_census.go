package game

import (
	"fmt"
	"strings"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// availableKillQuestTargets includes living actors and authored one-shot
// sources that have not fired. Absence before a boss unlock is not a kill.
// A failed/missing map is not evidence that its targets have been cleared.
func (g *MMGame) availableKillQuestTargets(q *quests.Quest, includePendingDeaths bool) (int, bool) {
	def := q.Definition
	wm := world.GlobalWorldManager
	// Source-bound hunts require a loaded world registry to census their seals.
	if def.EncounterOnly && wm == nil {
		return 0, false
	}
	if wm != nil {
		if (def.TargetMap == "" && len(wm.FailedMaps) > 0) || (def.TargetMap != "" && wm.WorldByKey(def.TargetMap) == nil) {
			return 0, false
		}
	}
	count := g.countQuestTargetsFromSource(def, q.ID, includePendingDeaths)
	matches := func(key, source string) bool {
		if def.EncounterOnly && source != q.ID {
			return false
		}
		if monster.MonsterConfig == nil {
			return false
		}
		md, err := monster.MonsterConfig.GetMonsterByKey(key)
		return err == nil && def.MatchesTarget(quests.NormalizeTarget(md.Name))
	}
	onMap := func(w *world.World3D, x, y float64) bool {
		if def.TargetMap == "" || wm == nil {
			return true
		}
		if wm.WorldByKey(def.TargetMap) != w {
			return false
		}
		if region := wm.OpenWorldRegionByKey(def.TargetMap); region != nil {
			return wm.OpenWorldRegionAtTile(TileIndex(x, g.config.GetTileSize()), TileIndex(y, g.config.GetTileSize())) == region
		}
		return true
	}
	if g.questManager != nil {
		for id, source := range g.questManager.Definitions() {
			for _, sp := range source.OnCompleteSpawns {
				if (def.TargetMap == "" || def.TargetMap == sp.Map) && matches(sp.Monster, "") && !g.questSpawnFired(id+"#"+sp.ID) {
					count++
				}
			}
		}
	}
	for _, pending := range g.pendingQuestSpawns {
		m := pending.monster
		if m != nil && m.IsAlive() && !m.QuestProgressIgnored && !def.EncounterOnly && def.MatchesTarget(questMonsterTag(m)) && onMap(pending.world, m.X, m.Y) {
			count++
		}
	}
	scan := func(w *world.World3D) {
		if w == nil {
			return
		}
		for _, npc := range w.NPCs {
			if npc == nil || npc.Visited || !onMap(w, npc.X, npc.Y) {
				continue
			}
			for _, summon := range npc.Summons {
				if summon != nil && matches(summon.Monster, summon.QuestID) {
					count++
					break // alternative offerings consume the same statue
				}
			}
			if enc := npc.EncounterData; enc != nil {
				for _, spawn := range enc.Monsters {
					if spawn != nil && matches(spawn.Type, enc.QuestID) {
						count += spawn.CountMin
					}
				}
			}
		}
	}
	if wm != nil {
		wm.EachWorld(func(_ string, w *world.World3D) { scan(w) })
	} else {
		scan(g.world)
	}
	return count, true
}

// questSpawnFired recognizes current stable spawn keys plus both legacy save
// forms used before spawn IDs became authoritative.
func (g *MMGame) questSpawnFired(ref string) bool {
	if g.questSpawnsDone[ref] {
		return true
	}
	questID, spawnID, ok := strings.Cut(ref, "#")
	if !ok || g.questManager == nil {
		return false
	}
	if g.questSpawnsDone[questID] {
		return true
	}
	def := g.questManager.Definitions()[questID]
	if def == nil {
		return false
	}
	for i, spawn := range def.OnCompleteSpawns {
		if spawn.ID == spawnID {
			return g.questSpawnsDone[fmt.Sprintf("%s#%d", questID, i)]
		}
	}
	return false
}
