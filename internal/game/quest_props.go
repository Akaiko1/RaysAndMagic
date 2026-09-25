package game

import (
	"fmt"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func cloneQuestPropLayouts(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

// Resolve all layouts at new-game time, including quests not yet accepted.
// Old saves without this field receive a choice once, then persist it normally.
func (g *MMGame) resetQuestPropLayouts(saved map[string]string) {
	g.questPropLayouts = map[string]string{}
	if g.questManager == nil {
		return
	}
	for _, id := range sortedMapKeys(g.questManager.Definitions()) {
		d := g.questManager.Definitions()[id]
		layouts := d.PropLayouts()
		if len(layouts) == 0 {
			continue
		}
		choice := saved[id]
		if d.PropsForLayout(choice) == nil {
			choice = layouts[rand.Intn(len(layouts))].ID
		}
		g.questPropLayouts[id] = choice
	}
}

func (g *MMGame) activeQuestPropPlacements(id string) []quests.QuestProp {
	d := g.questManager.Definitions()[id]
	if d == nil {
		return nil
	}
	// Minimal fixtures and newly added content may not have passed new-game setup.
	if len(d.PropLayouts()) > 0 && d.PropsForLayout(g.questPropLayouts[id]) == nil {
		g.resetQuestPropLayouts(g.questPropLayouts)
	}
	return d.PropsForLayout(g.questPropLayouts[id])
}

// syncQuestProps derives the physical roster from quest state. It never paints
// terrain or persists a second copy of activity progress in an NPC visited flag.
// Reusing surviving objects keeps dialog pointers stable across reconciliation.
func (g *MMGame) syncQuestProps() {
	if g.questManager == nil {
		return
	}
	type placement struct {
		owner string
		prop  quests.QuestProp
	}
	wanted := map[*world.World3D][]placement{}
	worlds := map[*world.World3D]bool{}
	if wm := world.GlobalWorldManager; wm != nil {
		wm.EachWorld(func(_ string, w *world.World3D) { worlds[w] = true })
	} else if g.world != nil {
		worlds[g.world] = true
	}
	for _, id := range sortedMapKeys(g.questManager.Definitions()) {
		q := g.questManager.GetQuest(id)
		if q == nil || q.RewardsClaimed || q.Status == quests.QuestStatusFailed {
			continue
		}
		for _, prop := range g.activeQuestPropPlacements(id) {
			if w := g.worldByKey(prop.Map); w != nil {
				worlds[w] = true
				wanted[w] = append(wanted[w], placement{id, prop})
			}
		}
	}
	for w := range worlds {
		if w == nil {
			continue
		}
		existing := map[string]*character.NPC{}
		kept := w.NPCs[:0]
		for _, npc := range w.NPCs {
			if npc != nil && npc.QuestPropOwner != "" {
				existing[npc.Key] = npc
			} else {
				kept = append(kept, npc)
			}
		}
		w.NPCs = kept
		for _, entry := range wanted[w] {
			npc := existing[entry.prop.NPC]
			tx, ty := projectTileToCurrentWorld(entry.prop.Map, entry.prop.X, entry.prop.Y)
			x, y := TileCenterFromTile(tx, ty, float64(g.config.GetTileSize()))
			if npc == nil {
				var err error
				npc, err = character.CreateNPCFromConfig(entry.prop.NPC, x, y)
				if err != nil {
					panic(fmt.Errorf("quest %q active_props: %w", entry.owner, err))
				}
			}
			npc.X, npc.Y, npc.QuestPropOwner, npc.Visited = x, y, entry.owner, false
			if !g.activityNPCAbsent(npc) {
				w.NPCs = append(w.NPCs, npc)
				delete(existing, npc.Key)
			}
		}
		for _, removed := range existing {
			if g.dialogNPC == removed {
				g.closeConversation()
			}
		}
	}
}

// Active props are stateless scenery for activities, whose sequence/selection
// is saved in the quest. Doors, merchants, encounters and painted ground have
// independent state/lifetimes and cannot silently opt into this mechanism.
func validateQuestActiveProps(qm *quests.QuestManager) error {
	if qm == nil || character.NPCConfigInstance == nil {
		return nil
	}
	owners := map[string]string{}
	positions := map[string]string{}
	wm := world.GlobalWorldManager
	placed := placedNPCKeys(wm)
	for _, id := range sortedMapKeys(qm.Definitions()) {
		def := qm.Definitions()[id]
		if def == nil {
			return fmt.Errorf("quest %q has empty definition", id)
		}
		for _, layout := range def.PropLayouts() {
			localOwners := map[string]bool{}
			localPositions := map[string]string{}
			for _, prop := range layout.Props {
				if old := owners[prop.NPC]; (old != "" && old != id) || localOwners[prop.NPC] {
					return fmt.Errorf("quests %q and %q share active_props NPC %q", old, id, prop.NPC)
				}
				owners[prop.NPC] = id
				localOwners[prop.NPC] = true
				if placed[prop.NPC] {
					return fmt.Errorf("quest %q active_props NPC %q is also placed directly on a map", id, prop.NPC)
				}
				npc, err := character.CreateNPCFromConfig(prop.NPC, 0, 0)
				if err != nil {
					return fmt.Errorf("quest %q active_props: %w", id, err)
				}
				owner, words := activityProp(npc)
				if owner != id || words == nil || npc.Type != character.NPCTypeEncounter || npc.RenderCategory != "scenery" || !npc.Transparent || npc.GroundTile != "" || npc.EncounterData != nil {
					return fmt.Errorf("quest %q active_props NPC %q must be its own activity scenery without ground_tile or encounter", id, prop.NPC)
				}
				position := fmt.Sprintf("%s:%d:%d", prop.Map, prop.X, prop.Y)
				// Only one layout per quest exists at a time; other quests may coexist.
				if old := positions[position]; (old != "" && old != id) || localPositions[position] != "" {
					return fmt.Errorf("active_props share position %s", position)
				}
				positions[position] = id
				localPositions[position] = prop.NPC

				if wm == nil || len(wm.FailedMaps) > 0 {
					continue
				}
				w := wm.WorldByKey(prop.Map)
				if w == nil {
					return fmt.Errorf("quest %q active_props: unknown map %q", id, prop.Map)
				}
				width, height := w.Width, w.Height
				if region := wm.OpenWorldRegionByKey(prop.Map); region != nil {
					width, height = region.LocalWidth, region.LocalHeight
				}
				tx, ty := projectTileToCurrentWorld(prop.Map, prop.X, prop.Y)
				if prop.X < 0 || prop.Y < 0 || prop.X >= width || prop.Y >= height || w.IsTileBlockingTerrainAt(tx, ty) {
					return fmt.Errorf("quest %q active_props NPC %q needs an in-bounds walkable tile", id, prop.NPC)
				}
				for _, existing := range w.NPCs {
					if existing != nil && existing.QuestPropOwner == "" && TileIndex(existing.X, config.GlobalConfig.GetTileSize()) == tx && TileIndex(existing.Y, config.GlobalConfig.GetTileSize()) == ty {
						return fmt.Errorf("quest %q active_props NPC %q overlaps NPC %q", id, prop.NPC, existing.Key)
					}
				}
			}
		}
	}
	return nil
}
