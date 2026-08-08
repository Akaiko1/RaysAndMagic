package game

import (
	"fmt"

	"ugataima/internal/character"
)

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
// choice with the given action, at any nesting depth.
func npcDialogueHasAction(npc *character.NPC, action string) bool {
	return npc != nil && npc.DialogueData != nil && npc.DialogueData.HasAction(action)
}

// questChainStepDone reports whether a quest is finished AND paid out - the
// condition a chained follow-up waits on.
func (g *MMGame) questChainStepDone(questID string) bool {
	if questID == "" {
		return true
	}
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(questID)
	return q != nil && q.Completed && q.RewardsClaimed
}

// partyHoldsQuest reports whether the party has taken a quest at all - active,
// done, or already paid out. Gates content that a quest is the licence for
// (the dragon seals answer only to someone who swore the hunt).
func (g *MMGame) partyHoldsQuest(questID string) bool {
	if questID == "" {
		return true
	}
	return g.questManager != nil && g.questManager.GetQuest(questID) != nil
}

// questAwaitingTurnIn reports whether this specific quest is done but unpaid -
// the only one of a giver's errands whose turn-in row should be live.
func (g *MMGame) questAwaitingTurnIn(questID string) bool {
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(questID)
	return q != nil && q.Completed && !q.RewardsClaimed
}

// choiceAvailable applies a choice's requires_quest gate: a chained offer stays
// hidden until its prerequisite has been turned in.
func (g *MMGame) choiceAvailable(c *character.NPCDialogueChoice) bool {
	return c != nil && g.questChainStepDone(c.RequiresQuest)
}

// activeChainQuestID is the quest an NPC is CURRENTLY about: the first of its
// quest choices whose prerequisite is met and which is not yet finished. A
// giver that hands out two errands in order (goblins, then wolves) would
// otherwise stay pinned to the first one forever and fall silent after it -
// a naive first-choice read never advances past choice one.
func (g *MMGame) activeChainQuestID(npc *character.NPC) string {
	choices := questChoicesOf(npc)
	last := ""
	for _, c := range choices {
		if !g.choiceAvailable(c) {
			continue
		}
		last = c.QuestID
		if !g.questChainStepDone(c.QuestID) {
			return c.QuestID
		}
	}
	return last // every step done: the last one keeps the NPC concluded
}

