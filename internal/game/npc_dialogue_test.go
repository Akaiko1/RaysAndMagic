package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

// questGiverNPC builds a minimal quest-giver NPC wired to questID, with the
// give_quest / turn_in_quest / leave choices and all four state messages.
func questGiverNPC(questID string) *character.NPC {
	return &character.NPC{
		Name: "Tester",
		DialogueData: &character.NPCDialogue{
			Greeting:         "offer",
			ActiveMessage:    "in-progress",
			CompletedMessage: "well done",
			VisitedMessage:   "farewell",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Take", Action: "give_quest", QuestID: questID},
				{Text: "Claim", Action: "turn_in_quest", QuestID: questID},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
}

func loadTestQuestManager(t *testing.T) *quests.QuestManager {
	t.Helper()
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests()
	return qm
}

// actions extracts the action of each visible choice for terse assertions.
func actions(choices []*character.NPCDialogueChoice) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c.Action
	}
	return out
}

// The quest-giver walks offer → active → completed → concluded, and only the
// state-appropriate choices/body are surfaced at each step.
func TestNPCDialogueState_QuestGiverLifecycle(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	const qid = "dragon_cliffs_troll_cull" // kill 3 mountain_troll
	npc := questGiverNPC(qid)

	want := func(state npcDialogState, body string, acts ...string) {
		t.Helper()
		if got := g.npcDialogueState(npc); got != state {
			t.Fatalf("state = %d, want %d", got, state)
		}
		if got := g.npcDialogueText(npc); got != body {
			t.Errorf("body = %q, want %q", got, body)
		}
		got := actions(g.visibleNPCChoices(npc))
		if len(got) != len(acts) {
			t.Fatalf("choices = %v, want %v", got, acts)
		}
		for i := range acts {
			if got[i] != acts[i] {
				t.Errorf("choice %d = %q, want %q", i, got[i], acts[i])
			}
		}
	}

	// 1) Not taken → offer: greeting + give_quest (+ leave). No turn-in yet.
	want(npcStateOffer, "offer", "give_quest", "leave")

	// 2) Taken, not done → active: in-progress text, no offer/turn-in.
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	want(npcStateActive, "in-progress", "leave")

	// 3) Done, not turned in → completed: turn_in available, offer gone.
	for i := 0; i < 3; i++ {
		g.questManager.OnMonsterKilled("mountain_troll")
	}
	want(npcStateCompleted, "well done", "turn_in_quest", "leave")

	// 4) Turned in (reward claimed) → concluded: farewell, no actionable choices.
	if !g.claimQuestReward(qid) {
		t.Fatalf("claim failed")
	}
	want(npcStateConcluded, "farewell")
}

// Generic turn-in at the NPC pays gold + XP and concludes the NPC (Visited).
func TestHandleTurnInQuest_GenericClaimsAndConcludes(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	const qid = "dragon_cliffs_troll_cull"
	npc := questGiverNPC(qid)

	g.questManager.ActivateQuest(qid)
	for i := 0; i < 3; i++ {
		g.questManager.OnMonsterKilled("mountain_troll")
	}

	goldBefore := g.party.Gold
	xpBefore := g.party.Members[0].Experience

	ih := NewInputHandler(g)
	g.dialogNPC = npc
	ih.handleTurnInQuest(qid)

	if g.party.Gold <= goldBefore {
		t.Errorf("turn-in should pay gold (%d -> %d)", goldBefore, g.party.Gold)
	}
	if g.party.Members[0].Experience <= xpBefore {
		t.Errorf("turn-in should grant XP (%d -> %d)", xpBefore, g.party.Members[0].Experience)
	}
	if q := g.questManager.GetQuest(qid); q == nil || !q.RewardsClaimed {
		t.Errorf("quest reward should be marked claimed")
	}
	if !npc.Visited {
		t.Errorf("NPC should conclude (Visited) after turn-in")
	}
	if g.npcDialogueState(npc) != npcStateConcluded {
		t.Errorf("NPC should be concluded after turn-in")
	}
}

// A turn_in_quest choice is hidden until the quest is actually completed (the
// Mage Tower bug: "I have slain the Lich King" must not show before the deed).
func TestVisibleNPCChoices_TurnInHiddenUntilComplete(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	const qid = "dragon_cliffs_ember_rites" // kill 2 archmage
	npc := questGiverNPC(qid)

	hasAction := func(a string) bool {
		for _, c := range g.visibleNPCChoices(npc) {
			if c.Action == a {
				return true
			}
		}
		return false
	}

	if hasAction("turn_in_quest") {
		t.Error("turn_in must be hidden in the offer state")
	}
	g.questManager.ActivateQuest(qid)
	if hasAction("turn_in_quest") || hasAction("give_quest") {
		t.Error("neither turn_in nor give_quest should show while the quest is active")
	}
	for i := 0; i < 2; i++ {
		g.questManager.OnMonsterKilled("archmage")
	}
	if !hasAction("turn_in_quest") || hasAction("give_quest") {
		t.Error("completed state should show turn_in only (no re-offer)")
	}
}
