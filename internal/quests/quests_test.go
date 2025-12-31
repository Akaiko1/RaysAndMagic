package quests

import (
	"testing"
)

func TestQuestManager_InitializeStartingQuests(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"goblin_hunt": {
				Name:            "Goblin Hunt",
				Description:     "Kill 5 goblins",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     5,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50, Experience: 100},
			},
			"wolf_pack": {
				Name:            "Wolf Pack",
				Description:     "Kill 3 wolves",
				Type:            QuestTypeKill,
				TargetMonster:   "wolf",
				TargetCount:     3,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 75, Experience: 150},
			},
			"secret_quest": {
				Name:            "Secret Quest",
				Description:     "Not a starting quest",
				Type:            QuestTypeKill,
				TargetMonster:   "dragon",
				TargetCount:     1,
				IsStartingQuest: false,
				Rewards:         QuestRewards{Gold: 1000, Experience: 5000},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// Should have 2 active quests (only starting quests)
	activeQuests := qm.GetActiveQuests()
	if len(activeQuests) != 2 {
		t.Errorf("Expected 2 active quests, got %d", len(activeQuests))
	}

	// Secret quest should not be active
	secretQuest := qm.GetQuest("secret_quest")
	if secretQuest != nil {
		t.Error("Secret quest should not be initialized")
	}

	// Goblin hunt should be active
	goblinQuest := qm.GetQuest("goblin_hunt")
	if goblinQuest == nil {
		t.Fatal("Goblin hunt quest should be active")
	}
	if goblinQuest.Status != QuestStatusActive {
		t.Errorf("Goblin quest status should be active, got %s", goblinQuest.Status)
	}
	if goblinQuest.CurrentCount != 0 {
		t.Errorf("Goblin quest count should be 0, got %d", goblinQuest.CurrentCount)
	}
}

func TestQuestManager_OnMonsterKilled(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"goblin_hunt": {
				Name:            "Goblin Hunt",
				Description:     "Kill 5 goblins",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     5,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50, Experience: 100},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// Kill 4 goblins - quest should not complete
	for i := 0; i < 4; i++ {
		completed := qm.OnMonsterKilled("goblin")
		if len(completed) != 0 {
			t.Errorf("Quest should not complete after %d kills", i+1)
		}
	}

	quest := qm.GetQuest("goblin_hunt")
	if quest.CurrentCount != 4 {
		t.Errorf("Expected 4 kills, got %d", quest.CurrentCount)
	}

	// Kill 5th goblin - quest should complete
	completed := qm.OnMonsterKilled("goblin")
	if len(completed) != 1 {
		t.Fatalf("Expected 1 completed quest, got %d", len(completed))
	}
	if completed[0].ID != "goblin_hunt" {
		t.Errorf("Expected goblin_hunt to complete, got %s", completed[0].ID)
	}

	quest = qm.GetQuest("goblin_hunt")
	if quest.Status != QuestStatusCompleted {
		t.Errorf("Quest status should be completed, got %s", quest.Status)
	}
	if !quest.Completed {
		t.Error("Quest.Completed should be true")
	}
}

func TestQuestManager_OnMonsterKilled_WrongMonster(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"goblin_hunt": {
				Name:            "Goblin Hunt",
				Description:     "Kill 5 goblins",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     5,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50, Experience: 100},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// Kill wolves - should not affect goblin quest
	completed := qm.OnMonsterKilled("wolf")
	if len(completed) != 0 {
		t.Error("Killing wolf should not complete goblin quest")
	}

	quest := qm.GetQuest("goblin_hunt")
	if quest.CurrentCount != 0 {
		t.Errorf("Goblin quest count should be 0, got %d", quest.CurrentCount)
	}
}