// npcHasPendingChainStep reports whether the NPC still has an unfinished quest
// step after the given one - i.e. turning that step in must NOT conclude them.
func (g *MMGame) npcHasPendingChainStep(npc *character.NPC, justFinished string) bool {
	for _, c := range questChoicesOf(npc) {
		if c.QuestID == justFinished {
			continue
		}
		if !g.questChainStepDone(c.QuestID) {
			return true
		}
	}
	return false
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
	qid := g.activeChainQuestID(npc)
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

// questStepMessage returns the body authored for the current quest in a
// multi-step chain. Single-quest and legacy NPCs keep using the shared fields.
func (g *MMGame) questStepMessage(npc *character.NPC, state npcDialogState) string {
	if npc == nil || npc.DialogueData == nil || len(npc.DialogueData.QuestMessages) == 0 {
		return ""
	}
	// A trader's quest copy belongs on its Quests tab, never over its shop
	// greeting on the primary tab. Only an OPEN shop has tabs - a service-gated
	// trader is a plain talker, and its quest copy is the whole conversation.
	if g.npcDialogKindFor(npc) == dialogKindSpellTrader && g.dialogTab != 1 {
		return ""
	}
	messages, ok := npc.DialogueData.QuestMessages[g.activeChainQuestID(npc)]
	if !ok {
		return ""
	}
	switch state {
	case npcStateOffer:
		return messages.Offer
	case npcStateActive:
		return messages.Active
	case npcStateCompleted:
		return messages.Completed
	default:
		return ""
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
	state := g.npcDialogueState(npc)
	if message := g.questStepMessage(npc, state); message != "" {
		return message
	}
	switch state {
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
		// A service-gated trader has no Spells tab yet, so the hook leads there
		// too - the shop welcome would promise a shop that will not open.
		if (g.dialogTab == 1 || !g.npcServiceGateOpen(npc)) && d.QuestGreeting != "" {
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

	// The tavern embeds roster and stash management instead of opening another
	// modal, so it needs enough room for the two working grids.
	tavernDialogWidth  = 760
	tavernDialogHeight = 620
)

type dialogueContentLayout struct {
	bodyLines []string
	// bodyFullLines is the UNCLIPPED wrapped copy; bodyLines may be a truncated
	// view of it. bodyClipped() reports whether anything was cut.
	bodyFullLines []string
	bodyWidth     int
	promptY       int
	choiceY       int
	exitY         int
	firstChoice   int
	choiceCount   int
}

// bodyClipped reports whether the rendered body is a truncated view of the
// authored copy - the cue for offering the full text on hover.
func (l dialogueContentLayout) bodyClipped() bool {
	return len(l.bodyFullLines) > len(l.bodyLines)
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
	fullLines := wrapDebugText(g.npcDialogueText(npc), textWidth)
	bodyLines := truncateWrappedLines(fullLines, maxBodyLines, textWidth)

	cursorY := dialogueBodyTextY + len(bodyLines)*dialogueLineHeight + 20
	layout := dialogueContentLayout{
		bodyLines: bodyLines,
		// The full copy travels with the layout so the renderer can offer it on
		// hover: long greetings are clipped to fit, never silently lost.
		bodyFullLines: fullLines,
		bodyWidth:     textWidth,
		promptY:       -1,
		exitY:         cursorY,
		choiceCount:   visibleChoices,
	}
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
	width, height := npcDialogWidth, npcDialogHeight
	if g.dialogNPC != nil && g.npcDialogKindFor(g.dialogNPC) == dialogKindTavern {
		width, height = tavernDialogWidth, tavernDialogHeight
	}
	return npcDialogRect{
		x: (g.config.GetScreenWidth() - width) / 2,
		y: (g.config.GetScreenHeight() - height) / 2,
		w: width,
		h: height,
	}
}

// switchDialogTab is the shared transition for mouse and keyboard tab changes.
// It also invalidates input queued against the previous tab.
func (g *MMGame) switchDialogTab(tab int) {
	g.dialogTab = tab
	g.selectedChoice = 0
	g.merchantBuyPage = 0
	g.pendingBuffService = nil
	g.pendingTavernAction = nil
	g.rosterSelectedActive = -1
	g.clearStashDrag()
	g.resetDialogClickTracker()
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
	dialogKindBuffService
	dialogKindTavern
)

// String names the kind for diagnostics - a boot error that says which
// capability won is the whole point of the checks that print one.
func (k npcDialogKind) String() string {
	switch k {
	case dialogKindGeneric:
		return "generic"
	case dialogKindSpellTrader:
		return "spell trader"
	case dialogKindSkillTrainer:
		return "skill trainer"
	case dialogKindChoices:
		return "choices"
	case dialogKindMerchant:
		return "merchant"
	case dialogKindCardCollector:
		return "card collector"
	case dialogKindArenaGladiator:
		return "arena gladiator"
	case dialogKindBuffService:
		return "buff service"
	case dialogKindTavern:
		return "tavern"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// isGatedService reports whether requires_quest withholds this dialog kind.
// DERIVED from the downgrade itself - the gate drops an NPC to its conversation
// (choices) or to nothing (generic), so every other kind is a service by
// construction and a new tabbed kind is covered the day it is added. A second
// hand-kept list would let authoring drift from what the runtime withholds.
func (k npcDialogKind) isGatedService() bool {
	return k != dialogKindGeneric && k != dialogKindChoices
}

// drawsDialogueRows reports whether this dialog kind has a surface for GENERIC
// authored rows - the encounter choice body, or a conversation tab that hosts it.
// The other kinds draw a fixed layout (a mastery grid, a card grid, a shop grid)
// and read only the specific actions they know about, so a row authored onto one
// of them is drawn nowhere. Kept beside the kinds so the answer moves with them.
func (k npcDialogKind) drawsDialogueRows() bool {
	switch k {
	case dialogKindChoices, dialogKindSpellTrader, dialogKindBuffService, dialogKindArenaGladiator:
		return true
	}
	return false
}

// npcShopHeaderLine is the line a TABBED shop prints above its rows: the NPC's
// visited_message once its errand is spent and it authored one, else its
// greeting, else the dialog's own stock phrase. A concluded giver keeps no
// dialogue rows, so its conversation tab is gone (npcDialogHasTalkTab) and this
// header is the ONLY surface left that can deliver the authored payoff line -
// without it "The tower keeps no books of mine now" is content nobody sees.
func (g *MMGame) npcShopHeaderLine(npc *character.NPC, stock string) string {
	if npc == nil || npc.DialogueData == nil {
		return stock
	}
	if g.npcDialogueState(npc) == npcStateConcluded && npc.DialogueData.VisitedMessage != "" {
		return npc.DialogueData.VisitedMessage
	}
	if npc.DialogueData.Greeting != "" {
		return npc.DialogueData.Greeting
	}
	return stock
}

// npcIsCardCollector reports whether the NPC runs the monster-card collection UI.
func npcIsCardCollector(npc *character.NPC) bool {
	return npc != nil && npc.Type == character.NPCTypeCardCollector
}

// questPropAction is the action a quest-prop row declares. executeEncounterChoice
// routes on the `prop:` block BEFORE the action switch, so a prop that borrowed a
// dispatched name (tavern_rest, cast_buff, ...) would read as that action to every
// rule that classifies rows - gatedServiceActions, choiceSurvivesConclusion - and
// still run the prop handler. One reserved name, checked both ways at load, closes
// the whole class instead of listing the names it must not collide with.
const questPropAction = "prop"

// dialogActions is THE dialogue dispatch: action name -> what pressing that row
// does. One object, so the name set the boot check validates authoring against
// and the code that runs cannot drift in either direction - a case added to a
// switch without a map entry used to make the validator reject valid content,
// and a name in the map with no case made a row that draws and does nothing.
//
// The reserved prop action is an entry like any other: with the pairing checked
// at load (prop block <-> action "prop"), a prop row no longer needs to be
// intercepted ahead of the dispatch.
var dialogActions = map[string]func(*InputHandler, *character.NPC, *character.NPCDialogueChoice){
	questPropAction: func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleQuestPropInteract(c.QuestID, c.Prop)
	},
	"info": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		// Branch deeper: show this choice's reply + its follow-up choices. The
		// conversation stays open (no quest taken) until the player picks a
		// terminal action inside the branch.
		ih.game.dialogNodePath = append(ih.game.dialogNodePath, c)
		ih.game.selectedChoice = 0
	},
	"back": func(ih *InputHandler, _ *character.NPC, _ *character.NPCDialogueChoice) {
		// Pop one conversation level (back toward the greeting).
		if n := len(ih.game.dialogNodePath); n > 0 {
			ih.game.dialogNodePath = ih.game.dialogNodePath[:n-1]
		}
		ih.game.selectedChoice = 0
	},
	"leave": func(ih *InputHandler, _ *character.NPC, _ *character.NPCDialogueChoice) {
		ih.game.dialogActive = false
		ih.game.dialogNPC = nil
	},
	"combat": func(ih *InputHandler, _ *character.NPC, _ *character.NPCDialogueChoice) {
		ih.startEncounter()
	},
	"enter_map": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.enterEncounterMap(c.Map)
	},
	"open_door": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.game.openLockedDoor(ih.game.dialogNPC, c.RuntimeOptionIndex)
	},
	"start_arena_duel": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.startArenaDuel(c)
	},
	"give_quest": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleGiveQuest(c.QuestID)
	},
	"turn_in_quest": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleTurnInQuest(c.QuestID)
	},
	"tavern_rest": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleTavernRest(c)
	},
	"wait_until_night": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleArenaWait(c, true)
	},
	"wait_until_dawn": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleArenaWait(c, false)
	},
	"buy_food": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleBuyFood(c)
	},
	"cast_buff": func(ih *InputHandler, _ *character.NPC, c *character.NPCDialogueChoice) {
		ih.handleCastBuff(c)
	},
	"summon_dragon": func(ih *InputHandler, npc *character.NPC, c *character.NPCDialogueChoice) {
		ih.summonDragonFromStatue(npc, c.RuntimeOptionIndex)
	},
	"open_roster": func(ih *InputHandler, _ *character.NPC, _ *character.NPCDialogueChoice) {
		ih.handleOpenRoster()
	},
	"manage_stash": func(ih *InputHandler, _ *character.NPC, _ *character.NPCDialogueChoice) {
		ih.handleManageStash()
	},
}

