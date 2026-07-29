package game

import (
	"testing"

	"ugataima/internal/quests"
)

// The shipped rumors must load, reference real quest keys, and rotate/retire
// with quest state.
func TestRumorsYAML_LoadsAndRotates(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	questManager := quests.NewQuestManager(questCfg)
	if err := LoadRumorConfig("../../assets/rumors.yaml", questManager); err != nil {
		t.Fatalf("load rumors: %v", err)
	}
	if len(globalRumors) == 0 {
		t.Fatal("no rumors shipped")
	}

	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.questManager = questManager

	// No prerequisites met: only after-gated rumors hide; the pool rotates by day.
	g.dayNightDay = 0
	first := g.currentRumorText()
	if first == "" {
		t.Fatal("empty rumor")
	}
	// Completing a goal quest retires its rumor from the pool.
	if err := g.questManager.ActivateQuest("culverts_valves"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted("culverts_valves")
	for day := 0; day < len(globalRumors)*2; day++ {
		g.dayNightDay = day
		if got := g.currentRumorText(); got == globalRumors[0].Text && globalRumors[0].Quest == "culverts_valves" {
			t.Fatalf("retired rumor still shown on day %d", day)
		}
	}
	// An after-gated rumor surfaces once its prerequisite completes.
	if err := g.questManager.ActivateQuest("water_purge"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted("water_purge")
	found := false
	for day := 0; day < len(globalRumors)*2 && !found; day++ {
		g.dayNightDay = day
		for _, r := range globalRumors {
			if r.After == "water_purge" && g.currentRumorText() == r.Text {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("after-gated rumor (return to the deep) never surfaced")
	}
}
