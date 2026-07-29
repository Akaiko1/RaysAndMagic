package game

import (
	"fmt"
	"os"

	"ugataima/internal/character"
	"ugataima/internal/quests"

	"gopkg.in/yaml.v3"
)

// Tavern rumors: the guide-rail hint system. Every tavern-capable NPC offers a
// "Listen for rumors" branch showing ONE rumor - the pool is every rumor whose
// prerequisite quest is complete and whose goal quest is not, and the shown
// entry rotates with the day/night clock (dayNightDay bumps on every phase
// flip), so a crowded pool cycles by itself with no timers or save fields.

// RumorDef is one authored rumor (assets/rumors.yaml).
type RumorDef struct {
	// Quest is the goal this rumor points at: once completed the rumor
	// retires. Empty = pure flavor/aftermath, never retires.
	Quest string `yaml:"quest,omitempty"`
	// After hides the rumor until that quest completes ("" = always eligible).
	After string `yaml:"after,omitempty"`
	Text  string `yaml:"text"`
}

type rumorConfig struct {
	Rumors []RumorDef `yaml:"rumors"`
}

var globalRumors []RumorDef

// LoadRumorConfig loads assets/rumors.yaml and validates every quest link
// against the same quest definitions used by gameplay.
func LoadRumorConfig(path string, questManager *quests.QuestManager) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("rumors: %w", err)
	}
	var cfg rumorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("rumors: %w", err)
	}
	for i, r := range cfg.Rumors {
		if r.Text == "" {
			return fmt.Errorf("rumors: entry %d has no text", i)
		}
		for _, ref := range []string{r.Quest, r.After} {
			if ref == "" {
				continue
			}
			if questManager == nil || questManager.Definitions()[ref] == nil {
				return fmt.Errorf("rumors: entry %d references unknown quest %q", i, ref)
			}
		}
	}
	globalRumors = cfg.Rumors
	return nil
}

// questCompleted reports whether a quest is completed in the current run.
func (g *MMGame) questCompleted(id string) bool {
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(id)
	return q != nil && q.Completed
}

// currentRumorText picks today's rumor from the eligible pool.
func (g *MMGame) currentRumorText() string {
	var pool []string
	for _, r := range globalRumors {
		if r.After != "" && !g.questCompleted(r.After) {
			continue
		}
		if r.Quest != "" && g.questCompleted(r.Quest) {
			continue
		}
		pool = append(pool, r.Text)
	}
	if len(pool) == 0 {
		return "The taproom is quiet tonight. Even the liars have nothing."
	}
	day := g.dayNightDay
	if day < 0 {
		day = 0
	}
	return pool[day%len(pool)]
}

// rumorDialogueChoice builds the synthetic view-only tavern branch (never
// written into DialogueData - the YAML dialogue pointer is shared).
func (g *MMGame) rumorDialogueChoice() *character.NPCDialogueChoice {
	return &character.NPCDialogueChoice{
		Text:     "Listen for rumors",
		Action:   "info",
		Response: g.currentRumorText(),
		Choices: []*character.NPCDialogueChoice{
			{Text: "Enough gossip", Action: "back"},
		},
	}
}
