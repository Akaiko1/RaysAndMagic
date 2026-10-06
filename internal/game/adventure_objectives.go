package game

import (
	"fmt"
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/quests"
)

func (g *MMGame) handleObjectiveProp(npc *character.NPC, id string, p *character.NPCPropCopy) bool {
	q := g.questManager.GetQuest(id)
	if q == nil || q.Definition.Activity == nil || len(q.Definition.Activity.Objectives) == 0 {
		return false
	}
	o := q.Definition.Activity.Objective(p.Token)
	if o == nil || !q.Definition.MatchesTarget(p.Tag) {
		return true
	}
	key, _, _ := g.canonicalPosition(npc.X, npc.Y)
	if key != o.Map || o.Event != "interact" {
		return true
	}
	if o.RequiresBoss != "" && !g.adventure.Records[o.RequiresBoss] {
		g.AddCombatMessage("The regulator still holds this record.")
		return true
	}
	itemName := ""
	if o.Item != "" {
		d, _ := config.GetItemDefinition(o.Item)
		if d == nil {
			return true
		}
		itemName = d.Name
		if g.party.CountItemsByName(itemName) < o.Count {
			g.AddCombatMessage(fmt.Sprintf("Bring %d %s.", o.Count, itemName))
			return true
		}
	}
	credited, completed, msg := g.questManager.CreditObjective(id, quests.ObjectiveEvent{Token: p.Token, Step: p.Step, Map: key, Event: "interact", Night: g.dayNightIsNight})
	if msg != "" {
		g.AddCombatMessage(msg)
	}
	if credited {
		if itemName != "" {
			g.party.RemoveItemsByName(itemName, o.Count)
		}
		g.AddCombatMessage(p.Took)
		if completed {
			g.applyCompletedQuestTiles()
			g.announceQuestCompletionWithMessage(q, p.Completed)
		}
	}
	return true
}

// Only committed traversal emits this event. Ordinary movement, travel and
// restored placement never grant credit. Spell-only objectives stay Jump-only.
func (g *MMGame) creditAdventureMovement(method string, oldX, oldY, newX, newY float64) {
	if g.questManager == nil || (method != "jump" && method != "spatial") {
		return
	}
	key, x0, y0 := g.canonicalPosition(oldX, oldY)
	dest, x1, y1 := g.canonicalPosition(newX, newY)
	if key != dest {
		return
	}
	tile := float64(g.config.GetTileSize())
	from, to := [2]int{TileIndex(x0, tile), TileIndex(y0, tile)}, [2]int{TileIndex(x1, tile), TileIndex(y1, tile)}
	for id, d := range g.questManager.Definitions() {
		if d.Activity == nil {
			continue
		}
		for _, o := range d.Activity.Objectives {
			if o.Event != "crossing" && !(o.Event == "jump" && method == "jump") {
				continue
			}
			_, done, _ := g.questManager.CreditObjective(id, quests.ObjectiveEvent{Token: o.Token, Map: key, Event: o.Event, Night: g.dayNightIsNight, From: from, To: to})
			if done {
				g.applyCompletedQuestTiles()
				g.announceQuestCompletionWithMessage(g.questManager.GetQuest(id), "The crossing is recorded.")
			}
		}
	}
}

func objectiveTokenKnown(a *quests.ActivityDefinition, token string) bool {
	return a.Objective(token) != nil || slices.Contains(a.Sequence, token) || a.TokenPhase(token) != ""
}