// gatedServiceActions are the dialogue actions that ARE a service, and so are
// withheld by requires_quest. The tabbed services (shop, trainer, cards, board)
// vanish on their own because the gate downgrades the dialog KIND - these are
// plain authored choices that would otherwise keep working: a gated tavern would
// still rest and heal the party, a gated pit would still run a bout.
var gatedServiceActions = map[string]bool{
	"tavern_rest":      true,
	"buy_food":         true,
	"open_roster":      true,
	"manage_stash":     true,
	"cast_buff":        true,
	"start_arena_duel": true,
	"wait_until_night": true, // the arena's paid rest (750g in npcs.yaml)
	"wait_until_dawn":  true,
}

// choiceSurvivesConclusion reports whether a row outlives the NPC's errand. A
// SERVICE does - a house that handed out its last quest still rents rooms - and
// anything that ADVANCES something is spent with it. The two sets coincide
// deliberately (a gate withholds exactly the services), but they are separate
// policies: adding an action to one is a decision about the other.
func choiceSurvivesConclusion(c *character.NPCDialogueChoice) bool {
	return c != nil && gatedServiceActions[c.Action]
}

// npcChoiceWithheldByGate reports whether this choice is a service the NPC's
// unpaid errand withholds. Applied in visibleNPCChoices, which is both what the
// player sees AND what every input path indexes into (keyboard selection, click
// targeting, executeEncounterChoice) - so filtering there withholds the service
// itself, not just its row.
func (g *MMGame) npcChoiceWithheldByGate(npc *character.NPC, c *character.NPCDialogueChoice) bool {
	return c != nil && gatedServiceActions[c.Action] && !g.npcServiceGateOpen(npc)
}