func TestQuestManager_ClaimRewards(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"goblin_hunt": {
				Name:            "Goblin Hunt",
				Description:     "Kill 5 goblins",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     2,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50, Experience: 100},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// Try to claim before completion - should fail
	_, err := qm.ClaimRewards("goblin_hunt")
	if err == nil {
		t.Error("Should not be able to claim rewards before completion")
	}

	// Complete the quest
	qm.OnMonsterKilled("goblin")
	qm.OnMonsterKilled("goblin")

	// Claim rewards - should succeed
	rewards, err := qm.ClaimRewards("goblin_hunt")
	if err != nil {
		t.Fatalf("Failed to claim rewards: %v", err)
	}
	if rewards.Gold != 50 {
		t.Errorf("Expected 50 gold, got %d", rewards.Gold)
	}
	if rewards.Experience != 100 {
		t.Errorf("Expected 100 experience, got %d", rewards.Experience)
	}

	// Try to claim again - should fail
	_, err = qm.ClaimRewards("goblin_hunt")
	if err == nil {
		t.Error("Should not be able to claim rewards twice")
	}

	quest := qm.GetQuest("goblin_hunt")
	if !quest.RewardsClaimed {
		t.Error("RewardsClaimed should be true after claiming")
	}
}

func TestQuest_GetProgressString(t *testing.T) {
	quest := &Quest{
		ID: "test",
		Definition: &QuestDefinition{
			Type:          QuestTypeKill,
			TargetMonster: "goblin",
			TargetCount:   5,
		},
		CurrentCount: 3,
	}

	progress := quest.GetProgressString()
	expected := "3/5 goblins killed"
	if progress != expected {
		t.Errorf("Expected '%s', got '%s'", expected, progress)
	}
}

func TestQuest_GetStatusString(t *testing.T) {
	tests := []struct {
		name           string
		status         QuestStatus
		completed      bool
		rewardsClaimed bool
		expected       string
	}{
		{"Active", QuestStatusActive, false, false, "In Progress"},
		{"Completed unclaimed", QuestStatusCompleted, true, false, "Complete! (Claim Reward)"},
		{"Completed claimed", QuestStatusCompleted, true, true, "Completed"},
		{"Failed", QuestStatusFailed, false, false, "Failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quest := &Quest{
				Status:         tt.status,
				Completed:      tt.completed,
				RewardsClaimed: tt.rewardsClaimed,
				Definition:     &QuestDefinition{},
			}
			if got := quest.GetStatusString(); got != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestQuestManager_ActivateQuest(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"secret_quest": {
				Name:            "Secret Quest",
				Description:     "Not a starting quest",
				Type:            QuestTypeKill,
				TargetMonster:   "dragon",
				TargetCount:     1,
				IsStartingQuest: false,
				Rewards:         QuestRewards{Gold: 1000, Experience: 5000},
			},
		},
	}

	qm := NewQuestManager(config)

	// Activate a quest manually
	err := qm.ActivateQuest("secret_quest")
	if err != nil {
		t.Fatalf("Failed to activate quest: %v", err)
	}

	quest := qm.GetQuest("secret_quest")
	if quest == nil {
		t.Fatal("Secret quest should be active")
	}
	if quest.Status != QuestStatusActive {
		t.Errorf("Quest status should be active, got %s", quest.Status)
	}

	// Try to activate again - should fail
	err = qm.ActivateQuest("secret_quest")
	if err == nil {
		t.Error("Should not be able to activate same quest twice")
	}

	// Try to activate non-existent quest
	err = qm.ActivateQuest("nonexistent")
	if err == nil {
		t.Error("Should not be able to activate non-existent quest")
	}
}

func TestQuestManager_GetCompletedQuests(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"quest1": {
				Name:            "Quest 1",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     1,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50},
			},
			"quest2": {
				Name:            "Quest 2",
				Type:            QuestTypeKill,
				TargetMonster:   "wolf",
				TargetCount:     1,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 75},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// No completed quests initially
	completed := qm.GetCompletedQuests()
	if len(completed) != 0 {
		t.Errorf("Expected 0 completed quests, got %d", len(completed))
	}

	// Complete quest1
	qm.OnMonsterKilled("goblin")

	completed = qm.GetCompletedQuests()
	if len(completed) != 1 {
		t.Errorf("Expected 1 completed quest, got %d", len(completed))
	}

	// Claim quest1 rewards
	qm.ClaimRewards("quest1")

	// Should not appear in completed (unclaimed) list anymore
	completed = qm.GetCompletedQuests()
	if len(completed) != 0 {
		t.Errorf("Expected 0 unclaimed completed quests, got %d", len(completed))
	}
}

