package character

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNPCTrainingContentValidation(t *testing.T) {
	previous := NPCConfigInstance
	t.Cleanup(func() { NPCConfigInstance = previous })
	for _, tc := range []struct {
		name, kind, offers string
		valid              bool
	}{
		{"missing", "skill_trainer", "", false},
		{"typo", "skill_trainer", "{ grand_master: 10000 }", false},
		{"novice", "skill_trainer", "{ novice: 100 }", false},
		{"free", "skill_trainer", "{ expert: 0 }", false},
		{"negative", "skill_trainer", "{ master: -10 }", false},
		{"wrong type", "quest_giver", "{ expert: 1000 }", false},
		{"city", "skill_trainer", "{ expert: 1000, master: 4000 }", true},
		{"desert", "skill_trainer", "{ grandmaster: 10000 }", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "npcs:\n  test:\n    name: Trainer\n    type: " + tc.kind + "\n    render_category: npc\n"
			if tc.offers != "" {
				text += "    training: " + tc.offers + "\n"
			}
			path := filepath.Join(t.TempDir(), "npcs.yaml")
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			err := LoadNPCConfig(path)
			if (err == nil) != tc.valid {
				t.Fatalf("load error=%v, want valid=%v", err, tc.valid)
			}
			if err != nil && !strings.Contains(err.Error(), "train") {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if tc.valid {
				npc, err := CreateNPCFromConfig("test", 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				rows := TrainingOfferLines(npc.Training)
				if len(rows) != len(npc.Training) {
					t.Fatal("editor omitted a training rule")
				}
				for _, row := range rows {
					if !strings.Contains(row, " -> ") || !strings.HasSuffix(row, " gold") {
						t.Fatalf("incomplete editor rule: %q", row)
					}
				}
			}
		})
	}
}
