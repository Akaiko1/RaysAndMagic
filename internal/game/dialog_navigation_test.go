package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

func clickDialogueNavigation(h *displayedModalHarness, back bool, count int) {
	h.t.Helper()
	dlg := npcDialogLayout(h.g)
	layout := h.g.dialogueLayout(h.g.dialogNPC, dlg.w, npcDialogHeight)
	rect := layout.leaveButton
	if back {
		rect = layout.backButton
	}
	h.clicks(false, dlg.x+rect.x+rect.w/2, dlg.y+rect.y+rect.h/2, count)
}

func TestRoninNavigationThroughDisplayedUI(t *testing.T) {
	for _, state := range []string{"offer", "active", "completed", "concluded"} {
		t.Run(state, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 680)
			if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
				t.Fatal(err)
			}
			npc, err := character.CreateNPCFromConfig("castle_oldman", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			h.g.questManager = loadTestQuestManager(t)
			if state != "offer" {
				status := quests.QuestStatusActive
				count := 0
				if state == "completed" || state == "concluded" {
					status = quests.QuestStatusCompleted
					count = 5
				}
				h.g.questManager.RestoreQuestProgress("castle_armory", status, count, 0, state == "concluded")
			}
			h.g.beginConversation(npc)
			if state != "concluded" {
				// Enter the exact shipped dead end through the production dispatcher.
				ih := NewInputHandler(h.g)
				h.g.selectedChoice = 0
				ih.executeEncounterChoice()
				if len(h.g.dialogNodePath) != 1 {
					t.Fatal("failed to enter ronin branch")
				}
				h.g.selectedChoice = 1
				ih.executeEncounterChoice()
				if len(h.g.dialogNodePath) != 2 {
					t.Fatal("failed to enter deeper topic")
				}
				clickDialogueNavigation(h, true, 3)
				if len(h.g.dialogNodePath) != 1 || !h.g.dialogActive {
					t.Fatal("one displayed Back must pop only one level")
				}
				clickDialogueNavigation(h, true, 1)
				if h.g.currentDialogNode() != nil || !h.g.dialogActive {
					t.Fatal("Back did not reach greeting")
				}
			}
			clickDialogueNavigation(h, false, 1)
			if h.g.dialogActive || h.g.dialogNPC != nil {
				t.Fatal("Leave failed without Escape")
			}
		})
	}
}

// Every shipped info node, even one with no authored way back or one whose
// children disappear after turn-in, must have independent navigation.
func TestDialogueNavigationAllAuthoredBranches(t *testing.T) {
	h := newDisplayedModalHarness(t, 1280, 720)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, key := range slices.Sorted(maps.Keys(character.NPCConfigInstance.NPCs)) {
		data := character.NPCConfigInstance.NPCs[key]
		if data.Dialogue == nil {
			continue
		}
		var paths [][]*character.NPCDialogueChoice
		var walk func([]*character.NPCDialogueChoice, []*character.NPCDialogueChoice)
		walk = func(choices, path []*character.NPCDialogueChoice) {
			for _, choice := range choices {
				if choice.Action == "info" {
					next := append(slices.Clone(path), choice)
					paths = append(paths, next)
					walk(choice.Choices, next)
				}
			}
		}
		walk(data.Dialogue.Choices, nil)
		for n, path := range paths {
			for _, visited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/concluded=%v", key, n, visited), func(t *testing.T) {
					npc, err := character.CreateNPCFromConfig(key, 0, 0)
					if err != nil {
						t.Fatal(err)
					}
					npc.Visited = visited
					h.g.beginConversation(npc)
					switch h.g.npcDialogKindFor(npc) {
					case dialogKindBuffService, dialogKindSpellTrader:
						h.g.switchDialogTab(1)
					}
					h.g.dialogNodePath = slices.Clone(path)
					clickDialogueNavigation(h, true, 1)
					if len(h.g.dialogNodePath) != len(path)-1 {
						t.Fatal("branch has no working parent navigation")
					}
					h.g.dialogNodePath = slices.Clone(path)
					clickDialogueNavigation(h, false, 1)
					if h.g.dialogActive {
						t.Fatal("branch has no working Leave")
					}
					checked++
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("did not visit shipped branches")
	}
	t.Logf("checked %d branch/state combinations", checked)
}

func TestDialogueNavigationLayoutAndLifecycle(t *testing.T) {
	h := newDisplayedModalHarness(t, 800, 680)
	choices := make([]*character.NPCDialogueChoice, 24)
	for i := range choices {
		choices[i] = &character.NPCDialogueChoice{Action: "info", Text: "A topic", Response: "An answer"}
	}
	npc := &character.NPC{Name: "Talker", DialogueData: &character.NPCDialogue{Greeting: strings.Repeat("A long tale. ", 200), Choices: choices}}
	h.g.beginConversation(npc)
	for _, selected := range []int{0, 12, 23} {
		h.g.selectedChoice = selected
		layout := h.g.dialogueLayout(npc, npcDialogWidth, npcDialogHeight)
		for i := layout.firstChoice; i < layout.firstChoice+layout.choiceCount; i++ {
			_, y, _, height := h.g.dialogueChoiceRect(npc, i, 0, 0, npcDialogWidth)
			if y+height > layout.leaveButton.y {
				t.Fatal("choice overlaps persistent navigation")
			}
		}
		if layout.leaveButton.bottom() > npcDialogHeight || layout.backButton.right() > layout.leaveButton.x {
			t.Fatal("navigation outside panel")
		}
	}
	h.g.dialogNodePath = []*character.NPCDialogueChoice{choices[0]}
	h.g.dialogLastClickZone = "encounter_choice"
	h.g.dialogLastClickedIdx = 1
	h.g.switchDialogTab(1)
	if h.g.currentDialogNode() != nil || h.g.dialogLastClickedIdx != -1 {
		t.Fatal("tab change retained branch/click state")
	}
	h.g.dialogNodePath = []*character.NPCDialogueChoice{choices[0]}
	h.g.beginConversation(npc)
	if h.g.currentDialogNode() != nil {
		t.Fatal("reopening must start at root")
	}
}

func TestDialogueCloseButtonAndChildModal(t *testing.T) {
	for _, child := range []bool{false, true} {
		t.Run(fmt.Sprint(child), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1280, 720)
			npc := &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer, DialogueData: &character.NPCDialogue{Greeting: "Welcome"}}
			h.g.beginConversation(npc)
			h.g.skillTrainerPopup = child
			rect := npcDialogCloseRect(npcDialogLayout(h.g))
			h.clicks(false, rect.x+8, rect.y+8, 1)
			if h.g.dialogActive != child {
				t.Fatal("close button ignored modal ownership")
			}
		})
	}
}

func TestDialogueCloseAllAuthoredNPCs(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	h := newDisplayedModalHarness(t, 800, 680)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, key := range slices.Sorted(maps.Keys(character.NPCConfigInstance.NPCs)) {
		t.Run(key, func(t *testing.T) {
			npc, err := character.CreateNPCFromConfig(key, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			// Test the service after its gate too, since the gated variant uses
			// ordinary choices and otherwise leaves service-specific layouts out.
			for _, unlocked := range []bool{false, true} {
				if unlocked {
					npc.RequiresQuest = ""
				}
				h.g.beginConversation(npc)
				rect := npcDialogCloseRect(npcDialogLayout(h.g))
				h.clicks(false, rect.x+rect.w/2, rect.y+rect.h/2, 2)
				if h.g.dialogActive {
					t.Fatalf("close failed for unlocked=%v", unlocked)
				}
			}
		})
	}
}
