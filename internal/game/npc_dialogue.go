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

// npcDialogueHasAction reports whether the NPC's dialogue tree contains a
// choice with the given action, at any nesting depth. One recursive walker for
// every "does this NPC offer capability X" test (duel grounds, tavern rest).
func npcDialogueHasAction(npc *character.NPC, action string) bool {
	if npc == nil || npc.DialogueData == nil {
		return false
	}
	var walk func([]*character.NPCDialogueChoice) bool
	walk = func(choices []*character.NPCDialogueChoice) bool {
		for _, c := range choices {
			if c != nil && (c.Action == action || walk(c.Choices)) {
				return true
			}
		}
		return false
	}
	return walk(npc.DialogueData.Choices)
}

// linkedQuestID returns the quest_id of the NPC's give_quest / turn_in_quest
// choice - the quest whose status drives the dialogue. "" for non-quest NPCs.
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

// currentDialogNode returns the "info" choice the player has descended into
// (the deepest entry of dialogNodePath), or nil at the conversation root.
func (g *MMGame) currentDialogNode() *character.NPCDialogueChoice {
	if n := len(g.dialogNodePath); n > 0 {
		return g.dialogNodePath[n-1]
	}
	return nil
}

// npcDialogueText is the body text for the NPC's current state, falling back to
// the greeting when a state-specific message is unset. When the player has
// branched into an "info" choice, its Response is shown instead.
func (g *MMGame) npcDialogueText(npc *character.NPC) string {
	if npc == nil || npc.DialogueData == nil {
		return ""
	}
	if node := g.currentDialogNode(); node != nil {
		return node.Response
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
		return d.VisitedMessage // may be "" -> renderer shows just "Press ESC"
	default:
		// Offer state. On a spell-trader's Quests tab, lead with the quest hook
		// rather than the shop-welcome Greeting (Spells tab keeps the Greeting).
		if g.dialogTab == 1 && d.QuestGreeting != "" {
			return d.QuestGreeting
		}
	}
	return d.Greeting
}

// Encounter-style dialogue body layout (shared by the renderer and the mouse
// handler so click targets always match the drawn rows).
const (
	dialogueBodyTextY    = 50 // body text offset from the dialog top
	dialogueLineHeight   = 16
	dialogueChoiceRowH   = 25
	dialogueChoiceHitH   = 20
	dialogueWrapColumns  = 70
	dialoguePromptHeight = 20

	// The standard centered NPC dialog box. Renderer and every mouse handler
	// must use npcDialogLayout - a hardcoded copy that drifts desyncs click
	// rects from drawn pixels.
	npcDialogWidth  = 600
	npcDialogHeight = 400
)

// npcDialogRect is the screen rect of the standard centered NPC dialog.
type npcDialogRect struct{ x, y, w, h int }

func npcDialogLayout(g *MMGame) npcDialogRect {
	return npcDialogRect{
		x: (g.config.GetScreenWidth() - npcDialogWidth) / 2,
		y: (g.config.GetScreenHeight() - npcDialogHeight) / 2,
		w: npcDialogWidth,
		h: npcDialogHeight,
	}
}

// npcDialogKind classifies which dialog UI/input an NPC gets. The input
// dispatcher, the dialog renderer and the HUD interaction prompt all switch on
// THIS, so the priority order (a spell trader with quest choices is still a
// spell trader - choices live on its Quests tab) can never drift between them.
type npcDialogKind int

const (
	dialogKindGeneric npcDialogKind = iota
	dialogKindSpellTrader
	dialogKindSkillTrainer
	dialogKindChoices
	dialogKindMerchant
	dialogKindCardCollector
	dialogKindArenaGladiator
)

// npcIsCardCollector reports whether the NPC runs the monster-card collection UI.
func npcIsCardCollector(npc *character.NPC) bool {
	return npc != nil && npc.Type == "card_collector"
}

func npcDialogKindFor(npc *character.NPC) npcDialogKind {
	switch {
	case npcIsCardCollector(npc):
		return dialogKindCardCollector
	case npcHasSpellTrading(npc):
		return dialogKindSpellTrader
	case npcHasSkillTraining(npc):
		return dialogKindSkillTrainer
	case npc.ArenaBoard && npcHasChoiceDialog(npc) && npcHasMerchant(npc):
		// Arena gladiators (authored arena_board: true): dialogue choices + a
		// points shop + the champions' board in one tabbed dialog. The explicit
		// flag keeps the board off future shop+choices NPCs.
		return dialogKindArenaGladiator
	case npcHasChoiceDialog(npc):
		return dialogKindChoices
	case npcHasMerchant(npc):
		return dialogKindMerchant
	default:
		return dialogKindGeneric
	}
}

// dialogueChoiceRect returns the screen rect of the i-th visible choice row in
// an encounter-style dialogue (the same rect the renderer highlights).
func (g *MMGame) dialogueChoiceRect(npc *character.NPC, i, dialogX, dialogY, dialogWidth int) (x, y, w, h int) {
	lines := wrapText(g.npcDialogueText(npc), dialogueWrapColumns)
	choicesY := dialogY + dialogueBodyTextY + len(lines)*dialogueLineHeight + 20
	if npc.DialogueData != nil && npc.DialogueData.ChoicePrompt != "" {
		choicesY += dialoguePromptHeight
	}
	return dialogX + 20, choicesY + i*dialogueChoiceRowH - 2, dialogWidth - 40, dialogueChoiceHitH
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
	// Inside an "info" branch, the follow-up choices are the node's own (still
	// state-filtered, so a give_quest deep in a branch obeys the same rules).
	source := npc.DialogueData.Choices
	if node := g.currentDialogNode(); node != nil {
		source = node.Choices
	}
	var out []*character.NPCDialogueChoice
	for _, c := range source {
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