func TestQuestManager_CreateEncounterQuest(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{},
	}

	qm := NewQuestManager(config)

	// Create an encounter quest
	questID := qm.CreateEncounterQuest("bandit_camp", "Clear the Camp", "Defeat all bandits", 100, 200)

	if questID != "bandit_camp" {
		t.Errorf("Expected quest ID 'bandit_camp', got '%s'", questID)
	}

	// Quest should be active
	quest := qm.GetQuest("bandit_camp")
	if quest == nil {
		t.Fatal("Encounter quest should exist")
	}
	if quest.Status != QuestStatusActive {
		t.Errorf("Quest status should be active, got %s", quest.Status)
	}
	if quest.Definition.Type != QuestTypeEncounter {
		t.Errorf("Quest type should be encounter, got %s", quest.Definition.Type)
	}
	if quest.Definition.Name != "Clear the Camp" {
		t.Errorf("Expected name 'Clear the Camp', got '%s'", quest.Definition.Name)
	}

	// Try to create same quest again - should return same ID without error
	questID2 := qm.CreateEncounterQuest("bandit_camp", "Different Name", "Different desc", 50, 50)
	if questID2 != "bandit_camp" {
		t.Errorf("Expected same quest ID 'bandit_camp', got '%s'", questID2)
	}

	// Should still only have one quest
	allQuests := qm.GetAllQuests()
	if len(allQuests) != 1 {
		t.Errorf("Expected 1 quest, got %d", len(allQuests))
	}
}

func TestQuestManager_CompleteEncounterQuest(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{},
	}

	qm := NewQuestManager(config)

	// Create an encounter quest
	qm.CreateEncounterQuest("bandit_camp", "Clear the Camp", "Defeat all bandits", 100, 200)

	// Complete the encounter quest
	rewards := qm.CompleteEncounterQuest("bandit_camp")
	if rewards == nil {
		t.Fatal("Should have received rewards")
	}
	if rewards.Gold != 100 {
		t.Errorf("Expected 100 gold, got %d", rewards.Gold)
	}
	if rewards.Experience != 200 {
		t.Errorf("Expected 200 experience, got %d", rewards.Experience)
	}

	// Quest should be completed and auto-claimed
	quest := qm.GetQuest("bandit_camp")
	if !quest.Completed {
		t.Error("Quest should be completed")
	}
	if !quest.RewardsClaimed {
		t.Error("Rewards should be auto-claimed")
	}
	if quest.Status != QuestStatusCompleted {
		t.Errorf("Quest status should be completed, got %s", quest.Status)
	}

	// Completing again should return nil
	rewards2 := qm.CompleteEncounterQuest("bandit_camp")
	if rewards2 != nil {
		t.Error("Should not be able to complete same quest twice")
	}
}

func TestQuestManager_CompleteEncounterQuest_NonEncounter(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{
			"goblin_hunt": {
				Name:            "Goblin Hunt",
				Type:            QuestTypeKill,
				TargetMonster:   "goblin",
				TargetCount:     5,
				IsStartingQuest: true,
				Rewards:         QuestRewards{Gold: 50, Experience: 100},
			},
		},
	}

	qm := NewQuestManager(config)
	qm.InitializeStartingQuests()

	// Try to complete a kill quest using encounter completion - should fail
	rewards := qm.CompleteEncounterQuest("goblin_hunt")
	if rewards != nil {
		t.Error("Should not be able to complete a kill quest as encounter")
	}

	// Quest should still be active
	quest := qm.GetQuest("goblin_hunt")
	if quest.Status != QuestStatusActive {
		t.Errorf("Quest status should still be active, got %s", quest.Status)
	}
}

func TestQuestManager_RemoveQuest(t *testing.T) {
	config := &QuestConfig{
		Quests: map[string]*QuestDefinition{},
	}

	qm := NewQuestManager(config)

	// Create an encounter quest
	qm.CreateEncounterQuest("bandit_camp", "Clear the Camp", "Defeat all bandits", 100, 200)

	// Verify it exists
	quest := qm.GetQuest("bandit_camp")
	if quest == nil {
		t.Fatal("Quest should exist")
	}

	// Remove the quest
	qm.RemoveQuest("bandit_camp")

	// Should no longer exist
	quest = qm.GetQuest("bandit_camp")
	if quest != nil {
		t.Error("Quest should have been removed")
	}
}
