package game

import (
	"fmt"
	"hash/fnv"
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

// eligibleRumors is the pool a tavern may draw from: prerequisite met, goal
// not yet completed. Authored order, so an offset indexes it reproducibly.
func (g *MMGame) eligibleRumors() []string {
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
	return pool
}

// tavernRumorSeed identifies the tavern doing the talking - its OWN region plus
// its tile, so each taproom rolls its own deck and two taverns in one region
// still differ. Derived, never saved.
func tavernRumorSeed(npc *character.NPC, mapKey string) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%d,%d", mapKey, npc.Key, int(npc.X), int(npc.Y))
	return h.Sum64()
}

// tavernRegionKey is the region the TAVERN stands in, not the party's. A
// corridor lands tight at some taverns, so the party can be inside the
// neighbouring region while talking - keying on CurrentMapKey there would hand
// one taproom two different decks.
func (g *MMGame) tavernRegionKey(npc *character.NPC) string {
	ts := float64(g.config.GetTileSize())
	if ts <= 0 {
		return currentMapKey()
	}
	return g.mapKeyAtTile(int(npc.X/ts), int(npc.Y/ts))
}

// rumorOrder is one tavern's private shuffle of the pool - its deck. Walking it
// by day visits every rumor once before any repeats.
func rumorOrder(n int, seed uint64) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	s := seed
	for i := n - 1; i > 0; i-- {
		s += 0x9e3779b97f4a7c15 // splitmix64
		x := s
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		j := int(x % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// currentRumorText picks this tavern's rumor for today. One shared pool, but
// each taproom rolls it independently - its own deck order, walked by the
// day/night clock. Two taverns landing on one hint is fine; they part ways the
// next flip.
func (g *MMGame) currentRumorText(seed uint64) string {
	pool := g.eligibleRumors()
	if len(pool) == 0 {
		return "The taproom is quiet tonight. Even the liars have nothing."
	}
	day := g.dayNightDay
	if day < 0 {
		day = 0
	}
	order := rumorOrder(len(pool), seed)
	return pool[order[day%len(order)]]
}

// rumorDialogueChoice builds the synthetic view-only tavern branch (never
// written into DialogueData - the YAML dialogue pointer is shared).
func (g *MMGame) rumorDialogueChoice(npc *character.NPC) *character.NPCDialogueChoice {
	return &character.NPCDialogueChoice{
		Text:     "Listen for rumors",
		Action:   "info",
		Response: g.currentRumorText(tavernRumorSeed(npc, g.tavernRegionKey(npc))),
		Choices: []*character.NPCDialogueChoice{
			{Text: "Enough gossip", Action: "back"},
		},
	}
}
