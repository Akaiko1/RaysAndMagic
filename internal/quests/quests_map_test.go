package quests

import "testing"

func mapScopedQuestManager() *QuestManager {
	cfg := &QuestConfig{Quests: map[string]*QuestDefinition{
		"wolf_cull": {
			Name:          "Wolf Cull",
			Type:          QuestTypeKill,
			TargetMonster: "wolf",
			TargetCount:   2,
			TargetMap:     "forest",
		},
	}}
	qm := NewQuestManager(cfg)
	_ = qm.ActivateQuest("wolf_cull")
	return qm
}

// A map-scoped kill quest only counts kills on its target map.
func TestOnMonsterKilled_MapScoped(t *testing.T) {
	qm := mapScopedQuestManager()

	qm.OnMonsterKilled("wolf", "city") // off-map wolf: no credit
	if got := qm.GetQuest("wolf_cull").CurrentCount; got != 0 {
		t.Errorf("off-map kill counted: %d", got)
	}

	qm.OnMonsterKilled("wolf", "forest")
	if got := qm.GetQuest("wolf_cull").CurrentCount; got != 1 {
		t.Errorf("on-map kill not counted: %d", got)
	}

	// Unknown map context still counts (callers without map info).
	completed := qm.OnMonsterKilled("wolf", "")
	if len(completed) != 1 || !qm.GetQuest("wolf_cull").Completed {
		t.Errorf("quest should complete at 2/2, completed=%v", completed)
	}
}
