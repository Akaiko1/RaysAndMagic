package game

import (
	"testing"

	"ugataima/internal/quests"
)

func TestDragonSlayerAcceptanceDoesNotCompleteBeforeSummons(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	questConfig, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}

	questManager := quests.NewQuestManager(questConfig)
	questManager.InitializeStartingQuests()
	cs.game.questManager = questManager

	previousQuestManager := quests.GlobalQuestManager
	quests.GlobalQuestManager = questManager
	t.Cleanup(func() {
		quests.GlobalQuestManager = previousQuestManager
	})

	ih := &InputHandler{game: cs.game}
	ih.handleGiveQuest("dragon_slayer")

	quest := questManager.GetQuest("dragon_slayer")
	if quest == nil {
		t.Fatal("dragon_slayer was not activated")
	}
	if quest.Completed {
		t.Fatal("dragon_slayer completed before any Elder Dragon was summoned")
	}
	if quest.Status != quests.QuestStatusActive {
		t.Fatalf("dragon_slayer status = %q, want active", quest.Status)
	}
	if quest.CurrentCount != 0 {
		t.Fatalf("dragon_slayer progress = %d, want 0", quest.CurrentCount)
	}
}
