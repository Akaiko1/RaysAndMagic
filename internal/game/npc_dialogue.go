package game

import "ugataima/internal/character"

// npcDialogState is the conversational state of an NPC, derived from its linked
// quest's status (for quest-givers) plus the Visited flag. It drives both the
// body text shown and which choices are offered, so the two can never disagree.
type npcDialogState int

const (
	npcStateOffer     npcDialogState = iota // quest not taken / encounter not cleared
	npcStateActive                          // quest taken, not yet done
	npcStateCompleted                       // quest done, not yet turned in
	npcStateConcluded                       // turned in / reward claimed / encounter cleared
)

// linkedQuestID returns the quest_id of the NPC's give_quest / turn_in_quest
// choice — the quest whose status drives the dialogue. "" for non-quest NPCs.
func linkedQuestID(npc *character.NPC) string {
	if npc == nil || npc.DialogueData == nil {
		return ""
	}
	for _, c := range npc.DialogueData.Choices {
		if c == nil {
			continue
		}
		if (c.Action == "give_quest" || c.Action == "turn_in_quest") && c.QuestID != "" {
			return c.QuestID
		}
	}
	return ""
}

// npcDialogueState computes an NPC's dialogue state from its linked quest and the
// Visited flag (set when an encounter is cleared or a quest is turned in).
func (g *MMGame) npcDialogueState(npc *character.NPC) npcDialogState {
	if npc == nil {
		return npcStateConcluded
	}
	if npc.Visited {
		// Repeatable encounters keep offering; everything else has concluded.
		if npc.EncounterData != nil && !npc.EncounterData.FirstVisitOnly {
			return npcStateOffer
		}
		return npcStateConcluded
	}
	qid := linkedQuestID(npc)
	if qid == "" || g.questManager == nil {
		return npcStateOffer // pure encounter / door NPC: offer until Visited
	}
	q := g.questManager.GetQuest(qid)
	switch {
	case q == nil:
		return npcStateOffer // never activated
	case q.Completed && q.RewardsClaimed:
		return npcStateConcluded // done and claimed
	case q.Completed:
		return npcStateCompleted // done, awaiting turn-in
	default:
		return npcStateActive // active, in progress
	}
}

// npcDialogueText is the body text for the NPC's current state, falling back to
// the greeting when a state-specific message is unset.
func (g *MMGame) npcDialogueText(npc *character.NPC) string {
	if npc == nil || npc.DialogueData == nil {
		return ""
	}
	d := npc.DialogueData
	switch g.npcDialogueState(npc) {
	case npcStateActive:
		if d.ActiveMessage != "" {
			return d.ActiveMessage
		}
	case npcStateCompleted:
		if d.CompletedMessage != "" {
			return d.CompletedMessage
		}
	case npcStateConcluded:
		return d.VisitedMessage // may be "" → renderer shows just "Press ESC"
	}
	return d.Greeting
}

// visibleNPCChoices filters the NPC's choices to those valid in its current
// state: give_quest only when offering, turn_in_quest only when the quest is
// completed, every other action whenever the NPC is still actionable. The
// renderer and the input handler both use this so a hidden choice can't be
// selected and indices stay aligned.
func (g *MMGame) visibleNPCChoices(npc *character.NPC) []*character.NPCDialogueChoice {
	if npc == nil || npc.DialogueData == nil {
		return nil
	}
	state := g.npcDialogueState(npc)
	if state == npcStateConcluded {
		return nil
	}
	var out []*character.NPCDialogueChoice
	for _, c := range npc.DialogueData.Choices {
		if c == nil {
			continue
		}
		switch c.Action {
		case "give_quest":
			if state == npcStateOffer {
				out = append(out, c)
			}
		case "turn_in_quest":
			if state == npcStateCompleted {
				out = append(out, c)
			}
		default:
			out = append(out, c)
		}
	}
	return out
}
