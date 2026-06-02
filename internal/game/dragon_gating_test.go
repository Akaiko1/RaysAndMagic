package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
)

// TestDragonWinGating verifies that only statue-summoned dragons (flagged with
// IsEncounterMonster + EncounterRewards.QuestID=="dragon_slayer") advance the
// win quest — a plain wild dragon must not count.
func TestDragonWinGating(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests() // dragon_slayer is a starting quest
	cs.game.questManager = qm

	progress := func() int {
		q := qm.GetQuest("dragon_slayer")
		if q == nil {
			t.Fatal("dragon_slayer quest missing")
		}
		return q.CurrentCount
	}
	if progress() != 0 {
		t.Fatalf("dragon_slayer should start at 0, got %d", progress())
	}

	// A wild (unflagged) dragon kill must NOT count.
	wild := monsterPkg.NewMonster3DFromConfig(0, 0, "dragon", cs.game.config)
	cs.updateQuestProgress(wild)
	if progress() != 0 {
		t.Errorf("wild dragon advanced the win quest to %d, want 0", progress())
	}

	// A statue-summoned dragon (flagged) must count.
	summoned := monsterPkg.NewMonster3DFromConfig(0, 0, "dragon", cs.game.config)
	summoned.IsEncounterMonster = true
	summoned.EncounterRewards = &monsterPkg.EncounterRewards{QuestID: "dragon_slayer"}
	cs.updateQuestProgress(summoned)
	if progress() != 1 {
		t.Errorf("summoned dragon advanced the win quest to %d, want 1", progress())
	}
}
