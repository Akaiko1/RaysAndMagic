package quests

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepeatScheduleContentValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"night", true}, {"day", true}, {"1d", true}, {"3d", true}, {"28d", true},
		{"true", false}, {"false", false}, {"0d", false}, {"-1d", false}, {"1.5d", false}, {"3h", false}, {"03d", false}, {"every night", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "quests.yaml")
			text := "quests:\n  errand:\n    name: Errand\n    type: kill\n    target_monster: wolf\n    target_count: 5\n    repeatable: " + tc.value + "\n"
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadQuestConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestTimedRepeatClaimRequiresClock(t *testing.T) {
	qm := NewQuestManager(&QuestConfig{Quests: map[string]*QuestDefinition{"errand": {Name: "Errand", Repeatable: "3d"}}})
	if err := qm.ActivateQuest("errand"); err != nil {
		t.Fatal(err)
	}
	qm.MarkCompleted("errand")
	for _, clock := range [][]float64{nil, {0}, {-1}} {
		if _, err := qm.ClaimRewards("errand", clock...); err == nil {
			t.Fatal("claim without a valid clock succeeded")
		}
		if qm.GetQuest("errand").RewardsClaimed {
			t.Fatal("invalid claim mutated state")
		}
	}
	if _, err := qm.ClaimRewards("errand", 2.25); err != nil {
		t.Fatal(err)
	}
}
