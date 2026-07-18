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
	// A locked door's body text reflects what the party can do to it right now:
	// the authored greeting when an unlock exists, else a sealed-shut notice.
	if lockedDoorClosed(npc) {
		return g.lockedDoorGreeting(npc, len(g.availableDoorUnlocks(npc)) > 0)
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

type dialogueContentLayout struct {
	bodyLines   []string
	promptY     int
	choiceY     int
	exitY       int
	firstChoice int
	choiceCount int
}

// dialogueLayout is the single geometry source for encounter text, rendered
// choice rows, and their hitboxes. It caps the body to the space left by the
// current choices so authored or generated copy cannot escape the dialog.
func (g *MMGame) dialogueLayout(npc *character.NPC, dialogWidth, dialogHeight int) dialogueContentLayout {
	innerWidth := dialogWidth - 40
	textWidth := min(innerWidth, dialogueWrapColumns*debugTextCharWidth)
	choices := g.visibleNPCChoices(npc)
	promptHeight := 0
	if npc.DialogueData != nil && npc.DialogueData.ChoicePrompt != "" {
		promptHeight = dialoguePromptHeight
	}
	maxVisibleChoices := (dialogHeight - dialogueBodyTextY - dialogueLineHeight - 40 - promptHeight) / dialogueChoiceRowH
	if maxVisibleChoices < 1 {
		maxVisibleChoices = 1
	}
	visibleChoices := min(len(choices), maxVisibleChoices)

	footerHeight := 20 + debugTextCharHeight // gap + "Press ESC"
	if len(choices) > 0 {
		footerHeight = 20 + visibleChoices*dialogueChoiceRowH + promptHeight
	}
	availableBodyHeight := dialogHeight - dialogueBodyTextY - 20 - footerHeight
	maxBodyLines := availableBodyHeight / dialogueLineHeight
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	bodyLines := truncateWrappedLines(wrapDebugText(g.npcDialogueText(npc), textWidth), maxBodyLines, textWidth)

	cursorY := dialogueBodyTextY + len(bodyLines)*dialogueLineHeight + 20
	layout := dialogueContentLayout{bodyLines: bodyLines, promptY: -1, exitY: cursorY, choiceCount: visibleChoices}
	if len(choices) == 0 {
		return layout
	}
	layout.firstChoice = g.selectedChoice - visibleChoices/2
	if layout.firstChoice < 0 {
		layout.firstChoice = 0
	}
	if maxFirst := len(choices) - visibleChoices; layout.firstChoice > maxFirst {
		layout.firstChoice = maxFirst
	}
	if npc.DialogueData != nil && npc.DialogueData.ChoicePrompt != "" {
		layout.promptY = cursorY
		cursorY += dialoguePromptHeight
	}
	layout.choiceY = cursorY - 2
	return layout
}

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
	layout := g.dialogueLayout(npc, dialogWidth, npcDialogHeight)
	if i < layout.firstChoice || i >= layout.firstChoice+layout.choiceCount {
		return 0, 0, 0, 0
	}
	row := i - layout.firstChoice
	return dialogX + 20, dialogY + layout.choiceY + row*dialogueChoiceRowH, dialogWidth - 40, dialogueChoiceHitH
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
	// Lock choices are a pure view of the current party and the authored door
	// spec. Do not write them into DialogueData: several map instances may share
	// the same YAML dialogue pointer, and UI state must not mutate that source.
	if lockedDoorClosed(npc) {
		return g.lockedDoorChoices(npc)
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
