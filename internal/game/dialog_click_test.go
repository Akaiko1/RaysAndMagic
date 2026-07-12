package game

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

// aldricLikeNPC builds a spell-trader with quest choices (the Aldric shape).
func aldricLikeNPC() *character.NPC {
	return &character.NPC{
		Name:           "Aldric",
		Type:           "spell_trader",
		RenderCategory: "standee",
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

func TestDialogueLayoutKeepsLongBodyAndChoicesInsideDialog(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	npc := &character.NPC{
		Name: "Verbose keeper",
		DialogueData: &character.NPCDialogue{
			Greeting:     strings.Repeat("A very long explanation ", 80) + strings.Repeat("X", 150),
			ChoicePrompt: strings.Repeat("Long prompt ", 20),
			Choices: []*character.NPCDialogueChoice{
				{Text: strings.Repeat("Long choice ", 20), Action: "info"},
				{Text: "Second", Action: "info"},
				{Text: "Third", Action: "info"},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
	dlg := npcDialogLayout(g)
	layout := g.dialogueLayout(npc, dlg.w, dlg.h)
	if len(layout.bodyLines) == 0 {
		t.Fatal("dialogue body has no visible lines")
	}
	for _, line := range layout.bodyLines {
		if width := debugTextWidth(line); width > dialogueWrapColumns*debugTextCharWidth {
			t.Errorf("body line width = %d: %q", width, line)
		}
	}
	last := len(npc.DialogueData.Choices) - 1
	_, y, _, h := g.dialogueChoiceRect(npc, last, dlg.x, dlg.y, dlg.w)
	if y+h > dlg.y+dlg.h-20 {
		t.Fatalf("last choice ends at %d, dialog content ends at %d", y+h, dlg.y+dlg.h-20)
	}
}

func TestDialogueLayoutScrollsLargeChoiceListAroundSelection(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	choices := make([]*character.NPCDialogueChoice, 20)
	for i := range choices {
		choices[i] = &character.NPCDialogueChoice{Text: fmt.Sprintf("Choice %d", i), Action: "info"}
	}
	npc := &character.NPC{DialogueData: &character.NPCDialogue{Greeting: "Choose.", ChoicePrompt: "Options:", Choices: choices}}
	g.selectedChoice = 15
	dlg := npcDialogLayout(g)
	layout := g.dialogueLayout(npc, dlg.w, dlg.h)
	if g.selectedChoice < layout.firstChoice || g.selectedChoice >= layout.firstChoice+layout.choiceCount {
		t.Fatalf("selected choice %d not in visible range [%d,%d)", g.selectedChoice, layout.firstChoice, layout.firstChoice+layout.choiceCount)
	}
	for i := layout.firstChoice; i < layout.firstChoice+layout.choiceCount; i++ {
		_, y, _, h := g.dialogueChoiceRect(npc, i, dlg.x, dlg.y, dlg.w)
		if y < dlg.y || y+h > dlg.y+dlg.h-20 {
			t.Fatalf("choice %d rect [%d,%d) escapes dialog [%d,%d)", i, y, y+h, dlg.y, dlg.y+dlg.h-20)
		}
	}
	if _, _, _, h := g.dialogueChoiceRect(npc, 0, dlg.x, dlg.y, dlg.w); h != 0 {
		t.Fatal("off-screen choice must not have an active hitbox")
	}
}
