package game

import (
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

// aldricLikeNPC builds a spell-trader with quest choices (the Aldric shape).
func aldricLikeNPC() *character.NPC {
	return &character.NPC{
		Name: "Aldric",
		Type: "spell_trader",
		SpellData: map[string]*character.NPCSpell{
			"walk_on_water": {Name: "Walk on Water", Cost: 500},
		},
		DialogueData: &character.NPCDialogue{
			Greeting: "hello",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Ask about the wolves", Action: "give_quest", QuestID: "forest_wolf_cull"},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
}

// queueClickAtChoice queues one left-click at the center of choice row i.
func queueClickAtChoice(g *MMGame, npc *character.NPC, i int) {
	dlg := npcDialogLayout(g)
	x, y, w, h := g.dialogueChoiceRect(npc, i, dlg.x, dlg.y, dlg.w)
	g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: x + w/2, y: y + h/2, at: time.Now().UnixMilli()})
}

// Quest choices on the spell trader's Quests tab require a DOUBLE click, same
// as every other dialog list: first click selects, second executes.
func TestSpellTraderQuestTab_ChoiceNeedsDoubleClick(t *testing.T) {
	cfg := loadTestConfig(t)
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	prevQM := quests.GlobalQuestManager
	t.Cleanup(func() { quests.GlobalQuestManager = prevQM })
	quests.GlobalQuestManager = quests.NewQuestManager(questCfg)

	g := newTestGame(cfg, newTestWorld(cfg))
	g.questManager = quests.GlobalQuestManager
	ih := &InputHandler{game: g}

	npc := aldricLikeNPC()
	g.dialogActive = true
	g.dialogNPC = npc
	g.dialogTab = 1 // Quests tab
	g.selectedChoice = 0

	// First click: must SELECT only - quest not taken, dialog still open.
	queueClickAtChoice(g, npc, 0)
	ih.handleSpellTraderInput()
	if q := g.questManager.GetQuest("forest_wolf_cull"); q != nil {
		t.Fatal("single click must not execute the choice (quest was activated)")
	}
	if !g.dialogActive {
		t.Fatal("single click must not close the dialog")
	}

	// Second click on the same row inside the double-click window: executes.
	queueClickAtChoice(g, npc, 0)
	ih.handleSpellTraderInput()
	if q := g.questManager.GetQuest("forest_wolf_cull"); q == nil {
		t.Fatal("double click should execute give_quest")
	}
}