// npcServiceGateOpen reports whether a gated NPC's SERVICE is available yet.
// The gate is authored (requires_quest) and reuses the chain-step rule: the
// quest must be finished AND paid out, so the turn-in itself opens the shop.
func (g *MMGame) npcServiceGateOpen(npc *character.NPC) bool {
	return npc == nil || npc.RequiresQuest == "" || g.questChainStepDone(npc.RequiresQuest)
}

func (g *MMGame) npcDialogKindFor(npc *character.NPC) npcDialogKind {
	if npc == nil {
		return dialogKindGeneric
	}
	kind := npcDialogKindUngated(npc)
	// A gated NPC is a person with a task until the task is settled: no shop, no
	// training, no board - only the authored conversation that hands the quest
	// out and takes it in. Gating HERE (the one dispatch) is what keeps the
	// renderer, the input handler and the HUD prompt from disagreeing.
	if kind.isGatedService() && !g.npcServiceGateOpen(npc) {
		if npcHasChoiceDialog(npc) {
			return dialogKindChoices
		}
		return dialogKindGeneric
	}
	return kind
}

// npcDialogKindUngated is the authored dispatch with no gate applied: which
// dialog this NPC would get with its errand settled. npcDialogKindFor is the
// one caller that matters - everything else asks THAT, so the gate is never
// skipped by accident.
func npcDialogKindUngated(npc *character.NPC) npcDialogKind {
	switch {
	case npcIsCardCollector(npc):
		return dialogKindCardCollector
	case tavernChoice(npc, "tavern_rest") != nil:
		return dialogKindTavern
	case npcHasBuffService(npc):
		// A paid-cast service is its own tabbed dialog (service rows + Talk),
		// checked before the generic choice dialog that would swallow it.
		return dialogKindBuffService
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

// filterNPCChoices applies the state rules to ONE list of choices - the root's
// or an info node's. Split out so npcChoiceRows can run the same rules over
// either list without touching the live dialog position.
func (g *MMGame) filterNPCChoices(npc *character.NPC, source []*character.NPCDialogueChoice) []*character.NPCDialogueChoice {
	var out []*character.NPCDialogueChoice
	state, questStep := g.npcDialogueState(npc), g.activeChainQuestID(npc)
	for _, c := range source {
		if g.choiceKeepsIn(npc, c, state, questStep) {
			out = append(out, c)
		}
	}
	return out
}

// choiceKeepsIn is THE row rule: whether this choice is offered in the NPC's
// given state. Split from the list builder so the yes/no form can share it
// exactly - two copies of "is this row live" is how a hidden choice becomes a
// clickable one.
//
// A CONCLUDED NPC is done with its errand - and with anything that advances one:
// quest rows and used-up props go, so a shut valve stops offering to be shut. Its
// SERVICE does not: a house that handed out its last errand still rents rooms,
// and a gate exists to open a service, not to spend it once.
func (g *MMGame) choiceKeepsIn(npc *character.NPC, c *character.NPCDialogueChoice, state npcDialogState, questStep string) bool {
	if c == nil || !g.choiceAvailable(c) || (c.QuestStep != "" && c.QuestStep != questStep) {
		return false
	}
	if g.npcChoiceWithheldByGate(npc, c) {
		return false
	}
	if state == npcStateConcluded && !choiceSurvivesConclusion(c) {
		return false
	}
	switch c.Action {
	case "give_quest":
		// Per CHOICE, not just per NPC state: a giver with several errands must
		// not keep offering one the party already took or finished.
		return state == npcStateOffer && !g.partyHoldsQuest(c.QuestID)
	case "turn_in_quest":
		return state == npcStateCompleted && g.questAwaitingTurnIn(c.QuestID)
	default:
		return true
	}
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
	source, atRoot := npc.DialogueData.Choices, true
	if node := g.currentDialogNode(); node != nil {
		source, atRoot = node.Choices, false // inside an info branch, its own follow-ups
	}
	return g.npcChoiceRows(npc, source, atRoot)
}

// npcChoiceRows is THE row list of a dialogue: the lock override, the state
// filter and the root-only strips in one place. visibleNPCChoices asks it about
// the live conversation position; a predicate about the ROOT rows (the trader's
// Quests tab) asks the same function with atRoot - so a strip can never apply to
// what the player sees and not to what decides the tab exists.
func (g *MMGame) npcChoiceRows(npc *character.NPC, source []*character.NPCDialogueChoice, atRoot bool) []*character.NPCDialogueChoice {
	if npc == nil || npc.DialogueData == nil {
		return nil
	}
	// Lock choices are a pure view of the current party and the authored door
	// spec. Do not write them into DialogueData: several map instances may share
	// the same YAML dialogue pointer, and UI state must not mutate that source.
	if lockedDoorClosed(npc) {
		return g.lockedDoorChoices(npc)
	}
	out := g.filterNPCChoices(npc, source)
	// A buff-service NPC shows its paid casts as ICON ROWS on its own tab, so
	// they must not also appear as text choices in the Talk tab list. Filtered
	// at the top level only - a cast authored deeper in a conversation stays a
	// normal choice and reachable.
	if atRoot && npcHasBuffService(npc) {
		kept := out[:0]
		for _, c := range out {
			if c.Action != "cast_buff" {
				kept = append(kept, c)
			}
		}
		out = kept
	}
	return out
}
