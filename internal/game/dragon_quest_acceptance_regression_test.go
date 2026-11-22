package game

import (
	"testing"

	"ugataima/internal/character"
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

	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	sylwen, err := character.CreateNPCFromConfig("desert_pilgrim_sylwen", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	ih := &InputHandler{game: cs.game}
	// Below level 24 she refuses in her own words and nothing starts.
	cs.game.dialogNPC = sylwen
	ih.handleGiveQuest("dragon_slayer")
	log := cs.game.combatLogHistory
	want := sylwen.DialogueData.QuestMessages["dragon_slayer"].LevelLocked
	if questManager.GetQuest("dragon_slayer") != nil || want == "" || len(log) == 0 || log[len(log)-1].Text != want {
		t.Fatalf("under-level party: quest active %v, last line %v; want her level_locked reply", questManager.GetQuest("dragon_slayer") != nil, log)
	}
	cs.game.maxPartyLevel = 24
	cs.game.dialogNPC = sylwen
	ih.handleGiveQuest("dragon_slayer")

	quest := questManager.GetQuest("dragon_slayer")
	if quest == nil {
		t.Fatal("dragon_slayer was not activated")
	}
	if quest.Completed {
		t.Fatal("dragon_slayer completed before any Elder Dragon was summoned")
	}
	if quest.Status() != quests.QuestStatusActive {
		t.Fatalf("dragon_slayer status = %q, want active", quest.Status())
	}
	if quest.CurrentCount != 0 {
		t.Fatalf("dragon_slayer progress = %d, want 0", quest.CurrentCount)
	}
}
